package storage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/cockroachdb/pebble"
	"github.com/metaid/utxo_indexer/config"
)

const (
	defaultShardCount = 1

	// Database directory names
	DBDirUTXO                        = "utxo"
	DBDirIncome                      = "income"
	DBDirSpend                       = "spend"
	DBDirMeta                        = "meta"
	DBDirContractFTUTXO              = "contract_ft_utxo"
	DBDirAddressFTIncome             = "address_ft_income"
	DBDirAddressFTSpend              = "address_ft_spend"
	DBDirContractFTInfo              = "contract_ft_info"
	DBDirContractFTGenesis           = "contract_ft_genesis"
	DBDirContractFTGenesisOutput     = "contract_ft_genesis_output"
	DBDirContractFTGenesisUTXO       = "contract_ft_genesis_utxo"
	DBDirAddressFTIncomeValid        = "address_ft_income_valid"
	DBDirUnCheckFtIncome             = "uncheck_ft_income"
	DBDirUsedFTIncome                = "used_ft_income"
	DBDirUniqueFTIncome              = "unique_ft_income"
	DBDirUniqueFTSpend               = "unique_ft_spend"
	DBDirInvalidFtOutpoint           = "invalid_ft_outpoint"
	DBDirContractFTInfoSensibleId    = "contract_ft_info_sensible_id"
	DBDirContractFTSupply            = "contract_ft_supply"
	DBDirContractFTBurn              = "contract_ft_burn"
	DBDirContractFTOwnersIncomeValid = "contract_ft_owners_income_valid"
	DBDirContractFTOwnersIncome      = "contract_ft_owners_income"
	DBDirContractFTOwnersSpend       = "contract_ft_owners_spend"
	DBDirContractFTAddressHistory    = "contract_ft_address_history"
	DBDirContractFTGenesisHistory    = "contract_ft_genesis_history"

	// NFT directories
	DBDirContractNFTUTXO               = "contract_nft_utxo"
	DBDirAddressNFTIncome              = "address_nft_income"
	DBDirAddressNFTSpend               = "address_nft_spend"
	DBDirCodeHashGenesisNFTIncome      = "codehash_genesis_nft_income"
	DBDirCodeHashGenesisNFTSpend       = "codehash_genesis_nft_spend"
	DBDirAddressSellNFTIncome          = "address_sell_nft_income"
	DBDirAddressSellNFTSpend           = "address_sell_nft_spend"
	DBDirCodeHashGenesisSellNFTIncome  = "codehash_genesis_sell_nft_income"
	DBDirCodeHashGenesisSellNFTSpend   = "codehash_genesis_sell_nft_spend"
	DBDirContractNFTInfo               = "contract_nft_info"
	DBDirContractNFTSummaryInfo        = "contract_nft_summary_info"
	DBDirContractNFTGenesis            = "contract_nft_genesis"
	DBDirContractNFTGenesisOutput      = "contract_nft_genesis_output"
	DBDirContractNFTGenesisUTXO        = "contract_nft_genesis_utxo"
	DBDirContractNFTOwnersIncomeValid  = "contract_nft_owners_income_valid"
	DBDirContractNFTOwnersIncome       = "contract_nft_owners_income"
	DBDirContractNFTOwnersSpend        = "contract_nft_owners_spend"
	DBDirContractNFTAddressHistory     = "contract_nft_address_history"
	DBDirContractNFTGenesisHistory     = "contract_nft_genesis_history"
	DBDirAddressNFTIncomeValid         = "address_nft_income_valid"
	DBDirCodeHashGenesisNFTIncomeValid = "codehash_genesis_nft_income_valid"
	DBDirUnCheckNftIncome              = "uncheck_nft_income"
	DBDirUsedNFTIncome                 = "used_nft_income"
	DBDirInvalidNftOutpoint            = "invalid_nft_outpoint"
)

var (
	// ErrNotFound is returned when a key is not found in the database
	ErrNotFound  = errors.New("not found")
	maxBatchSize = int(4) * 1024 * 1024
	// Create custom log disabler
	noopLogger = &customLogger{}
)

// Custom logger - outputs nothing
type customLogger struct{}

func (l *customLogger) Infof(format string, args ...interface{})  {}
func (l *customLogger) Fatalf(format string, args ...interface{}) {}
func (l *customLogger) Errorf(format string, args ...interface{}) {}

type PebbleStore struct {
	shards []*pebble.DB
	mu     sync.RWMutex
}

type MetaStore struct {
	db *pebble.DB
}

func DbInit(params config.IndexerParams) {
	maxBatchSize = params.MaxBatchSizeMB * 1024 * 1024
}
func (m *MetaStore) Get(key []byte) ([]byte, error) {
	value, closer, err := m.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), nil
}

func (m *MetaStore) Set(key, value []byte) error {
	return m.db.Set(key, value, pebble.Sync)
}

func (m *MetaStore) Close() error {
	// Sync before closing
	if err := m.db.LogData(nil, pebble.Sync); err != nil {
		return err
	}
	return m.db.Close()
}

func (m *MetaStore) Sync() error {
	return m.db.LogData(nil, pebble.Sync)
}

type StoreType int

const (
	StoreTypeUTXO StoreType = iota
	StoreTypeIncome
	StoreTypeSpend
	StoreTypeMeta
	StoreTypeContractFTUTXO
	StoreTypeAddressFTIncome
	StoreTypeAddressFTSpend
	StoreTypeContractFTInfo
	StoreTypeContractFTGenesis
	StoreTypeContractFTGenesisOutput
	StoreTypeContractFTGenesisUTXO
	StoreTypeAddressFTIncomeValid
	StoreTypeUnCheckFtIncome
	StoreTypeUsedFTIncome
	StoreTypeUniqueFTIncome
	StoreTypeUniqueFTSpend
	StoreTypeInvalidFtOutpoint

	StoreTypeContractFTInfoSensibleId
	StoreTypeContractFTSupply
	StoreTypeContractFTBurn
	StoreTypeContractFTOwnersIncomeValid
	StoreTypeContractFTOwnersIncome
	StoreTypeContractFTOwnersSpend
	StoreTypeContractFTAddressHistory
	StoreTypeContractFTGenesisHistory

	// NFT store types
	StoreTypeContractNFTUTXO
	StoreTypeAddressNFTIncome
	StoreTypeAddressNFTSpend
	StoreTypeCodeHashGenesisNFTIncome
	StoreTypeCodeHashGenesisNFTSpend
	StoreTypeAddressSellNFTIncome
	StoreTypeAddressSellNFTSpend
	StoreTypeCodeHashGenesisSellNFTIncome
	StoreTypeCodeHashGenesisSellNFTSpend
	StoreTypeContractNFTInfo
	StoreTypeContractNFTSummaryInfo
	StoreTypeContractNFTGenesis
	StoreTypeContractNFTGenesisOutput
	StoreTypeContractNFTGenesisUTXO

	StoreTypeContractNFTAddressHistory
	StoreTypeContractNFTGenesisHistory
	StoreTypeAddressNFTIncomeValid
	StoreTypeCodeHashGenesisNFTIncomeValid
	StoreTypeUnCheckNftIncome
	StoreTypeUsedNFTIncome
	StoreTypeInvalidNftOutpoint
	StoreTypeContractNFTOwnersIncomeValid
	StoreTypeContractNFTOwnersIncome
	StoreTypeContractNFTOwnersSpend
)

func NewMetaStore(dataDir string) (*MetaStore, error) {
	dbPath := filepath.Join(dataDir, "meta")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create meta directory: %w", err)
	}
	db, err := pebble.Open(dbPath, &pebble.Options{Logger: noopLogger})
	if err != nil {
		return nil, fmt.Errorf("failed to open meta store: %w", err)
	}
	return &MetaStore{db: db}, nil
}

// Configure database options

