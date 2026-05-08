package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/storage"
)

type confirmedBalanceRow struct {
	BalanceSatoshi int64 `json:"balance_satoshi"`
	UTXOCount      int64 `json:"utxo_count"`
}

const balanceIndexReadyMetaKey = "balance_index_ready"
const balanceIndexBootstrapProgressMetaKey = "balance_index_bootstrap_progress"

const (
	bootstrapProgressVersion   = 3
	bootstrapPhaseBalanceTable = "address_balance"
	bootstrapPhaseRankTable    = "balance_rank"
)

var bootstrapSourceStoreOptions = storage.PebbleOpenOptions{
	ReadOnly:                    true,
	CacheSizeBytes:              4 << 20,
	MemTableSizeBytes:           4 << 20,
	MemTableStopWritesThreshold: 2,
	MaxConcurrentCompactions:    1,
	MaxOpenFiles:                2048,
}

var bootstrapTargetStoreOptions = storage.PebbleOpenOptions{
	CacheSizeBytes:              8 << 20,
	MemTableSizeBytes:           8 << 20,
	MemTableStopWritesThreshold: 2,
	MaxConcurrentCompactions:    1,
	MaxOpenFiles:                2048,
}

type bootstrapOptions struct {
	CommitBatchSize int
	MaxAddresses    int
	SleepPerCommit  time.Duration
}

type bootstrapProgress struct {
	Version             int    `json:"version,omitempty"`
	Phase               string `json:"phase,omitempty"`
	CurrentShard        int    `json:"current_shard"`
	LastKey             string `json:"last_key,omitempty"`
	ScannedAddressCount int64  `json:"scanned_address_count"`
	IndexedAddressCount int64  `json:"indexed_address_count"`
	RankScannedCount    int64  `json:"rank_scanned_count,omitempty"`
	RankIndexedCount    int64  `json:"rank_indexed_count,omitempty"`
	Done                bool   `json:"done"`
}

func main() {
	var (
		dataDir         = flag.String("data-dir", "data", "Pebble data directory")
		chain           = flag.String("chain", "mvc", "chain name")
		network         = flag.String("network", "mainnet", "network name")
		shardCount      = flag.Int("shards", 4, "Pebble shard count")
		cpuCores        = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB        = flag.Int("memory", 2, "memory GB for storage tuning")
		commitBatchSize = flag.Int("commit-batch-size", 500, "number of addresses to write per commit")
		maxAddresses    = flag.Int("max-addresses", 0, "maximum number of addresses to process in this run; 0 means no limit")
		sleepMS         = flag.Int("sleep-ms", 50, "milliseconds to sleep after each committed batch")
	)
	flag.Parse()

	if err := runWithOptions(*dataDir, *shardCount, *cpuCores, *memoryGB, *chain, *network, bootstrapOptions{
		CommitBatchSize: *commitBatchSize,
		MaxAddresses:    *maxAddresses,
		SleepPerCommit:  time.Duration(*sleepMS) * time.Millisecond,
	}); err != nil {
		log.Fatalf("bootstrap confirmed balance indexes: %v", err)
	}
}

func run(dataDir string, shardCount, cpuCores, memoryGB int, chain, network string) error {
	return runWithOptions(dataDir, shardCount, cpuCores, memoryGB, chain, network, bootstrapOptions{
		CommitBatchSize: 500,
		SleepPerCommit:  50 * time.Millisecond,
	})
}

