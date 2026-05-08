package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

const (
	statusMatch      = "match"
	statusMismatch   = "mismatch"
	statusMissingRow = "missing_row"
	balanceIndexBootstrapProgressMetaKey = "balance_index_bootstrap_progress"
)

type confirmedBalanceRow struct {
	BalanceSatoshi int64 `json:"balance_satoshi"`
	UTXOCount      int64 `json:"utxo_count"`
	Tracked        bool  `json:"tracked,omitempty"`
}

type addressCheck struct {
	Address string               `json:"address"`
	Status  string               `json:"status"`
	Row     *confirmedBalanceRow `json:"row,omitempty"`
	History confirmedBalanceRow  `json:"history"`
}

type summary struct {
	AddressBalanceRowCount int64 `json:"address_balance_row_count"`
	TrackedRowCount        int64 `json:"tracked_row_count"`
	MatchCount             int   `json:"match_count"`
	MismatchCount          int   `json:"mismatch_count"`
	MissingRowCount        int   `json:"missing_row_count"`
}

type bootstrapProgress struct {
	CurrentShard        int   `json:"current_shard"`
	LastKey             string `json:"last_key,omitempty"`
	ScannedAddressCount int64 `json:"scanned_address_count"`
	IndexedAddressCount int64 `json:"indexed_address_count"`
	Done                bool  `json:"done"`
}

type report struct {
	DataDir           string             `json:"data_dir"`
	Ready             bool               `json:"ready"`
	ReadyRaw          string             `json:"ready_raw"`
	LastIndexedHeight string             `json:"last_indexed_height,omitempty"`
	BootstrapProgress *bootstrapProgress `json:"bootstrap_progress,omitempty"`
	Summary           summary            `json:"summary"`
	Checks            []addressCheck     `json:"checks"`
}

func main() {
	var (
		dataDir    = flag.String("data-dir", "data", "Pebble data directory")
		chain      = flag.String("chain", "mvc", "chain name")
		network    = flag.String("network", "mainnet", "network name")
		shardCount = flag.Int("shards", 4, "Pebble shard count")
		cpuCores   = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB   = flag.Int("memory", 2, "memory GB for storage tuning")
		addresses  = flag.String("addresses", "", "comma-separated addresses to inspect")
	)
	flag.Parse()

	list := splitAddresses(*addresses)
	if len(list) == 0 {
		log.Fatal("check balance index: -addresses is required")
	}

	result, err := inspect(*dataDir, *shardCount, *cpuCores, *memoryGB, *chain, *network, list)
	if err != nil {
		log.Fatalf("check balance index: %v", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		log.Fatalf("encode report: %v", err)
	}
}

func inspect(dataDir string, shardCount, cpuCores, memoryGB int, chain, network string, addresses []string) (report, error) {
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
		return report{}, fmt.Errorf("validate config: %w", err)
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
		return report{}, fmt.Errorf("open meta store: %w", err)
	}
	defer metaStore.Close()

	addressStore, err := storage.NewPebbleStore(indexerParams, dataDir, storage.StoreTypeIncome, shardCount)
	if err != nil {
		return report{}, fmt.Errorf("open income store: %w", err)
	}
	defer addressStore.Close()

	spendStore, err := storage.NewPebbleStore(indexerParams, dataDir, storage.StoreTypeSpend, shardCount)
	if err != nil {
		return report{}, fmt.Errorf("open spend store: %w", err)
	}
	defer spendStore.Close()

	balanceStore, err := storage.NewPebbleStore(indexerParams, dataDir, storage.StoreTypeAddressBalance, shardCount)
	if err != nil {
		return report{}, fmt.Errorf("open address_balance store: %w", err)
	}
	defer balanceStore.Close()

	readyRaw := getMetaValue(metaStore, "balance_index_ready")
	result := report{
		DataDir:           dataDir,
		Ready:             readyRaw == "1",
		ReadyRaw:          readyRaw,
		LastIndexedHeight: getMetaValue(metaStore, "last_indexed_height"),
		Checks:            make([]addressCheck, 0, len(addresses)),
	}
	progress, err := getBootstrapProgress(metaStore)
	if err != nil {
		return report{}, fmt.Errorf("load bootstrap progress: %w", err)
	}
	result.BootstrapProgress = progress

	rowCount, trackedRowCount, err := countBalanceRows(balanceStore)
	if err != nil {
		return report{}, fmt.Errorf("count balance rows: %w", err)
	}
	result.Summary.AddressBalanceRowCount = rowCount
	result.Summary.TrackedRowCount = trackedRowCount

	for _, address := range addresses {
		row, err := loadBalanceRow(balanceStore, address)
		if err != nil {
			return report{}, fmt.Errorf("load balance row for %s: %w", address, err)
		}
		history, err := loadHistoryBalance(addressStore, spendStore, address)
		if err != nil {
			return report{}, fmt.Errorf("load history balance for %s: %w", address, err)
		}
		check := addressCheck{
			Address: address,
			Row:     row,
			History: history,
			Status:  statusMissingRow,
		}
		if row != nil {
			if row.BalanceSatoshi == history.BalanceSatoshi && row.UTXOCount == history.UTXOCount {
				check.Status = statusMatch
				result.Summary.MatchCount++
			} else {
				check.Status = statusMismatch
				result.Summary.MismatchCount++
			}
		} else {
			result.Summary.MissingRowCount++
		}
		result.Checks = append(result.Checks, check)
	}

	return result, nil
}

