package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

type AddressGap struct {
	Address      string   `json:"address"`
	IncomeCount  int      `json:"income_count"`
	SpendCount   int      `json:"spend_count"`
	MissingCount int      `json:"missing_count"`
	TotalValue   int64    `json:"total_value_sat"`
	SampleMissing []string `json:"sample_missing,omitempty"`
}

var scanOpenOptions = storage.PebbleOpenOptions{
	ReadOnly:                    true,
	CacheSizeBytes:              128 << 20,
	MemTableSizeBytes:           64 << 20,
	MemTableStopWritesThreshold: 2,
	MaxConcurrentCompactions:    2,
	MaxOpenFiles:                4096,
}

func main() {
	var (
		dataDir    = flag.String("data-dir", "data", "Pebble data directory")
		shardCount = flag.Int("shards", 4, "Pebble shard count")
		cpuCores   = flag.Int("cpu", 1, "CPU cores")
		memoryGB   = flag.Int("memory", 2, "memory GB")
		chain      = flag.String("chain", "mvc", "chain name")
		network    = flag.String("network", "mainnet", "network name")
		topN       = flag.Int("top", 100, "show top N results")
		minValue   = flag.Int64("min-value", 0, "minimum total missing value (sat) to report")
		verbose    = flag.Bool("v", false, "verbose output")
	)
	flag.Parse()

	cfg := &config.Config{
		Chain:      *chain,
		Network:    *network,
		DataDir:    *dataDir,
		ShardCount: *shardCount,
		CPUCores:   *cpuCores,
		MemoryGB:   *memoryGB,
		RPC:        config.RPCConfig{Chain: *chain},
	}
	if err := cfg.ValidateChain(); err != nil {
		log.Fatalf("validate config: %v", err)
	}
	config.GlobalConfig = cfg
	if params, err := cfg.GetChainParams(); err == nil {
		config.GlobalNetwork = params
	}

	indexerParams := config.AutoConfigure(config.SystemResources{
		CPUCores:   *cpuCores,
		MemoryGB:   *memoryGB,
		HighPerf:   false,
		ShardCount: *shardCount,
	})
	common.InitBytePool(indexerParams.BytePoolSizeKB)
	storage.DbInit(indexerParams)

	log.Printf("Opening addressStore (income) read-only...")
	addressStore, err := storage.NewPebbleStoreWithOptions(indexerParams, *dataDir, storage.StoreTypeIncome, *shardCount, scanOpenOptions)
	if err != nil {
		log.Fatalf("open income store: %v", err)
	}
	defer addressStore.Close()

	log.Printf("Opening spendStore read-only...")
	spendStore, err := storage.NewPebbleStoreWithOptions(indexerParams, *dataDir, storage.StoreTypeSpend, *shardCount, scanOpenOptions)
	if err != nil {
		log.Fatalf("open spend store: %v", err)
	}
	defer spendStore.Close()

	log.Printf("Starting scan across %d shards...", *shardCount)
	startTime := time.Now()

	gaps := scanAll(addressStore, spendStore, *shardCount, *verbose)

	elapsed := time.Since(startTime)

	// Sort by total value descending
	sort.Slice(gaps, func(i, j int) bool {
		return gaps[i].TotalValue > gaps[j].TotalValue
	})

	// Filter by min value
	filtered := make([]AddressGap, 0, len(gaps))
	for _, g := range gaps {
		if g.TotalValue >= *minValue {
			filtered = append(filtered, g)
		}
	}
	if len(filtered) == 0 {
		filtered = gaps
	}

	// Output
	displayCount := *topN
	if displayCount > len(filtered) {
		displayCount = len(filtered)
	}

	log.Printf("=== RESULTS ===")
	log.Printf("Total addresses scanned: %d", len(gaps))
	log.Printf("Addresses with missing spends: %d", countWithGaps(gaps))
	log.Printf("Total value at risk (sat): %d", totalValueAtRisk(gaps))
	log.Printf("Scan time: %v", elapsed)
	log.Printf("")

	if displayCount > 0 {
		log.Printf("Top %d addresses by value at risk:", displayCount)
		log.Printf("%-42s %8s %8s %8s %14s", "Address", "Income", "Spend", "Missing", "ValueAtRisk")
		log.Printf("%s", strings.Repeat("-", 90))
		for i := 0; i < displayCount; i++ {
			g := filtered[i]
			samples := ""
			if len(g.SampleMissing) > 0 {
				samples = "  samples: " + strings.Join(g.SampleMissing, ", ")
			}
			log.Printf("%-42s %8d %8d %8d %14d%s",
				g.Address, g.IncomeCount, g.SpendCount, g.MissingCount, g.TotalValue, samples)
		}
	}

	// Also output JSON to stdout
	output := map[string]interface{}{
		"total_scanned":       len(gaps),
		"addresses_with_gaps": countWithGaps(gaps),
		"total_value_at_risk": totalValueAtRisk(gaps),
		"scan_time_seconds":   elapsed.Seconds(),
		"top_results":         filtered[:displayCount],
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(output)
}

func scanAll(addressStore, spendStore *storage.PebbleStore, shardCount int, verbose bool) []AddressGap {
	shards := addressStore.GetShards()
	var (
		mu     sync.Mutex
		gaps   []AddressGap
		wg     sync.WaitGroup
		total  int64
	)

	for shardIdx, db := range shards {
		wg.Add(1)
		go func(idx int, shard *pebble.DB) {
			defer wg.Done()

			iter, err := shard.NewIter(nil)
			if err != nil {
				log.Printf("ERROR: shard %d: create iterator: %v", idx, err)
				return
			}
			defer iter.Close()

			count := 0
			for iter.First(); iter.Valid(); iter.Next() {
				address := string(iter.Key())
				incomeRaw := iter.Value()

				gap := analyzeAddress(address, incomeRaw, spendStore)
				if gap != nil {
					mu.Lock()
					gaps = append(gaps, *gap)
					mu.Unlock()
				}

				count++
				if verbose && count%100000 == 0 {
					mu.Lock()
					total += 100000
					log.Printf("Shard %d: scanned %d addresses (total ~%d)...", idx, count, total)
					mu.Unlock()
				}
			}
		}(shardIdx, db)
	}

	wg.Wait()
	return gaps
}

func analyzeAddress(address string, incomeRaw []byte, spendStore *storage.PebbleStore) *AddressGap {
	// Parse income: ,txid@index@amount@blockTime,...
	incomeOutpoints := parseIncomeValue(incomeRaw)
	if len(incomeOutpoints) == 0 {
		return nil
	}

	// Get spend data
	spendRaw, _, err := spendStore.GetWithShard([]byte(address))
	spendOutpoints := make(map[string]bool)
	if err == nil && spendRaw != nil {
		spendOutpoints = parseSpendValue(spendRaw)
	}

	// Find missing
	var missing []string
	var totalMissingValue int64
	for point, amount := range incomeOutpoints {
		if !spendOutpoints[point] {
			missing = append(missing, point)
			totalMissingValue += amount
		}
	}

	if len(missing) == 0 {
		return nil
	}

	// Limit sample size
	sample := missing
	if len(sample) > 5 {
		sample = sample[:5]
	}

	return &AddressGap{
		Address:       address,
		IncomeCount:   len(incomeOutpoints),
		SpendCount:    len(spendOutpoints),
		MissingCount:  len(missing),
		TotalValue:    totalMissingValue,
		SampleMissing: sample,
	}
}

// parseIncomeValue parses value like: ,txid@index@amount@time,txid@index@amount@time,...
// Returns map[txid:index]amount
func parseIncomeValue(raw []byte) map[string]int64 {
	result := make(map[string]int64)
	s := string(raw)
	if s == "" {
		return result
	}

	// Handle optional leading comma
	start := 0
	if s[0] == ',' {
		start = 1
	}

	parts := strings.Split(s[start:], ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		fields := strings.Split(part, "@")
		if len(fields) < 3 {
			continue
		}
		// fields: txid, index, amount, [blockTime]
		point := fields[0] + ":" + fields[1]
		amount, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			continue
		}
		// Keep the largest amount if duplicate (due to re-indexing)
		if existing, ok := result[point]; !ok || amount > existing {
			result[point] = amount
		}
	}
	return result
}

// parseSpendValue parses value like: ,txid:index@blockTime@spendingTxId,...
// Returns set of txid:index
func parseSpendValue(raw []byte) map[string]bool {
	result := make(map[string]bool)
	s := string(raw)
	if s == "" {
		return result
	}

	start := 0
	if s[0] == ',' {
		start = 1
	}

	parts := strings.Split(s[start:], ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		fields := strings.Split(part, "@")
		if len(fields) < 1 {
			continue
		}
		// fields[0] = txid:index
		result[fields[0]] = true
	}
	return result
}

func countWithGaps(gaps []AddressGap) int {
	count := 0
	for _, g := range gaps {
		if g.MissingCount > 0 {
			count++
		}
	}
	return count
}

func totalValueAtRisk(gaps []AddressGap) int64 {
	var total int64
	for _, g := range gaps {
		total += g.TotalValue
	}
	return total
}
