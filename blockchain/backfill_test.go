package blockchain

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/storage"
)

func TestBackfillMVCTxIDAliasesResumesFromProgress(t *testing.T) {
	metaStore := newBackfillTestMetaStore(t)
	idx := newBackfillTestIndexer(t, metaStore)
	setBackfillTestLastIndexedHeight(t, metaStore, 3)
	if err := idx.SetTxIDAliasBackfillProgress(1); err != nil {
		t.Fatalf("SetTxIDAliasBackfillProgress: %v", err)
	}

	adapter := &recordingBackfillAdapter{blocks: map[int64]*indexer.Block{
		2: newBackfillAliasBlock(2, strings.Repeat("a", 64), strings.Repeat("b", 64)),
		3: newBackfillAliasBlock(3, strings.Repeat("c", 64), strings.Repeat("d", 64)),
	}}
	client := &Client{cfg: &config.Config{Chain: config.ChainMVC}, adapter: adapter}

	if err := client.BackfillMVCTxIDAliases(idx, nil); err != nil {
		t.Fatalf("BackfillMVCTxIDAliases: %v", err)
	}
	if !reflect.DeepEqual(adapter.calls, []int64{2, 3}) {
		t.Fatalf("GetBlock calls = %v, want [2 3]", adapter.calls)
	}
	progress, ok, err := idx.GetTxIDAliasBackfillProgress()
	if err != nil {
		t.Fatalf("GetTxIDAliasBackfillProgress: %v", err)
	}
	if !ok || progress != 3 {
		t.Fatalf("progress = (%d, %v), want (3, true)", progress, ok)
	}
	completeHeight, ok, err := idx.GetTxIDAliasBackfillCompleteHeight()
	if err != nil {
		t.Fatalf("GetTxIDAliasBackfillCompleteHeight: %v", err)
	}
	if !ok || completeHeight != 3 {
		t.Fatalf("complete height = (%d, %v), want (3, true)", completeHeight, ok)
	}
}

func TestBackfillMVCTxIDAliasesSkipsNonMVCAndMissingAdapter(t *testing.T) {
	t.Run("btc chain", func(t *testing.T) {
		metaStore := newBackfillTestMetaStore(t)
		idx := newBackfillTestIndexer(t, metaStore)
		setBackfillTestLastIndexedHeight(t, metaStore, 2)
		adapter := &recordingBackfillAdapter{blocks: map[int64]*indexer.Block{
			0: newBackfillAliasBlock(0, strings.Repeat("a", 64), strings.Repeat("b", 64)),
			1: newBackfillAliasBlock(1, strings.Repeat("c", 64), strings.Repeat("d", 64)),
			2: newBackfillAliasBlock(2, strings.Repeat("e", 64), strings.Repeat("f", 64)),
		}}
		client := &Client{cfg: &config.Config{Chain: config.ChainBTC}, adapter: adapter}

		if err := client.BackfillMVCTxIDAliases(idx, nil); err != nil {
			t.Fatalf("BackfillMVCTxIDAliases: %v", err)
		}
		if len(adapter.calls) != 0 {
			t.Fatalf("GetBlock calls = %v, want none for BTC", adapter.calls)
		}
		if _, ok, err := idx.GetTxIDAliasBackfillProgress(); err != nil || ok {
			t.Fatalf("progress after BTC backfill = ok:%v err:%v, want missing without error", ok, err)
		}
	})

	t.Run("mvc without adapter", func(t *testing.T) {
		metaStore := newBackfillTestMetaStore(t)
		idx := newBackfillTestIndexer(t, metaStore)
		setBackfillTestLastIndexedHeight(t, metaStore, 2)
		client := &Client{cfg: &config.Config{Chain: config.ChainMVC}}

		if err := client.BackfillMVCTxIDAliases(idx, nil); err != nil {
			t.Fatalf("BackfillMVCTxIDAliases: %v", err)
		}
		if _, ok, err := idx.GetTxIDAliasBackfillProgress(); err != nil || ok {
			t.Fatalf("progress after missing-adapter backfill = ok:%v err:%v, want missing without error", ok, err)
		}
	})
}

