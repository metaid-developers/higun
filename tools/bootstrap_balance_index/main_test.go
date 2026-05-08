package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/storage"
)

func createEmptyUTXOStore(t *testing.T, params config.IndexerParams, dataDir string, shardCount int) {
	t.Helper()

	utxoStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeUTXO, shardCount)
	if err != nil {
		t.Fatalf("NewPebbleStore utxo: %v", err)
	}
	if err := utxoStore.Close(); err != nil {
		t.Fatalf("close utxo store: %v", err)
	}
}

func seedIncomeStore(t *testing.T, params config.IndexerParams, dataDir string, shardCount int, rows map[string]string) {
	t.Helper()

	incomeStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, shardCount)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	for address, value := range rows {
		if err := incomeStore.Set([]byte(address), []byte(value)); err != nil {
			t.Fatalf("seed income %s: %v", address, err)
		}
	}
	if err := incomeStore.Close(); err != nil {
		t.Fatalf("close seeded income store: %v", err)
	}
}

func seedSpendStore(t *testing.T, params config.IndexerParams, dataDir string, shardCount int, rows map[string]string) {
	t.Helper()

	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, shardCount)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	for address, value := range rows {
		if err := spendStore.Set([]byte(address), []byte(value)); err != nil {
			t.Fatalf("seed spend %s: %v", address, err)
		}
	}
	if err := spendStore.Close(); err != nil {
		t.Fatalf("close seeded spend store: %v", err)
	}
}

func seedUTXOStore(t *testing.T, params config.IndexerParams, dataDir string, shardCount int, rows map[string]string) {
	t.Helper()

	utxoStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeUTXO, shardCount)
	if err != nil {
		t.Fatalf("NewPebbleStore utxo: %v", err)
	}
	for txid, value := range rows {
		if err := utxoStore.Set([]byte(txid), []byte(value)); err != nil {
			t.Fatalf("seed utxo %s: %v", txid, err)
		}
	}
	if err := utxoStore.Close(); err != nil {
		t.Fatalf("close seeded utxo store: %v", err)
	}
}

func TestRunBuildsConfirmedBalanceIndexesFromIncomeAndSpendHistory(t *testing.T) {
	dataDir := t.TempDir()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MemUTXOMaxCount: 1,
		BatchSize:       100,
		Workers:         1,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	params := config.IndexerParams{}
	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	addressStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	balanceStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore address_balance: %v", err)
	}
	rankStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore balance_rank: %v", err)
	}
	if err := metaStore.Close(); err != nil {
		t.Fatalf("close meta store before seed: %v", err)
	}
	if err := addressStore.Close(); err != nil {
		t.Fatalf("close income store before seed: %v", err)
	}
	if err := spendStore.Close(); err != nil {
		t.Fatalf("close spend store before seed: %v", err)
	}
	if err := balanceStore.Close(); err != nil {
		t.Fatalf("close balance store before seed: %v", err)
	}
	if err := rankStore.Close(); err != nil {
		t.Fatalf("close rank store before seed: %v", err)
	}

	seedIncomeStore(t, params, dataDir, 1, map[string]string{
		"addr-a": "tx-1@0@50@1700000000,tx-2@0@20@1700000001",
		"addr-b": "tx-3@0@30@1700000002",
	})
	seedSpendStore(t, params, dataDir, 1, map[string]string{
		"addr-a": "tx-1:0@1700000003@spend-a",
	})

	if err := run(dataDir, 1, 1, 1, "mvc", "mainnet"); err != nil {
		t.Fatalf("run: %v", err)
	}

	metaStore, err = storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("reopen metaStore: %v", err)
	}
	defer metaStore.Close()
	addressStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("reopen addressStore: %v", err)
	}
	defer addressStore.Close()
	spendStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("reopen spendStore: %v", err)
	}
	defer spendStore.Close()
	balanceStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("reopen balanceStore: %v", err)
	}
	defer balanceStore.Close()
	rankStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("reopen rankStore: %v", err)
	}
	defer rankStore.Close()

	idx := indexer.NewUTXOIndexer(params, nil, addressStore, metaStore, spendStore)
	idx.SetBalanceStores(balanceStore, rankStore)

	balanceA, err := idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 20 || balanceA.UTXOCount != 1 {
		t.Fatalf("unexpected addr-a balance: %+v", balanceA)
	}

	balanceB, err := idx.GetBalance("addr-b", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-b: %v", err)
	}
	if balanceB.ConfirmedBalanceSatoshi != 30 || balanceB.UTXOCount != 1 {
		t.Fatalf("unexpected addr-b balance: %+v", balanceB)
	}
}

