package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCoreClientBroadcastTransactionUsesConfiguredPathAndNormalizesLegacySuccess(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 2000, "msg": strings.Repeat("a", 64)})
	}))
	defer core.Close()

	client, err := NewCoreClient(core.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.BroadcastTransaction(context.Background(), ChainBTC, "deadbeef", "/btc/broadcast")
	if err != nil {
		t.Fatalf("BroadcastTransaction: %v", err)
	}
	if gotPath != "/btc/broadcast" {
		t.Fatalf("path = %s, want /btc/broadcast", gotPath)
	}
	if gotBody["rawTx"] != "deadbeef" {
		t.Fatalf("rawTx body was not forwarded as expected")
	}
	if got.TxID != strings.Repeat("a", 64) || !got.Accepted || got.Chain != ChainBTC {
		t.Fatalf("unexpected broadcast result: %+v", got)
	}
}

func TestCoreClientBroadcastTransactionMapsRejectedResponse(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": -2003, "msg": "txn-mempool-conflict"})
	}))
	defer core.Close()

	client, err := NewCoreClient(core.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	_, err = client.BroadcastTransaction(context.Background(), ChainBTC, "deadbeef", "/btc/broadcast")
	assertWalletError(t, err, http.StatusBadGateway, CodeBroadcastRejected)
}

func TestBroadcastLogDoesNotLeakRawTx(t *testing.T) {
	rawTx := "0200000001abcdef"
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "node rejected "+rawTx, http.StatusBadGateway)
	}))
	defer core.Close()

	client, err := NewCoreClient(core.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}

	var buf bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(previousOutput)

	_, _ = client.BroadcastTransaction(context.Background(), ChainBTC, rawTx, "/btc/broadcast")
	if strings.Contains(buf.String(), rawTx) {
		t.Fatalf("log leaked rawTx: %s", buf.String())
	}
}

func assertWalletError(t *testing.T, err error, wantStatus int, wantCode int) {
	t.Helper()
	walletErr, ok := err.(*WalletError)
	if !ok {
		t.Fatalf("err = %T %v, want *WalletError", err, err)
	}
	if walletErr.HTTPStatus != wantStatus || walletErr.Code != wantCode {
		t.Fatalf("wallet error = %+v, want status %d code %d", walletErr, wantStatus, wantCode)
	}
}
