package storage

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

// Rows written by the mempool (3-field entries) used to be merged onto
// indexer rows (4-field entries); positional lookups then resolved to the
// wrong output and misattributed spends. Resolution must match explicit
// vouts whenever any entry carries one.
func TestResolveUtxoRowEntryMixedRowMatchesByVout(t *testing.T) {
	// Shape observed in production: 2 mempool-era entries (first + last vout)
	// followed by the confirmed entries for vouts 0..N. Positional lookup of
	// vout 1 would return the 1LFYPB prefix entry; vout matching must return
	// the real 1M1AY output.
	row := []byte(",1M1AY@100300@1780902663,1LFYPB@175808@1780902663,1M1AY@100300@1780902651@0,1M1AY@100300@1780902651@1,1LFYPB@175808@1780902651@2")

	got, ok := resolveUtxoRowEntry(row, 1)
	if !ok || got.address != "1M1AY" || got.amount != 100300 {
		t.Fatalf("vout 1 = %+v ok=%v, want 1M1AY@100300", got, ok)
	}
	got, ok = resolveUtxoRowEntry(row, 2)
	if !ok || got.address != "1LFYPB" || got.amount != 175808 {
		t.Fatalf("vout 2 = %+v ok=%v, want 1LFYPB@175808", got, ok)
	}
	if _, ok = resolveUtxoRowEntry(row, 3); ok {
		t.Fatal("vout 3 should be unresolved in a mixed row")
	}
}

func TestResolveUtxoRowEntryPureMempoolRowIsPositional(t *testing.T) {
	row := []byte("a0@100@1700,a1@200@1700,a2@300@1700")
	got, ok := resolveUtxoRowEntry(row, 2)
	if !ok || got.address != "a2" || got.amount != 300 {
		t.Fatalf("index 2 = %+v ok=%v, want a2@300", got, ok)
	}
	if _, ok = resolveUtxoRowEntry(row, 3); ok {
		t.Fatal("index 3 should be unresolved")
	}
}

func TestResolveUtxoRowEntryPureConfirmedRow(t *testing.T) {
	row := []byte("addr0@100@1@0,addr1@200@1@1,addr2@300@1@2")
	for i, want := range []struct {
		address string
		amount  int64
	}{{"addr0", 100}, {"addr1", 200}, {"addr2", 300}} {
		got, ok := resolveUtxoRowEntry(row, i)
		if !ok || got.address != want.address || got.amount != want.amount {
			t.Fatalf("vout %d = %+v ok=%v, want %+v", i, got, ok, want)
		}
	}
}

func TestExtractHelpersAgree(t *testing.T) {
	row := []byte("a0@100@1700,a1@200@1700@1")
	if got := extractAddressFromValue(row, "1"); got != "a1" {
		t.Fatalf("extractAddressFromValue = %q, want a1", got)
	}
	if got, err := getAddressByStrSafe("txid:1", row, 4); err != nil || got != "a1" {
		t.Fatalf("getAddressByStrSafe = %q err=%v, want a1", got, err)
	}
	detail, ok := extractUTXODetailFromValue(row, "1")
	if !ok || detail.Address != "a1" || detail.Amount != 200 {
		t.Fatalf("extractUTXODetailFromValue = %+v ok=%v", detail, ok)
	}
}

// BulkSetMapConcurrent must replace rows wholesale so re-indexing the same
// block stays idempotent (merge used to duplicate every entry).
func TestBulkSetMapConcurrentIsIdempotent(t *testing.T) {
	store, err := NewPebbleStore(config.IndexerParams{}, t.TempDir(), StoreTypeUTXO, 2)
	if err != nil {
		t.Fatalf("NewPebbleStore: %v", err)
	}
	defer store.Close()

	rows := map[string][]string{
		"tx1": {"addr0@100@1@0", "addr1@200@1@1"},
	}
	for i := 0; i < 3; i++ {
		if err := store.BulkSetMapConcurrent(&rows, 2); err != nil {
			t.Fatalf("BulkSetMapConcurrent round %d: %v", i, err)
		}
	}

	got, err := store.Get([]byte("tx1"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := ",addr0@100@1@0,addr1@200@1@1"
	if string(got) != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
	detail, ok := extractUTXODetailFromValue(got, "1")
	if !ok || detail.Address != "addr1" || detail.Amount != 200 {
		t.Fatalf("tx1:1 = %+v ok=%v", detail, ok)
	}
}
