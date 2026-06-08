package indexer

import (
	"strings"
	"testing"

	"github.com/metaid/utxo_indexer/storage"
)

func TestTxIDAliasStorePersistsPublicToNodeMapping(t *testing.T) {
	metaStore, err := storage.NewMetaStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	t.Cleanup(func() { _ = metaStore.Close() })

	publicTxID := strings.Repeat("a", 64)
	nodeTxID := strings.Repeat("b", 64)
	idx := &UTXOIndexer{metaStore: metaStore}

	if err := idx.storeTxIDAliases([]*Transaction{
		{ID: publicTxID, NodeID: nodeTxID},
		{ID: strings.Repeat("c", 64), NodeID: strings.Repeat("c", 64)},
		{ID: strings.Repeat("d", 64)},
	}); err != nil {
		t.Fatalf("storeTxIDAliases: %v", err)
	}

	got, ok, err := idx.ResolveTxIDAlias(publicTxID)
	if err != nil {
		t.Fatalf("ResolveTxIDAlias: %v", err)
	}
	if !ok || got != nodeTxID {
		t.Fatalf("ResolveTxIDAlias = (%s, %v), want (%s, true)", got, ok, nodeTxID)
	}

	if _, ok, err := idx.ResolveTxIDAlias(strings.Repeat("c", 64)); err != nil || ok {
		t.Fatalf("same-id alias lookup = ok:%v err:%v, want not found", ok, err)
	}
}

func TestTxIDAliasResolverHandlesMissingMetaStore(t *testing.T) {
	idx := &UTXOIndexer{}
	if err := idx.storeTxIDAliases([]*Transaction{{ID: strings.Repeat("a", 64), NodeID: strings.Repeat("b", 64)}}); err != nil {
		t.Fatalf("storeTxIDAliases without meta store: %v", err)
	}
	if _, ok, err := idx.ResolveTxIDAlias(strings.Repeat("a", 64)); ok || err != nil {
		t.Fatalf("ResolveTxIDAlias without meta store = ok:%v err:%v, want no alias", ok, err)
	}
}

func TestTxIDAliasBackfillProgressPersists(t *testing.T) {
	dataDir := t.TempDir()
	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}

	idx := &UTXOIndexer{metaStore: metaStore}
	if _, ok, err := idx.GetTxIDAliasBackfillProgress(); err != nil || ok {
		t.Fatalf("initial progress = ok:%v err:%v, want missing without error", ok, err)
	}
	if err := idx.SetTxIDAliasBackfillProgress(42); err != nil {
		t.Fatalf("SetTxIDAliasBackfillProgress: %v", err)
	}
	if err := idx.MarkTxIDAliasBackfillComplete(42); err != nil {
		t.Fatalf("MarkTxIDAliasBackfillComplete: %v", err)
	}
	if err := metaStore.Close(); err != nil {
		t.Fatalf("Close first metaStore: %v", err)
	}

	metaStore, err = storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore reopen: %v", err)
	}
	t.Cleanup(func() { _ = metaStore.Close() })

	idx = &UTXOIndexer{metaStore: metaStore}
	progress, ok, err := idx.GetTxIDAliasBackfillProgress()
	if err != nil {
		t.Fatalf("GetTxIDAliasBackfillProgress: %v", err)
	}
	if !ok || progress != 42 {
		t.Fatalf("progress = (%d, %v), want (42, true)", progress, ok)
	}
	completeHeight, ok, err := idx.GetTxIDAliasBackfillCompleteHeight()
	if err != nil {
		t.Fatalf("GetTxIDAliasBackfillCompleteHeight: %v", err)
	}
	if !ok || completeHeight != 42 {
		t.Fatalf("complete height = (%d, %v), want (42, true)", completeHeight, ok)
	}
}
