package blockchain

import (
	"fmt"

	"github.com/metaid/utxo_indexer/config"
)

// NewChainAdapter is the adapter factory - creates corresponding chain adapter based on configuration
func NewChainAdapter(cfg *config.Config) (ChainAdapter, error) {
	switch cfg.Chain {
	case config.ChainBTC:
		return NewBTCAdapter(cfg)

	case config.ChainMVC:
		return NewMVCAdapter(cfg)

	case config.ChainDOGE:
		return NewDOGEAdapter(cfg)

	default:
		return nil, fmt.Errorf("unsupported chain: %s, supported chains: btc, mvc, doge", cfg.Chain)
	}
}
