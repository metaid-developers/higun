package blockchain

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

func TestClientIsUnspentUsesGetTxOutRPC(t *testing.T) {
	const txID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("rpcuser:rpcpass"))
		if r.Header.Get("Authorization") != wantAuth {
			t.Fatalf("unexpected Authorization header: %q", r.Header.Get("Authorization"))
		}
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "gettxout" {
			t.Fatalf("expected gettxout method, got %s", req.Method)
		}
		if got := string(req.Params[0]); got != `"`+txID+`"` {
			t.Fatalf("unexpected txid param: %s", got)
		}
		if got := string(req.Params[1]); got != "1" {
			t.Fatalf("unexpected vout param: %s", got)
		}
		if got := string(req.Params[2]); got != "true" {
			t.Fatalf("unexpected include_mempool param: %s", got)
		}
		_, _ = w.Write([]byte(`{"result":{"value":1.23,"confirmations":10},"error":null}`))
	}))
	defer server.Close()

	client := testRPCClient(t, server.URL)
	unspent, err := client.IsUnspent(txID, 1)
	if err != nil {
		t.Fatalf("IsUnspent: %v", err)
	}
	if !unspent {
		t.Fatalf("expected unspent result")
	}
}

func TestClientIsUnspentReturnsFalseForNullGetTxOut(t *testing.T) {
	const txID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":null,"error":null}`))
	}))
	defer server.Close()

	client := testRPCClient(t, server.URL)
	unspent, err := client.IsUnspent(txID, 1)
	if err != nil {
		t.Fatalf("IsUnspent: %v", err)
	}
	if unspent {
		t.Fatalf("expected null gettxout result to be treated as spent")
	}
}

func testRPCClient(t *testing.T, rawURL string) *Client {
	t.Helper()
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatalf("split test server host/port: %v", err)
	}
	return &Client{cfg: &config.Config{
		UTXOValidationRPCTimeoutSeconds: 1,
		RPC: config.RPCConfig{
			Host:     host,
			Port:     port,
			User:     "rpcuser",
			Password: "rpcpass",
		},
	}}
}
