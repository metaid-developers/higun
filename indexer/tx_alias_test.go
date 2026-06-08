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
