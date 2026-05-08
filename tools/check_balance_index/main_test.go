package main

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

func TestInspectReportsMatchBetweenHistoryAndBalanceRow(t *testing.T) {
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

	if err := metaStore.Set([]byte("balance_index_ready"), []byte("0")); err != nil {
		t.Fatalf("seed balance_index_ready: %v", err)
	}
	if err := metaStore.Set([]byte("last_indexed_height"), []byte("123")); err != nil {
		t.Fatalf("seed last_indexed_height: %v", err)
	}
	if err := addressStore.Set([]byte("addr-a"), []byte("tx-1@0@50@1700000000,tx-2@0@20@1700000001")); err != nil {
		t.Fatalf("seed income addr-a: %v", err)
	}
	if err := spendStore.Set([]byte("addr-a"), []byte("tx-1:0@1700000010@spend-1")); err != nil {
		t.Fatalf("seed spend addr-a: %v", err)
	}
	if err := balanceStore.Set([]byte("addr-a"), []byte(`{"balance_satoshi":20,"utxo_count":1}`)); err != nil {
		t.Fatalf("seed balance row addr-a: %v", err)
	}

	for _, closer := range []interface{ Close() error }{balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close seeded store: %v", err)
		}
	}

	report, err := inspect(dataDir, 1, 1, 1, "mvc", "mainnet", []string{"addr-a"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	if report.ReadyRaw != "0" {
		t.Fatalf("expected ready raw 0, got %q", report.ReadyRaw)
	}
	if report.LastIndexedHeight != "123" {
		t.Fatalf("expected last indexed height 123, got %q", report.LastIndexedHeight)
	}
	if report.Summary.AddressBalanceRowCount != 1 {
		t.Fatalf("expected address balance row count 1, got %d", report.Summary.AddressBalanceRowCount)
	}
	if report.Summary.TrackedRowCount != 0 {
		t.Fatalf("expected tracked row count 0, got %d", report.Summary.TrackedRowCount)
	}
	if report.Summary.MatchCount != 1 {
		t.Fatalf("expected match count 1, got %d", report.Summary.MatchCount)
	}
	if report.Summary.MissingRowCount != 0 {
		t.Fatalf("expected missing row count 0, got %d", report.Summary.MissingRowCount)
	}
	if len(report.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(report.Checks))
	}

	check := report.Checks[0]
	if check.Status != statusMatch {
		t.Fatalf("expected status %q, got %q", statusMatch, check.Status)
	}
	if check.Row == nil {
		t.Fatal("expected balance row to be present")
	}
	if check.Row.BalanceSatoshi != 20 || check.Row.UTXOCount != 1 {
		t.Fatalf("unexpected row: %+v", *check.Row)
	}
	if check.History.BalanceSatoshi != 20 || check.History.UTXOCount != 1 {
		t.Fatalf("unexpected history: %+v", check.History)
	}
}

func TestInspectReportsMissingRowAgainstHistory(t *testing.T) {
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

	if err := metaStore.Set([]byte("balance_index_ready"), []byte("0")); err != nil {
		t.Fatalf("seed balance_index_ready: %v", err)
	}
	if err := addressStore.Set([]byte("addr-b"), []byte("tx-3@0@30@1700000002")); err != nil {
		t.Fatalf("seed income addr-b: %v", err)
	}

	for _, closer := range []interface{ Close() error }{balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close seeded store: %v", err)
		}
	}

	report, err := inspect(dataDir, 1, 1, 1, "mvc", "mainnet", []string{"addr-b"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	if report.Summary.AddressBalanceRowCount != 0 {
		t.Fatalf("expected address balance row count 0, got %d", report.Summary.AddressBalanceRowCount)
	}
	if report.Summary.TrackedRowCount != 0 {
		t.Fatalf("expected tracked row count 0, got %d", report.Summary.TrackedRowCount)
	}
	if report.Summary.MatchCount != 0 {
		t.Fatalf("expected match count 0, got %d", report.Summary.MatchCount)
	}
	if report.Summary.MissingRowCount != 1 {
		t.Fatalf("expected missing row count 1, got %d", report.Summary.MissingRowCount)
	}

	if len(report.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(report.Checks))
	}

	check := report.Checks[0]
	if check.Status != statusMissingRow {
		t.Fatalf("expected status %q, got %q", statusMissingRow, check.Status)
	}
	if check.Row != nil {
		t.Fatalf("expected no balance row, got %+v", *check.Row)
	}
	if check.History.BalanceSatoshi != 30 || check.History.UTXOCount != 1 {
		t.Fatalf("unexpected history: %+v", check.History)
	}
}