func TestBackfillMVCTxIDAliasesStoresAliasForTxDetail(t *testing.T) {
	publicTxID := strings.Repeat("a", 64)
	nodeTxID := strings.Repeat("b", 64)

	metaStore := newBackfillTestMetaStore(t)
	idx := newBackfillTestIndexer(t, metaStore)
	setBackfillTestLastIndexedHeight(t, metaStore, 0)
	adapter := &recordingBackfillAdapter{blocks: map[int64]*indexer.Block{
		0: newBackfillAliasBlock(0, publicTxID, nodeTxID),
	}}
	backfillClient := &Client{cfg: &config.Config{Chain: config.ChainMVC}, adapter: adapter}

	if err := backfillClient.BackfillMVCTxIDAliases(idx, nil); err != nil {
		t.Fatalf("BackfillMVCTxIDAliases: %v", err)
	}
	resolved, ok, err := idx.ResolveTxIDAlias(publicTxID)
	if err != nil {
		t.Fatalf("ResolveTxIDAlias: %v", err)
	}
	if !ok || resolved != nodeTxID {
		t.Fatalf("ResolveTxIDAlias = (%s, %v), want (%s, true)", resolved, ok, nodeTxID)
	}

	server := newTxDetailRPCServer(t, func(txid string) (any, *testRPCError) {
		if txid == publicTxID {
			return nil, &testRPCError{Code: -5, Message: "No such mempool or blockchain transaction"}
		}
		if txid != nodeTxID {
			t.Fatalf("rpc txid = %s, want %s", txid, nodeTxID)
		}
		return map[string]any{
			"txid":          nodeTxID,
			"confirmations": 1,
			"vin":           []any{},
			"vout":          []any{},
		}, nil
	})
	defer server.Close()

	detailClient := newTestTxDetailClient(t, server.URL, config.ChainMVC)
	detailClient.SetTxIDAliasResolver(idx)
	got, err := detailClient.GetTransactionDetail(publicTxID)
	if err != nil {
		t.Fatalf("GetTransactionDetail: %v", err)
	}
	if got.TxID != publicTxID || !got.Confirmed {
		t.Fatalf("detail = %+v, want requested public txid and confirmed status", got)
	}
}

type recordingBackfillAdapter struct {
	blocks map[int64]*indexer.Block
	calls  []int64
}

func (a *recordingBackfillAdapter) Connect() error { return nil }
func (a *recordingBackfillAdapter) Shutdown()      {}
func (a *recordingBackfillAdapter) GetChainName() string {
	return config.ChainMVC
}
func (a *recordingBackfillAdapter) GetChainParams() *chaincfg.Params {
	return &chaincfg.MainNetParams
}
func (a *recordingBackfillAdapter) GetBlockCount() (int, error) {
	return len(a.blocks) - 1, nil
}
func (a *recordingBackfillAdapter) GetBlockHash(height int64) (string, error) {
	return fmt.Sprintf("%064d", height), nil
}
func (a *recordingBackfillAdapter) GetBlock(height int64) (*indexer.Block, error) {
	a.calls = append(a.calls, height)
	block, ok := a.blocks[height]
	if !ok {
		return nil, fmt.Errorf("missing block %d", height)
	}
	return block, nil
}
func (a *recordingBackfillAdapter) GetTransaction(string) (*indexer.Transaction, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *recordingBackfillAdapter) GetRawMempool() ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *recordingBackfillAdapter) FindReorgHeight() (int, int) { return 0, 0 }

func newBackfillAliasBlock(height int, publicTxID, nodeTxID string) *indexer.Block {
	return &indexer.Block{
		Height:       height,
		BlockHash:    fmt.Sprintf("%064d", height),
		Transactions: []*indexer.Transaction{{ID: publicTxID, NodeID: nodeTxID}},
	}
}

func newBackfillTestMetaStore(t *testing.T) *storage.MetaStore {
	t.Helper()
	metaStore, err := storage.NewMetaStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	t.Cleanup(func() { _ = metaStore.Close() })
	return metaStore
}

func newBackfillTestIndexer(t *testing.T, metaStore *storage.MetaStore) *indexer.UTXOIndexer {
	t.Helper()
	prev := config.GlobalConfig
	config.GlobalConfig = &config.Config{MemUTXOMaxCount: 1}
	t.Cleanup(func() { config.GlobalConfig = prev })
	return indexer.NewUTXOIndexer(config.IndexerParams{}, nil, nil, metaStore, nil)
}

func setBackfillTestLastIndexedHeight(t *testing.T, metaStore *storage.MetaStore, height int) {
	t.Helper()
	if err := metaStore.Set([]byte("last_indexed_height"), []byte(fmt.Sprintf("%d", height))); err != nil {
		t.Fatalf("set last_indexed_height: %v", err)
	}
}
