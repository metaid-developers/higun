package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// move is one spend-record relocation.
type move struct {
	from, to, record string
}

// repair_shifted_spends fixes spend records that a historical bug attributed
// to the wrong address.
//
// Background: mempool-era writes merged 3-field entries (no vout field) in
// front of the indexer's 4-field entries inside utxoStore rows, and the
// positional owner lookup then resolved spends to the wrong output — e.g.
// spends of gas-pool vout 1 were recorded under a topup address, while the
// topup address's churn-change spends were recorded under the gas pool.
//
// For every -addresses spend row, each record "outpoint@blockTime@spender"
// is re-resolved against the utxoStore row with vout-aware matching. Records
// whose true owner differs are moved to the owner's row. UtxoStore rows
// touched by moved records are rewritten without their vout-less mempool-era
// prefix entries. Rows without any vout information cannot be resolved and
// are only reported (their confirmed content needs a reindex).
//
// Modes:
//   default         read-only scan, prints planned moves
//   -apply          perform the moves and row rewrites
//   -clear-balance  additionally delete tracked address_balance rows for
//                   every touched address so /balance recomputes from the
//                   repaired stores
func main() {
	var (
		addresses  = flag.String("addresses", "", "comma-separated addresses whose spend rows to repair")
		dataDir    = flag.String("data-dir", "data", "Pebble data directory")
		shardCount = flag.Int("shards", 4, "Pebble shard count")
		cpuCores   = flag.Int("cpu", 1, "CPU cores for storage tuning")
		memoryGB   = flag.Int("memory", 2, "memory GB for storage tuning")
		apply      = flag.Bool("apply", false, "write repairs (default: scan only)")
		clearBals  = flag.Bool("clear-balance", false, "also delete tracked address_balance rows for touched addresses")
		movesFile  = flag.String("moves-file", "", "CSV from,to,record with explicit relocations for records the vout-aware scan cannot resolve (e.g. records stuck on rows whose confirmed content was destroyed); verified offline against the node")
	)
	flag.Parse()

	if *addresses == "" {
		log.Fatalf("-addresses is required")
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
	utxoStore, err := storage.NewPebbleStore(params, *dataDir, storage.StoreTypeUTXO, *shardCount)
	if err != nil {
		log.Fatalf("open utxo store: %v", err)
	}
	defer utxoStore.Close()
	var balanceStore *storage.PebbleStore
	if *clearBals {
		balanceStore, err = storage.NewPebbleStore(params, *dataDir, storage.StoreTypeAddressBalance, *shardCount)
		if err != nil {
			log.Fatalf("open balance store: %v", err)
		}
		defer balanceStore.Close()
	}

	seedAddresses := map[string]bool{}
	for _, a := range strings.Split(*addresses, ",") {
		if a = strings.TrimSpace(a); a != "" {
			seedAddresses[a] = true
		}
	}

	// Row cache: churn txs have thousands of spend records each.
	txRows := map[string][]byte{}
	txRow := func(txid string) []byte {
		if row, ok := txRows[txid]; ok {
			return row
		}
		row, err := utxoStore.Get([]byte(txid))
		if err != nil {
			row = nil
		}
		txRows[txid] = row
		return row
	}

	// Phase 1: scan the seed rows and plan the moves.
	var moves []move
	unresolvable := []string{}
	taintedTxids := map[string]bool{}
	plan := map[string][]string{} // address -> records it keeps
	scanned := 0

	for _, addr := range sortedKeys(seedAddresses) {
		row, _, err := spendStore.GetWithShard([]byte(addr))
		if err != nil || len(row) == 0 {
			plan[addr] = nil
			continue
		}
		records := strings.Split(strings.TrimPrefix(string(row), ","), ",")
		keep := make([]string, 0, len(records))
		for _, rec := range records {
			if rec == "" {
				continue
			}
			scanned++
			outpoint := strings.SplitN(rec, "@", 2)[0]
			parts := strings.SplitN(outpoint, ":", 2)
			if len(parts) != 2 {
				keep = append(keep, rec)
				continue
			}
			vout, err := strconv.Atoi(parts[1])
			if err != nil {
				keep = append(keep, rec)
				continue
			}
			row := txRow(parts[0])
			if !rowHasVoutEntries(row) {
				// Pure mempool-era row (no vout fields): its confirmed
				// content is gone and positional matching is not
				// trustworthy for ownership decisions.
				unresolvable = append(unresolvable, fmt.Sprintf("%s <- %s (utxo row has no vout entries)", addr, rec))
				taintedTxids[parts[0]] = true
				keep = append(keep, rec)
				continue
			}
			owner, ok := storage.ExtractAddressFromUtxoRow(row, vout)
			if !ok {
				unresolvable = append(unresolvable, fmt.Sprintf("%s <- %s (no vout entry in utxo row)", addr, rec))
				taintedTxids[parts[0]] = true
				keep = append(keep, rec)
				continue
			}
			if owner == addr {
				keep = append(keep, rec)
				continue
			}
			taintedTxids[parts[0]] = true
			moves = append(moves, move{from: addr, to: owner, record: rec})
			seedAddresses[owner] = true // destination rows get balance-cleared too
		}
		plan[addr] = keep
	}

	// Phase 2: assemble the final content of every touched spend row.
	if *movesFile != "" {
		extra, err := readMovesFile(*movesFile)
		if err != nil {
			log.Fatalf("read moves file: %v", err)
		}
		for _, m := range extra {
			moves = append(moves, m)
			seedAddresses[m.from] = true
			seedAddresses[m.to] = true
			if outpoint := strings.SplitN(m.record, "@", 2)[0]; strings.Contains(outpoint, ":") {
				taintedTxids[strings.SplitN(outpoint, ":", 2)[0]] = true
			}
		}
		fmt.Printf("moves from file: %d\n", len(extra))
	}
	incoming := map[string][]string{}
	for _, m := range moves {
		incoming[m.to] = append(incoming[m.to], m.record)
	}
	finalRows := map[string][]byte{}
	touchedRows := map[string]bool{}
	for _, m := range moves {
		touchedRows[m.from] = true
		touchedRows[m.to] = true
	}
	for addr := range touchedRows {
		var base []string
		if keep, ok := plan[addr]; ok {
			base = keep // a scanned seed row: its plan already excludes moved-out records
		} else {
			row, _, err := spendStore.GetWithShard([]byte(addr))
			if err != nil {
				log.Fatalf("read spend row %s: %v", addr, err)
			}
			if len(row) > 0 {
				base = strings.Split(strings.TrimPrefix(string(row), ","), ",")
			}
		}
		seen := map[string]bool{}
		final := make([]string, 0, len(base)+len(incoming[addr]))
		for _, rec := range append(base, incoming[addr]...) {
			if rec == "" || seen[rec] {
				continue
			}
			seen[rec] = true
			final = append(final, rec)
		}
		sort.Strings(final)
		finalRows[addr] = []byte(strings.Join(final, ","))
	}

	// Report.
	fmt.Printf("scanned %d records across %d seed rows\n", scanned, len(seedAddresses))
	fmt.Printf("moves planned: %d\n", len(moves))
	byPair := map[string]int{}
	for _, m := range moves {
		byPair[m.from+" -> "+m.to]++
	}
	for pair, n := range byPair {
		fmt.Printf("  %s: %d records\n", pair, n)
	}
	for _, u := range unresolvable {
		fmt.Printf("unresolvable (left in place): %s\n", u)
	}
	fmt.Printf("utxo rows to rewrite (vout-less prefix dropped): %d\n", len(taintedTxids))
	fmt.Printf("spend rows to rewrite: %d\n", len(finalRows))

	if !*apply {
		fmt.Println("scan only; rerun with -apply to write")
		return
	}

	// Phase 3: write. Utxo rows first (they define ownership), then spend rows.
	rewritten := 0
	for _, txid := range sortedKeys2(taintedTxids) {
		row := txRow(txid)
		if len(row) == 0 {
			continue
		}
		cleaned := dropVoutlessEntries(row)
		if len(cleaned) == len(row) {
			continue
		}
		if err := utxoStore.Set([]byte(txid), cleaned); err != nil {
			log.Fatalf("rewrite utxo row %s: %v", txid, err)
		}
		rewritten++
	}
	fmt.Printf("utxo rows rewritten: %d\n", rewritten)

	for _, addr := range sortedKeys2(finalRows) {
		if len(finalRows[addr]) == 0 {
			continue // nothing was there and nothing moved in; don't create an empty row
		}
		if err := spendStore.Set([]byte(addr), finalRows[addr]); err != nil {
			log.Fatalf("rewrite spend row %s: %v", addr, err)
		}
	}
	fmt.Printf("spend rows rewritten: %d\n", len(finalRows))

	if *clearBals {
		cleared := 0
		for addr := range touchedRows {
			if err := balanceStore.Delete([]byte(addr)); err != nil {
				log.Fatalf("clear balance row %s: %v", addr, err)
			}
			cleared++
		}
		fmt.Printf("tracked balance rows cleared: %d\n", cleared)
	}
	fmt.Println("done")
}

// rowHasVoutEntries reports whether any entry in a utxo row carries a vout
// field ("address@amount@time@vout" has 3 '@' separators).
func rowHasVoutEntries(row []byte) bool {
	seps := 0
	for _, b := range row {
		if b == '@' {
			seps++
			if seps >= 3 {
				return true
			}
		} else if b == ',' {
			seps = 0
		}
	}
	return false
}

func dropVoutlessEntries(row []byte) []byte {
	start := 0
	if len(row) > 0 && row[0] == ',' {
		start = 1
	}
	kept := make([][]byte, 0, 64)
	changed := false
	for i := start; i < len(row); {
		end := i
		for end < len(row) && row[end] != ',' {
			end++
		}
		seg := row[i:end]
		if entryHasVout(seg) {
			kept = append(kept, seg)
		} else {
			changed = true
		}
		i = end + 1
	}
	// A cleaned row must never end up empty: entries without vouts are the
	// only content some damaged rows have left.
	if !changed || len(kept) == 0 {
		return row
	}
	parts := make([]string, len(kept))
	for i, seg := range kept {
		parts[i] = string(seg)
	}
	return []byte(strings.Join(parts, ","))
}

// entryHasVout reports whether a utxo row entry is the indexer's 4-field
// format "address@amount@blockTime@vout" (exactly 3 '@' separators).
func entryHasVout(seg []byte) bool {
	count := 0
	for _, b := range seg {
		if b == '@' {
			count++
		}
	}
	return count == 3
}

// readMovesFile parses CSV lines "from,to,record" (record may contain commas?
// no — spend records never contain commas; but trim whitespace and skip
// blanks/comments). Records are moved verbatim, so curate them carefully.
func readMovesFile(path string) ([]move, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []move
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ",", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("moves file line %d: want from,to,record, got %q", i+1, line)
		}
		out = append(out, move{from: parts[0], to: parts[1], record: parts[2]})
	}
	return out, nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys2[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// local sanity tests for the row-shape helper
// (kept beside the helper; run via `go test` in this directory)
