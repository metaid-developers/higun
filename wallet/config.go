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