func runWithOptions(dataDir string, shardCount, cpuCores, memoryGB int, chain, network string, opts bootstrapOptions) error {
	cfg := &config.Config{
		Chain:      chain,
		Network:    network,
		DataDir:    dataDir,
		ShardCount: shardCount,
		CPUCores:   cpuCores,
		MemoryGB:   memoryGB,
		RPC: config.RPCConfig{
			Chain: chain,
		},
	}
	if err := cfg.ValidateChain(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	config.GlobalConfig = cfg
	if params, err := cfg.GetChainParams(); err == nil {
		config.GlobalNetwork = params
	}

	indexerParams := config.AutoConfigure(config.SystemResources{
		CPUCores:   cpuCores,
		MemoryGB:   memoryGB,
		HighPerf:   false,
		ShardCount: shardCount,
	})
	common.InitBytePool(indexerParams.BytePoolSizeKB)
	storage.DbInit(indexerParams)

	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		return fmt.Errorf("open meta store: %w", err)
	}
	defer metaStore.Close()

	addressStore, err := openBootstrapSourceStore(indexerParams, dataDir, storage.StoreTypeIncome, shardCount)
	if err != nil {
		return fmt.Errorf("open income store: %w", err)
	}
	defer addressStore.Close()

	spendStore, err := openBootstrapSourceStore(indexerParams, dataDir, storage.StoreTypeSpend, shardCount)
	if err != nil {
		return fmt.Errorf("open spend store: %w", err)
	}
	defer spendStore.Close()

	balanceStore, err := openBootstrapTargetStore(indexerParams, dataDir, storage.StoreTypeAddressBalance, shardCount)
	if err != nil {
		return fmt.Errorf("open address_balance store: %w", err)
	}
	defer balanceStore.Close()

	rankStore, err := openBootstrapTargetStore(indexerParams, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		return fmt.Errorf("open balance_rank store: %w", err)
	}
	defer rankStore.Close()

	log.Printf("[BalanceIndexBootstrap] rebuilding confirmed balance indexes in %s", dataDir)
	if err := rebuildConfirmedBalanceIndexes(metaStore, addressStore, spendStore, balanceStore, rankStore, opts); err != nil {
		return err
	}
	log.Printf("[BalanceIndexBootstrap] completed")
	return nil
}

func openBootstrapSourceStore(params config.IndexerParams, dataDir string, storeType storage.StoreType, shardCount int) (*storage.PebbleStore, error) {
	return storage.NewPebbleStoreWithOptions(params, dataDir, storeType, shardCount, bootstrapSourceStoreOptions)
}

func openBootstrapTargetStore(params config.IndexerParams, dataDir string, storeType storage.StoreType, shardCount int) (*storage.PebbleStore, error) {
	return storage.NewPebbleStoreWithOptions(params, dataDir, storeType, shardCount, bootstrapTargetStoreOptions)
}

func rebuildConfirmedBalanceIndexes(metaStore *storage.MetaStore, addressStore, spendStore, balanceStore, rankStore *storage.PebbleStore, opts bootstrapOptions) error {
	if opts.CommitBatchSize <= 0 {
		opts.CommitBatchSize = 500
	}
	if opts.MaxAddresses < 0 {
		opts.MaxAddresses = 0
	}

	progress, hasProgress, err := loadExistingBootstrapProgress(metaStore)
	if err != nil {
		return fmt.Errorf("load bootstrap progress: %w", err)
	}
	ready, err := isBalanceIndexReady(metaStore)
	if err != nil {
		return fmt.Errorf("read balance index ready state: %w", err)
	}
	if ready && hasProgress && progress.Version == bootstrapProgressVersion && progress.Done {
		log.Printf("[BalanceIndexBootstrap] balance index already ready; skipping")
		return nil
	}

	if !hasProgress || progress.Done || progress.Version != bootstrapProgressVersion {
		progress, err = initializeBootstrapProgress(metaStore, balanceStore, rankStore)
		if err != nil {
			return err
		}
	}

	remaining := opts.MaxAddresses
	if progress.Phase == "" {
		progress.Phase = bootstrapPhaseBalanceTable
	}

	if progress.Phase == bootstrapPhaseBalanceTable {
		log.Printf("[BalanceIndexBootstrap] scanning %d income shards sequentially from shard=%d last_key=%q", len(addressStore.GetShards()), progress.CurrentShard, progress.LastKey)
		limitReached, err := rebuildAddressBalanceFromHistory(addressStore, spendStore, balanceStore, metaStore, &progress, &remaining, opts)
		if err != nil {
			return err
		}
		if limitReached {
			log.Printf("[BalanceIndexBootstrap] paused in %s after scanned=%d indexed=%d shard=%d last_key=%q", progress.Phase, progress.ScannedAddressCount, progress.IndexedAddressCount, progress.CurrentShard, progress.LastKey)
			return nil
		}

		progress.Phase = bootstrapPhaseRankTable
		progress.CurrentShard = 0
		progress.LastKey = ""
		progress.Version = bootstrapProgressVersion
		progress.Done = false
		progress.RankScannedCount = 0
		progress.RankIndexedCount = 0
		if err := clearStore(rankStore); err != nil {
			return fmt.Errorf("clear rank store before rank rebuild: %w", err)
		}
		if err := rankStore.Sync(); err != nil {
			return fmt.Errorf("sync cleared rank store: %w", err)
		}
		if err := saveBootstrapProgress(metaStore, progress); err != nil {
			return fmt.Errorf("save rank phase bootstrap progress: %w", err)
		}
	}

	if progress.Phase == bootstrapPhaseRankTable {
		log.Printf("[BalanceIndexBootstrap] scanning %d balance shards sequentially from shard=%d last_key=%q", len(balanceStore.GetShards()), progress.CurrentShard, progress.LastKey)
		limitReached, err := rebuildRankFromBalanceStore(balanceStore, rankStore, metaStore, &progress, &remaining, opts)
		if err != nil {
			return err
		}
		if limitReached {
			log.Printf("[BalanceIndexBootstrap] paused in %s after rank_scanned=%d rank_indexed=%d shard=%d last_key=%q", progress.Phase, progress.RankScannedCount, progress.RankIndexedCount, progress.CurrentShard, progress.LastKey)
			return nil
		}
	}

	if err := balanceStore.Sync(); err != nil {
		return fmt.Errorf("sync balance store: %w", err)
	}
	if err := rankStore.Sync(); err != nil {
		return fmt.Errorf("sync rank store: %w", err)
	}
	if err := setBalanceIndexReady(metaStore, true); err != nil {
		return fmt.Errorf("mark balance index ready: %w", err)
	}

	progress.Version = bootstrapProgressVersion
	progress.Phase = bootstrapPhaseRankTable
	progress.Done = true
	progress.CurrentShard = len(balanceStore.GetShards())
	progress.LastKey = ""
	if err := saveBootstrapProgress(metaStore, progress); err != nil {
		return fmt.Errorf("save final bootstrap progress: %w", err)
	}

	log.Printf("[BalanceIndexBootstrap] indexed %d balances after scanning %d addresses and %d rank rows", progress.IndexedAddressCount, progress.ScannedAddressCount, progress.RankScannedCount)
	return nil
}

