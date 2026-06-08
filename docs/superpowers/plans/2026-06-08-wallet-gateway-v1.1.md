# Wallet Gateway v1.1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend Higun Wallet Gateway with BTC, MVC, and DOGE transaction broadcast, transaction detail/status, address history, and config-backed fee-rate APIs.

**Architecture:** Extend the existing `wallet` package instead of creating a second gateway. Wallet Gateway remains the public application API; it routes to configured chain-specific Higun core endpoints, normalizes responses, and keeps Metalet services untouched. A small additive core route is added for transaction detail/status because the current core HTTP surface does not expose enough confirmation metadata.

**Tech Stack:** Go 1.24, Gin, standard `net/http`, YAML config, btcd `rpcclient`/`btcjson`, Go test

---

## Implementation Notes

This plan implements the approved design in:

```text
docs/superpowers/specs/2026-06-08-wallet-gateway-v1.1-design.md
```

Existing v1 routes must remain unchanged:

```text
GET /wallet/v1/{chain}/address/{address}/balance
GET /wallet/v1/{chain}/address/{address}/utxos
```

New v1.1 routes:

```text
POST /wallet/v1/{chain}/tx/broadcast
GET  /wallet/v1/{chain}/tx/{txid}
GET  /wallet/v1/{chain}/address/{address}/history
GET  /wallet/v1/{chain}/fee-rate
```

Implementation constraints:

- Supported chains remain `btc`, `mvc`, and `doge`.
- History includes mempool/unconfirmed items by default.
- `confirmedOnly=true` filters mempool history and UTXOs.
- Fee rates are exposed to downstream applications by Higun; downstream clients do not configure rates.
- First fee-rate source is config, with stable response fields for a later dynamic estimator.
- Broadcast uses the current core legacy route `/btc/broadcast` by default, normalized behind `/wallet/v1/{chain}/tx/broadcast`.
- Transaction detail/status requires a core HTTP route that can report `confirmed`, `mempool`, and `confirmations`; the gateway must not guess those fields.
- No Metalet code is imported.
- Do not log `rawTx`.

## File Structure

Modify existing files:

- `config/config.go`: extend wallet gateway YAML/env config with fee-rate and optional broadcast path.
- `config.yaml`: document disabled-by-default v1.1 config example values.
- `blockchain/client.go`: add transaction detail/status structs and a `GetTransactionDetail` method using verbose raw transaction RPC data.
- `api/server.go`: add core `GET /tx/:txid` route, extend `/utxos/history` query handling, and add test injection hooks.
- `api/server_wallet_test.go`: expect new wallet routes when the gateway is enabled.
- `indexer/query.go`: preserve current history API and add filter/sort/page helper support for stable gateway history.
- `wallet/config.go`: map app config into wallet config, including fee-rate and broadcast path.
- `wallet/model.go`: add transaction, history, fee-rate, and broadcast models.
- `wallet/errors.go`: add v1.1 error codes.
- `wallet/client.go`: add core client calls for broadcast, tx detail, and history.
- `wallet/handlers.go`: add v1.1 handlers.
- `wallet/logging.go`: make upstream logging safe for txid and broadcast paths without logging raw transactions.
- `wallet/normalize.go`: add history sorting/filtering, timestamp parsing, tx detail normalization helpers.
- `wallet/response.go`: add standard v1.1 response renderers.
- `wallet/server.go`: extend `WalletService` and route registration.
- `docs/wallet-gateway-smoke-checks.md`: add v1.1 smoke checks.

Create new tests:

- `blockchain/client_tx_detail_test.go`
- `api/server_tx_detail_test.go`
- `indexer/history_options_test.go`
- `wallet/config_test.go`
- `wallet/broadcast_test.go`
- `wallet/tx_detail_test.go`
- `wallet/history_test.go`
- `wallet/fee_rate_test.go`

## Task Graph

Use this sequence:

1. Task 1: shared models, config, and error codes.
2. Task 2: fee-rate endpoint.
3. Task 3: broadcast endpoint.
4. Task 4: core transaction detail/status route.
5. Task 5: gateway transaction detail endpoint.
6. Task 6: core history filtering/sorting support.
7. Task 7: gateway history endpoint.
8. Task 8: route integration, smoke docs, and full verification.

Run tasks sequentially in subagent-driven development. Several tasks touch shared gateway files such as `wallet/server.go`, `wallet/handlers.go`, `wallet/response.go`, and `api/server.go`; sequential dispatch avoids avoidable merge conflicts. Use a fresh subagent per task, review and integrate that task, then dispatch the next task.

---

### Task 1: Shared Models, Config, And Error Codes

**Files:**
- Modify: `config/config.go`
- Modify: `config.yaml`
- Modify: `wallet/config.go`
- Modify: `wallet/model.go`
- Modify: `wallet/errors.go`
- Modify: `wallet/server.go`
- Modify: `wallet/handlers_test.go`
- Create: `wallet/config_test.go`

- [ ] **Step 1: Write failing config/model tests**

Create `wallet/config_test.go`:

```go
package wallet

import (
	"strings"
	"testing"
	"time"

	appconfig "github.com/metaid/utxo_indexer/config"
)

func TestFromAppConfigMapsV11ChainSettings(t *testing.T) {
	got := FromAppConfig(appconfig.WalletGatewayConfig{
		Enabled:        true,
		TimeoutSeconds: 7,
		Chains: map[string]appconfig.WalletGatewayChainConfig{
			"btc": {
				Enabled:       true,
				CoreURL:       "http://127.0.0.1:8066",
				BroadcastPath: "/tx/broadcast",
				FeeRate: appconfig.WalletGatewayFeeRateConfig{
					Unit:    "sat_per_byte",
					Slow:    1,
					Normal:  3,
					Fast:    5,
					Default: "fast",
				},
			},
		},
	})

	if !got.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if got.Timeout != 7*time.Second {
		t.Fatalf("Timeout = %s, want 7s", got.Timeout)
	}
	chain := got.Chains[ChainBTC]
	if !chain.Enabled || chain.CoreURL != "http://127.0.0.1:8066" {
		t.Fatalf("chain config not mapped: %+v", chain)
	}
	if chain.BroadcastPath != "/tx/broadcast" {
		t.Fatalf("BroadcastPath = %q, want /tx/broadcast", chain.BroadcastPath)
	}
	if chain.FeeRate.Unit != "sat_per_byte" || chain.FeeRate.Slow != 1 || chain.FeeRate.Normal != 3 || chain.FeeRate.Fast != 5 || chain.FeeRate.Default != "fast" {
		t.Fatalf("fee rate not mapped: %+v", chain.FeeRate)
	}
}

func TestGatewayServiceAppliesV11Defaults(t *testing.T) {
	service, err := NewGatewayService(Config{
		Enabled: true,
		Timeout: time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC: {
				Enabled: true,
				CoreURL: "http://127.0.0.1:8066",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}

	chain := service.chainConfig[ChainBTC]
	if chain.BroadcastPath != "/btc/broadcast" {
		t.Fatalf("BroadcastPath = %q, want /btc/broadcast", chain.BroadcastPath)
	}
	if chain.FeeRate.Source != FeeRateSourceConfig || chain.FeeRate.Unit != FeeRateUnitSatPerByte {
		t.Fatalf("unexpected fee defaults: %+v", chain.FeeRate)
	}
	if chain.FeeRate.Slow <= 0 || chain.FeeRate.Normal <= 0 || chain.FeeRate.Fast <= 0 {
		t.Fatalf("fee defaults must be positive: %+v", chain.FeeRate)
	}
	if chain.FeeRate.Default != "normal" {
		t.Fatalf("default tier = %q, want normal", chain.FeeRate.Default)
	}
}

func TestGatewayServiceRejectsInvalidFeeRate(t *testing.T) {
	_, err := NewGatewayService(Config{
		Enabled: true,
		Timeout: time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC: {
				Enabled: true,
				CoreURL: "http://127.0.0.1:8066",
				FeeRate: FeeRate{
					Source:  FeeRateSourceConfig,
					Unit:    FeeRateUnitSatPerByte,
					Slow:    -1,
					Normal:  3,
					Fast:    5,
					Default: "normal",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("NewGatewayService returned nil error for invalid fee rate")
	}
	if !strings.Contains(err.Error(), "invalid fee_rate") {
		t.Fatalf("error = %v, want invalid fee_rate", err)
	}
}

func TestNormalizeTxID(t *testing.T) {
	valid := strings.Repeat("a", 64)
	got, ok := NormalizeTxID(" " + valid + " ")
	if !ok || got != valid {
		t.Fatalf("NormalizeTxID valid = %q %v", got, ok)
	}
	for _, raw := range []string{"", "abc", strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if got, ok := NormalizeTxID(raw); ok {
			t.Fatalf("NormalizeTxID(%q) = %q true, want false", raw, got)
		}
	}
}

func TestGatewayServiceAppliesPerChainFeeDefaults(t *testing.T) {
	service, err := NewGatewayService(Config{
		Enabled: true,
		Timeout: time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC:  {Enabled: true, CoreURL: "http://127.0.0.1:8066"},
			ChainMVC:  {Enabled: true, CoreURL: "http://127.0.0.1:8067"},
			ChainDOGE: {Enabled: true, CoreURL: "http://127.0.0.1:8068"},
		},
	})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}
	tests := []struct {
		chain      Chain
		wantNormal int64
		wantFast   int64
	}{
		{chain: ChainBTC, wantNormal: 3, wantFast: 5},
		{chain: ChainMVC, wantNormal: 2, wantFast: 3},
		{chain: ChainDOGE, wantNormal: 2, wantFast: 5},
	}
	for _, tt := range tests {
		t.Run(string(tt.chain), func(t *testing.T) {
			got := service.chainConfig[tt.chain].FeeRate
			if got.Source != FeeRateSourceConfig || got.Unit != FeeRateUnitSatPerByte {
				t.Fatalf("unexpected source/unit: %+v", got)
			}
			if got.Slow != 1 || got.Normal != tt.wantNormal || got.Fast != tt.wantFast || got.Default != "normal" {
				t.Fatalf("unexpected default fee rate for %s: %+v", tt.chain, got)
			}
		})
	}
}
```

- [ ] **Step 2: Run the new config/model tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(FromAppConfigMapsV11ChainSettings|GatewayServiceAppliesV11Defaults|GatewayServiceRejectsInvalidFeeRate|GatewayServiceAppliesPerChainFeeDefaults|NormalizeTxID)' -v
```

Expected: FAIL with missing fields such as `BroadcastPath`, `FeeRate`, and `NormalizeTxID`.

- [ ] **Step 3: Extend application config structs**

Modify `config/config.go`:

```go
type WalletGatewayConfig struct {
	Enabled        bool                                `yaml:"enabled"`
	TimeoutSeconds int                                 `yaml:"timeout_seconds"`
	Chains         map[string]WalletGatewayChainConfig `yaml:"chains"`
}

type WalletGatewayChainConfig struct {
	Enabled       bool                       `yaml:"enabled"`
	CoreURL       string                     `yaml:"core_url"`
	BroadcastPath string                     `yaml:"broadcast_path"`
	FeeRate       WalletGatewayFeeRateConfig `yaml:"fee_rate"`
}

