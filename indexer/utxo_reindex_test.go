package indexer

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/metaid/utxo_indexer/storage"
)

func TestResetConfirmedHistoryForReindexClearsDerivedAndMemoryState(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}
	if err := idx.utxoStore.Set([]byte("tx-old"), []byte(",addr-reset@2000@1")); err != nil {
		t.Fatalf("seed utxoStore: %v", err)
	}
	if err := idx.addressStore.Set([]byte("addr-reset"), []byte("tx-old@0@2000@1")); err != nil {
		t.Fatalf("seed addressStore: %v", err)
	}
	if err := idx.spendStore.Set([]byte("addr-reset"), []byte("tx-old:0@2@tx-spend")); err != nil {
		t.Fatalf("seed spendStore: %v", err)
	}
	if err := idx.balanceStore.Set([]byte("addr-reset"), []byte(`{"balance_satoshi":2000,"utxo_count":1}`)); err != nil {
		t.Fatalf("seed balanceStore: %v", err)
	}
	if err := idx.rankStore.Set([]byte("rank-reset"), []byte(`{"address":"addr-reset"}`)); err != nil {
		t.Fatalf("seed rankStore: %v", err)
	}
	idx.memUTXO.Store("tx-old:0", "addr-reset@2000@1")
	atomic.StoreInt64(&idx.memUTXOCount, 1)

	if err := idx.ResetConfirmedHistoryForReindex(); err != nil {
		t.Fatalf("ResetConfirmedHistoryForReindex: %v", err)
	}

	assertStoreValue(t, idx.utxoStore, "tx-old", ",addr-reset@2000@1")
	assertStoreMissing(t, idx.addressStore, "addr-reset")
	assertStoreMissing(t, idx.spendStore, "addr-reset")
	assertStoreMissing(t, idx.balanceStore, "addr-reset")
	assertStoreMissing(t, idx.rankStore, "rank-reset")
	if idx.isBalanceIndexReady() {
		t.Fatalf("balance index should be marked not ready after reset")
	}
	if atomic.LoadInt64(&idx.memUTXOCount) != 0 {
		t.Fatalf("expected memUTXOCount reset to 0, got %d", atomic.LoadInt64(&idx.memUTXOCount))
	}
	if _, ok := idx.memUTXO.Load("tx-old:0"); ok {
		t.Fatalf("expected memUTXO entry to be removed")
	}
}

func assertStoreMissing(t *testing.T, store *storage.PebbleStore, key string) {
	t.Helper()

	if _, err := store.Get([]byte(key)); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected %s to be missing, got err=%v", key, err)
	}
}

func assertStoreValue(t *testing.T, store *storage.PebbleStore, key string, expected string) {
	t.Helper()

	got, err := store.Get([]byte(key))
	if err != nil {
		t.Fatalf("expected %s to be present: %v", key, err)
	}
	if string(got) != expected {
		t.Fatalf("expected %s value %q, got %q", key, expected, string(got))
	}
}

func TestReindexStateAllowsSingleOwner(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if !idx.BeginReindex() {
		t.Fatalf("expected first BeginReindex to acquire reindex state")
	}
	if !idx.IsReindexing() {
		t.Fatalf("expected IsReindexing to be true after BeginReindex")
	}
	if idx.BeginReindex() {
		t.Fatalf("expected second BeginReindex to be rejected")
	}

	idx.EndReindex()
	if idx.IsReindexing() {
		t.Fatalf("expected IsReindexing to be false after EndReindex")
	}
	if !idx.BeginReindex() {
		t.Fatalf("expected BeginReindex to acquire state after EndReindex")
	}
	idx.EndReindex()
}

