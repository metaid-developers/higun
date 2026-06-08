package wallet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCoreClientFetchTransaction(t *testing.T) {
	txid := strings.Repeat("a", 64)
	var gotPath string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"txid":          txid,
			"confirmed":     true,
			"mempool":       false,
			"confirmations": uint64(2),
			"inputs":        []map[string]any{{"txid": strings.Repeat("b", 64), "vout": 1, "satoshi": uint64(2000), "coinbase": "coinbase-data"}},
			"outputs":       []map[string]any{{"vout": 0, "address": "addr", "satoshi": uint64(1000)}},
			"size":          225,
			"vsize":         225,
		})
	}))
	defer core.Close()

	client, err := NewCoreClient(core.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.FetchTransaction(context.Background(), ChainBTC, txid)
	if err != nil {
		t.Fatalf("FetchTransaction: %v", err)
	}
	if gotPath != "/tx/"+txid {
		t.Fatalf("path = %s, want /tx/%s", gotPath, txid)
	}
	if got.TxID != txid || !got.Confirmed || got.Mempool || got.Confirmations != 2 {
		t.Fatalf("unexpected tx detail: %+v", got)
	}
	if len(got.Inputs) != 1 || got.Inputs[0].Satoshi == nil || *got.Inputs[0].Satoshi != 2000 {
		t.Fatalf("unexpected inputs: %+v", got.Inputs)
	}
	if got.Inputs[0].Coinbase != "coinbase-data" {
		t.Fatalf("coinbase = %q, want coinbase-data", got.Inputs[0].Coinbase)
	}
	if len(got.Outputs) != 1 || got.Outputs[0].Satoshi != 1000 {
		t.Fatalf("unexpected outputs: %+v", got.Outputs)
	}
}

func TestCoreClientFetchTransactionNotFound(t *testing.T) {
	txid := strings.Repeat("a", 64)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "transaction not found", http.StatusNotFound)
	}))
	defer core.Close()

	client, err := NewCoreClient(core.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	_, err = client.FetchTransaction(context.Background(), ChainBTC, txid)
	assertWalletError(t, err, http.StatusNotFound, CodeTxNotFound)
}

func TestCoreClientFetchTransactionRejectsInvalidStatus(t *testing.T) {
	txid := strings.Repeat("a", 64)
	tests := []struct {
		name          string
		confirmed     bool
		mempool       bool
		confirmations uint64
	}{
		{name: "neither confirmed nor mempool", confirmed: false, mempool: false, confirmations: 0},
		{name: "neither confirmed nor mempool with confirmations", confirmed: false, mempool: false, confirmations: 1},
		{name: "both confirmed and mempool", confirmed: true, mempool: true, confirmations: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeCoreTxDetail(ChainBTC, coreTxDetailResponse{
				TxID:          txid,
				Confirmed:     tt.confirmed,
				Mempool:       tt.mempool,
				Confirmations: tt.confirmations,
			})
			assertWalletError(t, err, http.StatusBadGateway, CodeInvalidUpstream)
		})
	}
}

func TestStandardTxDetailResponse(t *testing.T) {
	txid := strings.Repeat("a", 64)
	inputSatoshi := uint64(2000)
	body := marshalResponse(t, NewStandardTxDetailResponse(WalletTxDetail{
		Chain:         ChainBTC,
		TxID:          txid,
		Confirmed:     true,
		Confirmations: 2,
		Inputs:        []WalletTxInput{{TxID: strings.Repeat("b", 64), Vout: 0, Address: "addr-in", Satoshi: &inputSatoshi, Coinbase: "coinbase-data"}},
		Outputs:       []WalletTxOutput{{Vout: 0, Address: "addr", Satoshi: 1000}},
	}))
	for _, want := range []string{`"txid":"` + txid + `"`, `"confirmed":true`, `"confirmations":2`, `"amount":"0.00001000"`, `"vout":0`, `"coinbase":"coinbase-data"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
}