func initializeBootstrapProgress(metaStore *storage.MetaStore, balanceStore, rankStore *storage.PebbleStore) (bootstrapProgress, error) {
	if err := setBalanceIndexReady(metaStore, false); err != nil {
		return bootstrapProgress{}, fmt.Errorf("mark balance index not ready: %w", err)
	}
	if err := clearStore(balanceStore); err != nil {
		return bootstrapProgress{}, fmt.Errorf("clear balance store: %w", err)
	}
	if err := balanceStore.Sync(); err != nil {
		return bootstrapProgress{}, fmt.Errorf("sync cleared balance store: %w", err)
	}
	if err := clearStore(rankStore); err != nil {
		return bootstrapProgress{}, fmt.Errorf("clear rank store: %w", err)
	}
	if err := rankStore.Sync(); err != nil {
		return bootstrapProgress{}, fmt.Errorf("sync cleared rank store: %w", err)
	}

	progress := bootstrapProgress{
		Version: bootstrapProgressVersion,
		Phase:   bootstrapPhaseBalanceTable,
	}
	if err := saveBootstrapProgress(metaStore, progress); err != nil {
		return bootstrapProgress{}, fmt.Errorf("save initial bootstrap progress: %w", err)
	}
	return progress, nil
}

func rebuildAddressBalanceFromHistory(addressStore, spendStore, balanceStore *storage.PebbleStore, metaStore *storage.MetaStore, progress *bootstrapProgress, remaining *int, opts bootstrapOptions) (bool, error) {
	shards := addressStore.GetShards()
	if len(shards) == 0 {
		return false, nil
	}

	for shardIdx := progress.CurrentShard; shardIdx < len(shards); shardIdx++ {
		startKey := ""
		if shardIdx == progress.CurrentShard {
			startKey = progress.LastKey
		}
		limitReached, err := rebuildAddressBalanceShard(shardIdx, shards[shardIdx], spendStore, balanceStore, metaStore, progress, remaining, opts, startKey)
		if err != nil {
			return false, err
		}
		if limitReached {
			return true, nil
		}
		progress.CurrentShard = shardIdx + 1
		progress.LastKey = ""
		progress.Version = bootstrapProgressVersion
		progress.Phase = bootstrapPhaseBalanceTable
		if err := saveBootstrapProgress(metaStore, *progress); err != nil {
			return false, fmt.Errorf("advance bootstrap progress to income shard %d: %w", shardIdx+1, err)
		}
	}

	return false, nil
}

