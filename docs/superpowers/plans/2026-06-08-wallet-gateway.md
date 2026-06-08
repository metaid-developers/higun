# Wallet Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Higun Wallet Gateway that exposes BTC, MVC, and DOGE balance and UTXO APIs for new applications under `/wallet/v1`.

**Architecture:** Add a new `wallet` package that is separate from Higun core indexing. The wallet package owns chain routing, upstream core HTTP clients, response normalization, Metalet compatibility rendering, and Gin route registration; existing `/balance`, `/utxos`, and Metalet production routes stay untouched.

**Tech Stack:** Go 1.24, Gin, standard `net/http`, YAML config, Go test

---

## Implementation Notes

This plan implements the approved design in `docs/superpowers/specs/2026-06-07-wallet-gateway-design.md`.

Key requirements:

- First version supports `btc`, `mvc`, and `doge`.
- Public routes are:
  - `GET /wallet/v1/{chain}/address/{address}/balance`
  - `GET /wallet/v1/{chain}/address/{address}/utxos`
- Default response format is the new standard format.
- `format=metalet` returns compatibility-shaped responses.
- Standard amount fields use satoshi integers as canonical values and decimal strings for display.
- Standard responses must not expose float amount fields.
- UTXO responses include mempool entries by default.
- `confirmedOnly=true` filters out mempool UTXOs.
- One gateway can route each chain to a different Higun core endpoint.
- Existing Higun core and old Metalet routes remain unchanged.

First-version upstream contract:

- The Wallet Gateway calls each configured chain core through `/balance` and `/utxos`.
- `/utxos` is the merged source of truth for confirmed and mempool UTXOs in this first version. The gateway does not call `/mempool/utxos` because that route currently exposes a separate income/spend shape that is not normalized into a spendable UTXO list.
- `confirmedOnly=true` filters the merged `/utxos` response inside the gateway; it is not sent upstream.
- Unknown chains return HTTP 404 with `CodeUnsupportedChain`; supported but disabled or unconfigured chains return HTTP 503 with `CodeCoreUnavailable`; upstream network, HTTP, or schema failures return HTTP 502.
- Nonnegative satoshi amounts use `uint64`; only net deltas such as `unconfirmedSatoshi` use signed integers.

## File Structure

Create the new wallet package:

- `wallet/model.go`: chain constants, internal wallet models, request option models.
- `wallet/normalize.go`: amount formatting, safe balance calculation, UTXO filtering, dedupe, sorting.
- `wallet/client.go`: Higun core HTTP client and core response normalization.
- `wallet/errors.go`: wallet error codes and typed errors.
- `wallet/logging.go`: wallet upstream request logging and address redaction.
- `wallet/response.go`: standard response data and envelope helpers.
- `wallet/metalet_compat.go`: Metalet-compatible response rendering.
- `wallet/server.go`: gateway construction and route registration.
- `wallet/handlers.go`: Gin handlers for balance and UTXO routes.

Create tests:

- `wallet/normalize_test.go`
- `wallet/client_test.go`
- `wallet/logging_test.go`
- `wallet/response_test.go`
- `wallet/handlers_test.go`
- `wallet/testdata/metalet_btc_balance.json`
- `wallet/testdata/metalet_mvc_balance.json`
- `wallet/testdata/metalet_doge_balance.json`
- `wallet/testdata/metalet_utxos.json`

Modify existing files:

- `config/config.go`: add wallet gateway config and environment overrides.
- `api/server.go`: add `EnableWalletGateway` so `/wallet/v1` can be registered without changing core routes.
- `main.go`: enable Wallet Gateway when config says `wallet.enabled: true`.
- `config.yaml`: add disabled-by-default wallet config example.

---

### Task 1: Add wallet models and normalization helpers

**Files:**
- Create: `wallet/model.go`
- Create: `wallet/normalize.go`
- Create: `wallet/normalize_test.go`

- [ ] **Step 1: Write failing normalization tests**

Create `wallet/normalize_test.go`:

```go
package wallet

import "testing"

func TestSatoshiToDecimalString(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		want string
	}{
		{name: "zero", in: 0, want: "0"},
		{name: "small", in: 1000, want: "0.00001000"},
		{name: "btc", in: 100000000, want: "1.00000000"},
		{name: "fraction", in: 135758, want: "0.00135758"},
		{name: "large-doge-safe", in: uint64(1<<63) + 99, want: "92233720368.54775907"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SatoshiToDecimalString(tt.in); got != tt.want {
				t.Fatalf("SatoshiToDecimalString(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSignedSatoshiToDecimalString(t *testing.T) {
	if got, want := SignedSatoshiToDecimalString(-1), "-0.00000001"; got != want {
		t.Fatalf("SignedSatoshiToDecimalString(-1) = %q, want %q", got, want)
	}
	if got, want := SignedSatoshiToDecimalString(1000), "0.00001000"; got != want {
		t.Fatalf("SignedSatoshiToDecimalString(1000) = %q, want %q", got, want)
	}
}

func TestWalletBalanceSafeSatoshi(t *testing.T) {
	balance := WalletBalance{
		ConfirmedSatoshi:     135758,
		MempoolIncomeSatoshi: 100,
		MempoolSpendSatoshi:  50,
		UnsafeSatoshi:        134862,
	}
	if got, want := balance.SafeSatoshi(), uint64(946); got != want {
		t.Fatalf("SafeSatoshi() = %d, want %d", got, want)
	}

	negative := WalletBalance{
		ConfirmedSatoshi: 100,
		UnsafeSatoshi:    200,
	}
	if got := negative.SafeSatoshi(); got != 0 {
		t.Fatalf("negative SafeSatoshi() = %d, want 0", got)
	}
}

func TestWalletBalanceUnconfirmedCanBeNegative(t *testing.T) {
	balance := WalletBalance{
		MempoolIncomeSatoshi: 10,
		MempoolSpendSatoshi:  25,
	}
	if got, want := balance.UnconfirmedSatoshi(), int64(-15); got != want {
		t.Fatalf("UnconfirmedSatoshi() = %d, want %d", got, want)
	}
}

func TestWalletBalanceSupportsLargeDogeAmounts(t *testing.T) {
	const large = uint64(1<<63) + 99
	balance := WalletBalance{
		ConfirmedSatoshi: large,
		UnsafeSatoshi:    1,
	}
	if got, want := balance.SafeSatoshi(), large-1; got != want {
		t.Fatalf("SafeSatoshi() = %d, want %d", got, want)
	}
	if got, want := SatoshiToDecimalString(large), "92233720368.54775907"; got != want {
		t.Fatalf("large decimal = %q, want %q", got, want)
	}
}

func TestNormalizeUTXOsIncludesMempoolByDefaultAndSortsDesc(t *testing.T) {
	in := []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "b", Vout: 0, Satoshi: 200, Confirmed: true, Mempool: false, Height: int64Ptr(10)},
		{Chain: ChainBTC, Address: "addr", TxID: "a", Vout: 1, Satoshi: 300, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
		{Chain: ChainBTC, Address: "addr", TxID: "c", Vout: 0, Satoshi: 100, Confirmed: true, Mempool: false, Height: int64Ptr(11)},
	}
	got, err := NormalizeUTXOs(in, UTXOOptions{Sort: "desc"})
	if err != nil {
		t.Fatalf("NormalizeUTXOs: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Outpoint() != "a:1" || got[1].Outpoint() != "b:0" || got[2].Outpoint() != "c:0" {
		t.Fatalf("unexpected order: %s, %s, %s", got[0].Outpoint(), got[1].Outpoint(), got[2].Outpoint())
	}
	if !got[0].Mempool || got[0].Confirmed {
		t.Fatalf("first UTXO should be mempool and unconfirmed: %+v", got[0])
	}
}

func TestNormalizeUTXOsConfirmedOnlySortsAscAndDeduplicates(t *testing.T) {
	in := []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "dup", Vout: 0, Satoshi: 50, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
		{Chain: ChainBTC, Address: "addr", TxID: "dup", Vout: 0, Satoshi: 50, Confirmed: true, Mempool: false, Height: int64Ptr(12)},
		{Chain: ChainBTC, Address: "addr", TxID: "small", Vout: 0, Satoshi: 10, Confirmed: true, Mempool: false, Height: int64Ptr(12)},
		{Chain: ChainBTC, Address: "addr", TxID: "mem", Vout: 0, Satoshi: 5, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	}
	got, err := NormalizeUTXOs(in, UTXOOptions{ConfirmedOnly: true, Sort: "asc"})
	if err != nil {
		t.Fatalf("NormalizeUTXOs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Outpoint() != "small:0" || got[1].Outpoint() != "dup:0" {
		t.Fatalf("unexpected order: %s, %s", got[0].Outpoint(), got[1].Outpoint())
	}
	if !got[1].Confirmed || got[1].Mempool {
		t.Fatalf("dedupe should prefer confirmed UTXO over mempool duplicate: %+v", got[1])
	}
}

func TestNormalizeUTXOsRejectsInvalidSort(t *testing.T) {
	_, err := NormalizeUTXOs(nil, UTXOOptions{Sort: "largest"})
	if err == nil {
		t.Fatal("expected invalid sort error")
	}
}
```

