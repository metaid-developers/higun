package mempool

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// Mempool rows live under the mem: prefix so confirmed rows written by the
// indexer are never shadowed; GetUtxoAddress resolves confirmed rows first
// (by vout) and falls back to the mempool row.
func TestGetUtxoAddressPrefersConfirmedRowAndMemFallback(t *testing.T) {
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{ZMQAddress: []string{"tcp://127.0.0.1:28333"}}
	defer func() { config.GlobalConfig = oldConfig }()

	utxoStore, err := storage.NewPebbleStore(config.IndexerParams{}, t.TempDir(), storage.StoreTypeUTXO, 2)
	if err != nil {
		t.Fatalf("NewPebbleStore: %v", err)
	}
	defer utxoStore.Close()

	m := NewMempoolManager(t.TempDir(), utxoStore, nil, []string{"tcp://127.0.0.1:28332"})
	defer m.Stop()

	// Mixed confirmed row (mempool-era prefix + vout-carrying entries).
	if err := utxoStore.Set([]byte("txC"), []byte(",1M1AY@100300@1700,1LFYPB@175808@1700,1M1AY@100300@1700@0,1LFYPB@175808@1700@1")); err != nil {
		t.Fatalf("seed confirmed row: %v", err)
	}
	// Mempool-only row.
	if err := utxoStore.Set([]byte(mempoolTxRowPrefix+"txM"), []byte("1M1AY@100300@1700,1LFYPB@175808@1700")); err != nil {
		t.Fatalf("seed mempool row: %v", err)
	}

	got, err := m.GetUtxoAddress("txC", 1)
	if err != nil || got != "1LFYPB" {
		t.Fatalf("confirmed vout 1 = %q err=%v, want 1LFYPB", got, err)
	}
	got, err = m.GetUtxoAddress("txM", 0)
	if err != nil || got != "1M1AY" {
		t.Fatalf("mempool vout 0 = %q err=%v, want 1M1AY", got, err)
	}
	if _, err := m.GetUtxoAddress("txMissing", 0); err == nil {
		t.Fatal("expected error for missing tx")
	}
}