type WalletGatewayFeeRateConfig struct {
	Unit    string `yaml:"unit"`
	Slow    int64  `yaml:"slow"`
	Normal  int64  `yaml:"normal"`
	Fast    int64  `yaml:"fast"`
	Default string `yaml:"default"`
}
```

In `LoadConfig`, keep wallet disabled by default and add effective defaults:

```go
Wallet: WalletGatewayConfig{
	Enabled:        false,
	TimeoutSeconds: 10,
	Chains: map[string]WalletGatewayChainConfig{
		ChainBTC: {
			Enabled:       false,
			CoreURL:       "",
			BroadcastPath: "/btc/broadcast",
			FeeRate: WalletGatewayFeeRateConfig{
				Unit:    "sat_per_byte",
				Slow:    1,
				Normal:  3,
				Fast:    5,
				Default: "normal",
			},
		},
		ChainMVC: {
			Enabled:       false,
			CoreURL:       "",
			BroadcastPath: "/btc/broadcast",
			FeeRate: WalletGatewayFeeRateConfig{
				Unit:    "sat_per_byte",
				Slow:    1,
				Normal:  2,
				Fast:    3,
				Default: "normal",
			},
		},
		ChainDOGE: {
			Enabled:       false,
			CoreURL:       "",
			BroadcastPath: "/btc/broadcast",
			FeeRate: WalletGatewayFeeRateConfig{
				Unit:    "sat_per_byte",
				Slow:    1,
				Normal:  2,
				Fast:    5,
				Default: "normal",
			},
		},
	},
},
```

Update `ensureWalletChainConfig` so missing YAML sections get the same defaults:

```go
func defaultWalletChainConfig(chain string) WalletGatewayChainConfig {
	switch chain {
	case ChainMVC:
		return WalletGatewayChainConfig{
			BroadcastPath: "/btc/broadcast",
			FeeRate: WalletGatewayFeeRateConfig{Unit: "sat_per_byte", Slow: 1, Normal: 2, Fast: 3, Default: "normal"},
		}
	case ChainDOGE:
		return WalletGatewayChainConfig{
			BroadcastPath: "/btc/broadcast",
			FeeRate: WalletGatewayFeeRateConfig{Unit: "sat_per_byte", Slow: 1, Normal: 2, Fast: 5, Default: "normal"},
		}
	default:
		return WalletGatewayChainConfig{
			BroadcastPath: "/btc/broadcast",
			FeeRate: WalletGatewayFeeRateConfig{Unit: "sat_per_byte", Slow: 1, Normal: 3, Fast: 5, Default: "normal"},
		}
	}
}

func ensureWalletChainConfig(cfg *Config) {
	if cfg.Wallet.TimeoutSeconds <= 0 {
		cfg.Wallet.TimeoutSeconds = 10
	}
	if cfg.Wallet.Chains == nil {
		cfg.Wallet.Chains = make(map[string]WalletGatewayChainConfig)
	}
	for _, chain := range []string{ChainBTC, ChainMVC, ChainDOGE} {
		defaults := defaultWalletChainConfig(chain)
		chainCfg, exists := cfg.Wallet.Chains[chain]
		if !exists {
			cfg.Wallet.Chains[chain] = defaults
			continue
		}
		if strings.TrimSpace(chainCfg.BroadcastPath) == "" {
			chainCfg.BroadcastPath = defaults.BroadcastPath
		}
		if strings.TrimSpace(chainCfg.FeeRate.Unit) == "" {
			chainCfg.FeeRate.Unit = defaults.FeeRate.Unit
		}
		if chainCfg.FeeRate.Slow == 0 {
			chainCfg.FeeRate.Slow = defaults.FeeRate.Slow
		}
		if chainCfg.FeeRate.Normal == 0 {
			chainCfg.FeeRate.Normal = defaults.FeeRate.Normal
		}
		if chainCfg.FeeRate.Fast == 0 {
			chainCfg.FeeRate.Fast = defaults.FeeRate.Fast
		}
		if strings.TrimSpace(chainCfg.FeeRate.Default) == "" {
			chainCfg.FeeRate.Default = defaults.FeeRate.Default
		}
		cfg.Wallet.Chains[chain] = chainCfg
	}
}
```

- [ ] **Step 4: Add optional environment overrides**

Modify `config/config.go`:

```go
applyWalletChainEnv(cfg, ChainBTC, "WALLET_BTC_CORE_URL")
applyWalletChainEnv(cfg, ChainMVC, "WALLET_MVC_CORE_URL")
applyWalletChainEnv(cfg, ChainDOGE, "WALLET_DOGE_CORE_URL")
applyWalletV11Env(cfg, ChainBTC, "WALLET_BTC")
applyWalletV11Env(cfg, ChainMVC, "WALLET_MVC")
applyWalletV11Env(cfg, ChainDOGE, "WALLET_DOGE")
```

Add:

```go
func applyWalletV11Env(cfg *Config, chain string, prefix string) {
	chainCfg := cfg.Wallet.Chains[chain]
	if value := strings.TrimSpace(os.Getenv(prefix + "_BROADCAST_PATH")); value != "" {
		chainCfg.BroadcastPath = value
	}
	if value := strings.TrimSpace(os.Getenv(prefix + "_FEE_RATE_UNIT")); value != "" {
		chainCfg.FeeRate.Unit = value
	}
	if value := strings.TrimSpace(os.Getenv(prefix + "_FEE_RATE_DEFAULT")); value != "" {
		chainCfg.FeeRate.Default = value
	}
	if value, ok := parsePositiveInt64Env(prefix + "_FEE_RATE_SLOW"); ok {
		chainCfg.FeeRate.Slow = value
	}
	if value, ok := parsePositiveInt64Env(prefix + "_FEE_RATE_NORMAL"); ok {
		chainCfg.FeeRate.Normal = value
	}
	if value, ok := parsePositiveInt64Env(prefix + "_FEE_RATE_FAST"); ok {
		chainCfg.FeeRate.Fast = value
	}
	cfg.Wallet.Chains[chain] = chainCfg
}

func parsePositiveInt64Env(name string) (int64, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}
```

- [ ] **Step 5: Extend wallet models and errors**

Modify `wallet/model.go`:

```go
const (
	FeeRateSourceConfig   = "config"
	FeeRateUnitSatPerByte = "sat_per_byte"
)

type FeeRate struct {
	Source  string
	Unit    string
	Slow    int64
	Normal  int64
	Fast    int64
	Default string
}

func (r FeeRate) Validate() error {
	if strings.TrimSpace(r.Source) == "" {
		return fmt.Errorf("fee source is required")
	}
	if strings.TrimSpace(r.Unit) != FeeRateUnitSatPerByte {
		return fmt.Errorf("fee unit must be %s", FeeRateUnitSatPerByte)
	}
	if r.Slow <= 0 || r.Normal <= 0 || r.Fast <= 0 {
		return fmt.Errorf("fee tiers must be positive")
	}
	switch r.Default {
	case "slow", "normal", "fast":
		return nil
	default:
		return fmt.Errorf("fee default must be slow, normal, or fast")
	}
}

type BroadcastResult struct {
	Chain    Chain
	TxID     string
	Accepted bool
}

type WalletTxInput struct {
	TxID    string
	Vout    uint32
	Address string
	Satoshi *uint64
}

type WalletTxOutput struct {
	Vout    uint32
	Address string
	Satoshi uint64
}

type WalletTxDetail struct {
	Chain         Chain
	TxID          string
	Confirmed     bool
	Mempool       bool
	Confirmations uint64
	Height        *int64
	BlockHash     string
	BlockTime     *int64
	Inputs        []WalletTxInput
	Outputs       []WalletTxOutput
	FeeSatoshi    *uint64
	Size          int32
	Vsize         int32
}

type HistoryOptions struct {
	Page          int
	Limit         int
	ConfirmedOnly bool
	Sort          string
}

type WalletHistoryItem struct {
	TxID          string
	Direction     string
	IncomeSatoshi uint64
	SpendSatoshi  uint64
	NetSatoshi    int64
	Confirmed     bool
	Mempool       bool
	Confirmations *uint64
	Height        *int64
	Timestamp     int64
	Time          string
}

type WalletHistoryPage struct {
	Chain         Chain
	Address       string
	Page          int
	Limit         int
	ConfirmedOnly bool
	Sort          string
	Total         int64
	Items         []WalletHistoryItem
}

func NormalizeTxID(raw string) (string, bool) {
	txid := strings.ToLower(strings.TrimSpace(raw))
	if len(txid) != 64 {
		return "", false
	}
	for _, ch := range txid {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return "", false
		}
	}
	return txid, true
}
```

Modify `wallet/errors.go`:

```go
const (
	CodeSuccess          = 0
	CodeUnsupportedChain = -4001
	CodeInvalidAddress   = -4002
	CodeInvalidQuery     = -4003
	CodeInvalidRawTx     = -4004
	CodeTxNotFound       = -4041
	CodeCoreUnavailable  = -5001
	CodeInvalidUpstream  = -5002
	CodeInternal         = -5003
	CodeBroadcastRejected = -5004
	CodeFeeRateUnavailable = -5005
)
```

Run `gofmt` after editing; align names if gofmt reports spacing only.

- [ ] **Step 6: Extend wallet config mapping and service state**

Modify `wallet/config.go`:

```go
out.Chains[chain] = ChainConfig{
	Enabled:       rawChainCfg.Enabled,
	CoreURL:       rawChainCfg.CoreURL,
	BroadcastPath: rawChainCfg.BroadcastPath,
	FeeRate: FeeRate{
		Source:  FeeRateSourceConfig,
		Unit:    rawChainCfg.FeeRate.Unit,
		Slow:    rawChainCfg.FeeRate.Slow,
		Normal:  rawChainCfg.FeeRate.Normal,
		Fast:    rawChainCfg.FeeRate.Fast,
		Default: rawChainCfg.FeeRate.Default,
	},
}
```

Modify `wallet/server.go`:

```go
type ChainConfig struct {
	Enabled       bool
	CoreURL       string
	BroadcastPath string
	FeeRate       FeeRate
}

type WalletService interface {
	GetBalance(ctx context.Context, chain Chain, address string) (WalletBalance, error)
	GetUTXOs(ctx context.Context, chain Chain, address string) ([]WalletUTXO, error)
	GetFeeRate(ctx context.Context, chain Chain) (FeeRate, error)
	BroadcastTransaction(ctx context.Context, chain Chain, rawTx string) (BroadcastResult, error)
	GetTransaction(ctx context.Context, chain Chain, txid string) (WalletTxDetail, error)
	GetHistory(ctx context.Context, chain Chain, address string, options HistoryOptions) (WalletHistoryPage, error)
}

