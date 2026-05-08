package indexer

import (
	"errors"
	"testing"

	"github.com/metaid/utxo_indexer/storage"
)

func newTestMetaStore(t *testing.T) *storage.MetaStore {
	t.Helper()

	metaStore, err := storage.NewMetaStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	t.Cleanup(func() {
		if err := metaStore.Close(); err != nil {
			t.Fatalf("MetaStore.Close: %v", err)
		}
	})
	return metaStore
}

func TestGetRichListReturnsCacheNotReadyWhenCacheMissing(t *testing.T) {
	metaStore := newTestMetaStore(t)
	if err := metaStore.Set([]byte("total_address_count"), []byte("12")); err != nil {
		t.Fatalf("seed total_address_count: %v", err)
	}

	idx := &UTXOIndexer{metaStore: metaStore}

	_, _, err := idx.GetRichList(1, 10, 0)
	if !errors.Is(err, ErrRichListCacheNotReady) {
		t.Fatalf("expected ErrRichListCacheNotReady, got %v", err)
	}
}

func TestGetRichListReturnsCacheNotReadyWhenCachedListIsEmpty(t *testing.T) {
	metaStore := newTestMetaStore(t)
	if err := metaStore.Set([]byte("total_address_count"), []byte("12")); err != nil {
		t.Fatalf("seed total_address_count: %v", err)
	}
	if err := metaStore.Set([]byte(richListDBKey), []byte("[]")); err != nil {
		t.Fatalf("seed rich_list_cache: %v", err)
	}

	idx := &UTXOIndexer{metaStore: metaStore}

	_, _, err := idx.GetRichList(1, 10, 0)
	if !errors.Is(err, ErrRichListCacheNotReady) {
		t.Fatalf("expected ErrRichListCacheNotReady, got %v", err)
	}
}