func splitAddresses(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	addresses := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			addresses = append(addresses, part)
		}
	}
	return addresses
}

func getMetaValue(metaStore *storage.MetaStore, key string) string {
	data, err := metaStore.Get([]byte(key))
	if err != nil {
		return ""
	}
	return string(data)
}

func getBootstrapProgress(metaStore *storage.MetaStore) (*bootstrapProgress, error) {
	data, err := metaStore.Get([]byte(balanceIndexBootstrapProgressMetaKey))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var progress bootstrapProgress
	if err := json.Unmarshal(data, &progress); err != nil {
		return nil, fmt.Errorf("parse bootstrap progress: %w", err)
	}
	return &progress, nil
}

func loadBalanceRow(balanceStore *storage.PebbleStore, address string) (*confirmedBalanceRow, error) {
	data, err := balanceStore.Get([]byte(address))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var row confirmedBalanceRow
	if err := json.Unmarshal(data, &row); err != nil {
		return nil, fmt.Errorf("parse confirmed balance row: %w", err)
	}
	return &row, nil
}

func countBalanceRows(balanceStore *storage.PebbleStore) (int64, int64, error) {
	var rowCount int64
	var trackedRowCount int64

	err := balanceStore.IterateShards(func(_, value []byte) bool {
		rowCount++
		var row confirmedBalanceRow
		if err := json.Unmarshal(value, &row); err != nil {
			return false
		}
		if row.Tracked {
			trackedRowCount++
		}
		return true
	})
	if err != nil {
		return 0, 0, err
	}
	return rowCount, trackedRowCount, nil
}

func loadHistoryBalance(addressStore, spendStore *storage.PebbleStore, address string) (confirmedBalanceRow, error) {
	addrKey := []byte(address)

	spendMap := make(map[string]struct{})
	spendData, _, err := spendStore.GetWithShard(addrKey)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return confirmedBalanceRow{}, err
	}
	if err == nil {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			fields := strings.Split(spendTx, "@")
			if len(fields) > 0 && fields[0] != "" {
				spendMap[fields[0]] = struct{}{}
			}
		}
	}

	incomeData, _, err := addressStore.GetWithShard(addrKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return confirmedBalanceRow{}, nil
		}
		return confirmedBalanceRow{}, err
	}

	seen := make(map[string]struct{})
	var balance int64
	var utxoCount int64
	for _, part := range strings.Split(string(incomeData), ",") {
		fields := strings.Split(part, "@")
		if len(fields) < 3 {
			continue
		}
		outpoint := fields[0] + ":" + fields[1]
		if _, exists := seen[outpoint]; exists {
			continue
		}
		seen[outpoint] = struct{}{}

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

	return confirmedBalanceRow{
		BalanceSatoshi: balance,
		UTXOCount:      utxoCount,
	}, nil
}
