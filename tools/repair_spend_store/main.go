package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"

	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

type parsedSpendRecord struct {
	raw       string
	outpoint  string
	timestamp int64
	spender   string
}

func main() {
	var (
		startHeight = flag.Int("start", 0, "start block height, inclusive")
		endHeight   = flag.Int("end", 0, "end block height, inclusive")
		apply       = flag.Bool("apply", false, "write repaired spend records into Pebble when true")
		dataDir     = flag.String("data-dir", "data", "Pebble data directory")
		chain       = flag.String("chain", "mvc", "chain name")
		network     = flag.String("network", "mainnet", "network name")
		rpcHost     = flag.String("rpc-host", "127.0.0.1", "RPC host")
		rpcPort     = flag.String("rpc-port", "9882", "RPC port")
		rpcUser     = flag.String("rpc-user", "", "RPC username")
		rpcPass     = flag.String("rpc-pass", "", "RPC password")
		shardCount  = flag.Int("shards", 4, "Pebble shard count")
		cpuCores    = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB    = flag.Int("memory", 2, "memory GB for storage tuning")
	)
	flag.Parse()

	if *startHeight <= 0 || *endHeight < *startHeight {
		log.Fatalf("invalid height range: start=%d end=%d", *startHeight, *endHeight)
	}

	cfg := &config.Config{
		Chain:      *chain,
		Network:    *network,
		DataDir:    *dataDir,
		ShardCount: *shardCount,
		CPUCores:   *cpuCores,
		MemoryGB:   *memoryGB,
		RPC: config.RPCConfig{
			Chain:    *chain,
			Host:     *rpcHost,
			Port:     *rpcPort,
			User:     *rpcUser,
			Password: *rpcPass,
		},
	}
	config.GlobalConfig = cfg

	params := config.AutoConfigure(config.SystemResources{
		CPUCores:   *cpuCores,
		MemoryGB:   *memoryGB,
		ShardCount: *shardCount,
		HighPerf:   false,
	})

	utxoStore, err := storage.NewPebbleStore(params, *dataDir, storage.StoreTypeUTXO, *shardCount)
	if err != nil {
		log.Fatalf("open utxo store: %v", err)
	}
	defer utxoStore.Close()

	spendStore, err := storage.NewPebbleStore(params, *dataDir, storage.StoreTypeSpend, *shardCount)
	if err != nil {
		log.Fatalf("open spend store: %v", err)
	}
	defer spendStore.Close()

	client, err := blockchain.NewClient(cfg)
	if err != nil {
		log.Fatalf("new rpc client: %v", err)
	}
	defer client.Shutdown()

	repairs := make(map[string][]string)
	totalInputs := 0
	totalMapped := 0
	totalMissing := 0

	for height := *startHeight; height <= *endHeight; height++ {
		hash, err := client.GetBlockHash(int64(height))
		if err != nil {
			log.Fatalf("get block hash %d: %v", height, err)
		}
		block, err := client.GetBlock(hash)
		if err != nil {
			log.Fatalf("get block %d: %v", height, err)
		}

		blockTime := fmt.Sprintf("%d", block.Time)
		outpoints := make([]string, 0, 1024)
		spendingTxByOutpoint := make(map[string]string)

		for _, tx := range block.Tx {
			for _, in := range tx.Vin {
				if in.Txid == "" {
					continue
				}
				outpoint := fmt.Sprintf("%s:%d", in.Txid, in.Vout)
				outpoints = append(outpoints, outpoint)
				spendingTxByOutpoint[outpoint] = tx.Txid
			}
		}

		details, err := utxoStore.QueryUTXODetails(&outpoints)
		if err != nil {
			log.Fatalf("query utxo details at height %d: %v", height, err)
		}

		mapped := 0
		for outpoint, detail := range details {
			if detail.Address == "" {
				continue
			}
			record := fmt.Sprintf("%s@%s@%s", outpoint, blockTime, spendingTxByOutpoint[outpoint])
			repairs[detail.Address] = append(repairs[detail.Address], record)
			mapped++
		}

		totalInputs += len(outpoints)
		totalMapped += mapped
		totalMissing += len(outpoints) - mapped
		log.Printf("height=%d txs=%d inputs=%d mapped=%d missing=%d", height, len(block.Tx), len(outpoints), mapped, len(outpoints)-mapped)
	}

	addressesUpdated := 0
	recordsInserted := 0
	recordsAlreadyPresent := 0
	addressesNormalized := 0

	for address, records := range repairs {
		existing, err := spendStore.Get([]byte(address))
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			log.Fatalf("read spend store for %s: %v", address, err)
		}

		existingRecords := splitRecords(string(existing))
		existingSet := make(map[string]struct{}, len(existingRecords))
		for _, record := range existingRecords {
			existingSet[record] = struct{}{}
		}

		normalized := normalizeSpendRecords(existingRecords)
		normalizedLenBefore := len(normalized)

		uniqRecords := splitRecords(strings.Join(records, ","))
		toAdd := make([]string, 0, len(uniqRecords))
		for _, record := range uniqRecords {
			if _, ok := existingSet[record]; ok {
				recordsAlreadyPresent++
				continue
			}
			existingSet[record] = struct{}{}
			toAdd = append(toAdd, record)
		}

		if len(toAdd) > 0 {
			recordsInserted += len(toAdd)
			normalized = normalizeSpendRecords(append(normalized, toAdd...))
		}

		if len(existingRecords) != normalizedLenBefore || len(toAdd) > 0 {
			addressesNormalized++
		}

		if len(toAdd) == 0 && len(existingRecords) == len(normalized) {
			continue
		}

		addressesUpdated++

		if !*apply {
			continue
		}

		slices.Sort(normalized)
		if err := spendStore.Set([]byte(address), []byte(strings.Join(normalized, ","))); err != nil {
			log.Fatalf("write spend store for %s: %v", address, err)
		}
	}

	log.Printf(
		"done apply=%v heights=%d-%d addresses_updated=%d addresses_normalized=%d records_inserted=%d records_already_present=%d total_inputs=%d mapped=%d missing=%d",
		*apply,
		*startHeight,
		*endHeight,
		addressesUpdated,
		addressesNormalized,
		recordsInserted,
		recordsAlreadyPresent,
		totalInputs,
		totalMapped,
		totalMissing,
	)
}

func splitRecords(raw string) []string {
	raw = strings.TrimPrefix(raw, ",")
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func normalizeSpendRecords(records []string) []string {
	byLogicalKey := make(map[string]parsedSpendRecord, len(records))
	for _, record := range records {
		parts := strings.Split(record, "@")
		if len(parts) < 3 {
			byLogicalKey[record] = parsedSpendRecord{raw: record}
			continue
		}

		timestamp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			timestamp = 0
		}
		logicalKey := parts[0] + "|" + parts[2]
		current := parsedSpendRecord{
			raw:       record,
			outpoint:  parts[0],
			timestamp: timestamp,
			spender:   parts[2],
		}

		existing, ok := byLogicalKey[logicalKey]
		if !ok || shouldReplaceSpendRecord(existing, current) {
			byLogicalKey[logicalKey] = current
		}
	}

	out := make([]string, 0, len(byLogicalKey))
	for _, record := range byLogicalKey {
		out = append(out, record.raw)
	}
	return out
}

func shouldReplaceSpendRecord(existing, current parsedSpendRecord) bool {
	if existing.raw == "" {
		return true
	}
	if current.timestamp == 0 {
		return false
	}
	if existing.timestamp == 0 {
		return true
	}
	return current.timestamp < existing.timestamp
}
