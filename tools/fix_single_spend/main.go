package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// fix_single_spend appends a single spend record to an address's spend store
// entry. It is a surgical repair for a spend mark that the indexer silently
// dropped (e.g. the duplicate-UTXO-output / positional-vout bug).
func main() {
	var (
		address    = flag.String("address", "", "owner address (spend store key)")
		record     = flag.String("record", "", "spend record in the form outpoint@blockTime@spenderTxid")
		dataDir    = flag.String("data-dir", "data", "Pebble data directory")
		shardCount = flag.Int("shards", 4, "Pebble shard count")
		cpuCores   = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB   = flag.Int("memory", 2, "memory GB for storage tuning")
		dump       = flag.Bool("dump", false, "read-only dump of the address entry")
	)
	flag.Parse()

	if *address == "" {
		log.Fatalf("-address is required")
	}
	if !*dump && *record == "" {
		log.Fatalf("-record is required unless -dump is set")
	}
	if !*dump && !strings.Contains(*record, "@") {
		log.Fatalf("invalid record (want outpoint@blockTime@spenderTxid): %q", *record)
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

	spendStore, err := storage.NewPebbleStore(params, *dataDir, storage.StoreTypeSpend, *shardCount)
	if err != nil {
		log.Fatalf("open spend store: %v", err)
	}
	defer spendStore.Close()

	if *dump {
		got, err := spendStore.Get([]byte(*address))
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			log.Fatalf("dump Get: %v", err)
		}
		withShard, _, err := spendStore.GetWithShard([]byte(*address))
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			log.Fatalf("dump GetWithShard: %v", err)
		}
		shardIdx := xxhash.Sum64String(*address) % uint64(*shardCount)
		fmt.Printf("dump address=%s shard=%d/%d get_len=%d getwithshard_len=%d\n", *address, shardIdx, *shardCount, len(got), len(withShard))
		fmt.Printf("get_prefix=%q\n", string(got[:min(len(got), 200)]))
		fmt.Printf("withshard_prefix=%q\n", string(withShard[:min(len(withShard), 200)]))
		return
	}

	existing, err := spendStore.Get([]byte(*address))
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		log.Fatalf("read spend store for %s: %v", *address, err)
	}

	records := make([]string, 0, 128)
	seen := make(map[string]struct{}, 128)
	for _, r := range strings.Split(string(existing), ",") {
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		records = append(records, r)
	}

	added := false
	if _, ok := seen[*record]; !ok {
		seen[*record] = struct{}{}
		records = append(records, *record)
		added = true
	}

	slices.Sort(records)
	if err := spendStore.Set([]byte(*address), []byte(strings.Join(records, ","))); err != nil {
		log.Fatalf("write spend store for %s: %v", *address, err)
	}

	fmt.Printf("done address=%s records=%d added=%v record=%s\n", *address, len(records), added, *record)
}