- [ ] **Step 2: Run the normalization tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(SatoshiToDecimalString|SignedSatoshiToDecimalString|WalletBalance|NormalizeUTXOs)' -v
```

Expected: FAIL because package `wallet` files and symbols do not exist yet.

- [ ] **Step 3: Add the wallet model types**

Create `wallet/model.go`:

```go
package wallet

import (
	"fmt"
	"strings"
)

const (
	maxInt64AsUint64 = uint64(1<<63) - 1
	minInt64Abs      = uint64(1 << 63)
	maxUint64Value   = ^uint64(0)
)

type Chain string

const (
	ChainBTC  Chain = "btc"
	ChainMVC  Chain = "mvc"
	ChainDOGE Chain = "doge"
)

type ResponseFormat string

const (
	FormatStandard ResponseFormat = "standard"
	FormatMetalet  ResponseFormat = "metalet"
)

type WalletBalance struct {
	Chain                 Chain
	Address               string
	ConfirmedSatoshi      uint64
	MempoolIncomeSatoshi  uint64
	MempoolSpendSatoshi   uint64
	UnsafeSatoshi         uint64
	ConfirmedUTXOCount    uint64
	MempoolUTXOCount      uint64
}

func (b WalletBalance) UnconfirmedSatoshi() int64 {
	return uint64DeltaToInt64(b.MempoolIncomeSatoshi, b.MempoolSpendSatoshi)
}

func (b WalletBalance) SafeSatoshi() uint64 {
	total := saturatingAddUint64(b.ConfirmedSatoshi, b.MempoolIncomeSatoshi)
	if total <= b.MempoolSpendSatoshi {
		return 0
	}
	total -= b.MempoolSpendSatoshi
	if total <= b.UnsafeSatoshi {
		return 0
	}
	return total - b.UnsafeSatoshi
}

func (b WalletBalance) UTXOCount() uint64 {
	return b.ConfirmedUTXOCount + b.MempoolUTXOCount
}

func uint64DeltaToInt64(income, spend uint64) int64 {
	if income >= spend {
		diff := income - spend
		if diff > maxInt64AsUint64 {
			return int64(maxInt64AsUint64)
		}
		return int64(diff)
	}
	diff := spend - income
	if diff >= minInt64Abs {
		return -1 << 63
	}
	return -int64(diff)
}

func saturatingAddUint64(a, b uint64) uint64 {
	if maxUint64Value-a < b {
		return maxUint64Value
	}
	return a + b
}

type WalletUTXO struct {
	Chain     Chain
	Address   string
	TxID      string
	Vout      int
	Satoshi   uint64
	Confirmed bool
	Mempool   bool
	Height    *int64
}

func (u WalletUTXO) Outpoint() string {
	return fmt.Sprintf("%s:%d", u.TxID, u.Vout)
}

type UTXOOptions struct {
	ConfirmedOnly bool
	Sort          string
}

func int64Ptr(value int64) *int64 {
	return &value
}

func NormalizeChain(raw string) (Chain, bool) {
	switch Chain(strings.ToLower(strings.TrimSpace(raw))) {
	case ChainBTC:
		return ChainBTC, true
	case ChainMVC:
		return ChainMVC, true
	case ChainDOGE:
		return ChainDOGE, true
	default:
		return "", false
	}
}

func NormalizeFormat(raw string) (ResponseFormat, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return FormatStandard, true
	}
	switch ResponseFormat(value) {
	case FormatStandard:
		return FormatStandard, true
	case FormatMetalet:
		return FormatMetalet, true
	default:
		return "", false
	}
}
```

- [ ] **Step 4: Add normalization helpers**

Create `wallet/normalize.go`:

```go
package wallet

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

func SatoshiToDecimalString(value uint64) string {
	if value == 0 {
		return "0"
	}
	whole := value / 100000000
	frac := value % 100000000
	return fmt.Sprintf("%d.%08d", whole, frac)
}

func SignedSatoshiToDecimalString(value int64) string {
	if value == -1<<63 {
		return "-92233720368.54775808"
	}
	if value < 0 {
		return "-" + SatoshiToDecimalString(uint64(-value))
	}
	return SatoshiToDecimalString(uint64(value))
}

func NormalizeUTXOs(input []WalletUTXO, opts UTXOOptions) ([]WalletUTXO, error) {
	sortOrder := opts.Sort
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "sort must be asc or desc")
	}

	byOutpoint := make(map[string]WalletUTXO, len(input))
	order := make([]string, 0, len(input))
	for _, utxo := range input {
		if opts.ConfirmedOnly && !utxo.Confirmed {
			continue
		}
		key := utxo.Outpoint()
		existing, exists := byOutpoint[key]
		if !exists {
			order = append(order, key)
			byOutpoint[key] = utxo
			continue
		}
		if !existing.Confirmed && utxo.Confirmed {
			byOutpoint[key] = utxo
		}
	}

	result := make([]WalletUTXO, 0, len(byOutpoint))
	for _, key := range order {
		if utxo, ok := byOutpoint[key]; ok {
			result = append(result, utxo)
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Satoshi == result[j].Satoshi {
			return result[i].Outpoint() < result[j].Outpoint()
		}
		if sortOrder == "asc" {
			return result[i].Satoshi < result[j].Satoshi
		}
		return result[i].Satoshi > result[j].Satoshi
	})
	return result, nil
}

func parseVout(value string) (int, error) {
	vout, err := strconv.Atoi(value)
	if err != nil {
		return 0, NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "upstream utxo index is not an integer")
	}
	return vout, nil
}
```

- [ ] **Step 5: Add minimal errors needed by normalization**

Create `wallet/errors.go` with the initial error definitions used above:

```go
package wallet

import (
	"fmt"
	"net/http"
)

const (
	CodeSuccess          = 0
	CodeUnsupportedChain = -4001
	CodeInvalidAddress   = -4002
	CodeInvalidQuery     = -4003
	CodeCoreUnavailable  = -5001
	CodeInvalidUpstream  = -5002
	CodeInternal         = -5003
)

type WalletError struct {
	Code       int
	Message    string
	HTTPStatus int
}