func rebuildAddressBalanceShard(shardIdx int, db *pebble.DB, spendStore, balanceStore *storage.PebbleStore, metaStore *storage.MetaStore, progress *bootstrapProgress, remaining *int, opts bootstrapOptions, startKey string) (bool, error) {
	iter, err := db.NewIter(nil)
	if err != nil {
		return false, fmt.Errorf("open income shard iterator %d: %w", shardIdx, err)
	}
	defer iter.Close()

	balanceBatch := balanceStore.NewBatch()
	pending := 0
	var pendingScanned int64
	var pendingIndexed int64
	lastProcessedKey := startKey

	if startKey != "" {
		iter.SeekGE([]byte(startKey))
		if iter.Valid() && string(iter.Key()) == startKey {
			iter.Next()
		}
	} else {
		iter.First()
	}

	commit := func() error {
		if pending == 0 {
			return nil
		}
		if err := balanceBatch.Commit(); err != nil {
			return err
		}
		if err := balanceStore.Sync(); err != nil {
			return fmt.Errorf("sync balance store after shard %d flush: %w", shardIdx, err)
		}
		balanceBatch = balanceStore.NewBatch()
		progress.Version = bootstrapProgressVersion
		progress.Phase = bootstrapPhaseBalanceTable
		progress.CurrentShard = shardIdx
		progress.LastKey = lastProcessedKey
		progress.ScannedAddressCount += pendingScanned
		progress.IndexedAddressCount += pendingIndexed
		if err := saveBootstrapProgress(metaStore, *progress); err != nil {
			return err
		}
		log.Printf("[BalanceIndexBootstrap] phase=%s shard=%d scanned=%d indexed=%d last_key=%q", progress.Phase, progress.CurrentShard, progress.ScannedAddressCount, progress.IndexedAddressCount, progress.LastKey)

		pending = 0
		pendingScanned = 0
		pendingIndexed = 0
		if opts.SleepPerCommit > 0 {
			time.Sleep(opts.SleepPerCommit)
		}
		return nil
	}

	for ; iter.Valid(); iter.Next() {
		addressKey := append([]byte(nil), iter.Key()...)
		address := string(addressKey)
		spendData, _, err := spendStore.GetWithShard(addressKey)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return false, fmt.Errorf("load spend row for %s: %w", address, err)
		}
		if row, ok := buildConfirmedBalanceRow(iter.Value(), spendData); ok {
			rowData, err := json.Marshal(row)
			if err != nil {
				return false, fmt.Errorf("marshal confirmed balance row for %s: %w", address, err)
			}
			if err := balanceBatch.Set(addressKey, rowData); err != nil {
				return false, fmt.Errorf("set confirmed balance row for %s: %w", address, err)
			}
			pendingIndexed++
		}
		pending++
		pendingScanned++
		lastProcessedKey = address

		if pending >= opts.CommitBatchSize {
			if err := commit(); err != nil {
				return false, fmt.Errorf("commit income shard %d batch: %w", shardIdx, err)
			}
		}

		if remaining != nil && *remaining > 0 {
			*remaining--
			if *remaining == 0 {
				if err := commit(); err != nil {
					return false, fmt.Errorf("commit limited income shard %d batch: %w", shardIdx, err)
				}
				return true, nil
			}
		}
	}

	if err := iter.Error(); err != nil {
		return false, fmt.Errorf("iterate income shard %d: %w", shardIdx, err)
	}
	if err := commit(); err != nil {
		return false, fmt.Errorf("commit final income shard %d batch: %w", shardIdx, err)
	}
	return false, nil
}