func TestRunBuildsConfirmedBalanceIndexes(t *testing.T) {
	dataDir := t.TempDir()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MemUTXOMaxCount: 1,
		BatchSize:       100,
		Workers:         1,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	params := config.IndexerParams{}
	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	addressStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	balanceStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore address_balance: %v", err)
	}
	rankStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore balance_rank: %v", err)
	}
	if err := metaStore.Set([]byte("total_address_count"), []byte("2")); err != nil {
		t.Fatalf("seed total_address_count: %v", err)
	}
	for _, closer := range []interface{ Close() error }{rankStore, balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close seeded store before income/spend seed: %v", err)
		}
	}
	seedIncomeStore(t, params, dataDir, 1, map[string]string{
		"addr-a": "tx-2@0@20@1700000001",
		"addr-b": "tx-3@0@30@1700000002",
	})

	if err := run(dataDir, 1, 1, 1, "mvc", "mainnet"); err != nil {
		t.Fatalf("run: %v", err)
	}

	metaStore, err = storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("reopen metaStore: %v", err)
	}
	defer metaStore.Close()

	addressStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("reopen addressStore: %v", err)
	}
	defer addressStore.Close()

	spendStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("reopen spendStore: %v", err)
	}
	defer spendStore.Close()

	balanceStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("reopen balanceStore: %v", err)
	}
	defer balanceStore.Close()

	rankStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("reopen rankStore: %v", err)
	}
	defer rankStore.Close()

	idx := indexer.NewUTXOIndexer(params, nil, addressStore, metaStore, spendStore)
	idx.SetBalanceStores(balanceStore, rankStore)

	ready, err := metaStore.Get([]byte("balance_index_ready"))
	if err != nil {
		t.Fatalf("get balance_index_ready: %v", err)
	}
	if string(ready) != "1" {
		t.Fatalf("expected balance_index_ready=1, got %q", string(ready))
	}

	balanceA, err := idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 20 {
		t.Fatalf("expected addr-a confirmed balance 20, got %d", balanceA.ConfirmedBalanceSatoshi)
	}
	if balanceA.UTXOCount != 1 {
		t.Fatalf("expected addr-a utxo count 1, got %d", balanceA.UTXOCount)
	}

	balanceB, err := idx.GetBalance("addr-b", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-b: %v", err)
	}
	if balanceB.ConfirmedBalanceSatoshi != 30 {
		t.Fatalf("expected addr-b confirmed balance 30, got %d", balanceB.ConfirmedBalanceSatoshi)
	}
	if balanceB.UTXOCount != 1 {
		t.Fatalf("expected addr-b utxo count 1, got %d", balanceB.UTXOCount)
	}

	list, total, err := idx.GetRichList(1, 2, 0)
	if err != nil {
		t.Fatalf("GetRichList: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected rich-list total 2, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 rich-list entries, got %d", len(list))
	}
	if list[0].Address != "addr-b" {
		t.Fatalf("expected first rich-list address addr-b, got %s", list[0].Address)
	}
	if list[1].Address != "addr-a" {
		t.Fatalf("expected second rich-list address addr-a, got %s", list[1].Address)
	}
}