type GatewayService struct {
	clients     map[Chain]*CoreClient
	chainConfig map[Chain]ChainConfig
}
```

Replace `NewGatewayService` with a complete version that keeps both client and normalized chain config maps:

```go
func NewGatewayService(cfg Config) (*GatewayService, error) {
	clients := make(map[Chain]*CoreClient, len(cfg.Chains))
	chainConfig := make(map[Chain]ChainConfig, len(cfg.Chains))
	for chain, chainCfg := range cfg.Chains {
		if !chainCfg.Enabled {
			continue
		}
		normalizedChain, ok := NormalizeChain(string(chain))
		if !ok {
			continue
		}
		normalizedCfg := normalizeChainConfig(normalizedChain, chainCfg)
		if err := normalizedCfg.FeeRate.Validate(); err != nil {
			return nil, fmt.Errorf("invalid fee_rate for %s: %w", normalizedChain, err)
		}
		client, err := NewCoreClient(normalizedCfg.CoreURL, cfg.Timeout)
		if err != nil {
			return nil, err
		}
		clients[normalizedChain] = client
		chainConfig[normalizedChain] = normalizedCfg
	}
	if cfg.Enabled && len(clients) == 0 {
		return nil, fmt.Errorf("wallet gateway enabled but no enabled chains are configured")
	}
	return &GatewayService{clients: clients, chainConfig: chainConfig}, nil
}
```

Add:

```go
func normalizeChainConfig(chain Chain, cfg ChainConfig) ChainConfig {
	if strings.TrimSpace(cfg.BroadcastPath) == "" {
		cfg.BroadcastPath = "/btc/broadcast"
	}
	if cfg.FeeRate.Source == "" {
		cfg.FeeRate.Source = FeeRateSourceConfig
	}
	if cfg.FeeRate.Unit == "" {
		cfg.FeeRate.Unit = FeeRateUnitSatPerByte
	}
	defaults := defaultFeeRate(chain)
	if cfg.FeeRate.Slow == 0 {
		cfg.FeeRate.Slow = defaults.Slow
	}
	if cfg.FeeRate.Normal == 0 {
		cfg.FeeRate.Normal = defaults.Normal
	}
	if cfg.FeeRate.Fast == 0 {
		cfg.FeeRate.Fast = defaults.Fast
	}
	if cfg.FeeRate.Default == "" {
		cfg.FeeRate.Default = defaults.Default
	}
	return cfg
}

func defaultFeeRate(chain Chain) FeeRate {
	switch chain {
	case ChainMVC:
		return FeeRate{Source: FeeRateSourceConfig, Unit: FeeRateUnitSatPerByte, Slow: 1, Normal: 2, Fast: 3, Default: "normal"}
	case ChainDOGE:
		return FeeRate{Source: FeeRateSourceConfig, Unit: FeeRateUnitSatPerByte, Slow: 1, Normal: 2, Fast: 5, Default: "normal"}
	default:
		return FeeRate{Source: FeeRateSourceConfig, Unit: FeeRateUnitSatPerByte, Slow: 1, Normal: 3, Fast: 5, Default: "normal"}
	}
}
```

Add `strings` to `wallet/server.go` imports if it is not already present.

- [ ] **Step 7: Add temporary service stubs so Task 1 compiles**

Modify `wallet/server.go`:

```go
func (s *GatewayService) GetFeeRate(ctx context.Context, chain Chain) (FeeRate, error) {
	chainCfg, ok := s.chainConfig[chain]
	if !ok {
		return FeeRate{}, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "chain core is not configured")
	}
	if err := chainCfg.FeeRate.Validate(); err != nil {
		return FeeRate{}, NewHTTPWalletError(http.StatusInternalServerError, CodeFeeRateUnavailable, "fee rate unavailable")
	}
	return chainCfg.FeeRate, nil
}

func (s *GatewayService) BroadcastTransaction(ctx context.Context, chain Chain, rawTx string) (BroadcastResult, error) {
	return BroadcastResult{}, NewHTTPWalletError(http.StatusNotImplemented, CodeInternal, "internal wallet error")
}

func (s *GatewayService) GetTransaction(ctx context.Context, chain Chain, txid string) (WalletTxDetail, error) {
	return WalletTxDetail{}, NewHTTPWalletError(http.StatusNotImplemented, CodeInternal, "internal wallet error")
}

func (s *GatewayService) GetHistory(ctx context.Context, chain Chain, address string, options HistoryOptions) (WalletHistoryPage, error) {
	return WalletHistoryPage{}, NewHTTPWalletError(http.StatusNotImplemented, CodeInternal, "internal wallet error")
}
```

Modify `wallet/handlers_test.go` so `fakeWalletService` satisfies the expanded interface until later tasks replace these stubs with behavior-specific versions:

```go
func (s *fakeWalletService) GetFeeRate(ctx context.Context, chain Chain) (FeeRate, error) {
	if s.err != nil {
		return FeeRate{}, s.err
	}
	return FeeRate{Source: FeeRateSourceConfig, Unit: FeeRateUnitSatPerByte, Slow: 1, Normal: 3, Fast: 5, Default: "normal"}, nil
}

func (s *fakeWalletService) BroadcastTransaction(ctx context.Context, chain Chain, rawTx string) (BroadcastResult, error) {
	if s.err != nil {
		return BroadcastResult{}, s.err
	}
	return BroadcastResult{Chain: chain, TxID: strings.Repeat("b", 64), Accepted: true}, nil
}

func (s *fakeWalletService) GetTransaction(ctx context.Context, chain Chain, txid string) (WalletTxDetail, error) {
	if s.err != nil {
		return WalletTxDetail{}, s.err
	}
	return WalletTxDetail{Chain: chain, TxID: txid, Confirmed: true, Confirmations: 1}, nil
}

func (s *fakeWalletService) GetHistory(ctx context.Context, chain Chain, address string, options HistoryOptions) (WalletHistoryPage, error) {
	if s.err != nil {
		return WalletHistoryPage{}, s.err
	}
	return WalletHistoryPage{Chain: chain, Address: address, Page: options.Page, Limit: options.Limit, ConfirmedOnly: options.ConfirmedOnly, Sort: options.Sort}, nil
}
```

- [ ] **Step 8: Update example config**

Modify the `wallet.chains` section in `config.yaml`:

```yaml
wallet:
  enabled: false
  timeout_seconds: 10
  chains:
    btc:
      enabled: false
      core_url: "http://127.0.0.1:8066"
      broadcast_path: "/btc/broadcast"
      fee_rate:
        unit: sat_per_byte
        slow: 1
        normal: 3
        fast: 5
        default: normal
    mvc:
      enabled: false
      core_url: "http://127.0.0.1:8085"
      broadcast_path: "/btc/broadcast"
      fee_rate:
        unit: sat_per_byte
        slow: 1
        normal: 2
        fast: 3
        default: normal
    doge:
      enabled: false
      core_url: "http://127.0.0.1:8068"
      broadcast_path: "/btc/broadcast"
      fee_rate:
        unit: sat_per_byte
        slow: 1
        normal: 2
        fast: 5
        default: normal
```

- [ ] **Step 9: Run tests**

Run:

```bash
gofmt -w config/config.go wallet/config.go wallet/model.go wallet/errors.go wallet/server.go wallet/handlers_test.go wallet/config_test.go
go test ./wallet -run 'Test(FromAppConfigMapsV11ChainSettings|GatewayServiceAppliesV11Defaults|GatewayServiceRejectsInvalidFeeRate|NormalizeTxID)' -v
go test ./wallet ./api -run 'Test.*Wallet' -v
```

Expected: PASS.

- [ ] **Step 10: Commit**

Run:

```bash
git add config/config.go config.yaml wallet/config.go wallet/model.go wallet/errors.go wallet/server.go wallet/handlers_test.go wallet/config_test.go
git commit -m "feat: add wallet gateway v1.1 config models"
```

---

### Task 2: Fee-Rate Endpoint

**Files:**
- Modify: `wallet/server.go`
- Modify: `wallet/handlers.go`
- Modify: `wallet/response.go`
- Modify: `wallet/handlers_test.go`
- Create: `wallet/fee_rate_test.go`

- [ ] **Step 1: Write failing fee-rate response tests**

Create `wallet/fee_rate_test.go`:

```go
package wallet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStandardFeeRateResponse(t *testing.T) {
	body := marshalResponse(t, NewStandardFeeRateResponse(ChainBTC, FeeRate{
		Source:  FeeRateSourceConfig,
		Unit:    FeeRateUnitSatPerByte,
		Slow:    1,
		Normal:  3,
		Fast:    5,
		Default: "normal",
	}))

	for _, want := range []string{
		`"chain":"btc"`,
		`"source":"config"`,
		`"unit":"sat_per_byte"`,
		`"slow":1`,
		`"normal":3`,
		`"fast":5`,
		`"default":"normal"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fee-rate response missing %s: %s", want, body)
		}
	}

	var payload struct {
		Data struct {
			Slow int64 `json:"slow"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Data.Slow != 1 {
		t.Fatalf("slow = %d, want 1", payload.Data.Slow)
	}
}
```

Append handler tests to `wallet/handlers_test.go`, extend `fakeWalletService` with `feeRate FeeRate`, and replace the Task 1 `GetFeeRate` test stub with:

```go
func (s *fakeWalletService) GetFeeRate(ctx context.Context, chain Chain) (FeeRate, error) {
	if s.err != nil {
		return FeeRate{}, s.err
	}
	if s.feeRate.Source == "" {
		return FeeRate{Source: FeeRateSourceConfig, Unit: FeeRateUnitSatPerByte, Slow: 1, Normal: 3, Fast: 5, Default: "normal"}, nil
	}
	return s.feeRate, nil
}

func TestFeeRateHandler(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{feeRate: FeeRate{
		Source:  FeeRateSourceConfig,
		Unit:    FeeRateUnitSatPerByte,
		Slow:    2,
		Normal:  4,
		Fast:    8,
		Default: "fast",
	}})

	w := performWalletRequest(router, "/wallet/v1/btc/fee-rate")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"chain":"btc"`, `"slow":2`, `"normal":4`, `"fast":8`, `"default":"fast"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
}