func rebuildRankFromBalanceStore(balanceStore, rankStore *storage.PebbleStore, metaStore *storage.MetaStore, progress *bootstrapProgress, remaining *int, opts bootstrapOptions) (bool, error) {
	shards := balanceStore.GetShards()
	if len(shards) == 0 {
		return false, nil
	}

	for shardIdx := progress.CurrentShard; shardIdx < len(shards); shardIdx++ {
		startKey := ""
		if shardIdx == progress.CurrentShard {
			startKey = progress.LastKey
		}
		limitReached, err := rebuildRankShard(shardIdx, shards[shardIdx], rankStore, metaStore, progress, remaining, opts, startKey)
		if err != nil {
			return false, err
		}
		if limitReached {
			return true, nil
		}
		progress.CurrentShard = shardIdx + 1
		progress.LastKey = ""
		progress.Version = bootstrapProgressVersion
		progress.Phase = bootstrapPhaseRankTable
		if err := saveBootstrapProgress(metaStore, *progress); err != nil {
			return false, fmt.Errorf("advance bootstrap progress to rank shard %d: %w", shardIdx+1, err)
		}
	}

	return false, nil
}

func rebuildRankShard(shardIdx int, db *pebble.DB, rankStore *storage.PebbleStore, metaStore *storage.MetaStore, progress *bootstrapProgress, remaining *int, opts bootstrapOptions, startKey string) (bool, error) {
	iter, err := db.NewIter(nil)
	if err != nil {
		return false, fmt.Errorf("open balance shard iterator %d: %w", shardIdx, err)
	}
	defer iter.Close()

	rankBatch := rankStore.NewBatch()
	pending := 0
	var pendingScanned int64
	var pendingIndexed int64
	lastProcessedKey := startKey

	if startKey != "" {
		iter.SeekGE([]byte(startKey))
		if iter.Valid() && string(iter.Key()) == startKey {
			iter.Next()
		}
	} else {
		iter.First()
	}

	commit := func() error {
		if pending == 0 {
			return nil
		}
		if err := rankBatch.Commit(); err != nil {
			return err
		}
		if err := rankStore.Sync(); err != nil {
			return fmt.Errorf("sync rank store after shard %d flush: %w", shardIdx, err)
		}
		rankBatch = rankStore.NewBatch()
		progress.Version = bootstrapProgressVersion
		progress.Phase = bootstrapPhaseRankTable
		progress.CurrentShard = shardIdx
		progress.LastKey = lastProcessedKey
		progress.RankScannedCount += pendingScanned
		progress.RankIndexedCount += pendingIndexed
		if err := saveBootstrapProgress(metaStore, *progress); err != nil {
			return err
		}
		log.Printf("[BalanceIndexBootstrap] phase=%s shard=%d rank_scanned=%d rank_indexed=%d last_key=%q", progress.Phase, progress.CurrentShard, progress.RankScannedCount, progress.RankIndexedCount, progress.LastKey)

		pending = 0
		pendingScanned = 0
		pendingIndexed = 0
		if opts.SleepPerCommit > 0 {
			time.Sleep(opts.SleepPerCommit)
		}
		return nil
	}

	for ; iter.Valid(); iter.Next() {
		var row confirmedBalanceRow
		if err := json.Unmarshal(iter.Value(), &row); err != nil {
			return false, fmt.Errorf("parse confirmed balance row for %s: %w", string(iter.Key()), err)
		}

		if row.BalanceSatoshi > 0 {
			entryData, err := json.Marshal(indexer.AddressBalance{
				Address:        string(iter.Key()),
				BalanceSatoshi: row.BalanceSatoshi,
				Balance:        float64(row.BalanceSatoshi) / 1e8,
				UTXOCount:      row.UTXOCount,
			})
			if err != nil {
				return false, fmt.Errorf("marshal rank entry for %s: %w", string(iter.Key()), err)
			}
			if err := rankBatch.Set([]byte(balanceRankKey(string(iter.Key()), row.BalanceSatoshi)), entryData); err != nil {
				return false, fmt.Errorf("set rank entry for %s: %w", string(iter.Key()), err)
			}
			pendingIndexed++
		}

		pending++
		pendingScanned++
		lastProcessedKey = string(iter.Key())
		if pending >= opts.CommitBatchSize {
			if err := commit(); err != nil {
				return false, fmt.Errorf("commit rank shard %d batch: %w", shardIdx, err)
			}
		}

		if remaining != nil && *remaining > 0 {
			*remaining--
			if *remaining == 0 {
				if err := commit(); err != nil {
					return false, fmt.Errorf("commit limited rank shard %d batch: %w", shardIdx, err)
				}
				return true, nil
			}
		}
	}

	if err := iter.Error(); err != nil {
		return false, fmt.Errorf("iterate balance shard %d: %w", shardIdx, err)
	}
	if err := commit(); err != nil {
		return false, fmt.Errorf("commit final rank shard %d batch: %w", shardIdx, err)
	}
	return false, nil
}

