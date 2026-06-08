package indexer

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

type stubMempoolManager struct {
	income map[string]string
	spend  map[string]string
	utxos  []common.Utxo
}

func (m *stubMempoolManager) GetDataByAddress(address string) (map[string]string, map[string]string) {
	return m.income, m.spend
}

func (m *stubMempoolManager) GetUTXOsByAddress(address string) ([]common.Utxo, error) {
	return append([]common.Utxo(nil), m.utxos...), nil
}

func (m *stubMempoolManager) GetSpendUTXOs(txPoints []string) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}

func (m *stubMempoolManager) BatchDeleteIncom(list []string) error { return nil }

func (m *stubMempoolManager) BatchDeleteSpend(list []string) error { return nil }

func (m *stubMempoolManager) DeleteMempool() error { return nil }

func (m *stubMempoolManager) StartMempool() error { return nil }

type stubConfirmedUTXOValidator struct {
	unspent         map[string]bool
	detailedUnspent map[string]bool
	err             error
}

func (v *stubConfirmedUTXOValidator) IsUnspent(txID string, index uint32) (bool, error) {
	if v.err != nil {
		return false, v.err
	}
	return v.unspent[txID+":"+strconv.FormatUint(uint64(index), 10)], nil
}

func (v *stubConfirmedUTXOValidator) ValidateUTXO(txID string, index uint32, address string, amount uint64) (bool, error) {
	if v.err != nil {
		return false, v.err
	}
	if v.detailedUnspent == nil {
		return v.IsUnspent(txID, index)
	}
	key := txID + ":" + strconv.FormatUint(uint64(index), 10) + "|" + address + "|" + strconv.FormatUint(amount, 10)
	return v.detailedUnspent[key], nil
}

func newBalanceIndexTestIndexer(t *testing.T) *UTXOIndexer {
	t.Helper()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MemUTXOMaxCount: 1,
		BatchSize:       100,
		Workers:         1,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	dataDir := t.TempDir()
	params := config.IndexerParams{}

	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	t.Cleanup(func() {
		if err := metaStore.Close(); err != nil {
			t.Fatalf("MetaStore.Close: %v", err)
		}
	})

	addressStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	t.Cleanup(func() {
		if err := addressStore.Close(); err != nil {
			t.Fatalf("addressStore.Close: %v", err)
		}
	})

	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	t.Cleanup(func() {
		if err := spendStore.Close(); err != nil {
			t.Fatalf("spendStore.Close: %v", err)
		}
	})

	utxoStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeUTXO, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore utxo: %v", err)
	}
	t.Cleanup(func() {
		if err := utxoStore.Close(); err != nil {
			t.Fatalf("utxoStore.Close: %v", err)
		}
	})

	balanceStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeAddressBalance, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore address_balance: %v", err)
	}
	t.Cleanup(func() {
		if err := balanceStore.Close(); err != nil {
			t.Fatalf("balanceStore.Close: %v", err)
		}
	})

	rankStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore balance_rank: %v", err)
	}
	t.Cleanup(func() {
		if err := rankStore.Close(); err != nil {
			t.Fatalf("rankStore.Close: %v", err)
		}
	})

	return &UTXOIndexer{
		utxoStore:    utxoStore,
		metaStore:    metaStore,
		addressStore: addressStore,
		spendStore:   spendStore,
		balanceStore: balanceStore,
		rankStore:    rankStore,
	}
}

func TestGetBalanceUsesConfirmedBalanceIndex(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}

	if err := idx.putAddressBalance("addr-balance-1", 123456789, 3); err != nil {
		t.Fatalf("putAddressBalance: %v", err)
	}

	balance, err := idx.GetBalance("addr-balance-1", 600)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}

	if balance.ConfirmedBalanceSatoshi != 123456789 {
		t.Fatalf("expected confirmed_balance_satoshi 123456789, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.BalanceSatoshi != 123456789 {
		t.Fatalf("expected balance_satoshi 123456789, got %d", balance.BalanceSatoshi)
	}
	if balance.UTXOCount != 3 {
		t.Fatalf("expected confirmed_utxo_count 3, got %d", balance.UTXOCount)
	}
}