func TestOpenBootstrapSourceStoreIsReadOnly(t *testing.T) {
	dataDir := t.TempDir()
	params := config.IndexerParams{}

	seedStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("open seed income store: %v", err)
	}
	if err := seedStore.Set([]byte("addr-a"), []byte("tx-1@0@10@1700000000")); err != nil {
		t.Fatalf("seed income store: %v", err)
	}
	if err := seedStore.Close(); err != nil {
		t.Fatalf("close seed income store: %v", err)
	}

	sourceStore, err := openBootstrapSourceStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("open bootstrap source store: %v", err)
	}
	defer sourceStore.Close()

	value, _, err := sourceStore.GetWithShard([]byte("addr-a"))
	if err != nil {
		t.Fatalf("read existing row from bootstrap source store: %v", err)
	}
	if string(value) != "tx-1@0@10@1700000000" {
		t.Fatalf("unexpected bootstrap source value: %q", string(value))
	}

	if err := sourceStore.Set([]byte("addr-b"), []byte("tx-2@0@20@1700000001")); err == nil {
		t.Fatal("expected bootstrap source store to reject writes")
	}
}

func TestBuildConfirmedBalanceRowSkipsSpentAndDuplicateOutputs(t *testing.T) {
	t.Parallel()

	row, ok := buildConfirmedBalanceRow(
		[]byte(",tx-1@0@50@1700000000,broken,tx-1@0@50@1700000000,tx-2@0@20@1700000001,tx-3@0@30@1700000002"),
		[]byte("tx-2:0@1700000003@spend-a"),
	)

	if !ok {
		t.Fatal("expected row to be built")
	}
	if row.BalanceSatoshi != 80 || row.UTXOCount != 2 {
		t.Fatalf("unexpected confirmed balance row: %+v", row)
	}
}

func TestBuildConfirmedBalanceRowReturnsFalseForZeroBalance(t *testing.T) {
	t.Parallel()

	_, ok := buildConfirmedBalanceRow(
		[]byte("tx-1@0@50@1700000000"),
		[]byte("tx-1:0@1700000001@spend-a"),
	)
	if ok {
		t.Fatal("expected zero-balance row to be skipped")
	}
}

func TestRunBuildsConfirmedBalanceIndexesAcrossMultipleShards(t *testing.T) {
	const shardCount = 4

	dataDir := t.TempDir()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MemUTXOMaxCount: 1,
		BatchSize:       100,
		Workers:         1,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	params := config.IndexerParams{}
	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	addressStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, shardCount)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, shardCount)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	balanceStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, shardCount)
	if err != nil {
		t.Fatalf("NewPebbleStore address_balance: %v", err)
	}
	rankStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore balance_rank: %v", err)
	}
	addresses := make([]string, shardCount)
	for shardIdx := 0; shardIdx < shardCount; shardIdx++ {
		addresses[shardIdx] = addressForShard(shardIdx, shardCount)
	}
	if err := metaStore.Set([]byte("total_address_count"), []byte("4")); err != nil {
		t.Fatalf("seed total_address_count: %v", err)
	}
	for _, closer := range []interface{ Close() error }{rankStore, balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close seeded store before income/spend seed: %v", err)
		}
	}

	seedIncomeStore(t, params, dataDir, shardCount, map[string]string{
		addresses[0]: "tx-0@0@30@1700000001",
		addresses[1]: "tx-1@0@80@1700000002",
		addresses[2]: "tx-2@0@60@1700000003",
		addresses[3]: "tx-3@0@40@1700000004",
	})

	if err := run(dataDir, shardCount, 1, 1, "mvc", "mainnet"); err != nil {
		t.Fatalf("run: %v", err)
	}

	metaStore, err = storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("reopen metaStore: %v", err)
	}
	defer metaStore.Close()

	addressStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, shardCount)
	if err != nil {
		t.Fatalf("reopen addressStore: %v", err)
	}
	defer addressStore.Close()

	spendStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, shardCount)
	if err != nil {
		t.Fatalf("reopen spendStore: %v", err)
	}
	defer spendStore.Close()

	balanceStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, shardCount)
	if err != nil {
		t.Fatalf("reopen balanceStore: %v", err)
	}
	defer balanceStore.Close()

	rankStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("reopen rankStore: %v", err)
	}
	defer rankStore.Close()

	idx := indexer.NewUTXOIndexer(params, nil, addressStore, metaStore, spendStore)
	idx.SetBalanceStores(balanceStore, rankStore)

	expectedBalances := map[string]uint64{
		addresses[0]: 30,
		addresses[1]: 80,
		addresses[2]: 60,
		addresses[3]: 40,
	}
	for address, expected := range expectedBalances {
		balance, err := idx.GetBalance(address, 0)
		if err != nil {
			t.Fatalf("GetBalance %s: %v", address, err)
		}
		if balance.ConfirmedBalanceSatoshi != expected {
			t.Fatalf("expected %s confirmed balance %d, got %d", address, expected, balance.ConfirmedBalanceSatoshi)
		}
		if balance.UTXOCount != 1 {
			t.Fatalf("expected %s utxo count 1, got %d", address, balance.UTXOCount)
		}
	}

	list, total, err := idx.GetRichList(1, 4, 0)
	if err != nil {
		t.Fatalf("GetRichList: %v", err)
	}
	if total != 4 {
		t.Fatalf("expected rich-list total 4, got %d", total)
	}
	if len(list) != 4 {
		t.Fatalf("expected 4 rich-list entries, got %d", len(list))
	}
	if list[0].Address != addresses[1] {
		t.Fatalf("expected first rich-list address %s, got %s", addresses[1], list[0].Address)
	}
	if list[1].Address != addresses[2] {
		t.Fatalf("expected second rich-list address %s, got %s", addresses[2], list[1].Address)
	}
	if list[2].Address != addresses[3] {
		t.Fatalf("expected third rich-list address %s, got %s", addresses[3], list[2].Address)
	}
	if list[3].Address != addresses[0] {
		t.Fatalf("expected fourth rich-list address %s, got %s", addresses[0], list[3].Address)
	}
}

