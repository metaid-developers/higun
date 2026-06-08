package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/metaid/utxo_indexer/wallet"
)

func TestEnableWalletGatewayRegistersWalletRoutes(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"confirmed_balance_satoshi": uint64(135758),
				"confirmed_utxo_count":      uint64(1),
			})
		case "/utxos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"address": "addr",
				"utxos": []map[string]any{
					{"tx_id": "tx", "index": "0", "amount": uint64(1000), "is_mempool": true},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer core.Close()

	server := NewServer(nil, nil, make(chan struct{}))
	err := server.EnableWalletGateway(wallet.Config{
		Enabled: true,
		Timeout: 2 * time.Second,
		Chains: map[wallet.Chain]wallet.ChainConfig{
			wallet.ChainBTC: {Enabled: true, CoreURL: core.URL},
		},
	})
	if err != nil {
		t.Fatalf("EnableWalletGateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr/balance", nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"confirmedSatoshi":135758`) {
		t.Fatalf("missing wallet response: %s", rec.Body.String())
	}
}

func TestEnableWalletGatewayKeepsCoreRoutesRegistered(t *testing.T) {
	server := NewServer(nil, nil, make(chan struct{}))
	err := server.EnableWalletGateway(wallet.Config{
		Enabled: true,
		Timeout: 2 * time.Second,
		Chains: map[wallet.Chain]wallet.ChainConfig{
			wallet.ChainBTC: {Enabled: true, CoreURL: "http://127.0.0.1:1"},
		},
	})
	if err != nil {
		t.Fatalf("EnableWalletGateway: %v", err)
	}

	routes := make(map[string]bool)
	for _, route := range server.Router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /balance",
		"GET /utxos",
		"GET /mempool/utxos",
		"GET /wallet/v1/:chain/address/:address/balance",
		"GET /wallet/v1/:chain/address/:address/utxos",
	} {
		if !routes[expected] {
			t.Fatalf("missing route %s; got %#v", expected, routes)
		}
	}
}