func NewPebbleStore(params config.IndexerParams, dataDir string, storeType StoreType, shardCount int) (*PebbleStore, error) {
	if shardCount <= 0 {
		shardCount = defaultShardCount
	}
	// dbOptions := &pebble.Options{
	// 	Cache:        pebble.NewCache(int64(params.DBCacheSizeMB) * 1024 * 1024),
	// 	MemTableSize: uint64(params.MemTableSizeMB) * 1024 * 1024,
	// 	WALMinSyncInterval: func() time.Duration {
	// 		return time.Duration(params.WALSizeMB) * time.Millisecond
	// 	},
	// }
	dbOptions := &pebble.Options{
		//Logger: noopLogger,
		Levels: []pebble.LevelOptions{
			{
				Compression: pebble.NoCompression,
			},
		},
		// 优化内存表大小 - 增大可减少刷盘频率
		MemTableSize:                128 << 20, // 128MB (从64MB增加)
		MemTableStopWritesThreshold: 6,         // 允许更多内存表
		// Block cache - 降低到20MB，主要缓存Index/Filter blocks
		// 6 shard × 3 stores × 20MB = 360MB，节省内存优先给UTXO缓存
		Cache: pebble.NewCache(20 << 20), // 20MB per shard
		// 增大 L0 文件数量阈值，减少压缩触发频率
		L0CompactionThreshold: 10, // 从8增加到10
		L0StopWritesThreshold: 32, // 从24增加到32
		// 增加并发压缩数提高吞吐
		MaxConcurrentCompactions: func() int { return 6 }, // 从4增加到6
		// 增加最大打开文件数
		MaxOpenFiles: 10000, // 默认1000
	}
	store := &PebbleStore{
		shards: make([]*pebble.DB, shardCount),
	}

	for i := 0; i < shardCount; i++ {
		var dbPath string
		switch storeType {
		case StoreTypeUTXO:
			dbPath = filepath.Join(dataDir, DBDirUTXO, fmt.Sprintf("shard_%d", i))
		case StoreTypeIncome:
			dbPath = filepath.Join(dataDir, DBDirIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeSpend:
			dbPath = filepath.Join(dataDir, DBDirSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTUTXO:
			dbPath = filepath.Join(dataDir, DBDirContractFTUTXO, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressFTIncome:
			dbPath = filepath.Join(dataDir, DBDirAddressFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressFTSpend:
			dbPath = filepath.Join(dataDir, DBDirAddressFTSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTInfo:
			dbPath = filepath.Join(dataDir, DBDirContractFTInfo, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTGenesis:
			dbPath = filepath.Join(dataDir, DBDirContractFTGenesis, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTGenesisOutput:
			dbPath = filepath.Join(dataDir, DBDirContractFTGenesisOutput, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTGenesisUTXO:
			dbPath = filepath.Join(dataDir, DBDirContractFTGenesisUTXO, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressFTIncomeValid:
			dbPath = filepath.Join(dataDir, DBDirAddressFTIncomeValid, fmt.Sprintf("shard_%d", i))
		case StoreTypeUnCheckFtIncome:
			dbPath = filepath.Join(dataDir, DBDirUnCheckFtIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeUsedFTIncome:
			dbPath = filepath.Join(dataDir, DBDirUsedFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeUniqueFTIncome:
			dbPath = filepath.Join(dataDir, DBDirUniqueFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeUniqueFTSpend:
			dbPath = filepath.Join(dataDir, DBDirUniqueFTSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeInvalidFtOutpoint:
			dbPath = filepath.Join(dataDir, DBDirInvalidFtOutpoint, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTInfoSensibleId:
			dbPath = filepath.Join(dataDir, DBDirContractFTInfoSensibleId, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTSupply:
			dbPath = filepath.Join(dataDir, DBDirContractFTSupply, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTBurn:
			dbPath = filepath.Join(dataDir, DBDirContractFTBurn, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTOwnersIncomeValid:
			dbPath = filepath.Join(dataDir, DBDirContractFTOwnersIncomeValid, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTOwnersIncome:
			dbPath = filepath.Join(dataDir, DBDirContractFTOwnersIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTOwnersSpend:
			dbPath = filepath.Join(dataDir, DBDirContractFTOwnersSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTAddressHistory:
			dbPath = filepath.Join(dataDir, DBDirContractFTAddressHistory, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractFTGenesisHistory:
			dbPath = filepath.Join(dataDir, DBDirContractFTGenesisHistory, fmt.Sprintf("shard_%d", i))
		// NFT cases
		case StoreTypeContractNFTUTXO:
			dbPath = filepath.Join(dataDir, DBDirContractNFTUTXO, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressNFTIncome:
			dbPath = filepath.Join(dataDir, DBDirAddressNFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressNFTSpend:
			dbPath = filepath.Join(dataDir, DBDirAddressNFTSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeCodeHashGenesisNFTIncome:
			dbPath = filepath.Join(dataDir, DBDirCodeHashGenesisNFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeCodeHashGenesisNFTSpend:
			dbPath = filepath.Join(dataDir, DBDirCodeHashGenesisNFTSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressSellNFTIncome:
			dbPath = filepath.Join(dataDir, DBDirAddressSellNFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressSellNFTSpend:
			dbPath = filepath.Join(dataDir, DBDirAddressSellNFTSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeCodeHashGenesisSellNFTIncome:
			dbPath = filepath.Join(dataDir, DBDirCodeHashGenesisSellNFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeCodeHashGenesisSellNFTSpend:
			dbPath = filepath.Join(dataDir, DBDirCodeHashGenesisSellNFTSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTInfo:
			dbPath = filepath.Join(dataDir, DBDirContractNFTInfo, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTSummaryInfo:
			dbPath = filepath.Join(dataDir, DBDirContractNFTSummaryInfo, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTGenesis:
			dbPath = filepath.Join(dataDir, DBDirContractNFTGenesis, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTGenesisOutput:
			dbPath = filepath.Join(dataDir, DBDirContractNFTGenesisOutput, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTGenesisUTXO:
			dbPath = filepath.Join(dataDir, DBDirContractNFTGenesisUTXO, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTOwnersIncomeValid:
			dbPath = filepath.Join(dataDir, DBDirContractNFTOwnersIncomeValid, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTOwnersIncome:
			dbPath = filepath.Join(dataDir, DBDirContractNFTOwnersIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTOwnersSpend:
			dbPath = filepath.Join(dataDir, DBDirContractNFTOwnersSpend, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTAddressHistory:
			dbPath = filepath.Join(dataDir, DBDirContractNFTAddressHistory, fmt.Sprintf("shard_%d", i))
		case StoreTypeContractNFTGenesisHistory:
			dbPath = filepath.Join(dataDir, DBDirContractNFTGenesisHistory, fmt.Sprintf("shard_%d", i))
		case StoreTypeAddressNFTIncomeValid:
			dbPath = filepath.Join(dataDir, DBDirAddressNFTIncomeValid, fmt.Sprintf("shard_%d", i))
		case StoreTypeCodeHashGenesisNFTIncomeValid:
			dbPath = filepath.Join(dataDir, DBDirCodeHashGenesisNFTIncomeValid, fmt.Sprintf("shard_%d", i))
		case StoreTypeUnCheckNftIncome:
			dbPath = filepath.Join(dataDir, DBDirUnCheckNftIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeUsedNFTIncome:
			dbPath = filepath.Join(dataDir, DBDirUsedNFTIncome, fmt.Sprintf("shard_%d", i))
		case StoreTypeInvalidNftOutpoint:
			dbPath = filepath.Join(dataDir, DBDirInvalidNftOutpoint, fmt.Sprintf("shard_%d", i))
		}
		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}

		db, err := pebble.Open(dbPath, dbOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to open shard %d: %w", i, err)
		}
		store.shards[i] = db
	}

	return store, nil
}

func (s *PebbleStore) getShard(key string) *pebble.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := xxhash.Sum64String(key)
	return s.shards[h%uint64(len(s.shards))]
}

func (s *PebbleStore) GetWithShard(key []byte) ([]byte, *pebble.DB, error) {
	db := s.getShard(string(key))
	value, closer, err := db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, db, ErrNotFound
		}
		return nil, db, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), db, nil
}

// 估算统计
func (s *PebbleStore) IncrementalKeyCount(lastKeys map[int][]byte) (uint64, map[int][]byte, error) {
	var totalCount uint64
	updatedLastKeys := make(map[int][]byte) // 用于存储更新后的 lastKeys

	for shardIdx, db := range s.shards {
		iter, err := db.NewIter(nil)
		if err != nil {
			continue
		}
		defer iter.Close()

		// 获取当前分片的 lastKey
		lastKey, exists := lastKeys[shardIdx]

		// 如果有上次的最后一个键，从该键开始
		if exists && len(lastKey) > 0 {
			iter.SeekGE(lastKey)
			if iter.Valid() {
				iter.Next() // 跳过上次的最后一个键
			}
		} else {
			iter.First()
		}

		// 迭代统计
		var shardCount uint64
		var lastSeenKey []byte // 临时变量记录最后一个键
		for iter.Valid() {
			shardCount++
			lastSeenKey = append([]byte(nil), iter.Key()...) // 记录当前键
			iter.Next()
		}

		if err := iter.Error(); err != nil {
			return 0, nil, fmt.Errorf("iteration error on shard %d: %w", shardIdx, err)
		}

		// 更新当前分片的最后一个键
		if len(lastSeenKey) > 0 {
			updatedLastKeys[shardIdx] = lastSeenKey
		}

		totalCount += shardCount
	}

	return totalCount, updatedLastKeys, nil
}

// ScanRecentUTXOs scans UTXOs in reverse order (newest first) up to maxCount
// Returns map of "txid:index" -> "address@amount@blockTime"
func (s *PebbleStore) ScanRecentUTXOs(maxCount int, sampleRate int) (map[string]string, error) {
	result := make(map[string]string, maxCount)
	perShardLimit := (maxCount + len(s.shards) - 1) / len(s.shards)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for shardIdx, db := range s.shards {
		wg.Add(1)
		go func(idx int, shard *pebble.DB) {
			defer wg.Done()

			iter, err := shard.NewIter(nil)
			if err != nil {
				return
			}
			defer iter.Close()

			// Start from the end (newest UTXOs)
			if !iter.Last() {
				return
			}

			count := 0
			scanned := 0
			for iter.Valid() && count < perShardLimit {
				scanned++
				// Sample: only take every Nth item for faster warmup
				if scanned%sampleRate == 0 {
					key := string(iter.Key())
					value := string(iter.Value())

					mu.Lock()
					if len(result) < maxCount {
						result[key] = value
						count++
					}
					mu.Unlock()

					if len(result) >= maxCount {
						break
					}
				}

				iter.Prev()
			}
		}(shardIdx, db)
	}

	wg.Wait()
	return result, nil
}
func (s *PebbleStore) BulkQueryMapConcurrent(keys []string, concurrency int) (map[string][]byte, error) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	type job struct {
		key string
	}

	type result struct {
		key   string
		value []byte
		err   error
	}

	jobsCh := make(chan job, len(keys))
	resultsCh := make(chan result, len(keys))
	errCh := make(chan error, 1)

	var wg sync.WaitGroup

	// 启动并发查询的 Goroutines
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobsCh {
				db := s.getShard(j.key)
				value, closer, err := db.Get([]byte(j.key))
				if err != nil {
					if err == pebble.ErrNotFound {
						resultsCh <- result{key: j.key, value: nil, err: nil}
					} else {
						resultsCh <- result{key: j.key, err: err}
					}
					continue
				}

				// 复制数据并关闭资源
				valueCopy := append([]byte(nil), value...)
				closer.Close()

				resultsCh <- result{key: j.key, value: valueCopy, err: nil}
			}
		}()
	}

	// 分发任务
	go func() {
		for _, key := range keys {
			jobsCh <- job{key: key}
		}
		close(jobsCh)
	}()

	// 收集结果
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	results := make(map[string][]byte)
	var finalErr error

	for r := range resultsCh {
		if r.err != nil {
			finalErr = r.err
			break
		}
		if r.value != nil {
			results[r.key] = r.value
		}
	}

	// 检查是否有错误
	select {
	case err := <-errCh:
		return nil, err
	default:
		return results, finalErr
	}
}
func (s *PebbleStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	for _, db := range s.shards {
		if closeErr := db.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

type Batch struct {
	batches []*pebble.Batch
	store   *PebbleStore
}

func (s *PebbleStore) NewBatch() *Batch {
	return &Batch{
		batches: make([]*pebble.Batch, len(s.shards)),
		store:   s,
	}
}

func (b *Batch) Set(key, value []byte) error {
	db := b.store.getShard(string(key))
	shardIdx := b.store.getShardIndex(string(key))

	if b.batches[shardIdx] == nil {
		b.batches[shardIdx] = db.NewBatch()
	}
	return b.batches[shardIdx].Set(key, value, nil)
}

func (b *Batch) Commit() error {
	for _, batch := range b.batches {
		if batch != nil {
			if err := batch.Commit(nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *PebbleStore) getShardIndex(key string) int {
	h := xxhash.Sum64String(key)
	return int(h % uint64(len(s.shards)))
}

// BulkWriteMapConcurrent concurrently writes a map with many keys to corresponding shards
func (s *PebbleStore) BulkWriteMapConcurrent(data *map[string][]string, concurrency int) error {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	// Allocate workers based on shard count
	type job struct {
		shardIdx int
		key      string
		value    []byte
	}

	jobsCh := make(chan job, len(*data))
	errCh := make(chan error, 1)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var currentBatch *pebble.Batch
			var currentShardIdx int

			for job := range jobsCh {
				db := s.shards[job.shardIdx]

				// Commit current batch when switching shards
				if currentBatch != nil && currentShardIdx != job.shardIdx {
					if err := currentBatch.Commit(pebble.Sync); err != nil {
						select {
						case errCh <- fmt.Errorf("commit failed on shard %d: %w", currentShardIdx, err):
						default:
						}
						return
					}
					currentBatch.Reset()
					currentBatch = nil
				}

				// Initialize batch
				if currentBatch == nil {
					currentBatch = db.NewBatch()
					currentShardIdx = job.shardIdx
				}

				// Write data
				if err := currentBatch.Set([]byte(job.key), job.value, nil); err != nil {
					select {
					case errCh <- fmt.Errorf("set failed on shard %d: %w", job.shardIdx, err):
					default:
					}
					return
				}

				// Control batch size (e.g., 4MB)
				//if currentBatch.Len() > 4<<20 { // 4MB
				if currentBatch.Len() > maxBatchSize {
					if err := currentBatch.Commit(pebble.Sync); err != nil {
						select {
						case errCh <- fmt.Errorf("commit failed on shard %d: %w", job.shardIdx, err):
						default:
						}
						return
					}
					currentBatch = db.NewBatch()
				}
			}

			// Commit final batch
			if currentBatch != nil {
				if err := currentBatch.Commit(pebble.Sync); err != nil {
					select {
					case errCh <- fmt.Errorf("final commit failed on shard %d: %w", currentShardIdx, err):
					default:
					}
				}
			}
		}()
	}

	// Send tasks
	for key, values := range *data {
		fmt.Println("key:", key, "values:", strings.Join(values, ","))
		shardIdx := s.getShardIndex(key)
		valueBytes := []byte(strings.Join(values, ",")) // Can be replaced with other serialization methods
		jobsCh <- job{
			shardIdx: shardIdx,
			key:      key,
			value:    valueBytes,
		}
	}
	close(jobsCh)

	// Wait for completion
	go func() {
		wg.Wait()
	}()

	// Check for errors
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}
func (s *PebbleStore) Get(key []byte) ([]byte, error) {
	db := s.getShard(string(key))
	value, closer, err := db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), nil
}

func (s *PebbleStore) Delete(key []byte) error {
	db := s.getShard(string(key))
	return db.Delete(key, pebble.Sync)
}
func (s *PebbleStore) BatchDelete(keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	const batchLimit = 10000 // 每批最多处理 10000 个 key
	shardCount := len(s.shards)
	// 按分片分组
	shardKeys := make(map[int][][]byte, shardCount)
	for _, key := range keys {
		idx := s.getShardIndex(key)
		shardKeys[idx] = append(shardKeys[idx], []byte(key))
	}

	for idx, keyList := range shardKeys {
		db := s.shards[idx]
		total := len(keyList)
		for start := 0; start < total; start += batchLimit {
			end := start + batchLimit
			if end > total {
				end = total
			}
			batch := db.NewBatch()
			for _, key := range keyList[start:end] {
				if err := batch.Delete(key, nil); err != nil {
					batch.Close()
					return fmt.Errorf("delete failed on shard %d: %w", idx, err)
				}
			}
			if err := batch.Commit(pebble.Sync); err != nil {
				batch.Close()
				return fmt.Errorf("commit failed on shard %d: %w", idx, err)
			}
			batch.Close()
		}
	}
	return nil
}
func (s *PebbleStore) BatchDeleteByMap(data map[string][]string) error {
	if len(data) == 0 {
		return nil
	}
	// 1. 批量查询原有数据
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	originMap, err := s.BulkQueryMapConcurrent(keys, runtime.NumCPU())
	if err != nil {
		return fmt.Errorf("bulk query failed: %w", err)
	}

	// 2. 按分片分组待更新和待删除
	type op struct {
		key   []byte
		value []byte // 如果 value==nil 表示删除
	}
	shardOps := make(map[int][]op, len(s.shards))

	for k, delVals := range data {
		origin := ""
		if v, ok := originMap[k]; ok && v != nil {
			origin = string(v)
		}
		originSlice := []string{}
		if origin != "" {
			originSlice = strings.Split(origin, ",")
		}
		// 删除 delVals
		originSet := make(map[string]struct{}, len(originSlice))
		for _, v := range originSlice {
			originSet[v] = struct{}{}
		}
		for _, v := range delVals {
			delete(originSet, v)
		}
		// 重新拼接
		newVals := make([]string, 0, len(originSet))
		for v := range originSet {
			if v != "" {
				newVals = append(newVals, v)
			}
		}
		idx := s.getShardIndex(k)
		if len(newVals) == 0 {
			// 全部删除
			shardOps[idx] = append(shardOps[idx], op{key: []byte(k), value: nil})
		} else {
			// 更新
			shardOps[idx] = append(shardOps[idx], op{key: []byte(k), value: []byte(strings.Join(newVals, ","))})
		}
	}

	// 3. 批量执行
	const batchLimit = 10000
	for idx, ops := range shardOps {
		db := s.shards[idx]
		total := len(ops)
		for start := 0; start < total; start += batchLimit {
			end := start + batchLimit
			if end > total {
				end = total
			}
			batch := db.NewBatch()
			for _, op := range ops[start:end] {
				if op.value == nil {
					if err := batch.Delete(op.key, nil); err != nil {
						batch.Close()
						return fmt.Errorf("delete failed on shard %d: %w", idx, err)
					}
				} else {
					if err := batch.Set(op.key, op.value, nil); err != nil {
						batch.Close()
						return fmt.Errorf("set failed on shard %d: %w", idx, err)
					}
				}
			}
			if err := batch.Commit(pebble.Sync); err != nil {
				batch.Close()
				return fmt.Errorf("commit failed on shard %d: %w", idx, err)
			}
			batch.Close()
		}
	}
	return nil
}

// Sync 将所有分片的 WAL 刷新到磁盘
// 建议在每个区块处理完成后调用，确保数据持久化
func (s *PebbleStore) Sync() error {
	for i, db := range s.shards {
		if err := db.LogData(nil, pebble.Sync); err != nil {
			return fmt.Errorf("failed to sync shard %d: %w", i, err)
		}
	}
	return nil
}

func (s *PebbleStore) Set(key, value []byte) error {
	db := s.getShard(string(key))
	return db.Set(key, value, pebble.Sync)
}

func (s *PebbleStore) Put(key, value []byte) error {
	db := s.getShard(string(key))
	return db.Set(key, value, nil)
}

func (s *PebbleStore) GetLastHeight() (int, error) {
	key := []byte("last_height")
	data, err := s.Get(key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return strconv.Atoi(string(data))
}

func (s *PebbleStore) SaveLastHeight(height int) error {
	key := []byte("last_height")
	return s.Put(key, []byte(strconv.Itoa(height)))
}

// BulkMergeMapConcurrent performs concurrent bulk merge operations on the PebbleStore
func (s *PebbleStore) BulkMergeMapConcurrent(data *map[string][]string, concurrency int) error {
	if data == nil || len(*data) == 0 {
		return nil
	}

	shardCount := len(s.shards)
	type job struct {
		key   string
		value []byte
	}

	// One channel per shard
	shardChans := make([]chan job, shardCount)
	for i := range shardChans {
		shardChans[i] = make(chan job, 1024)
	}

	// 使用带缓冲的 errCh，避免阻塞
	errCh := make(chan error, shardCount)
	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//log.Printf("[BulkMerge] Starting with %d shards, %d items", shardCount, len(*data))

	// Start one goroutine per shard
	for shardIdx := range s.shards {
		wg.Add(1)
		go func(shardIdx int) {
			defer wg.Done()

			// 确保 batch 总是被关闭
			var batch *pebble.Batch
			defer func() {
				if batch != nil {
					batch.Close()
				}
				if r := recover(); r != nil {
					log.Printf("[Shard %d] panic recovered: %v", shardIdx, r)
					select {
					case errCh <- fmt.Errorf("shard %d panic: %v", shardIdx, r):
					default:
					}
				}
			}()

			db := s.shards[shardIdx]
			batch = db.NewBatch()
			count := 0
			processed := 0

			//log.Printf("[Shard %d] worker started", shardIdx)

		LOOP:
			for {
				select {
				case <-ctx.Done():
					//log.Printf("[Shard %d] context canceled, processed %d items", shardIdx, processed)
					break LOOP

				case job, ok := <-shardChans[shardIdx]:
					if !ok {
						//log.Printf("[Shard %d] channel closed, processed %d items", shardIdx, processed)
						break LOOP
					}

					if err := batch.Merge([]byte(job.key), job.value, pebble.NoSync); err != nil {
						//log.Printf("[Shard %d] merge error for key %s: %v", shardIdx, job.key, err)
						select {
						case errCh <- fmt.Errorf("shard %d merge failed: %w", shardIdx, err):
						case <-ctx.Done():
						}
						cancel()
						return
					}

					count++
					processed++

					// 批量提交控制 - 增大批次减少 fsync 次数
					if count >= 5000 || batch.Len() >= maxBatchSize {
						//log.Printf("[Shard %d] committing batch: count=%d, size=%d", shardIdx, count, batch.Len())
						if err := batch.Commit(pebble.NoSync); err != nil { // NoSync 减少 fsync 开销
							//log.Printf("[Shard %d] commit error: %v", shardIdx, err)
							select {
							case errCh <- fmt.Errorf("shard %d commit failed: %w", shardIdx, err):
							case <-ctx.Done():
							}
							cancel()
							return
						}
						batch.Reset()
						count = 0
					}
				}
			}

			// 最后一批提交
			if batch.Len() > 0 {
				//log.Printf("[Shard %d] final commit: size=%d", shardIdx, batch.Len())
				if err := batch.Commit(pebble.Sync); err != nil {
					//log.Printf("[Shard %d] final commit error: %v", shardIdx, err)
					select {
					case errCh <- fmt.Errorf("shard %d final commit failed: %w", shardIdx, err):
					case <-ctx.Done():
					}
					cancel()
					return
				}
			}

			//log.Printf("[Shard %d] completed successfully, total processed: %d", shardIdx, processed)
		}(shardIdx)
	}

	// Distribute tasks to respective channels
	//log.Printf("[Main] distributing %d tasks", len(*data))
	taskCount := 0
	for key, values := range *data {
		select {
		case <-ctx.Done():
			//log.Printf("[Main] context canceled during task distribution")
			goto WAIT_COMPLETION
		default:
		}

		shardIdx := s.getShardIndex(key)
		valueBytes := []byte("," + strings.Join(values, ","))

		select {
		case shardChans[shardIdx] <- job{key: key, value: valueBytes}:
			taskCount++
		case <-ctx.Done():
			//log.Printf("[Main] context canceled, distributed %d/%d tasks", taskCount, len(*data))
			goto WAIT_COMPLETION
		}
	}

	//log.Printf("[Main] distributed all %d tasks", taskCount)

WAIT_COMPLETION:
	// Close all channels
	for _, ch := range shardChans {
		close(ch)
		//log.Printf("[Main] closed channel for shard %d", i)
	}

	// 等待所有 goroutine 完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// 等待完成或错误
	select {
	case <-done:
		//log.Printf("[Main] all workers completed")
	case err := <-errCh:
		log.Printf("[Main] received error: %v", err)
		cancel() // 取消其他 goroutine
		<-done   // 等待所有 goroutine 退出
		return err
	}

	// 检查是否还有其他错误
	close(errCh)
	for err := range errCh {
		if err != nil {
			log.Printf("[Main] additional error: %v", err)
			return err
		}
	}

	//log.Printf("[Main] BulkMergeMapConcurrent completed successfully")
	return nil
}
func (s *PebbleStore) BulkMergeMapConcurrentBak2(data *map[string][]string, concurrency int) error {
	shardCount := len(s.shards)
	type job struct {
		key   string
		value []byte
	}
	// One channel per shard
	shardChans := make([]chan job, shardCount)
	for i := range shardChans {
		shardChans[i] = make(chan job, 1024)
	}
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	// Start one goroutine per shard
	for shardIdx := range s.shards {
		wg.Add(1)
		go func(shardIdx int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Println("panic in shard", shardIdx, ":", r)
				}
			}()
			db := s.shards[shardIdx]
			batch := db.NewBatch()
			defer batch.Close()
			count := 0
			for job := range shardChans[shardIdx] {
				if err := batch.Merge([]byte(job.key), job.value, pebble.Sync); err != nil {
					log.Println("==>Error merging batch:", err)
					select {
					case errCh <- err:
					default:
					}
					return
				}
				count++
				if count >= 1000 || batch.Len() >= maxBatchSize {
					if err := batch.Commit(pebble.Sync); err != nil {
						log.Println("==>Error committing batch:", err)
						select {
						case errCh <- err:
						default:
						}
						return
					}
					batch.Reset()
					count = 0
				}
			}
			if batch.Len() > 0 {
				err := batch.Commit(pebble.Sync)
				if err != nil {
					log.Println("==>Error2 committing batch:", err)
				}
			}
		}(shardIdx)
	}

	// Distribute tasks to respective channels
	for key, values := range *data {
		shardIdx := s.getShardIndex(key)
		valueBytes := []byte("," + strings.Join(values, ","))
		shardChans[shardIdx] <- job{
			key:   key,
			value: valueBytes,
		}
	}
	// Close all channels
	for _, ch := range shardChans {
		close(ch)
	}
	wg.Wait()
	// select {
	// case err := <-errCh:
	// 	fmt.Println("==>Error received:", err)
	// 	return err
	// default:
	// 	return nil
	// }
	close(errCh)
	for err := range errCh {
		if err != nil {
			fmt.Println("BulkMergeMapConcurrent error:", err)
			return err
		}
	}
	return nil
}
func (s *PebbleStore) QueryUTXOAddresses(outpoints *[]string, concurrency int) (map[string][]string, error) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	if len(*outpoints) == 0 {
		return make(map[string][]string), nil
	}

	type job struct {
		key string // txid:output_index
	}

	type result struct {
		key     string
		address string
		err     error
	}

	// 使用带缓冲的 channel，避免阻塞
	jobsCh := make(chan job, len(*outpoints))
	resultsCh := make(chan result, len(*outpoints))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	//log.Printf("[QueryUTXO] Starting with %d workers, %d outpoints", concurrency, len(*outpoints))

	// Start concurrent workers
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[QueryUTXO] worker %d panic: %v", workerID, r)
				}
			}()

			processed := 0
			for {
				select {
				case <-ctx.Done():
					//log.Printf("[QueryUTXO] worker %d canceled, processed %d", workerID, processed)
					return
				case j, ok := <-jobsCh:
					if !ok {
						//log.Printf("[QueryUTXO] worker %d finished, processed %d", workerID, processed)
						return
					}

					processed++

					// 优化：减少字符串分割次数
					colonIdx := strings.Index(j.key, ":")
					if colonIdx == -1 || colonIdx >= len(j.key)-1 {
						select {
						case resultsCh <- result{key: j.key, err: fmt.Errorf("invalid key format: %s", j.key)}:
						case <-ctx.Done():
							return
						}
						continue
					}

					txid := j.key[:colonIdx]
					db := s.getShard(txid)

					value, closer, err := db.Get([]byte(txid))
					if err != nil {
						if err == pebble.ErrNotFound {
							// 不发送空结果，直接跳过
							continue
						} else {
							select {
							case resultsCh <- result{key: j.key, err: err}:
							case <-ctx.Done():
								return
							}
						}
						continue
					}

					// 优化：直接在 worker 中解析地址
					address, parseErr := getAddressByStrSafe(j.key, value, colonIdx)
					closer.Close()

					if parseErr != nil || address == "errAddress" || address == "" {
						// 跳过无效地址，不发送结果
						continue
					}

					// 非阻塞发送结果
					select {
					case resultsCh <- result{key: j.key, address: address, err: nil}:
					case <-ctx.Done():
						return
					}
				}
			}
		}(w)
	}

	// Send tasks
	go func() {
		defer close(jobsCh)
		//log.Printf("[QueryUTXO] Sending %d jobs", len(*outpoints))
		for i, outkey := range *outpoints {
			select {
			case jobsCh <- job{key: outkey}:
				if i%10000 == 0 && i > 0 {
					//log.Printf("[QueryUTXO] Sent %d/%d jobs", i, len(*outpoints))
				}
			case <-ctx.Done():
				//log.Printf("[QueryUTXO] Job sender canceled at %d/%d", i, len(*outpoints))
				return
			}
		}
		//log.Printf("[QueryUTXO] All jobs sent")
	}()

	// 等待所有 worker 完成，然后关闭 resultsCh
	go func() {
		wg.Wait()
		//log.Printf("[QueryUTXO] All workers finished, closing resultsCh")
		close(resultsCh)
	}()

	// 直接构建最终结果，预分配容量优化
	finalResults := make(map[string][]string)
	var finalErr error
	resultCount := 0

	//log.Printf("[QueryUTXO] Starting result collection")

	// 收集结果，并设置超时保护
	timeout := time.NewTimer(5 * time.Minute) // 5分钟超时保护
	defer timeout.Stop()

	for {
		select {
		case r, ok := <-resultsCh:
			if !ok {
				// channel 已关闭，说明所有结果收集完成
				//log.Printf("[QueryUTXO] Result collection finished, got %d results", resultCount)
				goto DONE
			}

			if r.err != nil {
				log.Printf("[QueryUTXO] Got error result: %v", r.err)
				finalErr = r.err
				cancel() // 取消其他 goroutine
				goto DONE
			}

			// 只有有效地址才会到达这里
			if r.address != "" {
				if existing, exists := finalResults[r.address]; exists {
					finalResults[r.address] = append(existing, r.key)
				} else {
					finalResults[r.address] = make([]string, 0, 4) // 预分配容量
					finalResults[r.address] = append(finalResults[r.address], r.key)
				}
				resultCount++

				if resultCount%1000 == 0 {
					//log.Printf("[QueryUTXO] Collected %d results so far", resultCount)
				}
			}

		case <-timeout.C:
			log.Printf("[QueryUTXO] Timeout after 5 minutes, canceling")
			cancel()
			finalErr = fmt.Errorf("query timeout after 5 minutes")
			goto DONE

		case <-ctx.Done():
			//log.Printf("[QueryUTXO] Context canceled during result collection")
			goto DONE
		}
	}

DONE:
	// 强制清空剩余的 resultsCh
	go func() {
		drained := 0
		for range resultsCh {
			drained++
		}
		if drained > 0 {
			log.Printf("[QueryUTXO] Drained %d remaining results", drained)
		}
	}()

	// 确保所有 goroutine 退出
	cancel()

	//log.Printf("[QueryUTXO] Final results: %d addresses, error: %v", len(finalResults), finalErr)
	return finalResults, finalErr
}

// QueryUTXOAddresses2 is a simplified and optimized version using batch queries
func (s *PebbleStore) QueryUTXOAddresses2(outpoints *[]string) (map[string][]string, error) {
	if len(*outpoints) == 0 {
		return make(map[string][]string), nil
	}

	//t0 := time.Now()
	// Step 1: Group outpoints by txid to avoid duplicate queries
	// txid -> []index
	type outpointInfo struct {
		txid     string
		indexStr string
		fullKey  string
	}

	txidMap := make(map[string][]outpointInfo)
	for _, op := range *outpoints {
		colonIdx := strings.LastIndexByte(op, ':')
		if colonIdx == -1 {
			continue
		}
		txid := op[:colonIdx]
		indexStr := op[colonIdx+1:]

		txidMap[txid] = append(txidMap[txid], outpointInfo{
			txid:     txid,
			indexStr: indexStr,
			fullKey:  op,
		})
	}

	// Log deduplication effect
	//log.Printf("[QueryUTXO] Input: %d outpoints, Unique txids: %d, Dedup rate: %.1f%%",
	//	len(*outpoints), len(txidMap), float64(len(*outpoints)-len(txidMap))*100/float64(len(*outpoints)))

	// Step 2: Batch query all unique txids
	uniqueTxids := make([]string, 0, len(txidMap))
	for txid := range txidMap {
		uniqueTxids = append(uniqueTxids, txid)
	}

	// Step 3: Parallel batch query by shard
	type shardBatch struct {
		shardIdx int
		txids    []string
	}

	// Group txids by shard
	shardBatches := make(map[int][]string)
	for _, txid := range uniqueTxids {
		shardIdx := s.getShardIndex(txid)
		shardBatches[shardIdx] = append(shardBatches[shardIdx], txid)
	}

	// Query results cache: txid -> value (use sharded cache to reduce lock contention)
	numCacheShards := 64
	type cacheShardType struct {
		mu   sync.Mutex
		data map[string][]byte
	}
	cacheShards := make([]cacheShardType, numCacheShards)
	for i := range cacheShards {
		cacheShards[i].data = make(map[string][]byte)
	}
	var wg sync.WaitGroup

	// Parallel query each shard's txids with higher concurrency
	for shardIdx, txids := range shardBatches {
		wg.Add(1)
		go func(shardIdx int, txids []string) {
			defer wg.Done()
			db := s.shards[shardIdx]

			// Use mini worker pool within each shard for parallel Get
			miniConcurrency := 64 // 增加到64个并发，提升查询速度
			jobsCh := make(chan string, len(txids))
			var miniWg sync.WaitGroup

			for i := 0; i < miniConcurrency; i++ {
				miniWg.Add(1)
				go func() {
					defer miniWg.Done()
					// Local buffer to reduce lock frequency
					localCache := make(map[string][]byte, 32)
					for txid := range jobsCh {
						value, closer, err := db.Get([]byte(txid))
						if err != nil {
							continue
						}
						// Copy value before closer
						valueCopy := append([]byte(nil), value...)
						closer.Close()

						localCache[txid] = valueCopy
						// Batch write to shared cache every 32 items
						if len(localCache) >= 32 {
							for k, v := range localCache {
								cacheIdx := xxhash.Sum64String(k) % uint64(numCacheShards)
								cacheShards[cacheIdx].mu.Lock()
								cacheShards[cacheIdx].data[k] = v
								cacheShards[cacheIdx].mu.Unlock()
							}
							localCache = make(map[string][]byte, 32)
						}
					}
					// Flush remaining items
					for k, v := range localCache {
						cacheIdx := xxhash.Sum64String(k) % uint64(numCacheShards)
						cacheShards[cacheIdx].mu.Lock()
						cacheShards[cacheIdx].data[k] = v
						cacheShards[cacheIdx].mu.Unlock()
					}
				}()
			}

			// Send jobs
			for _, txid := range txids {
				jobsCh <- txid
			}
			close(jobsCh)
			miniWg.Wait()
		}(shardIdx, txids)
	}

	wg.Wait()

	// Step 4: Parse addresses from cached values
	numShards := 32
	type shardMap struct {
		mu   sync.Mutex
		data map[string][]string
	}
	shards := make([]shardMap, numShards)
	for i := range shards {
		shards[i].data = make(map[string][]string)
	}

	// Process all outpoints using cached txid data
	concurrency := runtime.NumCPU() * 4
	jobsCh := make(chan outpointInfo, len(*outpoints))

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for info := range jobsCh {
				// Use sharded cache
				cacheIdx := xxhash.Sum64String(info.txid) % uint64(numCacheShards)
				cacheShards[cacheIdx].mu.Lock()
				value, exists := cacheShards[cacheIdx].data[info.txid]
				cacheShards[cacheIdx].mu.Unlock()

				if !exists {
					continue
				}

				// Parse address from cached value
				address := extractAddressFromValue(value, info.indexStr)
				if address != "" {
					shardIdx := xxhash.Sum64String(address) % uint64(numShards)
					shards[shardIdx].mu.Lock()
					shards[shardIdx].data[address] = append(shards[shardIdx].data[address], info.fullKey)
					shards[shardIdx].mu.Unlock()
				}
			}
		}()
	}

	// Send all outpoint jobs
	for _, outpointList := range txidMap {
		for _, info := range outpointList {
			jobsCh <- info
		}
	}
	close(jobsCh)

	wg.Wait()

	// Merge results from all shards
	results := make(map[string][]string)
	for i := range shards {
		for k, v := range shards[i].data {
			results[k] = v
		}
	}

	return results, nil
}

func extractAddressFromValue(value []byte, indexStr string) string {
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return ""
	}

	start := 0
	// Handle potential leading comma
	if len(value) > 0 && value[0] == ',' {
		start = 1
	}

	current := 0
	for i := start; i < len(value); i++ {
		if value[i] == ',' {
			if current == index {
				return parseAddress(value[start:i])
			}
			current++
			start = i + 1
		}
	}

	if current == index {
		return parseAddress(value[start:])
	}
	return ""
}

func parseAddress(segment []byte) string {
	for i, b := range segment {
		if b == '@' {
			return string(segment[:i])
		}
	}
	return ""
}

func getAddressByStrSafe(key string, valueBytes []byte, colonIdx int) (string, error) {
	if colonIdx == -1 || colonIdx >= len(key)-1 {
		return "errAddress", fmt.Errorf("invalid key format: %s", key)
	}
	//str := strings.TrimPrefix(string(utxostr), ",")
	indexStr := key[colonIdx+1:]
	txIndex, err := strconv.Atoi(indexStr)
	if err != nil {
		return "errAddress", fmt.Errorf("invalid index: %s", indexStr)
	}

	// 转换为字符串（但比原版本少了一次完整拷贝）
	valueStr := string(valueBytes)

	// 手动查找目标段，避免完整 Split
	commaCount := 0
	start := 0
	targetIndex := txIndex + 1

	for i, ch := range valueStr {
		if ch == ',' {
			if commaCount == targetIndex {
				segment := valueStr[start:i]
				atIdx := strings.Index(segment, "@")
				if atIdx == -1 {
					return "errAddress", fmt.Errorf("invalid address format")
				}
				return segment[:atIdx], nil
			}
			commaCount++
			start = i + 1
		}
	}

	// 处理最后一段
	if commaCount == targetIndex && start < len(valueStr) {
		segment := valueStr[start:]
		atIdx := strings.Index(segment, "@")
		if atIdx == -1 {
			return "errAddress", fmt.Errorf("invalid address format")
		}
		return segment[:atIdx], nil
	}

	return "errAddress", fmt.Errorf("invalid address index: %d", txIndex)
}

// QueryUTXOAddresses optimized version - only necessary modifications
func (s *PebbleStore) QueryUTXOAddressesBak(outpoints *[]string, concurrency int) (map[string][]string, error) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	type job struct {
		key string // txid:output_index
	}

	type result struct {
		key     string
		address string
		err     error
	}

	jobsCh := make(chan job, len(*outpoints))
	resultsCh := make(chan result, len(*outpoints))

	var wg sync.WaitGroup
	// testMap := make(map[string]int)
	// testMap["66c93ec8bbb2548baba1502d6a7744271ca88e999d2e20e619168dd38898cd02"] = 1 // Special address test
	// testMap["1b158e20503c3f10fac31285308fbb44ec8b7a684a95384e6f643c3e654718f8"] = 2
	// testMap["ac7521d18f7ee7ad887832312088f64e0f4ffefbe6334b237aeb2b38c0bad2be"] = 3
	// testMap["c6b2309a4cc4b52a995cc10ed7ccc50c9842bb19c26f2824517f73185ee6ca04"] = 4
	// testMap2 := make(map[string]int)
	// Start concurrent workers
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobsCh {
				txArr := strings.Split(j.key, ":")
				if len(txArr) != 2 {
					resultsCh <- result{key: j.key, err: fmt.Errorf("invalid key format: %s", j.key)}
					continue
				}
				db := s.getShard(txArr[0])
				// if _, ok := testMap[txArr[0]]; ok {
				// 	fmt.Println("Found test transaction:", j.key)
				// 	testMap2[j.key] = 1
				// }
				value, closer, err := db.Get([]byte(txArr[0]))
				if err != nil {
					if err == pebble.ErrNotFound {
						resultsCh <- result{key: j.key, address: "", err: nil}
					} else {
						resultsCh <- result{key: j.key, err: err}
					}
					continue
				}

				// Fix 1: Immediately copy data and close resources to avoid defer accumulation in loops
				valueStr := string(append([]byte(nil), value...))

				closer.Close() // Close immediately instead of deferring

				resultsCh <- result{
					key:     j.key,
					address: valueStr,
				}
			}
		}()
	}

	// Send tasks
	go func() {
		for _, outkey := range *outpoints {
			jobsCh <- job{
				key: outkey,
			}
		}
		close(jobsCh)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	results := make(map[string]string)
	var finalErr error

	for r := range resultsCh {
		if r.err != nil {
			finalErr = r.err
			break
		}
		if r.address != "" {
			results[r.key], _ = getAddressByStr(r.key, r.address)
			// if _, ok := testMap2[r.key]; ok {
			// 	fmt.Println("Get test transaction result:", r.key, r.address)
			// }
		}

	}
	finalResults := make(map[string][]string)
	for k, v := range results {
		// if v == "16Cxq6PZNKa5Gnw5GFrco5jkyrzbNQfHsR" {
		// 	fmt.Println("Found special address:", v, k)
		// }
		if v == "errAddress" {
			continue
		}
		finalResults[v] = append(finalResults[v], k)
	}

	// Fix 2: Optimize memory usage
	results = nil // Allow early garbage collection

	return finalResults, finalErr
}

func getAddressByStr(key, results string) (string, error) {
	info := strings.Split(key, ":")
	if len(info) != 2 {
		return "", fmt.Errorf("invalid key format: %s", key)
	}
	txIndex, err := strconv.Atoi(info[1])
	if err != nil {
		return "", fmt.Errorf("invalid index: %s", info[1])
	}
	addressInfo := strings.Split(results, ",")
	if len(addressInfo) <= txIndex+1 {
		return "", fmt.Errorf("invalid address index: %d", txIndex)
	}
	arr := strings.Split(addressInfo[txIndex+1], "@")
	if len(arr) != 2 {
		return "", fmt.Errorf("invalid address: %s", arr)
	}
	return arr[0], nil
}

// QueryUTXOAddresses optimized version - only necessary modifications
func (s *PebbleStore) QueryFtUTXOAddresses(outpoints *[]string, concurrency int, txPointUsedMap map[string]string) (map[string][]string, map[string][]string, error) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	type job struct {
		key string // txid:output_index
	}

	type result struct {
		key       string
		valueData string
		err       error
	}

	jobsCh := make(chan job, len(*outpoints))
	resultsCh := make(chan result, len(*outpoints))

	var wg sync.WaitGroup

	// Start concurrent workers
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobsCh {
				txArr := strings.Split(j.key, ":")
				if len(txArr) != 2 {
					resultsCh <- result{key: j.key, err: fmt.Errorf("invalid key format: %s", j.key)}
					continue
				}
				db := s.getShard(txArr[0])
				value, closer, err := db.Get([]byte(txArr[0]))
				if err != nil {
					if err == pebble.ErrNotFound {
						resultsCh <- result{key: j.key, valueData: "", err: nil}
					} else {
						resultsCh <- result{key: j.key, err: err}
					}
					continue
				}

				// Fix 1: Immediately copy data and close resources to avoid defer accumulation in loops
				valueStr := string(append([]byte(nil), value...))
				closer.Close() // Close immediately instead of deferring

				resultsCh <- result{
					key:       j.key,
					valueData: valueStr,
				}
			}
		}()
	}

	// Send tasks
	go func() {
		for _, outkey := range *outpoints {
			jobsCh <- job{
				key: outkey,
			}
		}
		close(jobsCh)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	results := make(map[string]string)
	var finalErr error

	for r := range resultsCh {
		if r.err != nil {
			finalErr = r.err
			break
		}
		if r.valueData != "" {
			//key: txid:output_index
			//value: FtAddress@CodeHash@Genesis@sensibleId@Amount@Index@Value@height@contractType
			resultInfo, err := getFtAddressByStr(r.key, r.valueData)
			if err != nil {
				finalErr = err
				break
			}
			if resultInfo != "" {
				results[r.key] = resultInfo
			}
		}
	}

	finalFtResults := make(map[string][]string)
	finalUniqueResults := make(map[string][]string)
	for k, v := range results {
		kStrs := strings.Split(k, ":")
		if len(kStrs) != 2 {
			return nil, nil, fmt.Errorf("invalid kStrs: %s", kStrs)
		}
		vStrs := strings.Split(v, "@")
		if len(vStrs) != 9 {
			return nil, nil, fmt.Errorf("invalid vStrs: %s", vStrs)
		}
		usedTxId := ""
		if usedValue, ok := txPointUsedMap[kStrs[0]+":"+kStrs[1]]; ok {
			usedTxId = usedValue
		}

		if vStrs[8] == "ft" {
			// key: FtAddress
			// value: txid@index@codeHash@genesis@sensibleId@amount@value@height@usedTxId
			finalResultKey := vStrs[0] //key: FtAddress
			finalResultValue := kStrs[0] + "@" + kStrs[1] + "@" + vStrs[1] + "@" + vStrs[2] + "@" + vStrs[3] + "@" + vStrs[4] + "@" + vStrs[6] + "@" + vStrs[7] + "@" + usedTxId
			finalFtResults[finalResultKey] = append(finalFtResults[finalResultKey], finalResultValue)
		} else if vStrs[8] == "unique" {
			// key: codeHash@genesis
			// value: txid@index@usedTxId
			finalResultKey := vStrs[1] + "@" + vStrs[2] //key: codeHash@genesis
			finalResultValue := kStrs[0] + "@" + kStrs[1] + "@" + usedTxId
			finalUniqueResults[finalResultKey] = append(finalUniqueResults[finalResultKey], finalResultValue)
		}
	}

	// Fix 2: Optimize memory usage
	results = nil // Allow early garbage collection

	return finalFtResults, finalUniqueResults, finalErr
}

// QueryNftUTXOAddresses queries NFT UTXO addresses for given outpoints
func (s *PebbleStore) QueryNftUTXOAddresses(outpoints *[]string, concurrency int, txPointUsedMap map[string]string) (map[string][]string, map[string][]string, map[string][]string, map[string][]string, error) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	type job struct {
		key string // txid:output_index
	}

	type result struct {
		key       string
		valueData string
		err       error
	}

	jobsCh := make(chan job, len(*outpoints))
	resultsCh := make(chan result, len(*outpoints))

	var wg sync.WaitGroup

	// Start concurrent workers
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobsCh {
				txArr := strings.Split(j.key, ":")
				if len(txArr) != 2 {
					resultsCh <- result{key: j.key, err: fmt.Errorf("invalid key format: %s", j.key)}
					continue
				}
				db := s.getShard(txArr[0])
				// value:NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
				value, closer, err := db.Get([]byte(txArr[0]))
				if err != nil {
					if err == pebble.ErrNotFound {
						resultsCh <- result{key: j.key, valueData: "", err: nil}
					} else {
						resultsCh <- result{key: j.key, err: err}
					}
					continue
				}

				// Immediately copy data and close resources to avoid defer accumulation in loops
				valueStr := string(append([]byte(nil), value...))
				closer.Close() // Close immediately instead of deferring

				//key: txid:output_index, value:NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
				resultsCh <- result{
					key:       j.key,
					valueData: valueStr,
				}
			}
		}()
	}

	// Send tasks
	go func() {
		for _, outkey := range *outpoints {
			jobsCh <- job{
				key: outkey,
			}
		}
		close(jobsCh)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	results := make(map[string]string)
	var finalErr error

	for r := range resultsCh {
		if r.err != nil {
			finalErr = r.err
			break
		}
		if r.valueData != "" {
			//key: txid:output_index
			//value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
			resultInfo, err := getNftAddressByStr(r.key, r.valueData)
			if err != nil {
				finalErr = err
				break
			}
			if resultInfo != "" {
				results[r.key] = resultInfo
			}
		}
	}

	finalNftResults := make(map[string][]string)
	finalCodeHashGenesisNftIncomeResults := make(map[string][]string)
	finalNftSellResults := make(map[string][]string)
	finalCodeHashGenesisSellNftIncomeResults := make(map[string][]string)
	for k, v := range results {
		//key: txid:output_index
		kStrs := strings.Split(k, ":")
		if len(kStrs) != 2 {
			return nil, nil, nil, nil, fmt.Errorf("invalid kStrs: %s", kStrs)
		}
		//NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType
		vStrs := strings.Split(v, "@")
		if len(vStrs) != 12 {
			return nil, nil, nil, nil, fmt.Errorf("invalid vStrs: %s", vStrs)
		}
		usedTxId := ""
		if usedValue, ok := txPointUsedMap[kStrs[0]+":"+kStrs[1]]; ok {
			usedTxId = usedValue
		}

		if vStrs[11] == "nft" {
			// key: NftAddress
			// value: txid@index@codeHash@genesis@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId,...
			finalResultKey := vStrs[0] //key: NftAddress
			finalResultValue := kStrs[0] + "@" + kStrs[1] + "@" + vStrs[1] + "@" + vStrs[2] + "@" + vStrs[3] + "@" + vStrs[4] + "@" + vStrs[6] + "@" + vStrs[7] + "@" + vStrs[8] + "@" + vStrs[9] + "@" + vStrs[10] + "@" + usedTxId
			finalNftResults[finalResultKey] = append(finalNftResults[finalResultKey], finalResultValue)

			// key: codeHash@genesis, value: txid@index@NftAddress@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId,...
			finalCodeHashGenesisNftIncomeKey := vStrs[1] + "@" + vStrs[2]
			finalCodeHashGenesisNftIncomeValue := kStrs[0] + "@" + kStrs[1] + "@" + vStrs[0] + "@" + vStrs[3] + "@" + vStrs[4] + "@" + vStrs[6] + "@" + vStrs[7] + "@" + vStrs[8] + "@" + vStrs[9] + "@" + vStrs[10] + "@" + usedTxId
			finalCodeHashGenesisNftIncomeResults[finalCodeHashGenesisNftIncomeKey] = append(finalCodeHashGenesisNftIncomeResults[finalCodeHashGenesisNftIncomeKey], finalCodeHashGenesisNftIncomeValue)

		} else if vStrs[11] == "nft_sell" {
			//NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType

			// key: NftAddress
			// value: txid@index@codeHash@genesis@tokenIndex@value@height@usedTxId,...
			finalResultKey := vStrs[0] //key: NftAddress
			finalResultValue := kStrs[0] + "@" + kStrs[1] + "@" + vStrs[1] + "@" + vStrs[2] + "@" + vStrs[4] + "@" + vStrs[6] + "@" + vStrs[10] + "@" + usedTxId
			finalNftSellResults[finalResultKey] = append(finalNftSellResults[finalResultKey], finalResultValue)

			// key: codeHash@genesis,
			// value: txid@index@NftAddress@tokenIndex@value@height@usedTxId,...
			finalCodeHashGenesisSellNftIncomeKey := vStrs[1] + "@" + vStrs[2]
			finalCodeHashGenesisSellNftIncomeValue := kStrs[0] + "@" + kStrs[1] + "@" + vStrs[0] + "@" + vStrs[4] + "@" + vStrs[6] + "@" + vStrs[10] + "@" + usedTxId
			finalCodeHashGenesisSellNftIncomeResults[finalCodeHashGenesisSellNftIncomeKey] = append(finalCodeHashGenesisSellNftIncomeResults[finalCodeHashGenesisSellNftIncomeKey], finalCodeHashGenesisSellNftIncomeValue)
		}
	}

	// Optimize memory usage
	results = nil // Allow early garbage collection

	return finalNftResults, finalCodeHashGenesisNftIncomeResults, finalNftSellResults, finalCodeHashGenesisSellNftIncomeResults, finalErr
}

// NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@height@contractType
func getNftAddressByStr(key, results string) (string, error) {
	info := strings.Split(key, ":")
	if len(info) != 2 {
		return "", fmt.Errorf("invalid key format: %s", key)
	}
	targetValueInfo := ""
	valueInfoList := strings.Split(results, ",")
	//NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
	for _, valueInfo := range valueInfoList {
		//NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType
		arr := strings.Split(valueInfo, "@")
		if len(arr) != 12 {
			continue
		}
		//index
		if arr[5] == info[1] {
			targetValueInfo = valueInfo
			break
		}
	}
	if targetValueInfo == "" {
		return "", nil
	}

	targetArr := strings.Split(targetValueInfo, "@")
	if len(targetArr) != 12 {
		return "", fmt.Errorf("invalid targetArr: %s", targetArr)
	}
	return targetValueInfo, nil
}

// FtAddress@CodeHash@Genesis@sensibleId@Amount@Index@Value@height@contractType
func getFtAddressByStr(key, results string) (string, error) {
	info := strings.Split(key, ":")
	if len(info) != 2 {
		return "", fmt.Errorf("invalid key format: %s", key)
	}
	// index := info[1]
	targetValueInfo := ""
	valueInfoList := strings.Split(results, ",")
	// fmt.Printf("[getFtAddressByStr]key: %s, results: %s\n", key, results)
	for _, valueInfo := range valueInfoList {
		arr := strings.Split(valueInfo, "@")
		if len(arr) != 9 {
			continue
		}
		if arr[5] == info[1] {
			targetValueInfo = valueInfo
			break
		}
	}
	if targetValueInfo == "" {
		// return "", fmt.Errorf("invalid targetValueInfo: %s", info[1])
		return "", nil
	}

	targetArr := strings.Split(targetValueInfo, "@")
	if len(targetArr) != 9 {
		return "", fmt.Errorf("invalid targetArr: %s", targetArr)
	}
	return targetValueInfo, nil
}

func (s *PebbleStore) QueryUTXOAddress(outpoint string) (string, error) {
	txArr := strings.Split(outpoint, ":")
	if len(txArr) != 2 {
		return "", fmt.Errorf("invalid key format: %s", outpoint)
	}

	// Get the corresponding shard DB
	db := s.getShard(txArr[0])

	// Query transaction information
	value, closer, err := db.Get([]byte(txArr[0]))
	if err != nil {
		if err == pebble.ErrNotFound {
			return "", ErrNotFound
		}
		return "", err
	}
	defer closer.Close()

	// Copy data
	valueStr := string(append([]byte(nil), value...))

	// Parse address information
	address, err := getAddressByStr(outpoint, valueStr)
	if err != nil {
		return "", err
	}

	return address, nil
}

func (s *PebbleStore) GetAll() (allKey, allData [][]byte, err error) {
	for _, db := range s.shards {
		iter, err := db.NewIter(nil)
		if err != nil {
			continue
		}
		defer iter.Close()
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			allKey = append(allKey, []byte(key))
			allData = append(allData, []byte(fmt.Sprintf("%s:%s", key, value)))
		}
	}
	return allKey, allData, nil
}

// BulkMergeConcurrent for processing map[string]string type data
func (s *PebbleStore) BulkMergeConcurrent(data *map[string]string, concurrency int) error {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	type job struct {
		shardIdx int
		key      string
		value    []byte
	}

	jobsCh := make(chan job, len(*data))
	errCh := make(chan error, 1)

	var wg sync.WaitGroup
	shardMutexes := make([]sync.Mutex, len(s.shards))
	shardBatches := make([]*pebble.Batch, len(s.shards))

	// Initialize batch for each shard
	for i := range shardBatches {
		shardBatches[i] = s.shards[i].NewBatch()
	}

	maxBatchItems := 1000
	batchItemCounters := make([]int, len(s.shards))

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for job := range jobsCh {
				db := s.shards[job.shardIdx]

				shardMutexes[job.shardIdx].Lock()
				batch := shardBatches[job.shardIdx]
				if batch == nil {
					batch = db.NewBatch()
					shardBatches[job.shardIdx] = batch
				}

				if err := batch.Merge([]byte(job.key), job.value, pebble.Sync); err != nil {
					shardMutexes[job.shardIdx].Unlock()
					select {
					case errCh <- fmt.Errorf("merge failed on shard %d: %w", job.shardIdx, err):
					default:
					}
					return
				}

				batchItemCounters[job.shardIdx]++
				if batchItemCounters[job.shardIdx] >= maxBatchItems || batch.Len() >= maxBatchSize {
					if err := batch.Commit(pebble.Sync); err != nil {
						shardMutexes[job.shardIdx].Unlock()
						select {
						case errCh <- fmt.Errorf("commit failed on shard %d: %w", job.shardIdx, err):
						default:
						}
						return
					}
					batch.Reset()
					batchItemCounters[job.shardIdx] = 0
				}

				shardMutexes[job.shardIdx].Unlock()
			}
		}()
	}

	// Send tasks
	for key, value := range *data {
		shardIdx := s.getShardIndex(key)
		valueBytes := []byte(value)
		jobsCh <- job{
			shardIdx: shardIdx,
			key:      key,
			value:    valueBytes,
		}
	}
	close(jobsCh)

	go func() {
		wg.Wait()
		close(errCh)
	}()

	if err := <-errCh; err != nil {
		return err
	}

	for i, batch := range shardBatches {
		shardMutexes[i].Lock()
		if batch != nil && batch.Len() > 0 {
			commitOption := pebble.Sync
			if i == len(shardBatches)-1 {
				commitOption = pebble.Sync
			}
			if err := batch.Commit(commitOption); err != nil {
				_ = batch.Close()
				shardMutexes[i].Unlock()
				return fmt.Errorf("failed to commit shard %d: %w", i, err)
			}
			_ = batch.Close()
		}
		shardMutexes[i].Unlock()
	}

	return nil
}

// GetShards returns all shards
func (s *PebbleStore) GetShards() []*pebble.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shards
}

// BulkWriteConcurrent concurrently writes a map with many keys to corresponding shards
func (s *PebbleStore) BulkWriteConcurrent(data *map[string]string, concurrency int) error {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	// Allocate workers according to shard count
	type job struct {
		shardIdx int
		key      string
		value    []byte
	}

	jobsCh := make(chan job, len(*data))
	errCh := make(chan error, 1)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var currentBatch *pebble.Batch
			var currentShardIdx int

			for job := range jobsCh {
				db := s.shards[job.shardIdx]

				// Commit current batch when switching shard
				if currentBatch != nil && currentShardIdx != job.shardIdx {
					if err := currentBatch.Commit(pebble.Sync); err != nil {
						select {
						case errCh <- fmt.Errorf("commit failed on shard %d: %w", currentShardIdx, err):
						default:
						}
						return
					}
					currentBatch.Reset()
					currentBatch = nil
				}

				// Initialize batch
				if currentBatch == nil {
					currentBatch = db.NewBatch()
					currentShardIdx = job.shardIdx
				}

				// Write data
				if err := currentBatch.Set([]byte(job.key), job.value, nil); err != nil {
					select {
					case errCh <- fmt.Errorf("set failed on shard %d: %w", job.shardIdx, err):
					default:
					}
					return
				}

				// Control batch size
				if currentBatch.Len() > maxBatchSize {
					if err := currentBatch.Commit(pebble.Sync); err != nil {
						select {
						case errCh <- fmt.Errorf("commit failed on shard %d: %w", job.shardIdx, err):
						default:
						}
						return
					}
					currentBatch = db.NewBatch()
				}
			}

			// Commit final batch
			if currentBatch != nil {
				if err := currentBatch.Commit(pebble.Sync); err != nil {
					select {
					case errCh <- fmt.Errorf("final commit failed on shard %d: %w", currentShardIdx, err):
					default:
					}
				}
			}
		}()
	}

	// Send tasks
	for key, value := range *data {
		shardIdx := s.getShardIndex(key)
		jobsCh <- job{
			shardIdx: shardIdx,
			key:      key,
			value:    []byte(value),
		}
	}
	close(jobsCh)

	// Wait for completion
	go func() {
		wg.Wait()
	}()

	// Check for errors
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}