func TestReindexPreservesEarlierUTXOsForAddressAcrossBlocks(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	idx.balanceStore = nil
	idx.rankStore = nil

	address := "addr-reindex-history"
	block1 := &Block{
		Height:        1,
		BlockHash:     "block-1",
		Transactions:  []*Transaction{{ID: "tx-reindex-1", Outputs: []*Output{{Address: address, Amount: "2000"}}}},
		AddressIncome: make(map[string][]*Income),
	}
	allBlock1 := &Block{
		Height:     block1.Height,
		BlockHash:  block1.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}
	block2 := &Block{
		Height:        2,
		BlockHash:     "block-2",
		Transactions:  []*Transaction{{ID: "tx-reindex-2", Outputs: []*Output{{Address: address, Amount: "3000"}}}},
		AddressIncome: make(map[string][]*Income),
	}
	allBlock2 := &Block{
		Height:     block2.Height,
		BlockHash:  block2.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}

	if _, _, _, err := idx.IndexBlock(block1, allBlock1, false, "1700000000", true); err != nil {
		t.Fatalf("IndexBlock block1: %v", err)
	}
	if _, _, _, err := idx.IndexBlock(block2, allBlock2, false, "1700000100", true); err != nil {
		t.Fatalf("IndexBlock block2: %v", err)
	}

	balance, err := idx.GetBalance(address, 0)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 5000 {
		t.Fatalf("expected both reindexed UTXOs to remain, got balance %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.UTXOCount != 2 {
		t.Fatalf("expected 2 UTXOs, got %d", balance.UTXOCount)
	}

	utxos, err := idx.GetUTXOs(address)
	if err != nil {
		t.Fatalf("GetUTXOs: %v", err)
	}
	if len(utxos) != 2 {
		t.Fatalf("expected 2 UTXOs, got %d: %#v", len(utxos), utxos)
	}
}

func TestReindexReplacesExistingTxOutputsBeforeMerge(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	idx.balanceStore = nil
	idx.rankStore = nil

	if err := idx.utxoStore.Set([]byte("tx-order"), []byte(",zaddr@2000@1,aaddr@3000@1")); err != nil {
		t.Fatalf("seed utxoStore: %v", err)
	}

	block := &Block{
		Height:    3,
		BlockHash: "block-order",
		Transactions: []*Transaction{{
			ID: "tx-order",
			Outputs: []*Output{
				{Address: "aaddr", Amount: "3000"},
				{Address: "zaddr", Amount: "2000"},
			},
		}},
		AddressIncome: make(map[string][]*Income),
	}
	allBlock := &Block{
		Height:     block.Height,
		BlockHash:  block.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}

	if _, _, _, err := idx.IndexBlock(block, allBlock, false, "1700000200", true); err != nil {
		t.Fatalf("IndexBlock: %v", err)
	}

	assertStoreValue(t, idx.utxoStore, "tx-order", ",aaddr@3000@1700000200@0,zaddr@2000@1700000200@1")
}

func TestIndexBlockPreservesDuplicateOutputsByVoutIndex(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	block1 := &Block{
		Height:    10,
		BlockHash: "block-duplicate-outputs",
		Transactions: []*Transaction{{
			ID: "tx-duplicate-outputs",
			Outputs: []*Output{
				{Address: "addr-duplicate", Amount: "1000"},
				{Address: "addr-duplicate", Amount: "1000"},
			},
		}},
		AddressIncome: make(map[string][]*Income),
	}
	allBlock1 := &Block{
		Height:     block1.Height,
		BlockHash:  block1.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}
	if _, _, _, err := idx.IndexBlock(block1, allBlock1, false, "1700000000", false); err != nil {
		t.Fatalf("IndexBlock block1: %v", err)
	}

	details, err := idx.utxoStore.QueryUTXODetails(&[]string{"tx-duplicate-outputs:1"})
	if err != nil {
		t.Fatalf("QueryUTXODetails: %v", err)
	}
	if _, ok := details["tx-duplicate-outputs:1"]; !ok {
		t.Fatalf("expected duplicate vout index 1 to be queryable, got %#v", details)
	}

	block2 := &Block{
		Height:    11,
		BlockHash: "block-spend-duplicate-output",
		Transactions: []*Transaction{{
			ID:     "tx-spend-duplicate-output",
			Inputs: []*Input{{TxPoint: "tx-duplicate-outputs:1"}},
		}},
		AddressIncome: make(map[string][]*Income),
	}
	allBlock2 := &Block{
		Height:     block2.Height,
		BlockHash:  block2.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}
	if _, _, _, err := idx.IndexBlock(block2, allBlock2, false, "1700000100", false); err != nil {
		t.Fatalf("IndexBlock block2: %v", err)
	}

	balance, err := idx.GetBalance("addr-duplicate", 0)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 1000 {
		t.Fatalf("expected one duplicate output to remain after spending vout 1, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected one duplicate UTXO to remain after spending vout 1, got %d", balance.UTXOCount)
	}
}

func TestReindexModeRefreshesTouchedBalancesWithoutDoubleCounting(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}

	newBlock := func() *Block {
		return &Block{
			Height:    20,
			BlockHash: "block-reindex-idempotent",
			Transactions: []*Transaction{{
				ID:      "tx-reindex-idempotent",
				Outputs: []*Output{{Address: "addr-reindex-idempotent", Amount: "2500"}},
			}},
			AddressIncome: make(map[string][]*Income),
		}
	}

	for run := 1; run <= 2; run++ {
		block := newBlock()
		allBlock := &Block{
			Height:     block.Height,
			BlockHash:  block.BlockHash,
			UtxoData:   make(map[string][]string),
			IncomeData: make(map[string][]string),
			SpendData:  make(map[string][]string),
		}
		if _, _, _, err := idx.IndexBlock(block, allBlock, false, "1700000000", true); err != nil {
			t.Fatalf("IndexBlock reindex run %d: %v", run, err)
		}
	}

	balance, err := idx.GetBalance("addr-reindex-idempotent", 0)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 2500 {
		t.Fatalf("expected idempotent reindex balance 2500, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected idempotent reindex utxo count 1, got %d", balance.UTXOCount)
	}
}

func TestReindexSessionDefersTouchedBalanceRefreshUntilEnd(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}
	if err := idx.putAddressBalance("addr-reindex-deferred", 99, 1); err != nil {
		t.Fatalf("seed stale balance: %v", err)
	}
	if !idx.BeginReindex() {
		t.Fatalf("BeginReindex: expected to acquire reindex state")
	}

	block := &Block{
		Height:    30,
		BlockHash: "block-reindex-deferred",
		Transactions: []*Transaction{{
			ID:      "tx-reindex-deferred",
			Outputs: []*Output{{Address: "addr-reindex-deferred", Amount: "2500"}},
		}},
		AddressIncome: make(map[string][]*Income),
	}
	allBlock := &Block{
		Height:     block.Height,
		BlockHash:  block.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}
	if _, _, _, err := idx.IndexBlock(block, allBlock, false, "1700000000", true); err != nil {
		t.Fatalf("IndexBlock: %v", err)
	}

	balance, err := idx.GetBalance("addr-reindex-deferred", 0)
	if err != nil {
		t.Fatalf("GetBalance before EndReindex: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 99 {
		t.Fatalf("expected stale balance before EndReindex, got %d", balance.ConfirmedBalanceSatoshi)
	}

	idx.EndReindex()
	balance, err = idx.GetBalance("addr-reindex-deferred", 0)
	if err != nil {
		t.Fatalf("GetBalance after EndReindex: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 2500 {
		t.Fatalf("expected refreshed balance after EndReindex, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected refreshed utxo count after EndReindex, got %d", balance.UTXOCount)
	}
}