func TestFeeRateHandlerSupportsAllV11Chains(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})
	for _, chain := range []string{"btc", "mvc", "doge"} {
		t.Run(chain, func(t *testing.T) {
			w := performWalletRequest(router, "/wallet/v1/"+chain+"/fee-rate")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"chain":"`+chain+`"`) {
				t.Fatalf("response missing chain %s: %s", chain, w.Body.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run fee-rate tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(StandardFeeRateResponse|FeeRateHandler|FeeRateHandlerSupportsAllV11Chains)' -v
```

Expected: FAIL because `NewStandardFeeRateResponse` and the route do not exist.

- [ ] **Step 3: Implement fee-rate service and response**

Modify `wallet/server.go`:

```go
func (s *GatewayService) GetFeeRate(ctx context.Context, chain Chain) (FeeRate, error) {
	chainCfg, ok := s.chainConfig[chain]
	if !ok {
		return FeeRate{}, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "chain core is not configured")
	}
	if err := chainCfg.FeeRate.Validate(); err != nil {
		return FeeRate{}, NewHTTPWalletError(http.StatusInternalServerError, CodeFeeRateUnavailable, "fee rate unavailable")
	}
	return chainCfg.FeeRate, nil
}
```

Modify `wallet/response.go`:

```go
type StandardFeeRateData struct {
	Chain   Chain  `json:"chain"`
	Source  string `json:"source"`
	Unit    string `json:"unit"`
	Slow    int64  `json:"slow"`
	Normal  int64  `json:"normal"`
	Fast    int64  `json:"fast"`
	Default string `json:"default"`
}

func NewStandardFeeRateResponse(chain Chain, feeRate FeeRate) Envelope {
	return Success(StandardFeeRateData{
		Chain:   chain,
		Source:  feeRate.Source,
		Unit:    feeRate.Unit,
		Slow:    feeRate.Slow,
		Normal:  feeRate.Normal,
		Fast:    feeRate.Fast,
		Default: feeRate.Default,
	})
}
```

- [ ] **Step 4: Implement fee-rate handler and route**

Modify `wallet/handlers.go`:

```go
func (g *Gateway) getFeeRate(c *gin.Context) {
	if !g.ensureService(c) {
		return
	}
	chain, ok := g.parseChain(c)
	if !ok {
		return
	}
	feeRate, err := g.service.GetFeeRate(c.Request.Context(), chain)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewStandardFeeRateResponse(chain, feeRate))
}

func (g *Gateway) parseChain(c *gin.Context) (Chain, bool) {
	chain, ok := NormalizeChain(c.Param("chain"))
	if !ok {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusNotFound, CodeUnsupportedChain, "unsupported chain"))
		return "", false
	}
	return chain, true
}
```

Refactor `parseCommon` to call `parseChain`.

Modify `wallet/server.go`:

```go
router.GET("/wallet/v1/:chain/fee-rate", gateway.getFeeRate)
```

Update `publicServiceError` in `wallet/handlers.go`:

```go
case CodeFeeRateUnavailable:
	message = "fee rate unavailable"
```

- [ ] **Step 5: Run tests**

Run:

```bash
gofmt -w wallet/server.go wallet/handlers.go wallet/response.go wallet/handlers_test.go wallet/fee_rate_test.go
go test ./wallet -run 'Test(StandardFeeRateResponse|FeeRateHandler|FeeRateHandlerSupportsAllV11Chains|HandlerRejectsInvalidChainAndFormat|GatewayServiceDisabledChainReturnsServiceUnavailable)' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add wallet/server.go wallet/handlers.go wallet/response.go wallet/handlers_test.go wallet/fee_rate_test.go
git commit -m "feat: expose wallet fee rate endpoint"
```

---

### Task 3: Broadcast Endpoint

**Files:**
- Modify: `wallet/client.go`
- Modify: `wallet/server.go`
- Modify: `wallet/handlers.go`
- Modify: `wallet/response.go`
- Modify: `wallet/logging.go`
- Modify: `wallet/handlers_test.go`
- Create: `wallet/broadcast_test.go`

- [ ] **Step 1: Write failing broadcast client and handler tests**

Create `wallet/broadcast_test.go`:

```go
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
		t.Fatalf("rawTx body = %q, want deadbeef", gotBody["rawTx"])
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
```

Append handler tests to `wallet/handlers_test.go` and replace the Task 1 `BroadcastTransaction` test stub with:

```go
func (s *fakeWalletService) BroadcastTransaction(ctx context.Context, chain Chain, rawTx string) (BroadcastResult, error) {
	if s.err != nil {
		return BroadcastResult{}, s.err
	}
	return BroadcastResult{Chain: chain, TxID: strings.Repeat("b", 64), Accepted: true}, nil
}

func TestBroadcastHandler(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/wallet/v1/btc/tx/broadcast", strings.NewReader(`{"rawTx":"deadbeef"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"accepted":true`) || !strings.Contains(w.Body.String(), `"txid":"`+strings.Repeat("b", 64)+`"`) {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestBroadcastHandlerSupportsAllV11Chains(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})
	for _, chain := range []string{"btc", "mvc", "doge"} {
		t.Run(chain, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/wallet/v1/"+chain+"/tx/broadcast", strings.NewReader(`{"rawTx":"deadbeef"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"chain":"`+chain+`"`) {
				t.Fatalf("response missing chain %s: %s", chain, w.Body.String())
			}
		})
	}
}

func TestBroadcastHandlerRejectsInvalidRawTxBeforeService(t *testing.T) {
	service := &fakeWalletService{}
	router := newHandlerTestRouter(service)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/wallet/v1/btc/tx/broadcast", strings.NewReader(`{"rawTx":"not-hex"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid raw transaction") {
		t.Fatalf("response missing invalid raw transaction: %s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run broadcast tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(CoreClientBroadcast|BroadcastHandler|BroadcastHandlerSupportsAllV11Chains|BroadcastLog)' -v
```

Expected: FAIL because broadcast methods and route do not exist.

- [ ] **Step 3: Implement broadcast core client**

Modify `wallet/client.go`:

```go
type coreBroadcastResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (c *CoreClient) BroadcastTransaction(ctx context.Context, chain Chain, rawTx string, broadcastPath string) (BroadcastResult, error) {
	path := strings.TrimSpace(broadcastPath)
	if path == "" {
		path = "/btc/broadcast"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	body, err := json.Marshal(map[string]string{"rawTx": rawTx})
	if err != nil {
		return BroadcastResult{}, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, "internal wallet error")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return BroadcastResult{}, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, "internal wallet error")
	}
	req.Header.Set("Content-Type", "application/json")

	var payload coreBroadcastResponse
	if err := c.doJSON(req, &payload, logWalletUpstreamOptions{RedactBody: true}); err != nil {
		return BroadcastResult{}, err
	}
	if payload.Code != 2000 {
		return BroadcastResult{}, NewHTTPWalletError(http.StatusBadGateway, CodeBroadcastRejected, "broadcast rejected")
	}
	txid, ok := NormalizeTxID(payload.Msg)
	if !ok {
		return BroadcastResult{}, NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "invalid upstream response")
	}
	return BroadcastResult{Chain: chain, TxID: txid, Accepted: true}, nil
}
```

Refactor current `getJSON` to call a shared helper:

```go
type logWalletUpstreamOptions struct {
	RedactBody bool
}

func (c *CoreClient) getJSON(req *http.Request, out any) error {
	return c.doJSON(req, out, logWalletUpstreamOptions{})
}

func (c *CoreClient) doJSON(req *http.Request, out any, options logWalletUpstreamOptions) error {
	start := time.Now()
	status := 0
	resp, err := c.client.Do(req)
	if err != nil {
		logWalletUpstream(req, status, start, sanitizeRawTxError(err.Error(), options.RedactBody))
		return NewHTTPWalletError(http.StatusBadGateway, CodeCoreUnavailable, err.Error())
	}
	defer resp.Body.Close()

	status = resp.StatusCode
	if resp.StatusCode < http.StatusOK || resp.StatusCode > 299 {
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

func sanitizeRawTxError(message string, redact bool) string {
	if !redact {
		return message
	}
	return "redacted broadcast upstream error"
}
```

Add imports `bytes` and keep existing imports sorted by gofmt.

- [ ] **Step 4: Implement broadcast service, response, handler, and route**

Modify `wallet/server.go`:

```go
func (s *GatewayService) BroadcastTransaction(ctx context.Context, chain Chain, rawTx string) (BroadcastResult, error) {
	client, ok := s.clients[chain]
	if !ok {
		return BroadcastResult{}, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "chain core is not configured")
	}
	chainCfg := s.chainConfig[chain]
	return client.BroadcastTransaction(ctx, chain, rawTx, chainCfg.BroadcastPath)
}
```

Modify `wallet/response.go`:

```go
type StandardBroadcastData struct {
	Chain    Chain  `json:"chain"`
	TxID     string `json:"txid"`
	Accepted bool   `json:"accepted"`
}

func NewStandardBroadcastResponse(result BroadcastResult) Envelope {
	return Success(StandardBroadcastData{Chain: result.Chain, TxID: result.TxID, Accepted: result.Accepted})
}
```

Modify `wallet/handlers.go`:

```go
type broadcastRequest struct {
	RawTx string `json:"rawTx"`
}

func (g *Gateway) broadcastTransaction(c *gin.Context) {
	if !g.ensureService(c) {
		return
	}
	chain, ok := g.parseChain(c)
	if !ok {
		return
	}
	var req broadcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidRawTx, "invalid raw transaction"))
		return
	}
	rawTx := strings.TrimSpace(req.RawTx)
	if rawTx == "" || len(rawTx) > 1_000_000 {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidRawTx, "invalid raw transaction"))
		return
	}
	if _, err := hex.DecodeString(rawTx); err != nil {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidRawTx, "invalid raw transaction"))
		return
	}
	result, err := g.service.BroadcastTransaction(c.Request.Context(), chain, rawTx)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewStandardBroadcastResponse(result))
}
```

Add imports `encoding/hex`.

Modify `wallet/server.go`:

```go
router.POST("/wallet/v1/:chain/tx/broadcast", gateway.broadcastTransaction)
```

Update `publicServiceError`:

```go
case CodeBroadcastRejected:
	message = "broadcast rejected"
```

- [ ] **Step 5: Run tests**

Run:

```bash
gofmt -w wallet/client.go wallet/server.go wallet/handlers.go wallet/response.go wallet/logging.go wallet/handlers_test.go wallet/broadcast_test.go
go test ./wallet -run 'Test(CoreClientBroadcast|BroadcastHandler|BroadcastHandlerSupportsAllV11Chains|BroadcastLog)' -v
go test ./wallet -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add wallet/client.go wallet/server.go wallet/handlers.go wallet/response.go wallet/logging.go wallet/handlers_test.go wallet/broadcast_test.go
git commit -m "feat: add wallet transaction broadcast"
```

---

### Task 4: Core Transaction Detail And Status Route

**Files:**
- Modify: `blockchain/client.go`
- Modify: `api/server.go`
- Create: `blockchain/client_tx_detail_test.go`
- Create: `api/server_tx_detail_test.go`

- [ ] **Step 1: Write failing transaction-detail conversion tests**

Create `blockchain/client_tx_detail_test.go`:

```go
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
```

- [ ] **Step 2: Write failing core route tests**

Create `api/server_tx_detail_test.go`:

```go
package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metaid/utxo_indexer/blockchain"
)

func TestCoreTxDetailRoute(t *testing.T) {
	txid := strings.Repeat("a", 64)
	server := NewServer(nil, nil, make(chan struct{}))
	server.getTxDetailFn = func(gotTxID string) (*blockchain.TxDetail, error) {
		if gotTxID != txid {
			t.Fatalf("txid = %s, want %s", gotTxID, txid)
		}
		return &blockchain.TxDetail{TxID: txid, Confirmed: true, Confirmations: 2}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/tx/"+txid, nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"confirmations":2`) {
		t.Fatalf("response missing confirmations: %s", rec.Body.String())
	}
}

