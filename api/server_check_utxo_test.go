package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/storage"
)

type checkUTXOValidatorStub struct {
	unspent map[string]bool
}

func (v *checkUTXOValidatorStub) IsUnspent(txID string, index uint32) (bool, error) {
	return v.unspent[txID+":"+strconv.FormatUint(uint64(index), 10)], nil
}

func newCheckUTXOTestServer(t *testing.T, validator indexer.ConfirmedUTXOValidator) (*Server, *storage.PebbleStore, *storage.PebbleStore) {
	t.Helper()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{MemUTXOMaxCount: 1}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	dataDir := t.TempDir()
	params := config.IndexerParams{}

	metaStore, err := storage.NewMetaStore(dataDir)
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	t.Cleanup(func() {
		if err := metaStore.Close(); err != nil {
			t.Fatalf("MetaStore.Close: %v", err)
		}
	})

	addressStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeIncome, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore income: %v", err)
	}
	t.Cleanup(func() {
		if err := addressStore.Close(); err != nil {
			t.Fatalf("addressStore.Close: %v", err)
		}
	})

	spendStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeSpend, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore spend: %v", err)
	}
	t.Cleanup(func() {
		if err := spendStore.Close(); err != nil {
			t.Fatalf("spendStore.Close: %v", err)
		}
	})

	utxoStore, err := storage.NewPebbleStore(params, dataDir, storage.StoreTypeUTXO, 1)
	if err != nil {
		t.Fatalf("NewPebbleStore utxo: %v", err)
	}
	t.Cleanup(func() {
		if err := utxoStore.Close(); err != nil {
			t.Fatalf("utxoStore.Close: %v", err)
		}
	})

	idx := indexer.NewUTXOIndexer(params, utxoStore, addressStore, metaStore, spendStore)
	idx.SetConfirmedUTXOValidator(validator, true, 2)
	return NewServer(idx, metaStore, make(chan struct{})), addressStore, utxoStore
}

func TestCheckUtxoMarksConfirmedOutpointSpentWhenNodeUTXOSetMissing(t *testing.T) {
	const txID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	server, addressStore, utxoStore := newCheckUTXOTestServer(t, &checkUTXOValidatorStub{
		unspent: map[string]bool{},
	})
	if err := utxoStore.Set([]byte(txID), []byte("addr-check@5000@1700000000")); err != nil {
		t.Fatalf("seed utxoStore: %v", err)
	}
	if err := addressStore.Set([]byte("addr-check"), []byte(txID+"@0@5000@1700000000")); err != nil {
		t.Fatalf("seed addressStore: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/utxo/check", bytes.NewReader([]byte(`{"outPoints":["`+txID+`:0"]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data map[string]struct {
			SpendStatus string `json:"spendStatus"`
			SpendInfo   struct {
				Where string `json:"where"`
			} `json:"spendInfo"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	got := resp.Data[txID+":0"]
	if got.SpendStatus != "spend" {
		t.Fatalf("expected stale confirmed outpoint to be marked spend, got %q", got.SpendStatus)
	}
	if got.SpendInfo.Where != "block" {
		t.Fatalf("expected spendInfo.where block, got %q", got.SpendInfo.Where)
	}
}