func (e *WalletError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func NewWalletError(code int, message string) *WalletError {
	return NewHTTPWalletError(http.StatusInternalServerError, code, message)
}

func NewHTTPWalletError(status int, code int, message string) *WalletError {
	return &WalletError{Code: code, Message: message, HTTPStatus: status}
}
```

- [ ] **Step 6: Run the normalization tests and verify they pass**

Run:

```bash
go test ./wallet -run 'Test(SatoshiToDecimalString|SignedSatoshiToDecimalString|WalletBalance|NormalizeUTXOs)' -v
```

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

```bash
git add wallet/model.go wallet/normalize.go wallet/errors.go wallet/normalize_test.go
git commit -m "feat: add wallet normalization models"
```

---

### Task 2: Add the Higun core HTTP client

**Files:**
- Create: `wallet/client.go`
- Create: `wallet/client_test.go`
- Create: `wallet/logging.go`
- Create: `wallet/logging_test.go`

- [ ] **Step 1: Write failing client tests**

Create `wallet/client_test.go`:

```go
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
```

- [ ] **Step 2: Write failing logging helper tests**

Create `wallet/logging_test.go`:

```go
package wallet

import "testing"

func TestTruncateAddressForLog(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "short", in: "short", want: "short"},
		{name: "btc", in: "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ", want: "12ghVW...nMUikZ"},
		{name: "doge", in: "DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L", want: "DH5yai...R3mr7L"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateAddressForLog(tt.in); got != tt.want {
				t.Fatalf("truncateAddressForLog(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run the client tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(CoreClient|TruncateAddress)' -v
```

Expected: FAIL because `NewCoreClient`, `FetchBalance`, `FetchUTXOs`, and `truncateAddressForLog` do not exist.

- [ ] **Step 4: Implement the core HTTP client**

Create `wallet/client.go`:

```go
package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CoreClient struct {
	baseURL string
	client  *http.Client
}

type coreBalanceResponse struct {
	ConfirmedBalanceSatoshi uint64 `json:"confirmed_balance_satoshi"`
	BalanceSatoshi          uint64 `json:"balance_satoshi"`
	ConfirmedUTXOCount      uint64 `json:"confirmed_utxo_count"`
	MempoolIncomeSatoshi    uint64 `json:"mempool_income_satoshi"`
	MempoolSpendSatoshi     uint64 `json:"mempool_spend_satoshi"`
	MempoolUTXOCount        uint64 `json:"mempool_utxo_count"`
	UnsafeFeeSatoshi        uint64 `json:"unsafe_fee_satoshi"`
}

type coreUTXOResponse struct {
	Address string         `json:"address"`
	UTXOs   []coreUTXOItem `json:"utxos"`
	Count   int            `json:"count"`
}

type coreUTXOItem struct {
	TxID      string `json:"tx_id"`
	Index     string `json:"index"`
	Amount    uint64 `json:"amount"`
	IsMempool bool   `json:"is_mempool"`
	Height    *int64 `json:"height,omitempty"`
}

func NewCoreClient(baseURL string, timeout time.Duration) (*CoreClient, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "core_url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "core_url must be an absolute URL")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &CoreClient{
		baseURL: trimmed,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *CoreClient) FetchBalance(ctx context.Context, chain Chain, address string) (WalletBalance, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/balance", nil)
	if err != nil {
		return WalletBalance{}, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, err.Error())
	}
	query := req.URL.Query()
	query.Set("address", address)
	req.URL.RawQuery = query.Encode()

	var payload coreBalanceResponse
	if err := c.getJSON(req, &payload); err != nil {
		return WalletBalance{}, err
	}
	return WalletBalance{
		Chain:                 chain,
		Address:               address,
		ConfirmedSatoshi:      payload.ConfirmedBalanceSatoshi,
		MempoolIncomeSatoshi:  payload.MempoolIncomeSatoshi,
		MempoolSpendSatoshi:   payload.MempoolSpendSatoshi,
		UnsafeSatoshi:         payload.UnsafeFeeSatoshi,
		ConfirmedUTXOCount:    payload.ConfirmedUTXOCount,
		MempoolUTXOCount:      payload.MempoolUTXOCount,
	}, nil
}

func (c *CoreClient) FetchUTXOs(ctx context.Context, chain Chain, address string) ([]WalletUTXO, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/utxos", nil)
	if err != nil {
		return nil, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, err.Error())
	}
	query := req.URL.Query()
	query.Set("address", address)
	req.URL.RawQuery = query.Encode()

	var payload coreUTXOResponse
	if err := c.getJSON(req, &payload); err != nil {
		return nil, err
	}
	out := make([]WalletUTXO, 0, len(payload.UTXOs))
	for _, item := range payload.UTXOs {
		vout, err := parseVout(item.Index)
		if err != nil {
			return nil, err
		}
		height := item.Height
		if item.IsMempool && height == nil {
			height = int64Ptr(-1)
		}
		out = append(out, WalletUTXO{
			Chain:     chain,
			Address:   address,
			TxID:      item.TxID,
			Vout:      vout,
			Satoshi:   item.Amount,
			Confirmed: !item.IsMempool,
			Mempool:   item.IsMempool,
			Height:    height,
		})
	}
	return out, nil
}

func (c *CoreClient) getJSON(req *http.Request, out any) error {
	start := time.Now()
	status := 0
	resp, err := c.client.Do(req)
	if err != nil {
		logWalletUpstream(req, status, start, err.Error())
		return NewHTTPWalletError(http.StatusBadGateway, CodeCoreUnavailable, err.Error())
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errMessage := fmt.Sprintf("core returned HTTP %d", resp.StatusCode)
		logWalletUpstream(req, status, start, errMessage)
		return NewHTTPWalletError(http.StatusBadGateway, CodeCoreUnavailable, errMessage)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		logWalletUpstream(req, status, start, err.Error())
		return NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, err.Error())
	}
	logWalletUpstream(req, status, start, "")
	return nil
}
```

- [ ] **Step 5: Add upstream logging helpers**

Create `wallet/logging.go`:

```go
package wallet

import (
	"log"
	"net/http"
	"time"
)

func logWalletUpstream(req *http.Request, status int, start time.Time, errMessage string) {
	if req == nil || req.URL == nil {
		return
	}
	duration := time.Since(start).Milliseconds()
	address := truncateAddressForLog(req.URL.Query().Get("address"))
	if errMessage != "" {
		log.Printf("wallet upstream request host=%s path=%s status=%d duration_ms=%d address=%s error=%q", req.URL.Host, req.URL.Path, status, duration, address, errMessage)
		return
	}
	log.Printf("wallet upstream request host=%s path=%s status=%d duration_ms=%d address=%s", req.URL.Host, req.URL.Path, status, duration, address)
}

func truncateAddressForLog(address string) string {
	if len(address) <= 12 {
		return address
	}
	return address[:6] + "..." + address[len(address)-6:]
}
```

- [ ] **Step 6: Run wallet client tests**

Run:

```bash
go test ./wallet -run 'Test(CoreClient|TruncateAddress)' -v
```

Expected: PASS.

- [ ] **Step 7: Run all wallet tests**

Run:

```bash
go test ./wallet -v
```

Expected: PASS.

- [ ] **Step 8: Commit Task 2**

```bash
git add wallet/client.go wallet/client_test.go wallet/logging.go wallet/logging_test.go
git commit -m "feat: add wallet core client"
```

---

### Task 3: Add standard and Metalet-compatible response rendering

**Files:**
- Create: `wallet/response.go`
- Create: `wallet/metalet_compat.go`
- Create: `wallet/response_test.go`
- Create: `wallet/testdata/metalet_btc_balance.json`
- Create: `wallet/testdata/metalet_mvc_balance.json`
- Create: `wallet/testdata/metalet_doge_balance.json`

- [ ] **Step 1: Write failing response tests and balance golden fixtures**

Create `wallet/response_test.go`:

```go
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
		Chain:                 ChainBTC,
		Address:               "addr",
		ConfirmedSatoshi:      135758,
		MempoolIncomeSatoshi:  10,
		MempoolSpendSatoshi:   3,
		UnsafeSatoshi:         134862,
		ConfirmedUTXOCount:    1040,
		MempoolUTXOCount:      2,
	}
	resp := NewStandardBalanceResponse(balance)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{`"balance":0.`, `"safeBalance":0.`, `"unSafeBalance":0.`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("standard response must not expose float amount fields: %s", body)
		}
	}
	if !strings.Contains(body, `"confirmed":"0.00135758"`) {
		t.Fatalf("standard response should include decimal string: %s", body)
	}
	if !strings.Contains(body, `"confirmedSatoshi":135758`) {
		t.Fatalf("standard response should include satoshi integer: %s", body)
	}
}

func TestStandardUTXOResponse(t *testing.T) {
	resp := NewStandardUTXOResponse(ChainBTC, "addr", false, "desc", []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "tx", Vout: 1, Satoshi: 1000, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	})
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"outpoint":"tx:1"`) {
		t.Fatalf("missing outpoint: %s", body)
	}
	if !strings.Contains(body, `"amount":"0.00001000"`) {
		t.Fatalf("missing decimal amount string: %s", body)
	}
	if !strings.Contains(body, `"mempool":true`) || !strings.Contains(body, `"confirmed":false`) {
		t.Fatalf("missing mempool flags: %s", body)
	}
}

func TestStandardUTXOResponseOmitsMissingHeight(t *testing.T) {
	resp := NewStandardUTXOResponse(ChainBTC, "addr", false, "desc", []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "tx", Vout: 1, Satoshi: 1000, Confirmed: true, Mempool: false},
	})
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(raw)
	if strings.Contains(body, `"height"`) {
		t.Fatalf("height should be omitted when upstream does not provide it: %s", body)
	}
}

func TestMetaletBTCBalanceCompatibility(t *testing.T) {
	resp := NewMetaletBTCBalanceResponse(WalletBalance{
		Chain:                 ChainBTC,
		Address:               "addr",
		ConfirmedSatoshi:      135758,
		MempoolIncomeSatoshi:  0,
		MempoolSpendSatoshi:   0,
		UnsafeSatoshi:         134862,
		ConfirmedUTXOCount:    1040,
		MempoolUTXOCount:      0,
	})
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"balance":0.00135758`) {
		t.Fatalf("missing Metalet balance float: %s", body)
	}
	if !strings.Contains(body, `"safeBalance":0.00000896`) {
		t.Fatalf("missing Metalet safeBalance float: %s", body)
	}
	if !strings.Contains(body, `"unSafeBalance":0.00134862`) {
		t.Fatalf("missing Metalet unSafeBalance float: %s", body)
	}
}