func TestCoreTxDetailRouteNotFound(t *testing.T) {
	txid := strings.Repeat("a", 64)
	server := NewServer(nil, nil, make(chan struct{}))
	server.getTxDetailFn = func(string) (*blockchain.TxDetail, error) {
		return nil, blockchain.ErrTransactionNotFound
	}

	req := httptest.NewRequest(http.MethodGet, "/tx/"+txid, nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCoreTxDetailRouteRequiresValidTxID(t *testing.T) {
	server := NewServer(nil, nil, make(chan struct{}))
	req := httptest.NewRequest(http.MethodGet, "/tx/not-a-txid", nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCoreTxDetailRouteMapsGenericError(t *testing.T) {
	txid := strings.Repeat("a", 64)
	server := NewServer(nil, nil, make(chan struct{}))
	server.getTxDetailFn = func(string) (*blockchain.TxDetail, error) {
		return nil, errors.New("rpc unavailable")
	}

	req := httptest.NewRequest(http.MethodGet, "/tx/"+txid, nil)
	rec := httptest.NewRecorder()
	server.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 3: Run core tx tests and verify they fail**

Run:

```bash
CGO_ENABLED=0 go test ./blockchain ./api -run 'Test(TxDetail|CoreTxDetail)' -v
```

Expected: FAIL because `TxDetail`, `txDetailFromVerbose`, `ErrTransactionNotFound`, and `/tx/:txid` do not exist.

- [ ] **Step 4: Add blockchain transaction detail types and conversion**

Modify `blockchain/client.go`:

```go
var ErrTransactionNotFound = errors.New("transaction not found")

type TxDetail struct {
	TxID          string     `json:"txid"`
	Confirmed     bool       `json:"confirmed"`
	Mempool       bool       `json:"mempool"`
	Confirmations uint64     `json:"confirmations"`
	Height        *int64     `json:"height,omitempty"`
	BlockHash     string     `json:"blockHash,omitempty"`
	BlockTime     *int64     `json:"blockTime,omitempty"`
	Inputs        []TxInput  `json:"inputs"`
	Outputs       []TxOutput `json:"outputs"`
	Size          int32      `json:"size,omitempty"`
	Vsize         int32      `json:"vsize,omitempty"`
}

type TxInput struct {
	TxID     string `json:"txid,omitempty"`
	Vout     uint32 `json:"vout,omitempty"`
	Coinbase string `json:"coinbase,omitempty"`
}

type TxOutput struct {
	Vout    uint32 `json:"vout"`
	Address string `json:"address,omitempty"`
	Satoshi uint64 `json:"satoshi"`
}

func txDetailFromVerbose(raw *btcjson.TxRawResult) (*TxDetail, error) {
	if raw == nil || strings.TrimSpace(raw.Txid) == "" {
		return nil, ErrTransactionNotFound
	}
	detail := &TxDetail{
		TxID:          strings.ToLower(strings.TrimSpace(raw.Txid)),
		Confirmed:     raw.Confirmations > 0,
		Mempool:       raw.Confirmations == 0,
		Confirmations: raw.Confirmations,
		BlockHash:     raw.BlockHash,
		Inputs:        make([]TxInput, 0, len(raw.Vin)),
		Outputs:       make([]TxOutput, 0, len(raw.Vout)),
		Size:          raw.Size,
		Vsize:         raw.Vsize,
	}
	if raw.Blocktime > 0 {
		detail.BlockTime = int64Ptr(raw.Blocktime)
	}
	for _, in := range raw.Vin {
		detail.Inputs = append(detail.Inputs, TxInput{TxID: in.Txid, Vout: in.Vout, Coinbase: in.Coinbase})
	}
	for _, out := range raw.Vout {
		satoshi, err := rpcAmountToSatoshis(json.Number(strconv.FormatFloat(out.Value, 'f', 8, 64)))
		if err != nil {
			return nil, fmt.Errorf("parse tx output value: %w", err)
		}
		address := out.ScriptPubKey.Address
		if address == "" && len(out.ScriptPubKey.Addresses) > 0 {
			address = out.ScriptPubKey.Addresses[0]
		}
		detail.Outputs = append(detail.Outputs, TxOutput{Vout: out.N, Address: address, Satoshi: satoshi})
	}
	return detail, nil
}

func int64Ptr(value int64) *int64 {
	return &value
}
```

Add imports `errors` if not already present and ensure existing imports stay gofmt-sorted.

- [ ] **Step 5: Add blockchain client method**

Modify `blockchain/client.go`:

```go
func (c *Client) GetTransactionDetail(txid string) (*TxDetail, error) {
	txHash, err := chainhash.NewHashFromStr(strings.TrimSpace(txid))
	if err != nil {
		return nil, fmt.Errorf("invalid txid: %w", err)
	}
	if c == nil || c.rpcClient == nil {
		return nil, fmt.Errorf("rpc client not initialized")
	}
	raw, err := c.rpcClient.GetRawTransactionVerbose(txHash)
	if err != nil {
		if isTransactionNotFoundRPCError(err) {
			return nil, fmt.Errorf("%w: %v", ErrTransactionNotFound, err)
		}
		return nil, err
	}
	return txDetailFromVerbose(raw)
}

func isTransactionNotFoundRPCError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no information available") ||
		strings.Contains(message, "no such mempool or blockchain transaction") ||
		strings.Contains(message, "not found")
}
```

This method intentionally requires verbose transaction RPC metadata. It does not use the adapter `GetTransaction` method because that method only returns decoded inputs and outputs and cannot guarantee confirmation status.

- [ ] **Step 6: Add core HTTP route**

Modify `api/server.go`:

Add field:

```go
getTxDetailFn func(string) (*blockchain.TxDetail, error)
```

Register route:

```go
s.Router.GET("/tx/:txid", s.getTxDetail)
```

Add handler:

```go
func (s *Server) getTxDetail(c *gin.Context) {
	txid := strings.ToLower(strings.TrimSpace(c.Param("txid")))
	if len(txid) != 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid txid"})
		return
	}
	for _, ch := range txid {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid txid"})
			return
		}
	}

	fetch := s.getTxDetailFn
	if fetch == nil {
		if s.bcClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "blockchain client not configured"})
			return
		}
		fetch = s.bcClient.GetTransactionDetail
	}

	detail, err := fetch(txid)
	if err != nil {
		if errors.Is(err, blockchain.ErrTransactionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "transaction not found"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "transaction detail unavailable"})
		return
	}
	c.JSON(http.StatusOK, detail)
}
```

- [ ] **Step 7: Run tests**

Run:

```bash
gofmt -w blockchain/client.go blockchain/client_tx_detail_test.go api/server.go api/server_tx_detail_test.go
CGO_ENABLED=0 go test ./blockchain ./api -run 'Test(TxDetail|CoreTxDetail)' -v
go test ./api -run 'TestEnableWalletGatewayKeepsCoreRoutesRegistered' -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

Run:

```bash
git add blockchain/client.go blockchain/client_tx_detail_test.go api/server.go api/server_tx_detail_test.go
git commit -m "feat: expose core transaction detail"
```

---

### Task 5: Gateway Transaction Detail Endpoint

**Files:**
- Modify: `wallet/client.go`
- Modify: `wallet/server.go`
- Modify: `wallet/handlers.go`
- Modify: `wallet/response.go`
- Modify: `wallet/handlers_test.go`
- Create: `wallet/tx_detail_test.go`

- [ ] **Step 1: Write failing gateway tx detail tests**

Create `wallet/tx_detail_test.go`:

```go
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
			"inputs":        []map[string]any{{"txid": strings.Repeat("b", 64), "vout": 1}},
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
	body := marshalResponse(t, NewStandardTxDetailResponse(WalletTxDetail{
		Chain:         ChainBTC,
		TxID:          txid,
		Confirmed:     true,
		Confirmations: 2,
		Outputs:       []WalletTxOutput{{Vout: 0, Address: "addr", Satoshi: 1000}},
	}))
	for _, want := range []string{`"txid":"` + txid + `"`, `"confirmed":true`, `"confirmations":2`, `"amount":"0.00001000"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
}
```

Append handler tests to `wallet/handlers_test.go` and replace the Task 1 `GetTransaction` test stub with:

```go
func (s *fakeWalletService) GetTransaction(ctx context.Context, chain Chain, txid string) (WalletTxDetail, error) {
	if s.err != nil {
		return WalletTxDetail{}, s.err
	}
	return WalletTxDetail{Chain: chain, TxID: txid, Confirmed: true, Confirmations: 1}, nil
}

func TestTxDetailHandler(t *testing.T) {
	txid := strings.Repeat("a", 64)
	router := newHandlerTestRouter(&fakeWalletService{})

	w := performWalletRequest(router, "/wallet/v1/btc/tx/"+txid)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"txid":"`+txid+`"`) {
		t.Fatalf("response missing txid: %s", w.Body.String())
	}
}

func TestTxDetailHandlerSupportsAllV11Chains(t *testing.T) {
	txid := strings.Repeat("a", 64)
	router := newHandlerTestRouter(&fakeWalletService{})
	for _, chain := range []string{"btc", "mvc", "doge"} {
		t.Run(chain, func(t *testing.T) {
			w := performWalletRequest(router, "/wallet/v1/"+chain+"/tx/"+txid)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"chain":"`+chain+`"`) {
				t.Fatalf("response missing chain %s: %s", chain, w.Body.String())
			}
		})
	}
}

func TestTxDetailHandlerRejectsInvalidTxIDBeforeService(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})
	w := performWalletRequest(router, "/wallet/v1/btc/tx/not-a-txid")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid query parameter") {
		t.Fatalf("response missing invalid query message: %s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run tx detail gateway tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(CoreClientFetchTransaction|StandardTxDetailResponse|TxDetailHandler|TxDetailHandlerSupportsAllV11Chains)' -v
```

Expected: FAIL because gateway tx detail methods and response do not exist.

- [ ] **Step 3: Implement gateway core client transaction fetch**

Modify `wallet/client.go`:

```go
type coreTxDetailResponse struct {
	TxID          string              `json:"txid"`
	Confirmed     bool                `json:"confirmed"`
	Mempool       bool                `json:"mempool"`
	Confirmations uint64              `json:"confirmations"`
	Height        *int64              `json:"height"`
	BlockHash     string              `json:"blockHash"`
	BlockTime     *int64              `json:"blockTime"`
	Inputs        []coreTxInput       `json:"inputs"`
	Outputs       []coreTxOutput      `json:"outputs"`
	Size          int32               `json:"size"`
	Vsize         int32               `json:"vsize"`
}

type coreTxInput struct {
	TxID    string  `json:"txid"`
	Vout    uint32  `json:"vout"`
	Address string  `json:"address"`
	Satoshi *uint64 `json:"satoshi"`
}

type coreTxOutput struct {
	Vout    uint32 `json:"vout"`
	Address string `json:"address"`
	Satoshi uint64 `json:"satoshi"`
}

func (c *CoreClient) FetchTransaction(ctx context.Context, chain Chain, txid string) (WalletTxDetail, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/tx/"+txid, nil)
	if err != nil {
		return WalletTxDetail{}, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, "internal wallet error")
	}
	var payload coreTxDetailResponse
	if err := c.getJSON(req, &payload); err != nil {
		var walletErr *WalletError
		if errors.As(err, &walletErr) && walletErr.HTTPStatus == http.StatusNotFound {
			return WalletTxDetail{}, NewHTTPWalletError(http.StatusNotFound, CodeTxNotFound, "transaction not found")
		}
		return WalletTxDetail{}, err
	}
	return normalizeCoreTxDetail(chain, payload)
}
```

Update `getJSON` so upstream HTTP 404 maps to `CodeTxNotFound` for `/tx/` paths:

```go
if resp.StatusCode == http.StatusNotFound && strings.HasPrefix(req.URL.Path, "/tx/") {
	errMessage := "transaction not found"
	logWalletUpstream(req, status, start, errMessage)
	return NewHTTPWalletError(http.StatusNotFound, CodeTxNotFound, errMessage)
}
```

Add `normalizeCoreTxDetail` to `wallet/normalize.go`:

```go
func normalizeCoreTxDetail(chain Chain, payload coreTxDetailResponse) (WalletTxDetail, error) {
	txid, ok := NormalizeTxID(payload.TxID)
	if !ok {
		return WalletTxDetail{}, NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "invalid upstream response")
	}
	detail := WalletTxDetail{
		Chain:         chain,
		TxID:          txid,
		Confirmed:     payload.Confirmed,
		Mempool:       payload.Mempool,
		Confirmations: payload.Confirmations,
		Height:        payload.Height,
		BlockHash:     payload.BlockHash,
		BlockTime:     payload.BlockTime,
		Inputs:        make([]WalletTxInput, 0, len(payload.Inputs)),
		Outputs:       make([]WalletTxOutput, 0, len(payload.Outputs)),
		Size:          payload.Size,
		Vsize:         payload.Vsize,
	}
	for _, in := range payload.Inputs {
		detail.Inputs = append(detail.Inputs, WalletTxInput{TxID: in.TxID, Vout: in.Vout, Address: in.Address, Satoshi: in.Satoshi})
	}
	for _, out := range payload.Outputs {
		detail.Outputs = append(detail.Outputs, WalletTxOutput{Vout: out.Vout, Address: out.Address, Satoshi: out.Satoshi})
	}
	if detail.Confirmed && detail.Confirmations == 0 {
		return WalletTxDetail{}, NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "invalid upstream response")
	}
	if detail.Mempool && detail.Confirmations != 0 {
		return WalletTxDetail{}, NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "invalid upstream response")
	}
	return detail, nil
}
```

- [ ] **Step 4: Implement gateway tx service, response, handler, and route**

Modify `wallet/server.go`:

```go
func (s *GatewayService) GetTransaction(ctx context.Context, chain Chain, txid string) (WalletTxDetail, error) {
	client, ok := s.clients[chain]
	if !ok {
		return WalletTxDetail{}, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "chain core is not configured")
	}
	return client.FetchTransaction(ctx, chain, txid)
}
```

Modify `wallet/response.go`:

```go
type StandardTxDetailData struct {
	Chain         Chain                  `json:"chain"`
	TxID          string                 `json:"txid"`
	Confirmed     bool                   `json:"confirmed"`
	Mempool       bool                   `json:"mempool"`
	Confirmations uint64                 `json:"confirmations"`
	Height        *int64                 `json:"height"`
	BlockHash     string                 `json:"blockHash,omitempty"`
	BlockTime     *int64                 `json:"blockTime,omitempty"`
	Inputs        []StandardTxInputItem  `json:"inputs"`
	Outputs       []StandardTxOutputItem `json:"outputs"`
	FeeSatoshi    *uint64                `json:"feeSatoshi,omitempty"`
	Fee           string                 `json:"fee,omitempty"`
	Size          int32                  `json:"size,omitempty"`
	Vsize         int32                  `json:"vsize,omitempty"`
}

type StandardTxInputItem struct {
	TxID    string `json:"txid,omitempty"`
	Vout    uint32 `json:"vout,omitempty"`
	Address string `json:"address,omitempty"`
	Satoshi *uint64 `json:"satoshi,omitempty"`
	Amount  string `json:"amount,omitempty"`
}

type StandardTxOutputItem struct {
	Vout    uint32 `json:"vout"`
	Address string `json:"address,omitempty"`
	Satoshi uint64 `json:"satoshi"`
	Amount  string `json:"amount"`
}

func NewStandardTxDetailResponse(detail WalletTxDetail) Envelope {
	inputs := make([]StandardTxInputItem, 0, len(detail.Inputs))
	for _, in := range detail.Inputs {
		item := StandardTxInputItem{TxID: in.TxID, Vout: in.Vout, Address: in.Address, Satoshi: in.Satoshi}
		if in.Satoshi != nil {
			item.Amount = SatoshiToDecimalString(*in.Satoshi)
		}
		inputs = append(inputs, item)
	}
	outputs := make([]StandardTxOutputItem, 0, len(detail.Outputs))
	for _, out := range detail.Outputs {
		outputs = append(outputs, StandardTxOutputItem{Vout: out.Vout, Address: out.Address, Satoshi: out.Satoshi, Amount: SatoshiToDecimalString(out.Satoshi)})
	}
	data := StandardTxDetailData{
		Chain: detail.Chain, TxID: detail.TxID, Confirmed: detail.Confirmed, Mempool: detail.Mempool,
		Confirmations: detail.Confirmations, Height: detail.Height, BlockHash: detail.BlockHash, BlockTime: detail.BlockTime,
		Inputs: inputs, Outputs: outputs, FeeSatoshi: detail.FeeSatoshi, Size: detail.Size, Vsize: detail.Vsize,
	}
	if detail.FeeSatoshi != nil {
		data.Fee = SatoshiToDecimalString(*detail.FeeSatoshi)
	}
	return Success(data)
}
```

Modify `wallet/handlers.go`:

```go
func (g *Gateway) getTransaction(c *gin.Context) {
	if !g.ensureService(c) {
		return
	}
	chain, ok := g.parseChain(c)
	if !ok {
		return
	}
	txid, ok := NormalizeTxID(c.Param("txid"))
	if !ok {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "invalid query parameter"))
		return
	}
	detail, err := g.service.GetTransaction(c.Request.Context(), chain, txid)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewStandardTxDetailResponse(detail))
}
```

Modify `wallet/server.go`:

```go
router.GET("/wallet/v1/:chain/tx/:txid", gateway.getTransaction)
```

Update `publicServiceError`:

```go
case CodeTxNotFound:
	message = "transaction not found"
```

- [ ] **Step 5: Run tests**

Run:

```bash
gofmt -w wallet/client.go wallet/normalize.go wallet/server.go wallet/handlers.go wallet/response.go wallet/handlers_test.go wallet/tx_detail_test.go
go test ./wallet -run 'Test(CoreClientFetchTransaction|StandardTxDetailResponse|TxDetailHandler|TxDetailHandlerSupportsAllV11Chains)' -v
go test ./wallet -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add wallet/client.go wallet/normalize.go wallet/server.go wallet/handlers.go wallet/response.go wallet/handlers_test.go wallet/tx_detail_test.go
git commit -m "feat: add wallet transaction detail"
```

---

### Task 6: Core History Filtering, Sorting, And Timestamp Support

**Files:**
- Modify: `indexer/query.go`
- Modify: `api/server.go`
- Create: `indexer/history_options_test.go`

- [ ] **Step 1: Write failing pure history option tests**

Create `indexer/history_options_test.go`:

```go
package indexer

import "testing"

func TestFilterSortPaginateHistoryTxs(t *testing.T) {
	in := []HistoryTx{
		{TxID: "confirmed-old", TimestampUnix: 100, Income: 10, Type: "income", IsMempool: false},
		{TxID: "mempool-new", TimestampUnix: 300, Spend: 5, Type: "spend", IsMempool: true},
		{TxID: "confirmed-mid", TimestampUnix: 200, Income: 20, Type: "income", IsMempool: false},
	}

	got, total := filterSortPaginateHistoryTxs(in, HistoryQueryOptions{Page: 1, Limit: 2, Sort: "desc"})
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(got) != 2 || got[0].TxID != "mempool-new" || got[1].TxID != "confirmed-mid" {
		t.Fatalf("unexpected page: %+v", got)
	}

	got, total = filterSortPaginateHistoryTxs(in, HistoryQueryOptions{Page: 1, Limit: 10, Sort: "asc", ConfirmedOnly: true})
	if total != 2 {
		t.Fatalf("confirmed total = %d, want 2", total)
	}
	if len(got) != 2 || got[0].TxID != "confirmed-old" || got[1].TxID != "confirmed-mid" {
		t.Fatalf("unexpected confirmed asc page: %+v", got)
	}
}

func TestNormalizeHistoryQueryOptions(t *testing.T) {
	got := normalizeHistoryQueryOptions("0", "500", "true", "ASC")
	if got.Page != 1 || got.Limit != 100 || !got.ConfirmedOnly || got.Sort != "asc" {
		t.Fatalf("unexpected normalized options: %+v", got)
	}
	got = normalizeHistoryQueryOptions("2", "20", "false", "bad")
	if got.Page != 2 || got.Limit != 20 || got.ConfirmedOnly || got.Sort != "desc" {
		t.Fatalf("unexpected fallback options: %+v", got)
	}
}
```

- [ ] **Step 2: Run history option tests and verify they fail**

Run:

```bash
go test ./indexer -run 'Test(FilterSortPaginateHistoryTxs|NormalizeHistoryQueryOptions)' -v
```

Expected: FAIL because history options helpers and `TimestampUnix` do not exist.

- [ ] **Step 3: Extend history model and pure helpers**

Modify `indexer/query.go`:

```go
type HistoryTx struct {
	TxID          string `json:"tx_id"`
	Timestamp     string `json:"time"`
	TimestampUnix int64  `json:"timestamp,omitempty"`
	Income        uint64 `json:"income"`
	Spend         uint64 `json:"spend"`
	Type          string `json:"type"`
	IsMempool     bool   `json:"is_mempool"`
	Confirmations *uint64 `json:"confirmations,omitempty"`
	Height        *int64  `json:"height"`
}

type HistoryQueryOptions struct {
	Page          int
	Limit         int
	ConfirmedOnly bool
	Sort          string
}
```

When building history entries in `GetHistoryTxList`, set both time fields:

```go
txMap[txid] = &HistoryTx{
	TxID:          txid,
	Timestamp:     time.Unix(ts, 0).Format("2006-01-02 15:04:05"),
	TimestampUnix: ts,
	IsMempool:     isMempool,
	Confirmations: historyConfirmations(isMempool),
	Height:        nil,
}
```

Add:

```go
func historyConfirmations(isMempool bool) *uint64 {
	if !isMempool {
		return nil
	}
	zero := uint64(0)
	return &zero
}
```

This implementation does not fabricate confirmed transaction heights or
confirmation counts because the existing address history stores do not persist
block height. Confirmed history items use `confirmed=true`, `mempool=false`,
`height=null`, and omit `confirmations` until the index stores height. Mempool
history items explicitly return `confirmations=0`.

Add helpers:

```go
func normalizeHistoryQueryOptions(pageStr, limitStr, confirmedOnlyStr, sortStr string) HistoryQueryOptions {
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	sortOrder := strings.ToLower(strings.TrimSpace(sortStr))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	return HistoryQueryOptions{
		Page:          page,
		Limit:         limit,
		ConfirmedOnly: parseBoolQuery(confirmedOnlyStr),
		Sort:          sortOrder,
	}
}

func filterSortPaginateHistoryTxs(txs []HistoryTx, options HistoryQueryOptions) ([]HistoryTx, int64) {
	filtered := make([]HistoryTx, 0, len(txs))
	for _, tx := range txs {
		if options.ConfirmedOnly && tx.IsMempool {
			continue
		}
		filtered = append(filtered, tx)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if options.Sort == "asc" {
			return filtered[i].TimestampUnix < filtered[j].TimestampUnix
		}
		return filtered[i].TimestampUnix > filtered[j].TimestampUnix
	})
	total := int64(len(filtered))
	start := (options.Page - 1) * options.Limit
	if start >= len(filtered) {
		return []HistoryTx{}, total
	}
	end := start + options.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total
}
```

If `parseBoolQuery` is only in `api/server.go`, add an indexer-local helper:

```go
func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Add option-aware history method while preserving old method**

Modify `indexer/query.go`:

```go
func (i *UTXOIndexer) GetHistoryUTXOs(address string, pageStr string, limitStr string) ([]HistoryTx, int64, error) {
	return i.GetHistoryUTXOsWithOptions(address, pageStr, limitStr, "", "desc")
}

func (i *UTXOIndexer) GetHistoryUTXOsWithOptions(address string, pageStr string, limitStr string, confirmedOnlyStr string, sortStr string) ([]HistoryTx, int64, error) {
	txs, err := i.GetHistoryTxList(address)
	if err != nil {
		return nil, 0, err
	}
	options := normalizeHistoryQueryOptions(pageStr, limitStr, confirmedOnlyStr, sortStr)
	page, total := filterSortPaginateHistoryTxs(txs, options)
	return page, total, nil
}
```

- [ ] **Step 5: Extend API history route query handling**

Modify `api/server.go` in `getHistoryUTXOs`:

```go
confirmedOnly := c.DefaultQuery("confirmedOnly", "false")
sortOrder := c.DefaultQuery("sort", "desc")
utxos, total, err := s.indexer.GetHistoryUTXOsWithOptions(address, page, limit, confirmedOnly, sortOrder)
```

Keep the existing response envelope:

```go
c.JSON(http.StatusOK, gin.H{
	"address": address,
	"list":    utxos,
	"count":   len(utxos),
	"total":   total,
})
```

- [ ] **Step 6: Run tests**

Run:

```bash
gofmt -w indexer/query.go indexer/history_options_test.go api/server.go
go test ./indexer -run 'Test(FilterSortPaginateHistoryTxs|NormalizeHistoryQueryOptions)' -v
go test ./api -run 'TestEnableWalletGatewayKeepsCoreRoutesRegistered' -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add indexer/query.go indexer/history_options_test.go api/server.go
git commit -m "feat: support stable history queries"
```

---

### Task 7: Gateway Address History Endpoint

**Files:**
- Modify: `wallet/client.go`
- Modify: `wallet/server.go`
- Modify: `wallet/handlers.go`
- Modify: `wallet/normalize.go`
- Modify: `wallet/response.go`
- Modify: `wallet/handlers_test.go`
- Create: `wallet/history_test.go`

- [ ] **Step 1: Write failing gateway history tests**

Create `wallet/history_test.go`:

```go
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

func TestCoreClientFetchHistory(t *testing.T) {
	var gotQuery string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/utxos/history" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"address": "addr",
			"total":   1,
			"list": []map[string]any{
				{"tx_id": strings.Repeat("a", 64), "income": uint64(1000), "spend": uint64(0), "type": "income", "is_mempool": true, "timestamp": int64(1717833600), "time": "2026-06-08 12:00:00"},
			},
		})
	}))
	defer core.Close()

	client, err := NewCoreClient(core.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewCoreClient: %v", err)
	}
	got, err := client.FetchHistory(context.Background(), ChainBTC, "addr", HistoryOptions{Page: 1, Limit: 20, ConfirmedOnly: false, Sort: "desc"})
	if err != nil {
		t.Fatalf("FetchHistory: %v", err)
	}
	for _, want := range []string{"address=addr", "page=1", "limit=20", "confirmedOnly=false", "sort=desc"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query missing %s: %s", want, gotQuery)
		}
	}
	if got.Total != 1 || len(got.Items) != 1 || !got.Items[0].Mempool || got.Items[0].NetSatoshi != 1000 {
		t.Fatalf("unexpected history page: %+v", got)
	}
}

func TestNormalizeHistoryOptions(t *testing.T) {
	got, err := NormalizeHistoryOptions("0", "500", "true", "ASC")
	if err != nil {
		t.Fatalf("NormalizeHistoryOptions: %v", err)
	}
	if got.Page != 1 || got.Limit != 100 || !got.ConfirmedOnly || got.Sort != "asc" {
		t.Fatalf("unexpected options: %+v", got)
	}
	if _, err := NormalizeHistoryOptions("1", "20", "invalid", "desc"); err == nil {
		t.Fatal("expected invalid confirmedOnly error")
	}
	if _, err := NormalizeHistoryOptions("1", "20", "false", "sideways"); err == nil {
		t.Fatal("expected invalid sort error")
	}
}

func TestStandardHistoryResponse(t *testing.T) {
	txid := strings.Repeat("a", 64)
	body := marshalResponse(t, NewStandardHistoryResponse(WalletHistoryPage{
		Chain:   ChainBTC,
		Address: "addr",
		Page:    1,
		Limit:   20,
		Sort:    "desc",
		Total:   1,
		Items: []WalletHistoryItem{{
			TxID: txid, Direction: "income", IncomeSatoshi: 1000, NetSatoshi: 1000,
			Mempool: true, Confirmed: false, Timestamp: 1717833600, Time: "2026-06-08 12:00:00",
		}},
	}))
	for _, want := range []string{`"txid":"` + txid + `"`, `"mempool":true`, `"netSatoshi":1000`, `"net":"0.00001000"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
}
```

Append handler tests to `wallet/handlers_test.go` and replace the Task 1 `GetHistory` test stub with:

```go
func (s *fakeWalletService) GetHistory(ctx context.Context, chain Chain, address string, options HistoryOptions) (WalletHistoryPage, error) {
	if s.err != nil {
		return WalletHistoryPage{}, s.err
	}
	return WalletHistoryPage{
		Chain: chain, Address: address, Page: options.Page, Limit: options.Limit, ConfirmedOnly: options.ConfirmedOnly, Sort: options.Sort, Total: 1,
		Items: []WalletHistoryItem{{TxID: strings.Repeat("a", 64), Direction: "income", IncomeSatoshi: 1000, NetSatoshi: 1000, Mempool: true, Timestamp: 1717833600}},
	}, nil
}

func TestHistoryHandler(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})
	w := performWalletRequest(router, "/wallet/v1/btc/address/addr/history?page=1&limit=20")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"address":"addr"`) || !strings.Contains(w.Body.String(), `"mempool":true`) {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestHistoryHandlerSupportsAllV11Chains(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})
	for _, chain := range []string{"btc", "mvc", "doge"} {
		t.Run(chain, func(t *testing.T) {
			w := performWalletRequest(router, "/wallet/v1/"+chain+"/address/addr/history?page=1&limit=20")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"chain":"`+chain+`"`) {
				t.Fatalf("response missing chain %s: %s", chain, w.Body.String())
			}
		})
	}
}

func TestHistoryHandlerRejectsInvalidSortBeforeService(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})
	w := performWalletRequest(router, "/wallet/v1/btc/address/addr/history?sort=random")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sort must be asc or desc") {
		t.Fatalf("response missing sort validation message: %s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run history gateway tests and verify they fail**

Run:

```bash
go test ./wallet -run 'Test(CoreClientFetchHistory|NormalizeHistoryOptions|StandardHistoryResponse|HistoryHandler|HistoryHandlerSupportsAllV11Chains)' -v
```

Expected: FAIL because history client, normalizer, response, and route do not exist.

- [ ] **Step 3: Implement history option normalization**

Modify `wallet/normalize.go`:

```go
func NormalizeHistoryOptions(pageRaw, limitRaw, confirmedOnlyRaw, sortRaw string) (HistoryOptions, error) {
	page, err := strconv.Atoi(strings.TrimSpace(pageRaw))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(strings.TrimSpace(limitRaw))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	confirmedOnly, err := parseBoolDefault(confirmedOnlyRaw, false)
	if err != nil {
		return HistoryOptions{}, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "confirmedOnly must be true or false")
	}
	sortOrder := strings.ToLower(strings.TrimSpace(sortRaw))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return HistoryOptions{}, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "sort must be asc or desc")
	}
	return HistoryOptions{Page: page, Limit: limit, ConfirmedOnly: confirmedOnly, Sort: sortOrder}, nil
}
```

- [ ] **Step 4: Implement history core client**

Modify `wallet/client.go`:

```go
type coreHistoryResponse struct {
	Address string            `json:"address"`
	List    []coreHistoryItem `json:"list"`
	Count   int               `json:"count"`
	Total   int64             `json:"total"`
}

type coreHistoryItem struct {
	TxID          string `json:"tx_id"`
	Time          string `json:"time"`
	Timestamp     int64  `json:"timestamp"`
	Income        uint64 `json:"income"`
	Spend         uint64 `json:"spend"`
	Type          string `json:"type"`
	IsMempool     bool   `json:"is_mempool"`
	Confirmations *uint64 `json:"confirmations"`
	Height        *int64  `json:"height"`
}

func (c *CoreClient) FetchHistory(ctx context.Context, chain Chain, address string, options HistoryOptions) (WalletHistoryPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/utxos/history", nil)
	if err != nil {
		return WalletHistoryPage{}, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, "internal wallet error")
	}
	query := req.URL.Query()
	query.Set("address", address)
	query.Set("page", strconv.Itoa(options.Page))
	query.Set("limit", strconv.Itoa(options.Limit))
	query.Set("confirmedOnly", strconv.FormatBool(options.ConfirmedOnly))
	query.Set("sort", options.Sort)
	req.URL.RawQuery = query.Encode()

	var payload coreHistoryResponse
	if err := c.getJSON(req, &payload); err != nil {
		return WalletHistoryPage{}, err
	}
	return normalizeCoreHistory(chain, address, options, payload)
}
```

Add imports `strconv`.

Add `normalizeCoreHistory` to `wallet/normalize.go`:

```go
func normalizeCoreHistory(chain Chain, address string, options HistoryOptions, payload coreHistoryResponse) (WalletHistoryPage, error) {
	items := make([]WalletHistoryItem, 0, len(payload.List))
	for _, item := range payload.List {
		txid, ok := NormalizeTxID(item.TxID)
		if !ok {
			return WalletHistoryPage{}, NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "invalid upstream response")
		}
		net := uint64DeltaToInt64(item.Income, item.Spend)
		confirmed := !item.IsMempool
		historyItem := WalletHistoryItem{
			TxID: txid, Direction: item.Type, IncomeSatoshi: item.Income, SpendSatoshi: item.Spend, NetSatoshi: net,
			Confirmed: confirmed, Mempool: item.IsMempool, Confirmations: item.Confirmations, Height: item.Height,
			Timestamp: item.Timestamp, Time: item.Time,
		}
		if historyItem.Direction == "" {
			historyItem.Direction = directionFromAmounts(item.Income, item.Spend)
		}
		if historyItem.Mempool {
			zero := uint64(0)
			historyItem.Confirmations = &zero
		}
		items = append(items, historyItem)
	}
	return WalletHistoryPage{
		Chain: chain, Address: address, Page: options.Page, Limit: options.Limit,
		ConfirmedOnly: options.ConfirmedOnly, Sort: options.Sort, Total: payload.Total, Items: items,
	}, nil
}

func directionFromAmounts(income, spend uint64) string {
	if income > 0 && spend > 0 {
		return "mixed"
	}
	if spend > 0 {
		return "spend"
	}
	return "income"
}
```

- [ ] **Step 5: Implement history service, response, handler, and route**

Modify `wallet/server.go`:

```go
func (s *GatewayService) GetHistory(ctx context.Context, chain Chain, address string, options HistoryOptions) (WalletHistoryPage, error) {
	client, ok := s.clients[chain]
	if !ok {
		return WalletHistoryPage{}, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "chain core is not configured")
	}
	return client.FetchHistory(ctx, chain, address, options)
}
```

Modify `wallet/response.go`:

```go
type StandardHistoryData struct {
	Chain         Chain                 `json:"chain"`
	Address       string                `json:"address"`
	Page          int                   `json:"page"`
	Limit         int                   `json:"limit"`
	ConfirmedOnly bool                  `json:"confirmedOnly"`
	Sort          string                `json:"sort"`
	Total         int64                 `json:"total"`
	Items         []StandardHistoryItem `json:"items"`
}

