package storage

import (
	"strconv"
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

func TestQueryUTXODetailsReturnsOnlyRequestedOutpoints(t *testing.T) {
	store, err := NewPebbleStore(config.IndexerParams{}, t.TempDir(), StoreTypeUTXO, 2)
	if err != nil {
		t.Fatalf("NewPebbleStore: %v", err)
	}
	defer store.Close()

	if err := store.Put([]byte("tx1"), []byte(",addr0@100@1@0,addr1@200@1@1,addr2@300@1@2")); err != nil {
		t.Fatalf("Put tx1: %v", err)
	}
	if err := store.Put([]byte("tx2"), []byte(",addrA@400@1@0")); err != nil {
		t.Fatalf("Put tx2: %v", err)
	}

	outpoints := []string{"tx1:1", "tx2:0", "missing:0"}
	got, err := store.QueryUTXODetails(&outpoints)
	if err != nil {
		t.Fatalf("QueryUTXODetails: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got["tx1:1"].Address != "addr1" || got["tx1:1"].Amount != 200 {
		t.Fatalf("tx1:1 = %+v", got["tx1:1"])
	}
	if got["tx2:0"].Address != "addrA" || got["tx2:0"].Amount != 400 {
		t.Fatalf("tx2:0 = %+v", got["tx2:0"])
	}
}

func TestQueryUTXODetailsIgnoresInvalidOutpoints(t *testing.T) {
	store, err := NewPebbleStore(config.IndexerParams{}, t.TempDir(), StoreTypeUTXO, 2)
	if err != nil {
		t.Fatalf("NewPebbleStore: %v", err)
	}
	defer store.Close()

	empty := []string{}
	invalid := []string{"missing-colon", ":0", "tx1:"}
	for _, outpoints := range []*[]string{nil, &empty, &invalid} {
		got, err := store.QueryUTXODetails(outpoints)
		if err != nil {
			t.Fatalf("QueryUTXODetails(%v): %v", outpoints, err)
		}
		if len(got) != 0 {
			t.Fatalf("QueryUTXODetails(%v) len = %d, want 0: %+v", outpoints, len(got), got)
		}
	}
}

func TestQueryUTXODetailsReturnsMultipleOutputsFromSameTx(t *testing.T) {
	store, err := NewPebbleStore(config.IndexerParams{}, t.TempDir(), StoreTypeUTXO, 2)
	if err != nil {
		t.Fatalf("NewPebbleStore: %v", err)
	}
	defer store.Close()

	if err := store.Put([]byte("tx1"), []byte(",addr0@100@1@0,addr1@200@1@1,addr2@300@1@2")); err != nil {
		t.Fatalf("Put tx1: %v", err)
	}

	outpoints := []string{"tx1:0", "tx1:2"}
	got, err := store.QueryUTXODetails(&outpoints)
	if err != nil {
		t.Fatalf("QueryUTXODetails: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got["tx1:0"].Address != "addr0" || got["tx1:0"].Amount != 100 {
		t.Fatalf("tx1:0 = %+v", got["tx1:0"])
	}
	if got["tx1:2"].Address != "addr2" || got["tx1:2"].Amount != 300 {
		t.Fatalf("tx1:2 = %+v", got["tx1:2"])
	}
}

func BenchmarkQueryUTXODetailsRepeatedOutputs(b *testing.B) {
	store, err := NewPebbleStore(config.IndexerParams{}, b.TempDir(), StoreTypeUTXO, 2)
	if err != nil {
		b.Fatalf("NewPebbleStore: %v", err)
	}
	defer store.Close()

	value := ",addr0@100@1@0,addr1@200@1@1,addr2@300@1@2,addr3@400@1@3"
	for i := 0; i < 1000; i++ {
		key := []byte("tx" + strconv.Itoa(i))
		if err := store.Put(key, []byte(value)); err != nil {
			b.Fatalf("Put: %v", err)
		}
	}

	outpoints := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		outpoints = append(outpoints, "tx"+strconv.Itoa(i)+":2")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		got, err := store.QueryUTXODetails(&outpoints)
		if err != nil {
			b.Fatal(err)
		}
		if len(got) != 1000 {
			b.Fatalf("len(got) = %d", len(got))
		}
	}
}
