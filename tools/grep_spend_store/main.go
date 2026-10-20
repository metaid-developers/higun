package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// grep_spend_store scans every spend-store row and prints key/value pairs
// whose value contains any of the given outpoint substrings. Read-only.
func main() {
	var (
		needles    = flag.String("needles", "", "comma-separated outpoint substrings to search for")
		dataDir    = flag.String("data-dir", "data", "Pebble data directory")
		shardCount = flag.Int("shards", 4, "Pebble shard count")
		cpuCores   = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB   = flag.Int("memory", 2, "memory GB for storage tuning")
	)
	flag.Parse()
	if *needles == "" {
		log.Fatalf("-needles is required")
	}

	cfg := &config.Config{Chain: config.ChainMVC, Network: "mainnet", DataDir: *dataDir, ShardCount: *shardCount, CPUCores: *cpuCores, MemoryGB: *memoryGB}
	config.GlobalConfig = cfg
	params := config.AutoConfigure(config.SystemResources{CPUCores: *cpuCores, MemoryGB: *memoryGB, ShardCount: *shardCount, HighPerf: false})

	store, err := storage.NewPebbleStore(params, *dataDir, storage.StoreTypeSpend, *shardCount)
	if err != nil {
		log.Fatalf("open spend store: %v", err)
	}
	defer store.Close()

	hits := 0
	for shardIdx, db := range store.GetShards() {
		iter, err := db.NewIter(nil)
		if err != nil {
			log.Fatalf("shard %d iter: %v", shardIdx, err)
		}
		for iter.First(); iter.Valid(); iter.Next() {
			v := string(iter.Value())
			for _, n := range strings.Split(*needles, ",") {
				if n != "" && strings.Contains(v, n) {
					fmt.Printf("shard=%d address=%s value=%s\n", shardIdx, string(iter.Key()), v)
					hits++
				}
			}
		}
		iter.Close()
	}
	fmt.Printf("hits: %d\n", hits)
}
