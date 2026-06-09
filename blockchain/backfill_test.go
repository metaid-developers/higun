package blockchain

import (
	"errors"
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

func TestBackfillMVCTxIDAliasesHonorsConfiguredStartHeight(t *testing.T) {
	metaStore := newBackfillTestMetaStore(t)
	idx := newBackfillTestIndexer(t, metaStore)
	setBackfillTestLastIndexedHeight(t, metaStore, 150001)
	if err := idx.SetTxIDAliasBackfillProgress(125078); err != nil {
		t.Fatalf("SetTxIDAliasBackfillProgress: %v", err)
	}

	adapter := &recordingBackfillAdapter{blocks: map[int64]*indexer.Block{
		150000: newBackfillAliasBlock(150000, strings.Repeat("a", 64), strings.Repeat("b", 64)),
		150001: newBackfillAliasBlock(150001, strings.Repeat("c", 64), strings.Repeat("d", 64)),
	}}
	client := &Client{cfg: &config.Config{
		Chain:                           config.ChainMVC,
		MVCTxIDAliasBackfillStartHeight: 150000,
	}, adapter: adapter}

	if err := client.BackfillMVCTxIDAliases(idx, nil); err != nil {
		t.Fatalf("BackfillMVCTxIDAliases: %v", err)
	}
	if !reflect.DeepEqual(adapter.calls, []int64{150000, 150001}) {
		t.Fatalf("GetBlock calls = %v, want [150000 150001]", adapter.calls)
	}
	progress, ok, err := idx.GetTxIDAliasBackfillProgress()
	if err != nil {
		t.Fatalf("GetTxIDAliasBackfillProgress: %v", err)
	}
	if !ok || progress != 150001 {
		t.Fatalf("progress = (%d, %v), want (150001, true)", progress, ok)
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

func TestBackfillMVCTxIDAliasesUsesStreamingAdapterWithoutLoadingWholeBlock(t *testing.T) {
	metaStore := newBackfillTestMetaStore(t)
	idx := newBackfillTestIndexer(t, metaStore)
	setBackfillTestLastIndexedHeight(t, metaStore, 1)

	firstPublicTxID := strings.Repeat("a", 64)
	firstNodeTxID := strings.Repeat("b", 64)
	secondPublicTxID := strings.Repeat("c", 64)
	secondNodeTxID := strings.Repeat("d", 64)
	adapter := &streamingBackfillAdapter{
		recordingBackfillAdapter: recordingBackfillAdapter{blocks: map[int64]*indexer.Block{}},
		batches: map[int64][][]*indexer.Transaction{
			0: {
				{{ID: firstPublicTxID, NodeID: firstNodeTxID}},
			},
			1: {
				{{ID: secondPublicTxID, NodeID: secondNodeTxID}},
			},
		},
	}
	client := &Client{cfg: &config.Config{Chain: config.ChainMVC}, adapter: adapter}

	if err := client.BackfillMVCTxIDAliases(idx, nil); err != nil {
		t.Fatalf("BackfillMVCTxIDAliases: %v", err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("GetBlock calls = %v, want no whole-block loads", adapter.calls)
	}
	if !reflect.DeepEqual(adapter.streamCalls, []int64{0, 1}) {
		t.Fatalf("stream calls = %v, want [0 1]", adapter.streamCalls)
	}
	resolved, ok, err := idx.ResolveTxIDAlias(secondPublicTxID)
	if err != nil {
		t.Fatalf("ResolveTxIDAlias: %v", err)
	}
	if !ok || resolved != secondNodeTxID {
		t.Fatalf("ResolveTxIDAlias = (%s, %v), want (%s, true)", resolved, ok, secondNodeTxID)
	}
}

func TestBackfillMVCTxIDAliasesResumesStreamingBlockFromOffset(t *testing.T) {
	metaStore := newBackfillTestMetaStore(t)
	idx := newBackfillTestIndexer(t, metaStore)
	setBackfillTestLastIndexedHeight(t, metaStore, 125079)
	if err := idx.SetTxIDAliasBackfillProgress(125078); err != nil {
		t.Fatalf("SetTxIDAliasBackfillProgress: %v", err)
	}
	if err := idx.SetTxIDAliasBackfillOffset(125079, 3000); err != nil {
		t.Fatalf("SetTxIDAliasBackfillOffset: %v", err)
	}

	publicTxID := strings.Repeat("e", 64)
	nodeTxID := strings.Repeat("f", 64)
	adapter := &streamingBackfillAdapter{
		recordingBackfillAdapter: recordingBackfillAdapter{blocks: map[int64]*indexer.Block{}},
		batches: map[int64][][]*indexer.Transaction{
			125079: {
				{{ID: publicTxID, NodeID: nodeTxID}},
			},
		},
	}
	client := &Client{cfg: &config.Config{Chain: config.ChainMVC}, adapter: adapter}

	if err := client.BackfillMVCTxIDAliases(idx, nil); err != nil {
		t.Fatalf("BackfillMVCTxIDAliases: %v", err)
	}
	if got := adapter.startOffsets[125079]; got != 3000 {
		t.Fatalf("streaming start offset = %d, want 3000", got)
	}
	offset, ok, err := idx.GetTxIDAliasBackfillOffset(125079)
	if err != nil {
		t.Fatalf("GetTxIDAliasBackfillOffset: %v", err)
	}
	if !ok || offset != 3001 {
		t.Fatalf("offset = (%d, %v), want (3001, true)", offset, ok)
	}
	progress, ok, err := idx.GetTxIDAliasBackfillProgress()
	if err != nil {
		t.Fatalf("GetTxIDAliasBackfillProgress: %v", err)
	}
	if !ok || progress != 125079 {
		t.Fatalf("progress = (%d, %v), want (125079, true)", progress, ok)
	}
}

func TestTxIDAliasBackfillRetryRetriesTransientFailure(t *testing.T) {
	attempts := 0
	err := retryTxIDAliasBackfillOperation(3, 0, nil, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary rpc failure")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryTxIDAliasBackfillOperation: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestTxIDAliasBackfillRetryStopsBeforeNextAttempt(t *testing.T) {
	stopCh := make(chan struct{})
	close(stopCh)
	attempts := 0
	err := retryTxIDAliasBackfillOperation(3, 0, stopCh, func() error {
		attempts++
		return errors.New("temporary rpc failure")
	})
	if !errors.Is(err, errTxIDAliasBackfillStopped) {
		t.Fatalf("retryTxIDAliasBackfillOperation error = %v, want stopped", err)
	}
	if attempts != 0 {
		t.Fatalf("attempts = %d, want 0", attempts)
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

type streamingBackfillAdapter struct {
	recordingBackfillAdapter
	batches      map[int64][][]*indexer.Transaction
	streamCalls  []int64
	startOffsets map[int64]int
}

func (a *streamingBackfillAdapter) BackfillTxIDAliases(height int64, startOffset int, store func([]*indexer.Transaction) error, markOffset func(int) error, stopCh <-chan struct{}) error {
	if a.startOffsets == nil {
		a.startOffsets = make(map[int64]int)
	}
	a.streamCalls = append(a.streamCalls, height)
	a.startOffsets[height] = startOffset
	nextOffset := startOffset
	for _, batch := range a.batches[height] {
		if err := store(batch); err != nil {
			return err
		}
		nextOffset += len(batch)
		if err := markOffset(nextOffset); err != nil {
			return err
		}
	}
	return nil
}

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
