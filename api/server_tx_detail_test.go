package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metaid/utxo_indexer/blockchain"
)

func TestCoreTxDetailRoute(t *testing.T) {
	txid := strings.Repeat("a", 64)
	server := NewServer(nil, nil, make(chan struct{}))
	server.getTxDetailFn = func(gotTxID string) (*blockchain.TxDetail, error) {
		if gotTxID != txid {
			t.Fatalf("txid = %s, want %s", gotTxID, txid)
		}
		return &blockchain.TxDetail{TxID: txid, Confirmed: true, Confirmations: 2}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/tx/"+txid, nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"confirmations":2`) {
		t.Fatalf("response missing confirmations: %s", rec.Body.String())
	}
}

func TestCoreTxDetailRouteNotFound(t *testing.T) {
	txid := strings.Repeat("a", 64)
	server := NewServer(nil, nil, make(chan struct{}))
	server.getTxDetailFn = func(string) (*blockchain.TxDetail, error) {
		return nil, blockchain.ErrTransactionNotFound
	}

	req := httptest.NewRequest(http.MethodGet, "/tx/"+txid, nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCoreTxDetailRouteRequiresValidTxID(t *testing.T) {
	server := NewServer(nil, nil, make(chan struct{}))
	req := httptest.NewRequest(http.MethodGet, "/tx/not-a-txid", nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCoreTxDetailRouteMapsGenericError(t *testing.T) {
	txid := strings.Repeat("a", 64)
	server := NewServer(nil, nil, make(chan struct{}))
	server.getTxDetailFn = func(string) (*blockchain.TxDetail, error) {
		return nil, errors.New("rpc unavailable")
	}

	req := httptest.NewRequest(http.MethodGet, "/tx/"+txid, nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
}
