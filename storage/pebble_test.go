package storage

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

func TestNewPebbleStoreWithOptionsReadOnlyRejectsWrites(t *testing.T) {
	dataDir := t.TempDir()
	params := config.IndexerParams{}

	writableStore, err := NewPebbleStore(params, dataDir, StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("open writable store: %v", err)
	}
	if err := writableStore.Set([]byte("addr-a"), []byte("tx-1@0@10@1700000000")); err != nil {
		t.Fatalf("seed writable store: %v", err)
	}
	if err := writableStore.Close(); err != nil {
		t.Fatalf("close writable store: %v", err)
	}

	readOnlyStore, err := NewPebbleStoreWithOptions(params, dataDir, StoreTypeIncome, 1, PebbleOpenOptions{
		ReadOnly:       true,
		CacheSizeBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("open read-only store: %v", err)
	}
	defer readOnlyStore.Close()

	value, _, err := readOnlyStore.GetWithShard([]byte("addr-a"))
	if err != nil {
		t.Fatalf("read existing row from read-only store: %v", err)
	}
	if string(value) != "tx-1@0@10@1700000000" {
		t.Fatalf("unexpected read-only value: %q", string(value))
	}

	if err := readOnlyStore.Set([]byte("addr-b"), []byte("tx-2@0@20@1700000001")); err == nil {
		t.Fatal("expected read-only store to reject writes")
	}
}

func TestBatchCommitClosesAndResetsShardBatches(t *testing.T) {
	dataDir := t.TempDir()
	params := config.IndexerParams{}

	store, err := NewPebbleStore(params, dataDir, StoreTypeAddressBalance, 2)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	batch := store.NewBatch()
	if err := batch.Set([]byte("addr-a"), []byte("1")); err != nil {
		t.Fatalf("batch set addr-a: %v", err)
	}
	if err := batch.Set([]byte("addr-b"), []byte("2")); err != nil {
		t.Fatalf("batch set addr-b: %v", err)
	}

	allocated := 0
	for _, shardBatch := range batch.batches {
		if shardBatch != nil {
			allocated++
		}
	}
	if allocated == 0 {
		t.Fatal("expected at least one shard batch to be allocated before commit")
	}

	if err := batch.Commit(); err != nil {
		t.Fatalf("commit batch: %v", err)
	}

	for shardIdx, shardBatch := range batch.batches {
		if shardBatch != nil {
			t.Fatalf("expected shard batch %d to be released after commit", shardIdx)
		}
	}
}