func TestMetaletMVCDOGEBalanceCompatibility(t *testing.T) {
	resp := NewMetaletMVCDOGEBalanceResponse(WalletBalance{
		Chain:                 ChainDOGE,
		Address:               "doge-addr",
		ConfirmedSatoshi:      6187007540823,
		MempoolIncomeSatoshi:  500,
		MempoolSpendSatoshi:   100,
		ConfirmedUTXOCount:    1475,
		MempoolUTXOCount:      1,
	})
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"address":"doge-addr"`) {
		t.Fatalf("missing address: %s", body)
	}
	if !strings.Contains(body, `"confirmed":6187007540823`) {
		t.Fatalf("missing confirmed satoshi: %s", body)
	}
	if !strings.Contains(body, `"unconfirmed":400`) {
		t.Fatalf("missing unconfirmed satoshi: %s", body)
	}
	if !strings.Contains(body, `"utxoCount":1476`) {
		t.Fatalf("missing combined utxo count: %s", body)
	}
}

func TestMetaletBalanceCompatibilityGoldenFixtures(t *testing.T) {
	cases := []struct {
		name string
		file string
		resp Envelope
	}{
		{
			name: "btc",
			file: "testdata/metalet_btc_balance.json",
			resp: NewMetaletBTCBalanceResponse(WalletBalance{
				Chain:              ChainBTC,
				Address:            "addr",
				ConfirmedSatoshi:   135758,
				UnsafeSatoshi:      134862,
				ConfirmedUTXOCount: 1040,
			}),
		},
		{
			name: "mvc",
			file: "testdata/metalet_mvc_balance.json",
			resp: NewMetaletMVCDOGEBalanceResponse(WalletBalance{
				Chain:              ChainMVC,
				Address:            "mvc-addr",
				ConfirmedSatoshi:   135758,
				ConfirmedUTXOCount: 1040,
			}),
		},
		{
			name: "doge",
			file: "testdata/metalet_doge_balance.json",
			resp: NewMetaletMVCDOGEBalanceResponse(WalletBalance{
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
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONFixture(t, tt.file, tt.resp)
		})
	}
}

func assertJSONFixture(t *testing.T, file string, got any) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	wantRaw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", file, err)
	}
	var gotCompact bytes.Buffer
	var wantCompact bytes.Buffer
	if err := json.Compact(&gotCompact, gotRaw); err != nil {
		t.Fatalf("compact got: %v", err)
	}
	if err := json.Compact(&wantCompact, wantRaw); err != nil {
		t.Fatalf("compact fixture: %v", err)
	}
	if gotCompact.String() != wantCompact.String() {
		t.Fatalf("JSON mismatch for %s\n got: %s\nwant: %s", file, gotCompact.String(), wantCompact.String())
	}
}
```

Create `wallet/testdata/metalet_btc_balance.json`:

```json
{"code":0,"message":"success","data":{"balance":0.00135758,"block":{"incomeFee":0.00135758,"spendFee":0},"mempool":{"incomeFee":0,"spendFee":0},"pendingBalance":0,"safeBalance":0.00000896,"unSafeBalance":0.00134862,"inscriptionsBalance":0,"runesBalance":0,"pinsBalance":0,"mrc20UtxosBalance":0}}
```

Create `wallet/testdata/metalet_mvc_balance.json`:

```json
{"code":0,"message":"success","data":{"address":"mvc-addr","confirmed":135758,"unconfirmed":0,"utxoCount":1040}}
```

Create `wallet/testdata/metalet_doge_balance.json`:

```json
{"code":0,"message":"success","data":{"address":"doge-addr","confirmed":6187007540823,"unconfirmed":400,"utxoCount":1476}}
```

- [ ] **Step 2: Run response tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(Standard|Metalet)' -v
```

Expected: FAIL because response rendering functions do not exist.

- [ ] **Step 3: Implement standard response rendering**

Create `wallet/response.go`:

```go
package wallet

type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type StandardBalanceData struct {
	Chain                 string `json:"chain"`
	Address               string `json:"address"`
	ConfirmedSatoshi      uint64 `json:"confirmedSatoshi"`
	UnconfirmedSatoshi    int64  `json:"unconfirmedSatoshi"`
	MempoolIncomeSatoshi  uint64 `json:"mempoolIncomeSatoshi"`
	MempoolSpendSatoshi   uint64 `json:"mempoolSpendSatoshi"`
	UnsafeSatoshi         uint64 `json:"unsafeSatoshi"`
	SafeSatoshi           uint64 `json:"safeSatoshi"`
	UTXOCount             uint64 `json:"utxoCount"`
	Confirmed             string `json:"confirmed"`
	Unconfirmed           string `json:"unconfirmed"`
	MempoolIncome         string `json:"mempoolIncome"`
	MempoolSpend          string `json:"mempoolSpend"`
	Unsafe                string `json:"unsafe"`
	Safe                  string `json:"safe"`
}

type StandardUTXOData struct {
	Chain         string             `json:"chain"`
	Address       string             `json:"address"`
	ConfirmedOnly bool               `json:"confirmedOnly"`
	Sort          string             `json:"sort"`
	Total         int                `json:"total"`
	UTXOs         []StandardUTXOItem `json:"utxos"`
}

type StandardUTXOItem struct {
	TxID      string `json:"txid"`
	Vout      int    `json:"vout"`
	Outpoint  string `json:"outpoint"`
	Satoshi   uint64 `json:"satoshi"`
	Amount    string `json:"amount"`
	Confirmed bool   `json:"confirmed"`
	Mempool   bool   `json:"mempool"`
	Height    *int64 `json:"height,omitempty"`
}

func Success(data any) Envelope {
	return Envelope{Code: CodeSuccess, Message: "success", Data: data}
}

func ErrorEnvelope(err *WalletError) Envelope {
	return Envelope{Code: err.Code, Message: err.Message, Data: nil}
}

func NewStandardBalanceResponse(balance WalletBalance) Envelope {
	unconfirmed := balance.UnconfirmedSatoshi()
	safe := balance.SafeSatoshi()
	return Success(StandardBalanceData{
		Chain:                string(balance.Chain),
		Address:              balance.Address,
		ConfirmedSatoshi:     balance.ConfirmedSatoshi,
		UnconfirmedSatoshi:   unconfirmed,
		MempoolIncomeSatoshi: balance.MempoolIncomeSatoshi,
		MempoolSpendSatoshi:  balance.MempoolSpendSatoshi,
		UnsafeSatoshi:        balance.UnsafeSatoshi,
		SafeSatoshi:          safe,
		UTXOCount:            balance.UTXOCount(),
		Confirmed:            SatoshiToDecimalString(balance.ConfirmedSatoshi),
		Unconfirmed:          SignedSatoshiToDecimalString(unconfirmed),
		MempoolIncome:        SatoshiToDecimalString(balance.MempoolIncomeSatoshi),
		MempoolSpend:         SatoshiToDecimalString(balance.MempoolSpendSatoshi),
		Unsafe:               SatoshiToDecimalString(balance.UnsafeSatoshi),
		Safe:                 SatoshiToDecimalString(safe),
	})
}

func NewStandardUTXOResponse(chain Chain, address string, confirmedOnly bool, sortOrder string, utxos []WalletUTXO) Envelope {
	items := make([]StandardUTXOItem, 0, len(utxos))
	for _, utxo := range utxos {
		items = append(items, StandardUTXOItem{
			TxID:      utxo.TxID,
			Vout:      utxo.Vout,
			Outpoint:  utxo.Outpoint(),
			Satoshi:   utxo.Satoshi,
			Amount:    SatoshiToDecimalString(utxo.Satoshi),
			Confirmed: utxo.Confirmed,
			Mempool:   utxo.Mempool,
			Height:    utxo.Height,
		})
	}
	return Success(StandardUTXOData{
		Chain:         string(chain),
		Address:       address,
		ConfirmedOnly: confirmedOnly,
		Sort:          sortOrder,
		Total:         len(items),
		UTXOs:         items,
	})
}
```

- [ ] **Step 4: Implement Metalet compatibility rendering**

Create `wallet/metalet_compat.go`:

```go
package wallet

type metaletBTCBalanceData struct {
	Balance             float64                  `json:"balance"`
	Block               metaletBalanceBlock      `json:"block"`
	Mempool             metaletBalanceMempool    `json:"mempool"`
	PendingBalance      float64                  `json:"pendingBalance"`
	SafeBalance         float64                  `json:"safeBalance"`
	UnSafeBalance       float64                  `json:"unSafeBalance"`
	InscriptionsBalance float64                  `json:"inscriptionsBalance"`
	RunesBalance        float64                  `json:"runesBalance"`
	PinsBalance         float64                  `json:"pinsBalance"`
	Mrc20UtxosBalance   float64                  `json:"mrc20UtxosBalance"`
}

type metaletBalanceBlock struct {
	IncomeFee float64 `json:"incomeFee"`
	SpendFee  float64 `json:"spendFee"`
}

type metaletBalanceMempool struct {
	IncomeFee float64 `json:"incomeFee"`
	SpendFee  float64 `json:"spendFee"`
}

type metaletMVCDOGEBalanceData struct {
	Address     string `json:"address"`
	Confirmed   uint64 `json:"confirmed"`
	Unconfirmed int64  `json:"unconfirmed"`
	UTXOCount   uint64 `json:"utxoCount"`
}

func NewMetaletBTCBalanceResponse(balance WalletBalance) Envelope {
	return Success(metaletBTCBalanceData{
		Balance:        uint64ToFloatBTC(balance.ConfirmedSatoshi),
		Block:          metaletBalanceBlock{IncomeFee: uint64ToFloatBTC(balance.ConfirmedSatoshi), SpendFee: 0},
		Mempool:        metaletBalanceMempool{IncomeFee: uint64ToFloatBTC(balance.MempoolIncomeSatoshi), SpendFee: uint64ToFloatBTC(balance.MempoolSpendSatoshi)},
		PendingBalance: float64(balance.UnconfirmedSatoshi()) / 1e8,
		SafeBalance:    uint64ToFloatBTC(balance.SafeSatoshi()),
		UnSafeBalance:  uint64ToFloatBTC(balance.UnsafeSatoshi),
	})
}

func uint64ToFloatBTC(value uint64) float64 {
	return float64(value) / 1e8
}

func NewMetaletMVCDOGEBalanceResponse(balance WalletBalance) Envelope {
	return Success(metaletMVCDOGEBalanceData{
		Address:     balance.Address,
		Confirmed:   balance.ConfirmedSatoshi,
		Unconfirmed: balance.UnconfirmedSatoshi(),
		UTXOCount:   balance.UTXOCount(),
	})
}
```

- [ ] **Step 5: Run response tests**

Run:

```bash
go test ./wallet -run 'Test(Standard|Metalet)' -v
```

Expected: PASS.

- [ ] **Step 6: Run all wallet tests**

Run:

```bash
go test ./wallet -v
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

```bash
git add wallet/response.go wallet/metalet_compat.go wallet/response_test.go wallet/testdata/metalet_btc_balance.json wallet/testdata/metalet_mvc_balance.json wallet/testdata/metalet_doge_balance.json
git commit -m "feat: add wallet response rendering"
```

---

### Task 4: Add gateway service, handlers, and route tests

**Files:**
- Create: `wallet/server.go`
- Create: `wallet/handlers.go`
- Create: `wallet/handlers_test.go`
- Modify: `wallet/response_test.go`
- Create: `wallet/testdata/metalet_utxos.json`

- [ ] **Step 1: Write failing handler tests**

Create `wallet/handlers_test.go`:

```go
package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeWalletService struct {
	balance WalletBalance
	utxos   []WalletUTXO
	err     error
}

func (s fakeWalletService) GetBalance(ctx context.Context, chain Chain, address string) (WalletBalance, error) {
	if s.err != nil {
		return WalletBalance{}, s.err
	}
	out := s.balance
	out.Chain = chain
	out.Address = address
	return out, nil
}

func (s fakeWalletService) GetUTXOs(ctx context.Context, chain Chain, address string) ([]WalletUTXO, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]WalletUTXO, len(s.utxos))
	copy(out, s.utxos)
	for i := range out {
		out[i].Chain = chain
		out[i].Address = address
	}
	return out, nil
}

func newHandlerTestRouter(service WalletService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, NewGateway(service))
	return router
}

func TestBalanceHandlerStandard(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{balance: WalletBalance{
		ConfirmedSatoshi:     135758,
		UnsafeSatoshi:        134862,
		ConfirmedUTXOCount:   1040,
		MempoolUTXOCount:     0,
		MempoolIncomeSatoshi: 0,
		MempoolSpendSatoshi:  0,
	}})
	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr-btc/balance", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if resp.Code != CodeSuccess {
		t.Fatalf("code = %d, want 0", resp.Code)
	}
	if !strings.Contains(rec.Body.String(), `"safeSatoshi":896`) {
		t.Fatalf("missing safeSatoshi: %s", rec.Body.String())
	}
}

func TestBalanceHandlerMetalet(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{balance: WalletBalance{
		ConfirmedSatoshi: 135758,
		UnsafeSatoshi:    134862,
	}})
	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr-btc/balance?format=metalet", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"safeBalance":0.00000896`) {
		t.Fatalf("missing Metalet safeBalance: %s", rec.Body.String())
	}
}

