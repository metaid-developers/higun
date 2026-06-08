package wallet

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStandardBalanceResponseUsesIntegerAndStringAmounts(t *testing.T) {
	balance := WalletBalance{
		Chain:                ChainBTC,
		Address:              "addr",
		ConfirmedSatoshi:     135758,
		MempoolIncomeSatoshi: 10,
		MempoolSpendSatoshi:  3,
		UnsafeSatoshi:        134862,
		ConfirmedUTXOCount:   1040,
		MempoolUTXOCount:     2,
	}

	body := marshalResponse(t, NewStandardBalanceResponse(balance))
	for _, legacyFloatKey := range []string{`"balance":0.`, `"safeBalance":0.`, `"unSafeBalance":0.`} {
		if strings.Contains(body, legacyFloatKey) {
			t.Fatalf("standard response exposed legacy float key %s: %s", legacyFloatKey, body)
		}
	}
	if !strings.Contains(body, `"confirmed":"0.00135758"`) {
		t.Fatalf("standard response missing confirmed decimal string: %s", body)
	}
	if !strings.Contains(body, `"confirmedSatoshi":135758`) {
		t.Fatalf("standard response missing confirmed satoshi integer: %s", body)
	}
}

func TestStandardUTXOResponse(t *testing.T) {
	body := marshalResponse(t, NewStandardUTXOResponse(ChainBTC, "addr", false, "desc", []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "tx", Vout: 1, Satoshi: 1000, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	}))

	for _, want := range []string{
		`"outpoint":"tx:1"`,
		`"amount":"0.00001000"`,
		`"mempool":true`,
		`"confirmed":false`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("standard utxo response missing %s: %s", want, body)
		}
	}
}

func TestStandardUTXOResponseKeepsChainAndAddressTopLevelOnly(t *testing.T) {
	body := marshalResponse(t, NewStandardUTXOResponse(ChainBTC, "addr", false, "desc", []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "tx", Vout: 1, Satoshi: 1000, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	}))

	var payload struct {
		Data struct {
			Chain   Chain             `json:"chain"`
			Address string            `json:"address"`
			UTXOs   []json.RawMessage `json:"utxos"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Data.Chain != ChainBTC {
		t.Fatalf("top-level chain = %q, want %q; body: %s", payload.Data.Chain, ChainBTC, body)
	}
	if payload.Data.Address != "addr" {
		t.Fatalf("top-level address = %q, want addr; body: %s", payload.Data.Address, body)
	}
	if len(payload.Data.UTXOs) != 1 {
		t.Fatalf("utxo len = %d, want 1; body: %s", len(payload.Data.UTXOs), body)
	}
	var item map[string]any
	if err := json.Unmarshal(payload.Data.UTXOs[0], &item); err != nil {
		t.Fatalf("Unmarshal item: %v", err)
	}
	if _, ok := item["chain"]; ok {
		t.Fatalf("utxo item should not include chain: %s", string(payload.Data.UTXOs[0]))
	}
	if _, ok := item["address"]; ok {
		t.Fatalf("utxo item should not include address: %s", string(payload.Data.UTXOs[0]))
	}
}

func TestStandardUTXOResponseOmitsMissingHeight(t *testing.T) {
	body := marshalResponse(t, NewStandardUTXOResponse(ChainBTC, "addr", true, "asc", []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "tx", Vout: 1, Satoshi: 1000, Confirmed: true, Mempool: false, Height: nil},
	}))

	if strings.Contains(body, `"height"`) {
		t.Fatalf("standard utxo response should omit missing height: %s", body)
	}
}

func TestMetaletBTCBalanceCompatibility(t *testing.T) {
	body := marshalResponse(t, NewMetaletBTCBalanceResponse(WalletBalance{
		Chain:              ChainBTC,
		Address:            "addr",
		ConfirmedSatoshi:   135758,
		UnsafeSatoshi:      134862,
		ConfirmedUTXOCount: 1040,
	}))

	for _, want := range []string{
		`"balance":0.00135758`,
		`"safeBalance":0.00000896`,
		`"unSafeBalance":0.00134862`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metalet btc response missing %s: %s", want, body)
		}
	}
}

func TestMetaletMVCDOGEBalanceCompatibility(t *testing.T) {
	body := marshalResponse(t, NewMetaletMVCDOGEBalanceResponse(WalletBalance{
		Chain:                ChainDOGE,
		Address:              "doge-addr",
		ConfirmedSatoshi:     6187007540823,
		MempoolIncomeSatoshi: 500,
		MempoolSpendSatoshi:  100,
		ConfirmedUTXOCount:   1475,
		MempoolUTXOCount:     1,
	}))

	for _, want := range []string{
		`"address":"doge-addr"`,
		`"confirmed":6187007540823`,
		`"unconfirmed":400`,
		`"utxoCount":1476`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metalet mvc/doge response missing %s: %s", want, body)
		}
	}
}

func TestMetaletBalanceCompatibilityGoldenFixtures(t *testing.T) {
	tests := []struct {
		name string
		path string
		got  Envelope
	}{
		{
			name: "btc",
			path: "testdata/metalet_btc_balance.json",
			got: NewMetaletBTCBalanceResponse(WalletBalance{
				Chain:              ChainBTC,
				Address:            "addr",
				ConfirmedSatoshi:   135758,
				UnsafeSatoshi:      134862,
				ConfirmedUTXOCount: 1040,
			}),
		},
		{
			name: "mvc",
			path: "testdata/metalet_mvc_balance.json",
			got: NewMetaletMVCDOGEBalanceResponse(WalletBalance{
				Chain:              ChainMVC,
				Address:            "mvc-addr",
				ConfirmedSatoshi:   135758,
				ConfirmedUTXOCount: 1040,
			}),
		},
		{
			name: "doge",
			path: "testdata/metalet_doge_balance.json",
			got: NewMetaletMVCDOGEBalanceResponse(WalletBalance{
				Chain:                ChainDOGE,
				Address:              "doge-addr",
				ConfirmedSatoshi:     6187007540823,
				MempoolIncomeSatoshi: 500,
				MempoolSpendSatoshi:  100,
				ConfirmedUTXOCount:   1475,
				MempoolUTXOCount:     1,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", tt.path, err)
			}
			assertCompactJSONEqual(t, want, []byte(marshalResponse(t, tt.got)))
		})
	}
}

func TestMetaletUTXOCompatibilityGoldenFixture(t *testing.T) {
	resp := NewMetaletUTXOResponse(ChainBTC, []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "tx", Vout: 1, Satoshi: 1000, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	})
	assertJSONFixture(t, "testdata/metalet_utxos.json", resp)
}

func marshalResponse(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(body)
}

func assertCompactJSONEqual(t *testing.T, wantRaw, gotRaw []byte) {
	t.Helper()
	var want bytes.Buffer
	if err := json.Compact(&want, wantRaw); err != nil {
		t.Fatalf("compact want: %v", err)
	}
	var got bytes.Buffer
	if err := json.Compact(&got, gotRaw); err != nil {
		t.Fatalf("compact got: %v", err)
	}
	if !bytes.Equal(want.Bytes(), got.Bytes()) {
		t.Fatalf("JSON mismatch\ngot:  %s\nwant: %s", got.String(), want.String())
	}
}

func assertJSONFixture(t *testing.T, path string, got any) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	assertCompactJSONEqual(t, want, []byte(marshalResponse(t, got)))
}
