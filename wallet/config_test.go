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
