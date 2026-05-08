package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

type readProbeResult struct {
	Address    string `json:"address"`
	IncomeRead string `json:"income_read"`
	SpendRead  string `json:"spend_read"`
	Panic      string `json:"panic,omitempty"`
}

var diagnoseOpenOptions = storage.PebbleOpenOptions{
	ReadOnly:                    true,
	CacheSizeBytes:              4 << 20,
	MemTableSizeBytes:           4 << 20,
	MemTableStopWritesThreshold: 2,
	MaxConcurrentCompactions:    1,
	MaxOpenFiles:                2048,
}

func main() {
	var (
		dataDir      = flag.String("data-dir", "data", "Pebble data directory")
		chain        = flag.String("chain", "mvc", "chain name")
		network      = flag.String("network", "mainnet", "network name")
		shardCount   = flag.Int("shards", 4, "Pebble shard count")
		cpuCores     = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB     = flag.Int("memory", 2, "memory GB for storage tuning")
		currentShard = flag.Int("current-shard", 0, "address shard to inspect")
		lastKey      = flag.String("last-key", "", "resume after this key")
		limit        = flag.Int("limit", 20, "maximum number of keys to inspect")
	)
	flag.Parse()

	if *limit <= 0 {
		log.Fatal("limit must be greater than 0")
	}

	results, err := diagnose(*dataDir, *shardCount, *cpuCores, *memoryGB, *chain, *network, *currentShard, *lastKey, *limit)
	if err != nil {
		log.Fatalf("diagnose pebble read: %v", err)
	}

	enc := json.NewEncoder(log.Writer())
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		log.Fatalf("encode diagnose results: %v", err)
	}
}

func diagnose(dataDir string, shardCount, cpuCores, memoryGB int, chain, network string, currentShard int, lastKey string, limit int) ([]readProbeResult, error) {
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
		return nil, fmt.Errorf("validate config: %w", err)
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

	addressStore, err := storage.NewPebbleStoreWithOptions(indexerParams, dataDir, storage.StoreTypeIncome, shardCount, diagnoseOpenOptions)
	if err != nil {
		return nil, fmt.Errorf("open income store: %w", err)
	}
	defer addressStore.Close()

	spendStore, err := storage.NewPebbleStoreWithOptions(indexerParams, dataDir, storage.StoreTypeSpend, shardCount, diagnoseOpenOptions)
	if err != nil {
		return nil, fmt.Errorf("open spend store: %w", err)
	}
	defer spendStore.Close()

	shards := addressStore.GetShards()
	if currentShard < 0 || currentShard >= len(shards) {
		return nil, fmt.Errorf("current shard %d out of range [0,%d)", currentShard, len(shards))
	}

	iter, err := shards[currentShard].NewIter(nil)
	if err != nil {
		return nil, fmt.Errorf("open iterator for shard %d: %w", currentShard, err)
	}
	defer iter.Close()

	if lastKey != "" {
		iter.SeekGE([]byte(lastKey))
		if iter.Valid() && string(iter.Key()) == lastKey {
			iter.Next()
		}
	} else {
		iter.First()
	}

	results := make([]readProbeResult, 0, limit)
	for ; iter.Valid() && len(results) < limit; iter.Next() {
		address := string(append([]byte(nil), iter.Key()...))
		incomeRead := "ok"
		if _, _, err := addressStore.GetWithShard([]byte(address)); err != nil {
			incomeRead = err.Error()
		}

		result := readProbeResult{
			Address:    address,
			IncomeRead: incomeRead,
			SpendRead:  "ok",
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					result.Panic = fmt.Sprintf("%v", r)
				}
			}()

			if _, _, err := spendStore.GetWithShard([]byte(address)); err != nil {
				result.SpendRead = err.Error()
			}
		}()

		results = append(results, result)
		if result.Panic != "" {
			break
		}
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterate shard %d: %w", currentShard, err)
	}
	return results, nil
}
