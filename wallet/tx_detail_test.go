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
			"inputs":        []map[string]any{{"txid": strings.Repeat("b", 64), "vout": 1, "satoshi": uint64(2000)}},
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

func TestStandardTxDetailResponse(t *testing.T) {
	txid := strings.Repeat("a", 64)
	inputSatoshi := uint64(2000)
	body := marshalResponse(t, NewStandardTxDetailResponse(WalletTxDetail{
		Chain:         ChainBTC,
		TxID:          txid,
		Confirmed:     true,
		Confirmations: 2,
		Inputs:        []WalletTxInput{{TxID: strings.Repeat("b", 64), Vout: 0, Address: "addr-in", Satoshi: &inputSatoshi}},
		Outputs:       []WalletTxOutput{{Vout: 0, Address: "addr", Satoshi: 1000}},
	}))
	for _, want := range []string{`"txid":"` + txid + `"`, `"confirmed":true`, `"confirmations":2`, `"amount":"0.00001000"`, `"vout":0`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
}