type StandardHistoryItem struct {
	TxID          string  `json:"txid"`
	Direction     string  `json:"direction"`
	IncomeSatoshi uint64  `json:"incomeSatoshi"`
	SpendSatoshi  uint64  `json:"spendSatoshi"`
	NetSatoshi    int64   `json:"netSatoshi"`
	Income        string  `json:"income"`
	Spend         string  `json:"spend"`
	Net           string  `json:"net"`
	Confirmed     bool    `json:"confirmed"`
	Mempool       bool    `json:"mempool"`
	Confirmations *uint64 `json:"confirmations,omitempty"`
	Height        *int64  `json:"height"`
	Timestamp     int64   `json:"timestamp"`
	Time          string  `json:"time,omitempty"`
}

func NewStandardHistoryResponse(page WalletHistoryPage) Envelope {
	items := make([]StandardHistoryItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, StandardHistoryItem{
			TxID: item.TxID, Direction: item.Direction,
			IncomeSatoshi: item.IncomeSatoshi, SpendSatoshi: item.SpendSatoshi, NetSatoshi: item.NetSatoshi,
			Income: SatoshiToDecimalString(item.IncomeSatoshi), Spend: SatoshiToDecimalString(item.SpendSatoshi), Net: SignedSatoshiToDecimalString(item.NetSatoshi),
			Confirmed: item.Confirmed, Mempool: item.Mempool, Confirmations: item.Confirmations, Height: item.Height,
			Timestamp: item.Timestamp, Time: item.Time,
		})
	}
	return Success(StandardHistoryData{
		Chain: page.Chain, Address: page.Address, Page: page.Page, Limit: page.Limit,
		ConfirmedOnly: page.ConfirmedOnly, Sort: page.Sort, Total: page.Total, Items: items,
	})
}
```

Modify `wallet/handlers.go`:

```go
func (g *Gateway) getHistory(c *gin.Context) {
	if !g.ensureService(c) {
		return
	}
	chain, ok := g.parseChain(c)
	if !ok {
		return
	}
	address := strings.TrimSpace(c.Param("address"))
	if address == "" {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidAddress, "address is required"))
		return
	}
	options, err := NormalizeHistoryOptions(c.DefaultQuery("page", "1"), c.DefaultQuery("limit", "20"), c.Query("confirmedOnly"), c.DefaultQuery("sort", "desc"))
	if err != nil {
		var walletErr *WalletError
		if errors.As(err, &walletErr) {
			g.writeWalletError(c, walletErr)
			return
		}
		g.writeServiceError(c, err)
		return
	}
	page, err := g.service.GetHistory(c.Request.Context(), chain, address, options)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewStandardHistoryResponse(page))
}
```

Modify `wallet/server.go`:

```go
router.GET("/wallet/v1/:chain/address/:address/history", gateway.getHistory)
```

- [ ] **Step 6: Run tests**

Run:

```bash
gofmt -w wallet/client.go wallet/server.go wallet/handlers.go wallet/normalize.go wallet/response.go wallet/handlers_test.go wallet/history_test.go
go test ./wallet -run 'Test(CoreClientFetchHistory|NormalizeHistoryOptions|StandardHistoryResponse|HistoryHandler|HistoryHandlerSupportsAllV11Chains)' -v
go test ./wallet -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add wallet/client.go wallet/server.go wallet/handlers.go wallet/normalize.go wallet/response.go wallet/handlers_test.go wallet/history_test.go
git commit -m "feat: add wallet address history"
```

---

### Task 8: Route Integration, Smoke Docs, And Full Verification

**Files:**
- Modify: `api/server_wallet_test.go`
- Modify: `docs/wallet-gateway-smoke-checks.md`

- [ ] **Step 1: Update route registration tests**

Modify `api/server_wallet_test.go` in `TestEnableWalletGatewayKeepsCoreRoutesRegistered` to include:

```go
for _, expected := range []string{
	"GET /balance",
	"GET /utxos",
	"GET /mempool/utxos",
	"GET /tx/:txid",
	"GET /wallet/v1/:chain/address/:address/balance",
	"GET /wallet/v1/:chain/address/:address/utxos",
	"POST /wallet/v1/:chain/tx/broadcast",
	"GET /wallet/v1/:chain/tx/:txid",
	"GET /wallet/v1/:chain/address/:address/history",
	"GET /wallet/v1/:chain/fee-rate",
} {
	if !routes[expected] {
		t.Fatalf("missing route %s; got %#v", expected, routes)
	}
}
```

Update `hasWalletRoutes`:

```go
func hasWalletRoutes(server *Server) bool {
	routes := routeSet(server)
	return routes["GET /wallet/v1/:chain/address/:address/balance"] ||
		routes["GET /wallet/v1/:chain/address/:address/utxos"] ||
		routes["POST /wallet/v1/:chain/tx/broadcast"] ||
		routes["GET /wallet/v1/:chain/tx/:txid"] ||
		routes["GET /wallet/v1/:chain/address/:address/history"] ||
		routes["GET /wallet/v1/:chain/fee-rate"]
}
```

- [ ] **Step 2: Update smoke check documentation**

Append to `docs/wallet-gateway-smoke-checks.md`:

````markdown
## Wallet Gateway v1.1 Smoke Checks

Set these variables before running checks:

```bash
BASE_URL="http://127.0.0.1:3001"
CHAIN="btc"
ADDRESS="replace-with-funded-address"
TXID="replace-with-known-transaction-id"
```

Fee-rate check:

```bash
curl -s "$BASE_URL/wallet/v1/$CHAIN/fee-rate" | jq .
```

Expected:

- `code` is `0`
- `data.source` is `config`
- `data.unit` is `sat_per_byte`
- `data.slow`, `data.normal`, and `data.fast` are positive integers

History check:

```bash
curl -s "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/history?page=1&limit=20" | jq .
curl -s "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/history?page=1&limit=20&confirmedOnly=true" | jq .
```

Expected:

- default history may include mempool items
- `confirmedOnly=true` excludes items where `mempool` is `true`
- `timestamp` is numeric when the core has timestamp data

Transaction detail check:

```bash
curl -s "$BASE_URL/wallet/v1/$CHAIN/tx/$TXID" | jq .
```

Expected:

- `code` is `0`
- `data.txid` equals `$TXID`
- `data.confirmed`, `data.mempool`, and `data.confirmations` are present
- confirmed transactions have `confirmations >= 1`
- mempool transactions have `confirmations = 0`

Broadcast check:

```bash
SIGNED_RAW_TX="replace-with-signed-raw-transaction-hex"
curl -s -X POST "$BASE_URL/wallet/v1/$CHAIN/tx/broadcast" \
  -H 'Content-Type: application/json' \
  -d "{\"rawTx\":\"$SIGNED_RAW_TX\"}" | jq .
