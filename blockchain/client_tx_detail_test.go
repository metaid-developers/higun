package blockchain

import (
	"encoding/json"
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
