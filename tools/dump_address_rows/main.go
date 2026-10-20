package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// dump_address_rows is a read-only diagnostic that dumps the raw income and
// spend store rows of one address, plus optional utxoStore rows, so store
// corruption can be inspected without going through the HTTP API.
func main() {
	var (
		address    = flag.String("address", "", "address to dump")
		txids      = flag.String("txids", "", "comma-separated txids to dump from the utxo store")
		store      = flag.String("store", "all", "which store to read: all|income|spend")
		dataDir    = flag.String("data-dir", "data", "Pebble data directory")
		shardCount = flag.Int("shards", 4, "Pebble shard count")
		cpuCores   = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB   = flag.Int("memory", 2, "memory GB for storage tuning")
	)
	flag.Parse()

	if *address == "" && *txids == "" {
		log.Fatalf("-address or -txids is required")
	}

	cfg := &config.Config{
		Chain:      config.ChainMVC,
		Network:    "mainnet",
		DataDir:    *dataDir,
		ShardCount: *shardCount,
		CPUCores:   *cpuCores,
		MemoryGB:   *memoryGB,
	}
	config.GlobalConfig = cfg

	params := config.AutoConfigure(config.SystemResources{
		CPUCores:   *cpuCores,
		MemoryGB:   *memoryGB,
		ShardCount: *shardCount,
		HighPerf:   false,
	})

	openOpts := storage.PebbleOpenOptions{
		ReadOnly:                    true,
		CacheSizeBytes:              64 << 20,
		MemTableSizeBytes:           32 << 20,
		MemTableStopWritesThreshold: 2,
		MaxConcurrentCompactions:    1,
		MaxOpenFiles:                2048,
	}

	if *address != "" && (*store == "all" || *store == "income") {
		incomeStore, err := storage.NewPebbleStoreWithOptions(params, *dataDir, storage.StoreTypeIncome, *shardCount, openOpts)
		if err != nil {
			log.Fatalf("open income store: %v", err)
		}
		row, _, err := incomeStore.GetWithShard([]byte(*address))
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			log.Fatalf("read income row: %v", err)
		}
		incomeStore.Close()
		fmt.Printf("== income row for %s ==\n", *address)
		printRow(row)
	}

	if *address != "" && (*store == "all" || *store == "spend") {
		spendStore, err := storage.NewPebbleStoreWithOptions(params, *dataDir, storage.StoreTypeSpend, *shardCount, openOpts)
		if err != nil {
			log.Fatalf("open spend store: %v", err)
		}
		row, _, err := spendStore.GetWithShard([]byte(*address))
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			log.Fatalf("read spend row: %v", err)
		}
		spendStore.Close()
		fmt.Printf("== spend row for %s ==\n", *address)
		printRow(row)
	}

	if *txids != "" {
		utxoStore, err := storage.NewPebbleStoreWithOptions(params, *dataDir, storage.StoreTypeUTXO, *shardCount, openOpts)
		if err != nil {
			log.Fatalf("open utxo store: %v", err)
		}
		defer utxoStore.Close()
		for _, txid := range strings.Split(*txids, ",") {
			if txid == "" {
				continue
			}
			row, err := utxoStore.Get([]byte(txid))
			if err != nil && !errors.Is(err, storage.ErrNotFound) {
				log.Printf("read utxo row %s: %v", txid, err)
				continue
			}
			fmt.Printf("== utxo row for %s ==\n", txid)
			printRow(row)
		}
	}
}

func printRow(row []byte) {
	if len(row) == 0 {
		fmt.Println("<empty>")
		return
	}
	parts := strings.Split(strings.TrimPrefix(string(row), ","), ",")
	fmt.Printf("entries=%d bytes=%d\n", len(parts), len(row))
	for _, p := range parts {
		fmt.Println("  " + p)
	}
}