```

Expected on accepted transaction:

- `code` is `0`
- `data.accepted` is `true`
- `data.txid` is present

Expected on rejected transaction:

- `code` is `-5004`
- `message` is `broadcast rejected`
````

Keep the existing v1 smoke checks in the same file.

- [ ] **Step 3: Run focused test suites**

Run:

```bash
gofmt -w api/server_wallet_test.go
go test ./wallet -v
go test ./api -run 'Test(EnableWalletGateway|CoreTxDetail)' -v
go test ./indexer -run 'Test(FilterSortPaginateHistoryTxs|NormalizeHistoryQueryOptions)' -v
CGO_ENABLED=0 go test ./blockchain -run 'TestTxDetail' -v
```

Expected: PASS.

- [ ] **Step 4: Run broader verification**

Run:

```bash
CGO_ENABLED=0 go test ./wallet ./api ./indexer ./blockchain
```

Expected: PASS. If an unrelated pre-existing package failure appears outside the changed areas, capture the exact failure and run the focused suites above again before reporting it.

- [ ] **Step 5: Confirm v1 behavior remains stable**

Run:

```bash
go test ./wallet -run 'Test(BalanceHandler|UTXOHandler|StandardBalance|StandardUTXO|Metalet)' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add api/server_wallet_test.go docs/wallet-gateway-smoke-checks.md
git commit -m "docs: add wallet gateway v1.1 smoke checks"
```

---

## Final Verification Checklist

Run after all tasks are merged:

```bash
git status --short
CGO_ENABLED=0 go test ./wallet ./api ./indexer ./blockchain
go test ./wallet -v
go test ./api -run 'Test(EnableWalletGateway|CoreTxDetail)' -v
go test ./indexer -run 'Test(FilterSortPaginateHistoryTxs|NormalizeHistoryQueryOptions)' -v
CGO_ENABLED=0 go test ./blockchain -run 'TestTxDetail' -v
```

Expected:

- working tree is clean except for intentional uncommitted integration changes;
- all focused wallet gateway suites pass;
- v1 balance and UTXO tests still pass;
- v1.1 fee-rate, broadcast, tx detail, and history tests pass;
- no logs include raw transaction bodies.

## Review Focus

Before merging implementation work, review these risks:

- `rawTx` must not appear in logs, public errors, test failure strings, or upstream sanitized messages.
- Transaction detail must not claim confirmed status unless the core supplies `confirmations > 0`.
- History filtering and sorting must happen before pagination.
- Fee-rate defaults must exist for every enabled chain without downstream configuration.
- Existing v1 Metalet compatibility tests must still pass.
