package storage

import "testing"

func TestGetUtxoByKeyParsesTimestampedMempoolKeys(t *testing.T) {
	db, err := NewSimpleDB(t.TempDir())
	if err != nil {
		t.Fatalf("NewSimpleDB: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("SimpleDB.Close: %v", err)
		}
	})

	if err := db.AddMempolRecord("addr-1_tx-1:0_111", []byte("5000")); err != nil {
		t.Fatalf("AddMempolRecord tx-1:0: %v", err)
	}
	if err := db.AddMempolRecord("addr-1_tx-1:2_111", []byte("7000")); err != nil {
		t.Fatalf("AddMempolRecord tx-1:2: %v", err)
	}

	utxos, err := db.GetUtxoByKey("addr-1")
	if err != nil {
		t.Fatalf("GetUtxoByKey: %v", err)
	}
	if len(utxos) != 2 {
		t.Fatalf("expected 2 utxos, got %d", len(utxos))
	}
	if utxos[0].Address != "addr-1" || utxos[0].TxID == "" {
		t.Fatalf("expected parsed address and txid, got %+v", utxos[0])
	}
}