func TestRunResumesBootstrapFromSavedProgress(t *testing.T) {
	dataDir := t.TempDir()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MemUTXOMaxCount: 1,
		BatchSize:       100,
		Workers:         1,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	params := config.IndexerParams{}
	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	addressStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	balanceStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore address_balance: %v", err)
	}
	rankStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore balance_rank: %v", err)
	}
	for _, closer := range []interface{ Close() error }{rankStore, balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close empty stores before income/spend seed: %v", err)
		}
	}
	seedIncomeStore(t, params, dataDir, 1, map[string]string{
		"addr-a": "tx-1@0@50@1700000000",
		"addr-b": "tx-2@0@30@1700000001",
	})

	opts := bootstrapOptions{
		CommitBatchSize: 1,
		MaxAddresses:    1,
		SleepPerCommit:  0,
	}
	if err := runWithOptions(dataDir, 1, 1, 1, "mvc", "mainnet", opts); err != nil {
		t.Fatalf("first runWithOptions: %v", err)
	}

	metaStore, err = storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("reopen metaStore after first run: %v", err)
	}
	progress, err := loadBootstrapProgress(metaStore)
	if err != nil {
		t.Fatalf("loadBootstrapProgress after first run: %v", err)
	}
	if progress.Done {
		t.Fatal("expected bootstrap progress to remain incomplete after limited run")
	}
	if progress.CurrentShard != 0 {
		t.Fatalf("expected current shard 0 after limited run, got %d", progress.CurrentShard)
	}
	if progress.LastKey != "addr-a" {
		t.Fatalf("expected last key addr-a after limited run, got %q", progress.LastKey)
	}
	if progress.ScannedAddressCount != 1 {
		t.Fatalf("expected scanned address count 1 after limited run, got %d", progress.ScannedAddressCount)
	}
	if progress.IndexedAddressCount != 1 {
		t.Fatalf("expected indexed address count 1 after limited run, got %d", progress.IndexedAddressCount)
	}
	ready, err := metaStore.Get([]byte(balanceIndexReadyMetaKey))
	if err != nil {
		t.Fatalf("get balance_index_ready after first run: %v", err)
	}
	if string(ready) != "0" {
		t.Fatalf("expected balance_index_ready=0 after limited run, got %q", string(ready))
	}
	if err := metaStore.Close(); err != nil {
		t.Fatalf("close metaStore after first run: %v", err)
	}

	opts.MaxAddresses = 0
	if err := runWithOptions(dataDir, 1, 1, 1, "mvc", "mainnet", opts); err != nil {
		t.Fatalf("second runWithOptions: %v", err)
	}

	metaStore, err = storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("reopen metaStore after second run: %v", err)
	}
	defer metaStore.Close()
	addressStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("reopen addressStore after second run: %v", err)
	}
	defer addressStore.Close()
	spendStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("reopen spendStore after second run: %v", err)
	}
	defer spendStore.Close()
	balanceStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("reopen balanceStore after second run: %v", err)
	}
	defer balanceStore.Close()
	rankStore, err = storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("reopen rankStore after second run: %v", err)
	}
	defer rankStore.Close()

	progress, err = loadBootstrapProgress(metaStore)
	if err != nil {
		t.Fatalf("loadBootstrapProgress after second run: %v", err)
	}
	if !progress.Done {
		t.Fatal("expected bootstrap progress to be complete after resume run")
	}
	if progress.ScannedAddressCount != 2 {
		t.Fatalf("expected scanned address count 2 after resume run, got %d", progress.ScannedAddressCount)
	}
	if progress.IndexedAddressCount != 2 {
		t.Fatalf("expected indexed address count 2 after resume run, got %d", progress.IndexedAddressCount)
	}
	ready, err = metaStore.Get([]byte(balanceIndexReadyMetaKey))
	if err != nil {
		t.Fatalf("get balance_index_ready after second run: %v", err)
	}
	if string(ready) != "1" {
		t.Fatalf("expected balance_index_ready=1 after resume run, got %q", string(ready))
	}

	idx := indexer.NewUTXOIndexer(params, nil, addressStore, metaStore, spendStore)
	idx.SetBalanceStores(balanceStore, rankStore)

	balanceA, err := idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a after resume: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 50 || balanceA.UTXOCount != 1 {
		t.Fatalf("unexpected addr-a balance after resume: %+v", balanceA)
	}
	balanceB, err := idx.GetBalance("addr-b", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-b after resume: %v", err)
	}
	if balanceB.ConfirmedBalanceSatoshi != 30 || balanceB.UTXOCount != 1 {
		t.Fatalf("unexpected addr-b balance after resume: %+v", balanceB)
	}
}