func TestUTXOHandlerDefaultsToMempoolIncluded(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{utxos: []WalletUTXO{
		{TxID: "mem", Vout: 0, Satoshi: 300, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
		{TxID: "conf", Vout: 0, Satoshi: 100, Confirmed: true, Mempool: false, Height: int64Ptr(10)},
	}})
	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr-btc/utxos", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"total":2`) || !strings.Contains(body, `"mempool":true`) {
		t.Fatalf("default response should include mempool UTXO: %s", body)
	}
	if strings.Index(body, `"txid":"mem"`) > strings.Index(body, `"txid":"conf"`) {
		t.Fatalf("default desc sort should put larger mempool UTXO first: %s", body)
	}
}

func TestUTXOHandlerConfirmedOnly(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{utxos: []WalletUTXO{
		{TxID: "mem", Vout: 0, Satoshi: 300, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
		{TxID: "conf", Vout: 0, Satoshi: 100, Confirmed: true, Mempool: false, Height: int64Ptr(10)},
	}})
	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr-btc/utxos?confirmedOnly=true", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"total":1`) || strings.Contains(body, `"txid":"mem"`) {
		t.Fatalf("confirmedOnly response should exclude mempool UTXO: %s", body)
	}
}

func TestHandlerRejectsInvalidChainAndFormat(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{})

	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/ltc/address/addr/balance", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "unsupported chain") {
		t.Fatalf("invalid chain status/body = %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr/balance?format=legacy", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "format must be standard or metalet") {
		t.Fatalf("invalid format status/body = %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerMapsCoreUnavailableToBadGateway(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{err: NewHTTPWalletError(http.StatusBadGateway, CodeCoreUnavailable, "core unavailable")})
	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr/balance", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayServiceDisabledChainReturnsServiceUnavailable(t *testing.T) {
	service, err := NewGatewayService(Config{
		Timeout: 2 * time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC: {Enabled: false, CoreURL: ""},
		},
	})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}
	_, err = service.GetBalance(context.Background(), ChainBTC, "addr")
	var walletErr *WalletError
	if !errors.As(err, &walletErr) {
		t.Fatalf("expected WalletError, got %T %v", err, err)
	}
	if walletErr.HTTPStatus != http.StatusServiceUnavailable || walletErr.Code != CodeCoreUnavailable {
		t.Fatalf("unexpected disabled chain error: %+v", walletErr)
	}
}

