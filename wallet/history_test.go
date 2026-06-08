package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCoreClientFetchHistory(t *testing.T) {
	txid := strings.Repeat("a", 64)
	var gotPath string
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"address": "addr",
			"total":   1,
			"list": []map[string]any{
				{
					"tx_id":      txid,
					"income":     uint64(1000),
					"spend":      uint64(0),
					"type":       "income",
					"is_mempool": true,
					"timestamp":  int64(1717833600),
					"time":       "2026-06-08 12:00:00",
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewCoreClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.FetchHistory(context.Background(), ChainBTC, "addr", HistoryOptions{
		Page:          1,
		Limit:         20,
		ConfirmedOnly: false,
		Sort:          "desc",
	})
	if err != nil {
		t.Fatalf("FetchHistory: %v", err)
	}
	if gotPath != "/utxos/history" {
		t.Fatalf("path = %s, want /utxos/history", gotPath)
	}
	wantQuery := map[string]string{
		"address":       "addr",
		"page":          "1",
		"limit":         "20",
		"confirmedOnly": "false",
		"sort":          "desc",
	}
	for key, want := range wantQuery {
		if got := gotQuery.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q; raw query: %s", key, got, want, gotQuery.Encode())
		}
	}
	if got.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Total)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len Items = %d, want 1", len(got.Items))
	}
	item := got.Items[0]
	if item.TxID != txid || !item.Mempool || item.NetSatoshi != 1000 {
		t.Fatalf("unexpected history item: %+v", item)
	}
}

func TestNormalizeHistoryOptions(t *testing.T) {
	got, err := NormalizeHistoryOptions("0", "500", "true", "ASC")
	if err != nil {
		t.Fatalf("NormalizeHistoryOptions valid: %v", err)
	}
	if got.Page != 1 || got.Limit != 100 || !got.ConfirmedOnly || got.Sort != "asc" {
		t.Fatalf("options = %+v, want page 1 limit 100 confirmedOnly true sort asc", got)
	}

	_, err = NormalizeHistoryOptions("1", "20", "invalid", "desc")
	assertWalletErrorMessage(t, err, http.StatusBadRequest, CodeInvalidQuery, "confirmedOnly must be true or false")

	_, err = NormalizeHistoryOptions("1", "20", "false", "sideways")
	assertWalletErrorMessage(t, err, http.StatusBadRequest, CodeInvalidQuery, "sort must be asc or desc")
}

func TestStandardHistoryResponse(t *testing.T) {
	txid := strings.Repeat("a", 64)
	confirmations := uint64(0)
	body := marshalResponse(t, NewStandardHistoryResponse(WalletHistoryPage{
		Chain:         ChainBTC,
		Address:       "addr",
		Page:          1,
		Limit:         20,
		ConfirmedOnly: false,
		Sort:          "desc",
		Total:         1,
		Items: []WalletHistoryItem{
			{
				TxID:          txid,
				Direction:     "income",
				IncomeSatoshi: 1000,
				SpendSatoshi:  0,
				NetSatoshi:    1000,
				Confirmed:     false,
				Mempool:       true,
				Confirmations: &confirmations,
				Timestamp:     1717833600,
				Time:          "2026-06-08 12:00:00",
			},
		},
	}))

	for _, want := range []string{
		`"txid":"` + txid + `"`,
		`"mempool":true`,
		`"netSatoshi":1000`,
		`"net":"0.00001000"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("standard history response missing %s: %s", want, body)
		}
	}
}

func TestNormalizeCoreHistoryRejectsInvalidTxID(t *testing.T) {
	_, err := normalizeCoreHistory(ChainBTC, "addr", HistoryOptions{Page: 1, Limit: 20, Sort: "desc"}, coreHistoryResponse{
		Address: "addr",
		Total:   1,
		List: []coreHistoryItem{
			{TxID: "not-a-txid", Income: 1000, Type: "income"},
		},
	})
	assertInvalidUpstreamWalletError(t, err)
}

func TestStandardHistoryResponseUsesSignedSpendNetString(t *testing.T) {
	txid := strings.Repeat("b", 64)
	body := marshalResponse(t, NewStandardHistoryResponse(WalletHistoryPage{
		Chain:   ChainBTC,
		Address: "addr",
		Page:    1,
		Limit:   20,
		Sort:    "desc",
		Total:   1,
		Items: []WalletHistoryItem{
			{
				TxID:          txid,
				Direction:     "spend",
				IncomeSatoshi: 0,
				SpendSatoshi:  1000,
				NetSatoshi:    -1000,
				Confirmed:     true,
			},
		},
	}))

	for _, want := range []string{
		`"netSatoshi":-1000`,
		`"net":"-0.00001000"`,
		`"spend":"0.00001000"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("standard history response missing %s: %s", want, body)
		}
	}
}

func assertWalletErrorMessage(t *testing.T, err error, wantStatus int, wantCode int, wantMessage string) {
	t.Helper()
	var walletErr *WalletError
	if !errors.As(err, &walletErr) {
		t.Fatalf("err = %T %v, want *WalletError", err, err)
	}
	if walletErr.HTTPStatus != wantStatus || walletErr.Code != wantCode || walletErr.Message != wantMessage {
		t.Fatalf("wallet error = %+v, want status %d code %d message %q", walletErr, wantStatus, wantCode, wantMessage)
	}
}