func buildConfirmedBalanceRow(incomeData, spendData []byte) (confirmedBalanceRow, bool) {
	spendMap := make(map[string]struct{})
	if len(spendData) > 0 {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			parts := strings.Split(spendTx, "@")
			if len(parts) < 1 || parts[0] == "" {
				continue
			}
			spendMap[parts[0]] = struct{}{}
		}
	}

	seenIncome := make(map[string]struct{})
	var balance int64
	var utxoCount int64
	for _, part := range strings.Split(string(incomeData), ",") {
		if part == "" {
			continue
		}
		fields := strings.Split(part, "@")
		if len(fields) < 3 {
			continue
		}
		outpoint := fields[0] + ":" + fields[1]
		if _, exists := seenIncome[outpoint]; exists {
			continue
		}
		seenIncome[outpoint] = struct{}{}

		amount, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			continue
		}
		if _, spent := spendMap[outpoint]; spent {
			continue
		}
		balance += amount
		utxoCount++
	}

	if balance <= 0 || utxoCount <= 0 {
		return confirmedBalanceRow{}, false
	}
	return confirmedBalanceRow{
		BalanceSatoshi: balance,
		UTXOCount:      utxoCount,
	}, true
}

func clearStore(store *storage.PebbleStore) error {
	if store == nil {
		return nil
	}

	const commitBatchSize = 5000
	batch := store.NewBatch()
	pending := 0
	var callbackErr error

	if err := store.IterateShards(func(key, _ []byte) bool {
		if callbackErr != nil {
			return false
		}
		keyCopy := append([]byte(nil), key...)
		if err := batch.Delete(keyCopy); err != nil {
			callbackErr = err
			return false
		}
		pending++
		if pending >= commitBatchSize {
			if err := batch.Commit(); err != nil {
				callbackErr = err
				return false
			}
			batch = store.NewBatch()
			pending = 0
		}
		return true
	}); err != nil {
		return err
	}
	if callbackErr != nil {
		return callbackErr
	}
	if pending > 0 {
		return batch.Commit()
	}
	return nil
}

func setBalanceIndexReady(metaStore *storage.MetaStore, ready bool) error {
	value := "0"
	if ready {
		value = "1"
	}
	return metaStore.Set([]byte(balanceIndexReadyMetaKey), []byte(value))
}

func isBalanceIndexReady(metaStore *storage.MetaStore) (bool, error) {
	data, err := metaStore.Get([]byte(balanceIndexReadyMetaKey))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return string(data) == "1", nil
}

func loadBootstrapProgress(metaStore *storage.MetaStore) (bootstrapProgress, error) {
	progress, _, err := loadBootstrapProgressWithExists(metaStore)
	return progress, err
}

func loadExistingBootstrapProgress(metaStore *storage.MetaStore) (bootstrapProgress, bool, error) {
	return loadBootstrapProgressWithExists(metaStore)
}

func loadBootstrapProgressWithExists(metaStore *storage.MetaStore) (bootstrapProgress, bool, error) {
	data, err := metaStore.Get([]byte(balanceIndexBootstrapProgressMetaKey))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return bootstrapProgress{}, false, nil
		}
		return bootstrapProgress{}, false, err
	}
	var progress bootstrapProgress
	if err := json.Unmarshal(data, &progress); err != nil {
		return bootstrapProgress{}, true, fmt.Errorf("parse bootstrap progress: %w", err)
	}
	return progress, true, nil
}

func saveBootstrapProgress(metaStore *storage.MetaStore, progress bootstrapProgress) error {
	data, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("marshal bootstrap progress: %w", err)
	}
	return metaStore.Set([]byte(balanceIndexBootstrapProgressMetaKey), data)
}

func balanceRankKey(address string, balanceSatoshi int64) string {
	reversed := uint64(math.MaxInt64 - balanceSatoshi)
	return fmt.Sprintf("%019d:%s", reversed, address)
}
