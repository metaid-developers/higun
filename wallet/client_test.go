package wallet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCoreClientFetchBalance(t *testing.T) {
	var gotPath string
	var gotAddress string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAddress = r.URL.Query().Get("address")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"confirmed_balance_satoshi": uint64(135758),
			"balance_satoshi":           uint64(135765),
			"confirmed_utxo_count":      uint64(1040),
			"mempool_income_satoshi":    uint64(10),
			"mempool_spend_satoshi":     uint64(3),
			"mempool_utxo_count":        uint64(2),
			"unsafe_fee_satoshi":        uint64(134862),
		})
	}))
	defer server.Close()

	client, err := NewCoreClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.FetchBalance(context.Background(), ChainBTC, "addr-btc")
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}
	if gotPath != "/balance" {
		t.Fatalf("path = %s, want /balance", gotPath)
	}
	if gotAddress != "addr-btc" {
		t.Fatalf("address = %s, want addr-btc", gotAddress)
	}
	if got.ConfirmedSatoshi != 135758 || got.MempoolIncomeSatoshi != 10 || got.MempoolSpendSatoshi != 3 {
		t.Fatalf("unexpected balance: %+v", got)
	}
	if got.ConfirmedUTXOCount != 1040 || got.MempoolUTXOCount != 2 {
		t.Fatalf("unexpected utxo counts: %+v", got)
	}
	if got.UnsafeSatoshi != 134862 {
		t.Fatalf("unsafe = %d, want 134862", got.UnsafeSatoshi)
	}
}

func TestCoreClientDoesNotPromoteBalanceSatoshiToConfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"confirmed_balance_satoshi": uint64(0),
			"balance_satoshi":           uint64(100),
			"confirmed_utxo_count":      uint64(0),
			"mempool_income_satoshi":    uint64(100),
			"mempool_spend_satoshi":     uint64(0),
			"mempool_utxo_count":        uint64(1),
			"unsafe_fee_satoshi":        uint64(0),
		})
	}))
	defer server.Close()

	client, err := NewCoreClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.FetchBalance(context.Background(), ChainBTC, "addr-btc")
	if err != nil {
		t.Fatalf("FetchBalance: %v", err)
	}
	if got.ConfirmedSatoshi != 0 {
		t.Fatalf("ConfirmedSatoshi = %d, want 0; balance_satoshi must not be treated as confirmed", got.ConfirmedSatoshi)
	}
	if got.MempoolIncomeSatoshi != 100 || got.UnconfirmedSatoshi() != 100 {
		t.Fatalf("unexpected mempool state: %+v", got)
	}
}

func TestCoreClientFetchUTXOs(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"address": "addr-btc",
			"count":   2,
			"utxos": []map[string]any{
				{"tx_id": "tx-a", "index": "0", "amount": uint64(100), "is_mempool": false},
				{"tx_id": "tx-b", "index": "1", "amount": uint64(200), "is_mempool": true},
			},
		})
	}))
	defer server.Close()

	client, err := NewCoreClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.FetchUTXOs(context.Background(), ChainBTC, "addr-btc")
	if err != nil {
		t.Fatalf("FetchUTXOs: %v", err)
	}
	if gotPath != "/utxos" {
		t.Fatalf("path = %s, want /utxos", gotPath)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Outpoint() != "tx-a:0" || !got[0].Confirmed || got[0].Mempool {
		t.Fatalf("unexpected first utxo: %+v", got[0])
	}
	if got[1].Outpoint() != "tx-b:1" || got[1].Confirmed || !got[1].Mempool {
		t.Fatalf("unexpected second utxo: %+v", got[1])
	}
	if got[1].Height == nil || *got[1].Height != -1 {
		t.Fatalf("mempool height = %v, want -1", got[1].Height)
	}
}

func TestCoreClientFetchUTXOsUsesMergedUTXOEndpointOnly(t *testing.T) {
	var gotPaths []string
	var sawMempoolEndpoint bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		switch r.URL.Path {
		case "/utxos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"address": "addr-btc",
				"utxos": []map[string]any{
					{"tx_id": "mem", "index": "0", "amount": uint64(200), "is_mempool": true},
				},
			})
		case "/mempool/utxos":
			sawMempoolEndpoint = true
			http.Error(w, "unexpected mempool endpoint", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewCoreClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.FetchUTXOs(context.Background(), ChainBTC, "addr-btc")
	if err != nil {
		t.Fatalf("FetchUTXOs: %v", err)
	}
	if len(gotPaths) != 1 || gotPaths[0] != "/utxos" {
		t.Fatalf("paths = %#v, want only /utxos", gotPaths)
	}
	if sawMempoolEndpoint {
		t.Fatalf("client must not call /mempool/utxos in the first Wallet Gateway version")
	}
	if len(got) != 1 || !got[0].Mempool {
		t.Fatalf("expected merged /utxos response to carry mempool entry: %+v", got)
	}
}

func TestCoreClientReportsInvalidUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad upstream", http.StatusBadGateway)
	}))
	defer server.Close()

	client, err := NewCoreClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	_, err = client.FetchBalance(context.Background(), ChainBTC, "addr-btc")
	if err == nil {
		t.Fatal("expected upstream error")
	}
}