func TestRunMarksReadyOnlyAfterAllShardsFinish(t *testing.T) {
	dataDir := t.TempDir()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MemUTXOMaxCount: 1,
		BatchSize:       100,
		Workers:         1,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	params := config.IndexerParams{}
	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	addressStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	balanceStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore address_balance: %v", err)
	}
	rankStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore balance_rank: %v", err)
	}
	for _, closer := range []interface{ Close() error }{rankStore, balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close empty stores before income/spend seed: %v", err)
		}
	}
	seedIncomeStore(t, params, dataDir, 1, map[string]string{
		"addr-a": "tx-1@0@50@1700000000",
		"addr-b": "tx-2@0@30@1700000001",
	})

	if err := runWithOptions(dataDir, 1, 1, 1, "mvc", "mainnet", bootstrapOptions{
		CommitBatchSize: 1,
		MaxAddresses:    1,
		SleepPerCommit:  time.Millisecond,
	}); err != nil {
		t.Fatalf("limited runWithOptions: %v", err)
	}

	metaStore, err = storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("reopen metaStore: %v", err)
	}
	defer metaStore.Close()

	ready, err := metaStore.Get([]byte(balanceIndexReadyMetaKey))
	if err != nil {
		t.Fatalf("get balance_index_ready after limited run: %v", err)
	}
	if string(ready) != "0" {
		t.Fatalf("expected balance_index_ready=0 after limited run, got %q", string(ready))
	}
	progress, err := loadBootstrapProgress(metaStore)
	if err != nil {
		t.Fatalf("loadBootstrapProgress after limited run: %v", err)
	}
	if progress.Done {
		t.Fatal("expected progress.Done to be false after limited run")
	}
}

func addressForShard(targetShard, shardCount int) string {
	for i := 0; ; i++ {
		address := fmt.Sprintf("addr-%d", i)
		if int(xxhash.Sum64String(address)%uint64(shardCount)) == targetShard {
			return address
		}
	}
}