func TestHandlerMapsPlainErrorToInternalError(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{err: errors.New("plain error")})
	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr/balance", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGatewayServiceUsesConfiguredCoreClient(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{"confirmed_balance_satoshi": uint64(1)})
		case "/utxos":
			_ = json.NewEncoder(w).Encode(map[string]any{"address": "addr", "utxos": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer core.Close()

	service, err := NewGatewayService(Config{
		Timeout: 2 * time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC: {Enabled: true, CoreURL: core.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}
	got, err := service.GetBalance(context.Background(), ChainBTC, "addr")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if got.ConfirmedSatoshi != 1 {
		t.Fatalf("ConfirmedSatoshi = %d, want 1", got.ConfirmedSatoshi)
	}
}

func TestGatewayServiceRoutesEachChainToConfiguredCore(t *testing.T) {
	newCore := func(value uint64, hits *atomic.Int64) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			if r.URL.Path != "/balance" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"confirmed_balance_satoshi": value})
		}))
	}

	var btcHits atomic.Int64
	var mvcHits atomic.Int64
	var dogeHits atomic.Int64
	btcCore := newCore(1, &btcHits)
	mvcCore := newCore(2, &mvcHits)
	dogeCore := newCore(3, &dogeHits)
	defer btcCore.Close()
	defer mvcCore.Close()
	defer dogeCore.Close()

	service, err := NewGatewayService(Config{
		Timeout: 2 * time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC:  {Enabled: true, CoreURL: btcCore.URL},
			ChainMVC:  {Enabled: true, CoreURL: mvcCore.URL},
			ChainDOGE: {Enabled: true, CoreURL: dogeCore.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}

	for chain, want := range map[Chain]uint64{ChainBTC: 1, ChainMVC: 2, ChainDOGE: 3} {
		got, err := service.GetBalance(context.Background(), chain, "addr")
		if err != nil {
			t.Fatalf("%s GetBalance: %v", chain, err)
		}
		if got.ConfirmedSatoshi != want {
			t.Fatalf("%s ConfirmedSatoshi = %d, want %d", chain, got.ConfirmedSatoshi, want)
		}
	}

	if btcHits.Load() != 1 || mvcHits.Load() != 1 || dogeHits.Load() != 1 {
		t.Fatalf("unexpected core hit counts: btc=%d mvc=%d doge=%d", btcHits.Load(), mvcHits.Load(), dogeHits.Load())
	}
}

func TestGatewayServiceRoutesEachChainUTXOsToConfiguredCore(t *testing.T) {
	newCore := func(value uint64, hits *atomic.Int64) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			if r.URL.Path != "/utxos" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"address": "addr",
				"utxos": []map[string]any{
					{"tx_id": "tx", "index": "0", "amount": value, "is_mempool": false},
				},
			})
		}))
	}

	var btcHits atomic.Int64
	var mvcHits atomic.Int64
	var dogeHits atomic.Int64
	btcCore := newCore(11, &btcHits)
	mvcCore := newCore(22, &mvcHits)
	dogeCore := newCore(33, &dogeHits)
	defer btcCore.Close()
	defer mvcCore.Close()
	defer dogeCore.Close()

	service, err := NewGatewayService(Config{
		Timeout: 2 * time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC:  {Enabled: true, CoreURL: btcCore.URL},
			ChainMVC:  {Enabled: true, CoreURL: mvcCore.URL},
			ChainDOGE: {Enabled: true, CoreURL: dogeCore.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}

	for chain, want := range map[Chain]uint64{ChainBTC: 11, ChainMVC: 22, ChainDOGE: 33} {
		got, err := service.GetUTXOs(context.Background(), chain, "addr")
		if err != nil {
			t.Fatalf("%s GetUTXOs: %v", chain, err)
		}
		if len(got) != 1 || got[0].Satoshi != want {
			t.Fatalf("%s UTXOs = %+v, want one amount %d", chain, got, want)
		}
	}

	if btcHits.Load() != 1 || mvcHits.Load() != 1 || dogeHits.Load() != 1 {
		t.Fatalf("unexpected UTXO core hit counts: btc=%d mvc=%d doge=%d", btcHits.Load(), mvcHits.Load(), dogeHits.Load())
	}
}
```

- [ ] **Step 2: Run handler tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(BalanceHandler|UTXOHandler|Handler|GatewayService)' -v
```

Expected: FAIL because `WalletService`, `Gateway`, `RegisterRoutes`, and gateway handlers do not exist.

- [ ] **Step 3: Implement gateway service and route registration**

Create `wallet/server.go`:

```go
package wallet

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Config struct {
	Enabled bool
	Timeout time.Duration
	Chains  map[Chain]ChainConfig
}

type ChainConfig struct {
	Enabled bool
	CoreURL string
}

type WalletService interface {
	GetBalance(ctx context.Context, chain Chain, address string) (WalletBalance, error)
	GetUTXOs(ctx context.Context, chain Chain, address string) ([]WalletUTXO, error)
}

type GatewayService struct {
	clients map[Chain]*CoreClient
}

type Gateway struct {
	service WalletService
}

func NewGateway(service WalletService) *Gateway {
	return &Gateway{service: service}
}

func NewGatewayService(cfg Config) (*GatewayService, error) {
	clients := make(map[Chain]*CoreClient)
	for chain, chainCfg := range cfg.Chains {
		if !chainCfg.Enabled {
			continue
		}
		client, err := NewCoreClient(chainCfg.CoreURL, cfg.Timeout)
		if err != nil {
			return nil, err
		}
		clients[chain] = client
	}
	return &GatewayService{clients: clients}, nil
}

func (s *GatewayService) GetBalance(ctx context.Context, chain Chain, address string) (WalletBalance, error) {
	client, ok := s.clients[chain]
	if !ok {
		return WalletBalance{}, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "chain core is not configured")
	}
	return client.FetchBalance(ctx, chain, address)
}

func (s *GatewayService) GetUTXOs(ctx context.Context, chain Chain, address string) ([]WalletUTXO, error) {
	client, ok := s.clients[chain]
	if !ok {
		return nil, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "chain core is not configured")
	}
	return client.FetchUTXOs(ctx, chain, address)
}

func RegisterRoutes(router gin.IRouter, gateway *Gateway) {
	group := router.Group("/wallet/v1")
	group.GET("/:chain/address/:address/balance", gateway.getBalance)
	group.GET("/:chain/address/:address/utxos", gateway.getUTXOs)
}
```

- [ ] **Step 4: Implement handlers and error mapping**

Create `wallet/handlers.go`:

```go
package wallet

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func (g *Gateway) getBalance(c *gin.Context) {
	chain, format, ok := g.parseCommon(c)
	if !ok {
		return
	}
	address := strings.TrimSpace(c.Param("address"))
	if address == "" {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidAddress, "address is required"))
		return
	}
	balance, err := g.service.GetBalance(c.Request.Context(), chain, address)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	if format == FormatMetalet {
		if chain == ChainBTC {
			c.JSON(http.StatusOK, NewMetaletBTCBalanceResponse(balance))
			return
		}
		c.JSON(http.StatusOK, NewMetaletMVCDOGEBalanceResponse(balance))
		return
	}
	c.JSON(http.StatusOK, NewStandardBalanceResponse(balance))
}

func (g *Gateway) getUTXOs(c *gin.Context) {
	chain, format, ok := g.parseCommon(c)
	if !ok {
		return
	}
	address := strings.TrimSpace(c.Param("address"))
	if address == "" {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidAddress, "address is required"))
		return
	}
	confirmedOnly, err := parseBoolDefault(c.Query("confirmedOnly"), false)
	if err != nil {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "confirmedOnly must be true or false"))
		return
	}
	sortOrder := strings.TrimSpace(c.DefaultQuery("sort", "desc"))
	utxos, err := g.service.GetUTXOs(c.Request.Context(), chain, address)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	normalized, err := NormalizeUTXOs(utxos, UTXOOptions{ConfirmedOnly: confirmedOnly, Sort: sortOrder})
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	if format == FormatMetalet {
		c.JSON(http.StatusOK, NewMetaletUTXOResponse(chain, normalized))
		return
	}
	c.JSON(http.StatusOK, NewStandardUTXOResponse(chain, address, confirmedOnly, sortOrder, normalized))
}

func (g *Gateway) parseCommon(c *gin.Context) (Chain, ResponseFormat, bool) {
	chain, ok := NormalizeChain(c.Param("chain"))
	if !ok {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusNotFound, CodeUnsupportedChain, "unsupported chain"))
		return "", "", false
	}
	format, ok := NormalizeFormat(c.Query("format"))
	if !ok {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "format must be standard or metalet"))
		return "", "", false
	}
	return chain, format, true
}

func (g *Gateway) writeServiceError(c *gin.Context, err error) {
	var walletErr *WalletError
	if errors.As(err, &walletErr) {
		if walletErr.HTTPStatus == 0 {
			walletErr.HTTPStatus = http.StatusInternalServerError
		}
		g.writeWalletError(c, walletErr)
		return
	}
	g.writeWalletError(c, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, err.Error()))
}

func (g *Gateway) writeWalletError(c *gin.Context, err *WalletError) {
	if err.HTTPStatus == 0 {
		err.HTTPStatus = http.StatusInternalServerError
	}
	c.JSON(err.HTTPStatus, ErrorEnvelope(err))
}

func parseBoolDefault(raw string, defaultValue bool) (bool, error) {
	if raw == "" {
		return defaultValue, nil
	}
	return strconv.ParseBool(raw)
}
```

- [ ] **Step 5: Add UTXO compatibility renderer used by handlers**

Append this to `wallet/metalet_compat.go`:

```go
type metaletUTXOItem struct {
	TxID         string `json:"txId"`
	Vout         int    `json:"vout"`
	Satoshi      uint64 `json:"satoshi"`
	Confirmed    bool   `json:"confirmed"`
	Inscriptions []any  `json:"inscriptions"`
}

func NewMetaletUTXOResponse(chain Chain, utxos []WalletUTXO) Envelope {
	items := make([]metaletUTXOItem, 0, len(utxos))
	for _, utxo := range utxos {
		items = append(items, metaletUTXOItem{
			TxID:         utxo.TxID,
			Vout:         utxo.Vout,
			Satoshi:      utxo.Satoshi,
			Confirmed:    utxo.Confirmed,
			Inscriptions: []any{},
		})
	}
	return Success(items)
}
```

- [ ] **Step 6: Add UTXO Metalet golden fixture test**

Append to `wallet/response_test.go`:

```go
func TestMetaletUTXOCompatibilityGoldenFixture(t *testing.T) {
	resp := NewMetaletUTXOResponse(ChainBTC, []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "tx", Vout: 1, Satoshi: 1000, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	})
	assertJSONFixture(t, "testdata/metalet_utxos.json", resp)
}
```

Create `wallet/testdata/metalet_utxos.json`:

```json
{"code":0,"message":"success","data":[{"txId":"tx","vout":1,"satoshi":1000,"confirmed":false,"inscriptions":[]}]}
```

- [ ] **Step 7: Run handler and UTXO compatibility tests**

Run:

```bash
go test ./wallet -run 'Test(BalanceHandler|UTXOHandler|Handler|GatewayService|MetaletUTXO)' -v
```

Expected: PASS.

- [ ] **Step 8: Run all wallet tests**

Run:

```bash
go test ./wallet -v
```

Expected: PASS.

- [ ] **Step 9: Commit Task 4**

```bash
git add wallet/server.go wallet/handlers.go wallet/handlers_test.go wallet/metalet_compat.go wallet/response_test.go wallet/testdata/metalet_utxos.json
git commit -m "feat: add wallet gateway handlers"
```

---

### Task 5: Add wallet configuration and server integration

**Files:**
- Modify: `config/config.go`
- Modify: `api/server.go`
- Modify: `main.go`
- Modify: `config.yaml`
- Create: `wallet/config.go`
- Create: `api/server_wallet_test.go`

- [ ] **Step 1: Write failing API integration test**

Create `api/server_wallet_test.go`:

```go
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
```

- [ ] **Step 2: Run API wallet integration test and verify it fails**

Run:

```bash
go test ./api -run 'TestEnableWalletGateway(RegistersWalletRoutes|KeepsCoreRoutesRegistered)' -v
```

Expected: FAIL because `EnableWalletGateway` does not exist.

- [ ] **Step 3: Add config structs and environment overrides**

Modify `config/config.go`.

Add these structs after `RPCConfig`:

```go
type WalletGatewayConfig struct {
	Enabled        bool                               `yaml:"enabled"`
	TimeoutSeconds int                               `yaml:"timeout_seconds"`
	Chains         map[string]WalletGatewayChainConfig `yaml:"chains"`
}

type WalletGatewayChainConfig struct {
	Enabled bool   `yaml:"enabled"`
	CoreURL string `yaml:"core_url"`
}
```

Add this field to `Config`:

```go
Wallet WalletGatewayConfig `yaml:"wallet"`
```

Add default config values in `LoadConfig`:

```go
Wallet: WalletGatewayConfig{
	Enabled:        false,
	TimeoutSeconds: 10,
	Chains: map[string]WalletGatewayChainConfig{
		ChainBTC:  {Enabled: false, CoreURL: ""},
		ChainMVC:  {Enabled: false, CoreURL: ""},
		ChainDOGE: {Enabled: false, CoreURL: ""},
	},
},
```

Add environment overrides before chain validation:

```go
if enabled := os.Getenv("WALLET_GATEWAY_ENABLED"); enabled != "" {
	if val, err := strconv.ParseBool(enabled); err == nil {
		cfg.Wallet.Enabled = val
	}
}
if timeout := os.Getenv("WALLET_GATEWAY_TIMEOUT_SECONDS"); timeout != "" {
	val, err := strconv.Atoi(timeout)
	if err == nil && val > 0 {
		cfg.Wallet.TimeoutSeconds = val
	}
}
ensureWalletChainConfig(cfg)
applyWalletChainEnv(cfg, ChainBTC, "WALLET_BTC_CORE_URL")
applyWalletChainEnv(cfg, ChainMVC, "WALLET_MVC_CORE_URL")
applyWalletChainEnv(cfg, ChainDOGE, "WALLET_DOGE_CORE_URL")
```

Add helper functions near the bottom of `config/config.go`:

```go
func ensureWalletChainConfig(cfg *Config) {
	if cfg.Wallet.TimeoutSeconds <= 0 {
		cfg.Wallet.TimeoutSeconds = 10
	}
	if cfg.Wallet.Chains == nil {
		cfg.Wallet.Chains = make(map[string]WalletGatewayChainConfig)
	}
	for _, chain := range []string{ChainBTC, ChainMVC, ChainDOGE} {
		if _, exists := cfg.Wallet.Chains[chain]; !exists {
			cfg.Wallet.Chains[chain] = WalletGatewayChainConfig{}
		}
	}
}

func applyWalletChainEnv(cfg *Config, chain string, envName string) {
	value := strings.TrimSpace(os.Getenv(envName))
	if value == "" {
		return
	}
	chainCfg := cfg.Wallet.Chains[chain]
	chainCfg.Enabled = true
	chainCfg.CoreURL = value
	cfg.Wallet.Chains[chain] = chainCfg
}
```

- [ ] **Step 4: Add wallet config conversion**

Create `wallet/config.go`:

```go
package wallet

import (
	"time"

	"github.com/metaid/utxo_indexer/config"
)

func FromAppConfig(cfg config.WalletGatewayConfig) Config {
	out := Config{
		Enabled: cfg.Enabled,
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		Chains:  make(map[Chain]ChainConfig),
	}
	for rawChain, rawChainCfg := range cfg.Chains {
		chain, ok := NormalizeChain(rawChain)
		if !ok {
			continue
		}
		out.Chains[chain] = ChainConfig{
			Enabled: rawChainCfg.Enabled,
			CoreURL: rawChainCfg.CoreURL,
		}
	}
	return out
}
```

- [ ] **Step 5: Add API server registration method**

Modify `api/server.go` imports to include:

```go
	"github.com/metaid/utxo_indexer/wallet"
```

Add this method after `setupRoutes`:

```go
func (s *Server) EnableWalletGateway(cfg wallet.Config) error {
	if !cfg.Enabled {
		return nil
	}
	service, err := wallet.NewGatewayService(cfg)
	if err != nil {
		return err
	}
	wallet.RegisterRoutes(s.Router, wallet.NewGateway(service))
	return nil
}
```

- [ ] **Step 6: Wire Wallet Gateway from main config**

Modify `main.go` imports to include:

```go
	"github.com/metaid/utxo_indexer/wallet"
```

After:

```go
ApiServer = api.NewServer(idx, metaStore, stopCh)
ApiServer.SetMempoolManager(mempoolMgr, bcClient)
```

add:

```go
if cfg.Wallet.Enabled {
	walletCfg := wallet.FromAppConfig(cfg.Wallet)
	if err := ApiServer.EnableWalletGateway(walletCfg); err != nil {
		log.Fatalf("Failed to enable wallet gateway: %v", err)
	}
	log.Printf("Wallet Gateway enabled")
}
```

- [ ] **Step 7: Add disabled-by-default config example**

Append this block to `config.yaml`:

```yaml
wallet:
  enabled: false
  timeout_seconds: 10
  chains:
    btc:
      enabled: false
      core_url: "http://127.0.0.1:8066"
    mvc:
      enabled: false
      core_url: "http://127.0.0.1:8085"
    doge:
      enabled: false
      core_url: "http://127.0.0.1:8066"
```

- [ ] **Step 8: Run API and wallet integration tests**

Run:

```bash
go test ./wallet ./api -run 'TestEnableWalletGatewayRegistersWalletRoutes|TestGatewayService|Test(BalanceHandler|UTXOHandler)' -v
```

Expected: PASS.

- [ ] **Step 9: Run config package tests by compiling the config package**

Run:

```bash
go test ./config -v
```

Expected: PASS.

- [ ] **Step 10: Commit Task 5**

```bash
git add config/config.go config.yaml wallet/config.go api/server.go api/server_wallet_test.go main.go
git commit -m "feat: wire wallet gateway config"
```

---

### Task 6: Add handler tests for all first-version acceptance paths

**Files:**
- Modify: `wallet/handlers_test.go`
- Modify: `wallet/response_test.go`

- [ ] **Step 1: Extend tests for BTC, MVC, and DOGE balance routes**

Append to `wallet/handlers_test.go`:

```go
func TestBalanceHandlerSupportsAllFirstVersionChains(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{balance: WalletBalance{ConfirmedSatoshi: 1}})
	for _, chain := range []string{"btc", "mvc", "doge"} {
		req := httptest.NewRequest(http.MethodGet, "/wallet/v1/"+chain+"/address/addr/balance", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", chain, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"chain":"`+chain+`"`) {
			t.Fatalf("%s response missing chain: %s", chain, rec.Body.String())
		}
	}
}
```

- [ ] **Step 2: Extend tests for BTC, MVC, and DOGE UTXO routes**

Append to `wallet/handlers_test.go`:

```go
func TestUTXOHandlerSupportsAllFirstVersionChains(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{utxos: []WalletUTXO{
		{TxID: "tx", Vout: 0, Satoshi: 1, Confirmed: true, Height: int64Ptr(10)},
	}})
	for _, chain := range []string{"btc", "mvc", "doge"} {
		req := httptest.NewRequest(http.MethodGet, "/wallet/v1/"+chain+"/address/addr/utxos", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", chain, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"chain":"`+chain+`"`) {
			t.Fatalf("%s response missing chain: %s", chain, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"total":1`) {
			t.Fatalf("%s response missing UTXO item: %s", chain, rec.Body.String())
		}
	}
}
```

- [ ] **Step 3: Extend tests for invalid query parameters**

Append to `wallet/handlers_test.go`:

```go
func TestUTXOHandlerRejectsInvalidConfirmedOnlyAndSort(t *testing.T) {
	router := newHandlerTestRouter(fakeWalletService{})

	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr/utxos?confirmedOnly=maybe", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "confirmedOnly must be true or false") {
		t.Fatalf("invalid confirmedOnly status/body = %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr/utxos?sort=random", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sort must be asc or desc") {
		t.Fatalf("invalid sort status/body = %d %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 4: Add response test proving standard JSON has no float amount fields**

Append to `wallet/response_test.go`:

```go
func TestStandardResponsesDoNotContainFloatAmountKeys(t *testing.T) {
	resp := NewStandardBalanceResponse(WalletBalance{
		Chain:            ChainBTC,
		Address:          "addr",
		ConfirmedSatoshi: 100000000,
		UnsafeSatoshi:    1,
	})
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	body := string(raw)
	for _, forbidden := range []string{`"balance":`, `"safeBalance":`, `"unSafeBalance":`, `"confirmed_balance":`, `"unsafe_fee":`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("standard response contains forbidden legacy/float key %s: %s", forbidden, body)
		}
	}
}
```

- [ ] **Step 5: Run first-version acceptance tests**

Run:

```bash
go test ./wallet ./api -run 'Test(BalanceHandlerSupportsAllFirstVersionChains|UTXOHandlerSupportsAllFirstVersionChains|UTXOHandlerRejectsInvalid|StandardResponsesDoNotContainFloat|EnableWalletGateway)' -v
```

Expected: PASS.

- [ ] **Step 6: Commit Task 6**

```bash
git add wallet/handlers_test.go wallet/response_test.go
git commit -m "test: cover wallet gateway acceptance paths"
```

---

### Task 7: Add local smoke commands and endpoint comparison notes

**Files:**
- Create: `docs/wallet-gateway-smoke-checks.md`

- [ ] **Step 1: Add smoke-check documentation**

Create `docs/wallet-gateway-smoke-checks.md`:

```markdown
# Wallet Gateway Smoke Checks

Use these checks after enabling `wallet.enabled: true`.

## Upstream Core Health

Set the core URLs used by this gateway:

```bash
BTC_CORE='http://127.0.0.1:8066'
MVC_CORE='http://127.0.0.1:8085'
DOGE_CORE='http://127.0.0.1:8066'
```

Check each configured core before checking the gateway:

```bash
curl -sS "$BTC_CORE/cleanedHeight/get"
curl -sS "$BTC_CORE/balance?address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ"
curl -sS "$BTC_CORE/utxos?address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ"

curl -sS "$MVC_CORE/cleanedHeight/get"
curl -sS "$MVC_CORE/balance?address=1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK"
curl -sS "$MVC_CORE/utxos?address=1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK"

curl -sS "$DOGE_CORE/cleanedHeight/get"
curl -sS "$DOGE_CORE/balance?address=DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L"
curl -sS "$DOGE_CORE/utxos?address=DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L"
```

The Wallet Gateway first version expects each configured core `/utxos` response to be the merged confirmed plus mempool UTXO source. If a core only exposes mempool data through `/mempool/utxos`, fix or wrap that core before enabling the gateway for that chain.

## Local Standard Responses

BTC balance:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/balance'
```

BTC UTXOs, default includes mempool:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos'
```

BTC UTXOs, confirmed only:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos?confirmedOnly=true'
```

MVC balance:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/balance'
```

MVC UTXOs, default includes mempool:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/utxos'
```

MVC UTXOs, confirmed only:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/utxos?confirmedOnly=true'
```

DOGE balance:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/balance'
```

DOGE UTXOs, default includes mempool:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/utxos'
```

DOGE UTXOs, confirmed only:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/utxos?confirmedOnly=true'
```

## Metalet Compatibility Responses

BTC balance compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/balance?format=metalet'
```

MVC balance compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/balance?format=metalet'
```

DOGE balance compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/balance?format=metalet'
```

## Comparison Targets

Compare sampled values against existing services before publishing the gateway:

```bash
curl -sS 'https://www.metalet.space/wallet-api/v3/address/btc-balance?net=livenet&address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ'
curl -sS 'http://8.217.251.101:8066/balance?address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ'
curl -sS 'https://www.metalet.space/wallet-api/v4/mvc/address/balance-info?net=livenet&address=1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK'
curl -sS 'https://www.metalet.space/wallet-api/v4/doge/address/balance-info?net=livenet&address=DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L'
```

Expected checks:

- standard responses use satoshi integer fields and decimal strings;
- standard responses do not expose float amount fields;
- `format=metalet` responses preserve old balance field names;
- UTXO default responses include mempool entries when the configured core `/utxos` reports them;
- `confirmedOnly=true` excludes mempool entries;
- existing Metalet endpoints are not changed by enabling Wallet Gateway.

## Log Checks

While running the curl commands, check the Higun process logs for wallet upstream lines:

```bash
rg 'wallet upstream request' higun.log
```

Expected log properties:

- includes upstream `host`, `path`, HTTP `status`, and `duration_ms`;
- logs a truncated address such as `12ghVW...nMUikZ`;
- does not log a full wallet address;
- failed upstream calls include an `error` field.
```

- [ ] **Step 2: Run Markdown grep checks for stale endpoint names**

Run:

```bash
rg -n "wallet-api/v1" docs/wallet-gateway-smoke-checks.md docs/superpowers/specs/2026-06-07-wallet-gateway-design.md
```

Expected: no output.

- [ ] **Step 3: Commit Task 7**

```bash
git add docs/wallet-gateway-smoke-checks.md
git commit -m "docs: add wallet gateway smoke checks"
```

---

### Task 8: Final verification

**Files:**
- No new files.

- [ ] **Step 1: Run all wallet and integration package tests**

Run:

```bash
go test ./wallet ./api ./config -v
```

Expected: PASS.

- [ ] **Step 2: Run broader tests that should not be affected by new wallet routes**

Run:

```bash
go test ./api ./indexer ./storage -v
```

Expected: PASS.

- [ ] **Step 3: Run full repository tests if local CGO and SDK state allow it**

Run:

```bash
go test ./...
```

Expected: PASS. If this fails due to a known local CGO or SDK environment issue, record the exact failing package and rerun the targeted commands from Steps 1 and 2.

- [ ] **Step 4: Verify old core routes still respond in tests**

Run:

```bash
go test ./api -run 'Test(CheckUtxo|StartMempool|RichList|EnableWalletGateway)' -v
```

Expected: PASS.

- [ ] **Step 5: Confirm final diff only includes Wallet Gateway work**

Run:

```bash
git status --short
git log --oneline -8
```

Expected: working tree is clean after commits, and recent commits are the Wallet Gateway implementation commits from this plan.
