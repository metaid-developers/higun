package blockchain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
)

func TestTxDetailFromVerboseConfirmed(t *testing.T) {
	raw := &btcjson.TxRawResult{
		Txid:          strings.Repeat("a", 64),
		Size:          225,
		Vsize:         225,
		BlockHash:     strings.Repeat("b", 64),
		Confirmations: 3,
		Blocktime:     1717833600,
		Vin: []btcjson.Vin{
			{Txid: strings.Repeat("c", 64), Vout: 1},
		},
		Vout: []btcjson.Vout{
			{N: 0, Value: 0.00001000, ScriptPubKey: btcjson.ScriptPubKeyResult{Address: "addr-out"}},
		},
	}

	got, err := txDetailFromVerbose(raw)
	if err != nil {
		t.Fatalf("txDetailFromVerbose: %v", err)
	}
	if got.TxID != raw.Txid || !got.Confirmed || got.Mempool || got.Confirmations != 3 {
		t.Fatalf("unexpected status: %+v", got)
	}
	if got.BlockHash != raw.BlockHash || got.BlockTime == nil || *got.BlockTime != 1717833600 {
		t.Fatalf("unexpected block fields: %+v", got)
	}
	if len(got.Inputs) != 1 || got.Inputs[0].TxID != strings.Repeat("c", 64) || got.Inputs[0].Vout != 1 {
		t.Fatalf("unexpected inputs: %+v", got.Inputs)
	}
	if len(got.Outputs) != 1 || got.Outputs[0].Address != "addr-out" || got.Outputs[0].Satoshi != 1000 {
		t.Fatalf("unexpected outputs: %+v", got.Outputs)
	}
}

func TestTxDetailFromVerboseMempool(t *testing.T) {
	raw := &btcjson.TxRawResult{Txid: strings.Repeat("a", 64)}
	got, err := txDetailFromVerbose(raw)
	if err != nil {
		t.Fatalf("txDetailFromVerbose: %v", err)
	}
	if got.Confirmed || !got.Mempool || got.Confirmations != 0 {
		t.Fatalf("unexpected mempool status: %+v", got)
	}
}

func TestTxDetailJSONUsesWalletFieldNames(t *testing.T) {
	raw := &btcjson.TxRawResult{Txid: strings.Repeat("a", 64), Confirmations: 1}
	got, err := txDetailFromVerbose(raw)
	if err != nil {
		t.Fatalf("txDetailFromVerbose: %v", err)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{`"txid":"`, `"confirmed":true`, `"mempool":false`, `"confirmations":1`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("body missing %s: %s", want, string(body))
		}
	}
}

func TestTxDetailJSONIncludesInputVoutZero(t *testing.T) {
	detail := &TxDetail{
		TxID:   strings.Repeat("a", 64),
		Inputs: []TxInput{{TxID: strings.Repeat("b", 64), Vout: 0}},
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(body), `"vout":0`) {
		t.Fatalf("body missing input vout zero: %s", string(body))
	}
}

func TestTxDetailFromRawJSONPreservesLargeOutputSatoshis(t *testing.T) {
	body := []byte(`{
		"txid":"` + strings.Repeat("a", 64) + `",
		"size":225,
		"vsize":225,
		"confirmations":1,
		"vin":[{"txid":"` + strings.Repeat("b", 64) + `","vout":0}],
		"vout":[{"n":0,"value":92233720368.54775907,"scriptPubKey":{"address":"addr-out"}}]
	}`)

	got, err := txDetailFromRawJSON(body)
	if err != nil {
		t.Fatalf("txDetailFromRawJSON: %v", err)
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("outputs len = %d, want 1", len(got.Outputs))
	}
	if got.Outputs[0].Satoshi != 9223372036854775907 {
		t.Fatalf("satoshi = %d, want 9223372036854775907", got.Outputs[0].Satoshi)
	}
}

func TestTxDetailRPCErrorDoesNotMatchGenericMethodNotFound(t *testing.T) {
	for _, err := range []error{
		errors.New("Method not found"),
		errors.New("rpc: method not found"),
	} {
		if isTransactionNotFoundRPCError(err) {
			t.Fatalf("isTransactionNotFoundRPCError(%q) = true, want false", err.Error())
		}
	}
}