func TestGetBalanceFallsBackToHistoryWhenBalanceIndexNotReady(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.addressStore.Set([]byte("addr-history"), []byte("tx-h@0@42@1700000000")); err != nil {
		t.Fatalf("seed income addr-history: %v", err)
	}

	balance, err := idx.GetBalance("addr-history", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-history: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 42 {
		t.Fatalf("expected confirmed balance 42 from history fallback, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected utxo count 1 from history fallback, got %d", balance.UTXOCount)
	}
}

func TestGetBalanceHistoryFallbackCountsOnlyUnspentUTXOs(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.addressStore.Set([]byte("addr-history-spent"), []byte("tx-1@0@50@1700000000,tx-2@0@20@1700000001")); err != nil {
		t.Fatalf("seed income addr-history-spent: %v", err)
	}
	if err := idx.spendStore.Set([]byte("addr-history-spent"), []byte("tx-1:0@1700000010@spend-1")); err != nil {
		t.Fatalf("seed spend addr-history-spent: %v", err)
	}

	balance, err := idx.GetBalance("addr-history-spent", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-history-spent: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 20 {
		t.Fatalf("expected confirmed balance 20 from history fallback, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected unspent utxo count 1 from history fallback, got %d", balance.UTXOCount)
	}
}

func TestGetBalanceFallsBackToHistoryWhenConfirmedBalanceRowMissing(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}

	if err := idx.addressStore.Set([]byte("addr-missing-row"), []byte("tx-h@0@42@1700000000")); err != nil {
		t.Fatalf("seed income addr-missing-row: %v", err)
	}

	balance, err := idx.GetBalance("addr-missing-row", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-missing-row: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 42 {
		t.Fatalf("expected confirmed balance 42 from history fallback, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.BalanceSatoshi != 42 {
		t.Fatalf("expected balance 42 from history fallback, got %d", balance.BalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected utxo count 1 from history fallback, got %d", balance.UTXOCount)
	}
}

func TestGetBalanceIgnoresPartialBalanceIndexWhenNotReady(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.addressStore.Set([]byte("addr-partial"), []byte("tx-h@0@42@1700000000")); err != nil {
		t.Fatalf("seed income addr-partial: %v", err)
	}
	if err := idx.putAddressBalance("addr-partial", 999999, 99); err != nil {
		t.Fatalf("putAddressBalance addr-partial: %v", err)
	}

	balance, err := idx.GetBalance("addr-partial", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-partial: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 42 {
		t.Fatalf("expected confirmed balance 42 from history fallback, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.BalanceSatoshi != 42 {
		t.Fatalf("expected balance 42 from history fallback, got %d", balance.BalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected utxo count 1 from history fallback, got %d", balance.UTXOCount)
	}
}

func TestGetBalanceMaterializesAddressBalanceRowFromHistoryFallback(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.addressStore.Set([]byte("addr-cache"), []byte("tx-h@0@42@1700000000")); err != nil {
		t.Fatalf("seed income addr-cache: %v", err)
	}

	balance, err := idx.GetBalance("addr-cache", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-cache: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 42 {
		t.Fatalf("expected confirmed balance 42 from history fallback, got %d", balance.ConfirmedBalanceSatoshi)
	}

	row, err := idx.getAddressBalanceRow("addr-cache")
	if err != nil {
		t.Fatalf("getAddressBalanceRow addr-cache: %v", err)
	}
	if row.BalanceSatoshi != 42 {
		t.Fatalf("expected cached balance row 42, got %d", row.BalanceSatoshi)
	}
	if row.UTXOCount != 1 {
		t.Fatalf("expected cached utxo count 1, got %d", row.UTXOCount)
	}

	if err := idx.addressStore.Delete([]byte("addr-cache")); err != nil {
		t.Fatalf("delete income addr-cache: %v", err)
	}

	balance, err = idx.GetBalance("addr-cache", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-cache from cached row: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 42 {
		t.Fatalf("expected confirmed balance 42 from cached row, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.UTXOCount != 1 {
		t.Fatalf("expected utxo count 1 from cached row, got %d", balance.UTXOCount)
	}
}

func TestGetBalanceDeduplicatesMempoolIncomeRecords(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.addressStore.Set([]byte("addr-mempool"), []byte("confirmed-tx@0@12000@1700000000")); err != nil {
		t.Fatalf("seed income addr-mempool: %v", err)
	}
	idx.mempoolManager = &stubMempoolManager{
		income: map[string]string{
			"addr-mempool_tx-mem:0_111": "5000",
			"addr-mempool_tx-mem:2_111": "7000",
			"addr-mempool_tx-mem:0_222": "5000",
			"addr-mempool_tx-mem:2_222": "7000",
		},
		spend: map[string]string{
			"addr-mempool_confirmed-tx:0_111": "tx-mem",
		},
	}

	balance, err := idx.GetBalance("addr-mempool", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-mempool: %v", err)
	}
	if balance.ConfirmedBalanceSatoshi != 12000 {
		t.Fatalf("expected confirmed balance 12000, got %d", balance.ConfirmedBalanceSatoshi)
	}
	if balance.MempoolIncome != 12000 {
		t.Fatalf("expected mempool income 12000, got %d", balance.MempoolIncome)
	}
	if balance.MempoolSpend != 12000 {
		t.Fatalf("expected mempool spend 12000, got %d", balance.MempoolSpend)
	}
	if balance.BalanceSatoshi != 12000 {
		t.Fatalf("expected final balance 12000, got %d", balance.BalanceSatoshi)
	}
}

func TestGetUTXOsDeduplicatesMempoolIncomeRecords(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.addressStore.Set([]byte("addr-utxos"), []byte("confirmed-tx@0@12000@1700000000")); err != nil {
		t.Fatalf("seed income addr-utxos: %v", err)
	}
	idx.mempoolManager = &stubMempoolManager{
		income: map[string]string{
			"addr-utxos_tx-mem:0_111": "5000",
			"addr-utxos_tx-mem:2_111": "7000",
			"addr-utxos_tx-mem:0_222": "5000",
			"addr-utxos_tx-mem:2_222": "7000",
		},
		spend: map[string]string{
			"addr-utxos_confirmed-tx:0_111": "tx-mem",
		},
	}

	utxos, err := idx.GetUTXOs("addr-utxos")
	if err != nil {
		t.Fatalf("GetUTXOs addr-utxos: %v", err)
	}
	if len(utxos) != 2 {
		t.Fatalf("expected 2 mempool utxos after dedupe, got %d", len(utxos))
	}
	if utxos[0].TxID == utxos[1].TxID && utxos[0].Index == utxos[1].Index {
		t.Fatalf("expected unique mempool utxos, got duplicates: %+v", utxos)
	}
}

func TestGetUTXOsFiltersConfirmedOutputsMissingFromNodeUTXOSet(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.addressStore.Set([]byte("addr-rpc-check"), []byte("tx-stale@0@5000@1700000000,tx-good@1@6000@1700000001")); err != nil {
		t.Fatalf("seed addressStore: %v", err)
	}
	idx.SetConfirmedUTXOValidator(&stubConfirmedUTXOValidator{
		unspent: map[string]bool{
			"tx-good:1": true,
		},
	}, true, 2)

	utxos, err := idx.GetUTXOs("addr-rpc-check")
	if err != nil {
		t.Fatalf("GetUTXOs addr-rpc-check: %v", err)
	}
	if len(utxos) != 1 {
		t.Fatalf("expected only 1 RPC-confirmed UTXO, got %d: %+v", len(utxos), utxos)
	}
	if utxos[0].TxID != "tx-good" || utxos[0].Index != "1" {
		t.Fatalf("expected tx-good:1 to remain, got %+v", utxos[0])
	}
}

func TestGetUTXOsFiltersConfirmedOutputsWhenNodeUTXODetailDiffers(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.addressStore.Set([]byte("addr-rpc-detail"), []byte("tx-shifted@2@4933697893434@1700000000,tx-good@1@6000@1700000001")); err != nil {
		t.Fatalf("seed addressStore: %v", err)
	}
	idx.SetConfirmedUTXOValidator(&stubConfirmedUTXOValidator{
		unspent: map[string]bool{
			"tx-shifted:2": true,
			"tx-good:1":    true,
		},
		detailedUnspent: map[string]bool{
			"tx-good:1|addr-rpc-detail|6000": true,
		},
	}, true, 2)

	utxos, err := idx.GetUTXOs("addr-rpc-detail")
	if err != nil {
		t.Fatalf("GetUTXOs addr-rpc-detail: %v", err)
	}
	if len(utxos) != 1 {
		t.Fatalf("expected only detail-matched UTXO, got %d: %+v", len(utxos), utxos)
	}
	if utxos[0].TxID != "tx-good" || utxos[0].Index != "1" {
		t.Fatalf("expected tx-good:1 to remain, got %+v", utxos[0])
	}
}

func TestGetUTXOsFailsClosedWhenConfirmedUTXOValidationFails(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.addressStore.Set([]byte("addr-rpc-error"), []byte("tx-a@0@5000@1700000000")); err != nil {
		t.Fatalf("seed addressStore: %v", err)
	}
	idx.SetConfirmedUTXOValidator(&stubConfirmedUTXOValidator{err: errors.New("rpc unavailable")}, true, 2)

	_, err := idx.GetUTXOs("addr-rpc-error")
	if err == nil {
		t.Fatalf("expected GetUTXOs to fail closed when RPC validation fails")
	}
}

func TestGetRichListReadsFromBalanceRankIndex(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.putAddressBalance("addr-rank-1", 10, 1); err != nil {
		t.Fatalf("putAddressBalance addr-rank-1: %v", err)
	}
	if err := idx.putAddressBalance("addr-rank-2", 30, 2); err != nil {
		t.Fatalf("putAddressBalance addr-rank-2: %v", err)
	}
	if err := idx.putAddressBalance("addr-rank-3", 20, 1); err != nil {
		t.Fatalf("putAddressBalance addr-rank-3: %v", err)
	}
	if err := idx.putBalanceRank("addr-rank-1", 10, 1); err != nil {
		t.Fatalf("putBalanceRank addr-rank-1: %v", err)
	}
	if err := idx.putBalanceRank("addr-rank-2", 30, 2); err != nil {
		t.Fatalf("putBalanceRank addr-rank-2: %v", err)
	}
	if err := idx.putBalanceRank("addr-rank-3", 20, 1); err != nil {
		t.Fatalf("putBalanceRank addr-rank-3: %v", err)
	}

	list, total, err := idx.GetRichList(1, 2, 0)
	if err != nil {
		t.Fatalf("GetRichList: %v", err)
	}

	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
	if list[0].Address != "addr-rank-2" {
		t.Fatalf("expected first address addr-rank-2, got %s", list[0].Address)
	}
	if list[1].Address != "addr-rank-3" {
		t.Fatalf("expected second address addr-rank-3, got %s", list[1].Address)
	}
}

func TestGetRichListReturnsCacheNotReadyUntilBalanceIndexReady(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.metaStore.Set([]byte("total_address_count"), []byte("12")); err != nil {
		t.Fatalf("seed total_address_count: %v", err)
	}
	if err := idx.putAddressBalance("addr-rank-partial", 99, 1); err != nil {
		t.Fatalf("putAddressBalance addr-rank-partial: %v", err)
	}
	if err := idx.putBalanceRank("addr-rank-partial", 99, 1); err != nil {
		t.Fatalf("putBalanceRank addr-rank-partial: %v", err)
	}

	_, _, err := idx.GetRichList(1, 10, 0)
	if err == nil {
		t.Fatal("expected ErrRichListCacheNotReady, got nil")
	}
	if !errors.Is(err, ErrRichListCacheNotReady) {
		t.Fatalf("expected ErrRichListCacheNotReady, got %v", err)
	}
}

func TestGetRichListReturnsCacheNotReadyWhenRankIndexIsEmpty(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}

	if err := idx.metaStore.Set([]byte("total_address_count"), []byte("12")); err != nil {
		t.Fatalf("seed total_address_count: %v", err)
	}

	_, _, err := idx.GetRichList(1, 10, 0)
	if err == nil {
		t.Fatal("expected ErrRichListCacheNotReady, got nil")
	}
	if !errors.Is(err, ErrRichListCacheNotReady) {
		t.Fatalf("expected ErrRichListCacheNotReady, got %v", err)
	}
}

func TestIndexBlockUpdatesBalanceAndRankStores(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}

	block1 := &Block{
		Height:    100,
		BlockHash: "block-100",
		Transactions: []*Transaction{
			{
				ID:     "tx-100",
				Inputs: []*Input{},
				Outputs: []*Output{
					{Address: "addr-a", Amount: "50"},
				},
			},
		},
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

	balanceA, err := idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a after block1: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 50 {
		t.Fatalf("expected addr-a confirmed balance 50 after block1, got %d", balanceA.ConfirmedBalanceSatoshi)
	}
	if balanceA.UTXOCount != 1 {
		t.Fatalf("expected addr-a utxo count 1 after block1, got %d", balanceA.UTXOCount)
	}

	block2 := &Block{
		Height:    101,
		BlockHash: "block-101",
		Transactions: []*Transaction{
			{
				ID: "tx-101",
				Inputs: []*Input{
					{TxPoint: "tx-100:0"},
				},
				Outputs: []*Output{
					{Address: "addr-b", Amount: "30"},
					{Address: "addr-a", Amount: "20"},
				},
			},
		},
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

	balanceA, err = idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a after block2: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 20 {
		t.Fatalf("expected addr-a confirmed balance 20 after block2, got %d", balanceA.ConfirmedBalanceSatoshi)
	}
	if balanceA.UTXOCount != 1 {
		t.Fatalf("expected addr-a utxo count 1 after block2, got %d", balanceA.UTXOCount)
	}

	balanceB, err := idx.GetBalance("addr-b", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-b after block2: %v", err)
	}
	if balanceB.ConfirmedBalanceSatoshi != 30 {
		t.Fatalf("expected addr-b confirmed balance 30 after block2, got %d", balanceB.ConfirmedBalanceSatoshi)
	}
	if balanceB.UTXOCount != 1 {
		t.Fatalf("expected addr-b utxo count 1 after block2, got %d", balanceB.UTXOCount)
	}

	list, total, err := idx.GetRichList(1, 2, 0)
	if err != nil {
		t.Fatalf("GetRichList after block2: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected rich-list total 2 after block2, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected rich-list length 2 after block2, got %d", len(list))
	}
	if list[0].Address != "addr-b" {
		t.Fatalf("expected first rich-list address addr-b after block2, got %s", list[0].Address)
	}
	if list[1].Address != "addr-a" {
		t.Fatalf("expected second rich-list address addr-a after block2, got %s", list[1].Address)
	}
}

func TestIndexBlockMaintainsMaterializedBalanceRowsWhenIndexNotReady(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	block1 := &Block{
		Height:    100,
		BlockHash: "block-100",
		Transactions: []*Transaction{
			{
				ID:     "tx-100",
				Inputs: []*Input{},
				Outputs: []*Output{
					{Address: "addr-a", Amount: "50"},
				},
			},
		},
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

	rowA, err := idx.getAddressBalanceRow("addr-a")
	if err != nil {
		t.Fatalf("getAddressBalanceRow addr-a after block1: %v", err)
	}
	if rowA.BalanceSatoshi != 50 || rowA.UTXOCount != 1 || !rowA.Tracked {
		t.Fatalf("unexpected addr-a cached row after block1: %+v", rowA)
	}

	block2 := &Block{
		Height:    101,
		BlockHash: "block-101",
		Transactions: []*Transaction{
			{
				ID: "tx-101",
				Inputs: []*Input{
					{TxPoint: "tx-100:0"},
				},
				Outputs: []*Output{
					{Address: "addr-b", Amount: "30"},
					{Address: "addr-a", Amount: "20"},
				},
			},
		},
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

	rowA, err = idx.getAddressBalanceRow("addr-a")
	if err != nil {
		t.Fatalf("getAddressBalanceRow addr-a after block2: %v", err)
	}
	if rowA.BalanceSatoshi != 20 || rowA.UTXOCount != 1 || !rowA.Tracked {
		t.Fatalf("unexpected addr-a cached row after block2: %+v", rowA)
	}

	rowB, err := idx.getAddressBalanceRow("addr-b")
	if err != nil {
		t.Fatalf("getAddressBalanceRow addr-b after block2: %v", err)
	}
	if rowB.BalanceSatoshi != 30 || rowB.UTXOCount != 1 || !rowB.Tracked {
		t.Fatalf("unexpected addr-b cached row after block2: %+v", rowB)
	}
}

func TestIndexBlockRecoversBrokenBalanceIndexAndKeepsReady(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}

	block1 := &Block{
		Height:    100,
		BlockHash: "block-100",
		Transactions: []*Transaction{
			{
				ID:     "tx-100",
				Inputs: []*Input{},
				Outputs: []*Output{
					{Address: "addr-a", Amount: "50"},
				},
			},
		},
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

	if err := idx.putAddressBalance("addr-a", 0, 1); err != nil {
		t.Fatalf("corrupt addr-a balance row: %v", err)
	}

	block2 := &Block{
		Height:    101,
		BlockHash: "block-101",
		Transactions: []*Transaction{
			{
				ID: "tx-101",
				Inputs: []*Input{
					{TxPoint: "tx-100:0"},
				},
				Outputs: []*Output{
					{Address: "addr-b", Amount: "30"},
					{Address: "addr-a", Amount: "20"},
				},
			},
		},
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

	if !idx.isBalanceIndexReady() {
		t.Fatal("expected balance index to stay ready after touched-address recovery")
	}

	balanceA, err := idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a after block2: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 20 {
		t.Fatalf("expected addr-a confirmed balance 20 via history fallback, got %d", balanceA.ConfirmedBalanceSatoshi)
	}

	balanceB, err := idx.GetBalance("addr-b", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-b after block2: %v", err)
	}
	if balanceB.ConfirmedBalanceSatoshi != 30 {
		t.Fatalf("expected addr-b confirmed balance 30 via history fallback, got %d", balanceB.ConfirmedBalanceSatoshi)
	}

	list, total, err := idx.GetRichList(1, 2, 0)
	if err != nil {
		t.Fatalf("GetRichList after recovery: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected rich-list total 2 after recovery, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected rich-list length 2 after recovery, got %d", len(list))
	}
	if list[0].Address != "addr-b" {
		t.Fatalf("expected first rich-list address addr-b after recovery, got %s", list[0].Address)
	}
	if list[1].Address != "addr-a" {
		t.Fatalf("expected second rich-list address addr-a after recovery, got %s", list[1].Address)
	}
}

func TestBootstrapConfirmedBalanceIndexesFromHistory(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	if err := idx.addressStore.Set([]byte("addr-a"), []byte("tx-1@0@50@1700000000,tx-2@0@20@1700000001")); err != nil {
		t.Fatalf("seed income addr-a: %v", err)
	}
	if err := idx.spendStore.Set([]byte("addr-a"), []byte("tx-1:0@1700000010@spend-1")); err != nil {
		t.Fatalf("seed spend addr-a: %v", err)
	}
	if err := idx.addressStore.Set([]byte("addr-b"), []byte("tx-3@0@30@1700000002")); err != nil {
		t.Fatalf("seed income addr-b: %v", err)
	}

	if err := idx.metaStore.Set([]byte("total_address_count"), []byte("2")); err != nil {
		t.Fatalf("seed total_address_count: %v", err)
	}

	if err := idx.BootstrapConfirmedBalanceIndexes(); err != nil {
		t.Fatalf("BootstrapConfirmedBalanceIndexes: %v", err)
	}

	balanceA, err := idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a after bootstrap: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 20 {
		t.Fatalf("expected addr-a confirmed balance 20 after bootstrap, got %d", balanceA.ConfirmedBalanceSatoshi)
	}
	if balanceA.UTXOCount != 1 {
		t.Fatalf("expected addr-a utxo count 1 after bootstrap, got %d", balanceA.UTXOCount)
	}

	balanceB, err := idx.GetBalance("addr-b", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-b after bootstrap: %v", err)
	}
	if balanceB.ConfirmedBalanceSatoshi != 30 {
		t.Fatalf("expected addr-b confirmed balance 30 after bootstrap, got %d", balanceB.ConfirmedBalanceSatoshi)
	}
	if balanceB.UTXOCount != 1 {
		t.Fatalf("expected addr-b utxo count 1 after bootstrap, got %d", balanceB.UTXOCount)
	}

	list, total, err := idx.GetRichList(1, 2, 0)
	if err != nil {
		t.Fatalf("GetRichList after bootstrap: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected rich-list total 2 after bootstrap, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected rich-list length 2 after bootstrap, got %d", len(list))
	}
	if list[0].Address != "addr-b" {
		t.Fatalf("expected first rich-list address addr-b after bootstrap, got %s", list[0].Address)
	}
	if list[1].Address != "addr-a" {
		t.Fatalf("expected second rich-list address addr-a after bootstrap, got %s", list[1].Address)
	}
}

func TestDoDeleteRollsBackBalanceIndexes(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)
	if err := idx.setBalanceIndexReady(true); err != nil {
		t.Fatalf("setBalanceIndexReady: %v", err)
	}

	block1 := &Block{
		Height:    100,
		BlockHash: "block-100",
		Transactions: []*Transaction{
			{
				ID:     "tx-100",
				Inputs: []*Input{},
				Outputs: []*Output{
					{Address: "addr-a", Amount: "50"},
				},
			},
		},
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

	block2 := &Block{
		Height:    101,
		BlockHash: "block-101",
		Transactions: []*Transaction{
			{
				ID: "tx-101",
				Inputs: []*Input{
					{TxPoint: "tx-100:0"},
				},
				Outputs: []*Output{
					{Address: "addr-b", Amount: "30"},
					{Address: "addr-a", Amount: "20"},
				},
			},
		},
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

	rollbackBlock2 := &Block{
		Height:    block2.Height,
		BlockHash: block2.BlockHash,
		UtxoData: map[string][]string{
			"tx-101": {"addr-b@30@1700000100,addr-a@20@1700000100"},
		},
		IncomeData: map[string][]string{
			"addr-b": {"tx-101@0@30@1700000100"},
			"addr-a": {"tx-101@1@20@1700000100"},
		},
		SpendData: map[string][]string{
			"addr-a": {"tx-100:0@1700000100@tx-101"},
		},
	}

	if err := idx.DoDelete(rollbackBlock2); err != nil {
		t.Fatalf("DoDelete block2: %v", err)
	}

	balanceA, err := idx.GetBalance("addr-a", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-a after rollback: %v", err)
	}
	if balanceA.ConfirmedBalanceSatoshi != 50 {
		t.Fatalf("expected addr-a confirmed balance 50 after rollback, got %d", balanceA.ConfirmedBalanceSatoshi)
	}
	if balanceA.UTXOCount != 1 {
		t.Fatalf("expected addr-a utxo count 1 after rollback, got %d", balanceA.UTXOCount)
	}

	balanceB, err := idx.GetBalance("addr-b", 0)
	if err != nil {
		t.Fatalf("GetBalance addr-b after rollback: %v", err)
	}
	if balanceB.ConfirmedBalanceSatoshi != 0 {
		t.Fatalf("expected addr-b confirmed balance 0 after rollback, got %d", balanceB.ConfirmedBalanceSatoshi)
	}
	if balanceB.UTXOCount != 0 {
		t.Fatalf("expected addr-b utxo count 0 after rollback, got %d", balanceB.UTXOCount)
	}
}

func TestDoDeleteMaintainsMaterializedBalanceRowsWhenIndexNotReady(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	block1 := &Block{
		Height:    100,
		BlockHash: "block-100",
		Transactions: []*Transaction{
			{
				ID:     "tx-100",
				Inputs: []*Input{},
				Outputs: []*Output{
					{Address: "addr-a", Amount: "50"},
				},
			},
		},
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

	block2 := &Block{
		Height:    101,
		BlockHash: "block-101",
		Transactions: []*Transaction{
			{
				ID: "tx-101",
				Inputs: []*Input{
					{TxPoint: "tx-100:0"},
				},
				Outputs: []*Output{
					{Address: "addr-b", Amount: "30"},
					{Address: "addr-a", Amount: "20"},
				},
			},
		},
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

	rollbackBlock2 := &Block{
		Height:    block2.Height,
		BlockHash: block2.BlockHash,
		UtxoData: map[string][]string{
			"tx-101": {"addr-b@30@1700000100,addr-a@20@1700000100"},
		},
		IncomeData: map[string][]string{
			"addr-b": {"tx-101@0@30@1700000100"},
			"addr-a": {"tx-101@1@20@1700000100"},
		},
		SpendData: map[string][]string{
			"addr-a": {"tx-100:0@1700000100@tx-101"},
		},
	}

	if err := idx.DoDelete(rollbackBlock2); err != nil {
		t.Fatalf("DoDelete block2: %v", err)
	}

	rowA, err := idx.getAddressBalanceRow("addr-a")
	if err != nil {
		t.Fatalf("getAddressBalanceRow addr-a after rollback: %v", err)
	}
	if rowA.BalanceSatoshi != 50 || rowA.UTXOCount != 1 || !rowA.Tracked {
		t.Fatalf("unexpected addr-a cached row after rollback: %+v", rowA)
	}

	_, err = idx.getAddressBalanceRow("addr-b")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected addr-b cached row to be deleted after rollback, got %v", err)
	}
}

func TestStartBalanceIndexBootstrapIfNeededRunsAsync(t *testing.T) {
	idx := newBalanceIndexTestIndexer(t)

	started := make(chan struct{})
	release := make(chan struct{})
	idx.bootstrapBalanceIndexFn = func() error {
		close(started)
		<-release
		return nil
	}

	doneCh := idx.StartBalanceIndexBootstrapIfNeeded()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected bootstrap goroutine to start")
	}

	select {
	case err := <-doneCh:
		t.Fatalf("expected bootstrap to still be running, got err=%v", err)
	default:
	}

	close(release)

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("expected nil bootstrap error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected bootstrap goroutine to finish")
	}
}