func TestInspectSummaryCountsTrackedRowsAndMismatches(t *testing.T) {
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

	if err := addressStore.Set([]byte("addr-c"), []byte("tx-4@0@80@1700000003")); err != nil {
		t.Fatalf("seed income addr-c: %v", err)
	}
	if err := balanceStore.Set([]byte("addr-c"), []byte(`{"balance_satoshi":70,"utxo_count":1,"tracked":true}`)); err != nil {
		t.Fatalf("seed tracked balance row addr-c: %v", err)
	}

	for _, closer := range []interface{ Close() error }{balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close seeded store: %v", err)
		}
	}

	report, err := inspect(dataDir, 1, 1, 1, "mvc", "mainnet", []string{"addr-c"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	if report.Summary.AddressBalanceRowCount != 1 {
		t.Fatalf("expected address balance row count 1, got %d", report.Summary.AddressBalanceRowCount)
	}
	if report.Summary.TrackedRowCount != 1 {
		t.Fatalf("expected tracked row count 1, got %d", report.Summary.TrackedRowCount)
	}
	if report.Summary.MismatchCount != 1 {
		t.Fatalf("expected mismatch count 1, got %d", report.Summary.MismatchCount)
	}
	if report.Checks[0].Status != statusMismatch {
		t.Fatalf("expected mismatch status, got %q", report.Checks[0].Status)
	}
}

func TestInspectHistoryCountsOnlyUnspentUTXOs(t *testing.T) {
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

	if err := addressStore.Set([]byte("addr-d"), []byte("tx-1@0@50@1700000000,tx-2@0@20@1700000001")); err != nil {
		t.Fatalf("seed income addr-d: %v", err)
	}
	if err := spendStore.Set([]byte("addr-d"), []byte("tx-1:0@1700000010@spend-1")); err != nil {
		t.Fatalf("seed spend addr-d: %v", err)
	}

	for _, closer := range []interface{ Close() error }{balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close seeded store: %v", err)
		}
	}

	report, err := inspect(dataDir, 1, 1, 1, "mvc", "mainnet", []string{"addr-d"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	if len(report.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(report.Checks))
	}

	check := report.Checks[0]
	if check.History.BalanceSatoshi != 20 {
		t.Fatalf("expected confirmed balance 20 after spend deduction, got %d", check.History.BalanceSatoshi)
	}
	if check.History.UTXOCount != 1 {
		t.Fatalf("expected unspent UTXOCount 1, got %d", check.History.UTXOCount)
	}
}

func TestInspectIncludesBootstrapProgress(t *testing.T) {
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

	if err := metaStore.Set([]byte("balance_index_bootstrap_progress"), []byte(`{"current_shard":2,"last_key":"addr-z","scanned_address_count":123,"indexed_address_count":45,"done":false}`)); err != nil {
		t.Fatalf("seed balance_index_bootstrap_progress: %v", err)
	}

	for _, closer := range []interface{ Close() error }{balanceStore, spendStore, addressStore, metaStore} {
		if err := closer.Close(); err != nil {
			t.Fatalf("close seeded store: %v", err)
		}
	}

	report, err := inspect(dataDir, 1, 1, 1, "mvc", "mainnet", []string{"addr-none"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	if report.BootstrapProgress == nil {
		t.Fatal("expected bootstrap progress to be present")
	}
	if report.BootstrapProgress.CurrentShard != 2 {
		t.Fatalf("expected current shard 2, got %d", report.BootstrapProgress.CurrentShard)
	}
	if report.BootstrapProgress.LastKey != "addr-z" {
		t.Fatalf("expected last key addr-z, got %q", report.BootstrapProgress.LastKey)
	}
	if report.BootstrapProgress.ScannedAddressCount != 123 {
		t.Fatalf("expected scanned count 123, got %d", report.BootstrapProgress.ScannedAddressCount)
	}
	if report.BootstrapProgress.IndexedAddressCount != 45 {
		t.Fatalf("expected indexed count 45, got %d", report.BootstrapProgress.IndexedAddressCount)
	}
	if report.BootstrapProgress.Done {
		t.Fatal("expected progress.Done to be false")
	}
}
