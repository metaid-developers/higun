package wallet

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Config struct {
	Enabled bool
	Timeout time.Duration
	Chains  map[Chain]ChainConfig
}

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

type Gateway struct {
	service WalletService
}

func NewGateway(service WalletService) *Gateway {
	return &Gateway{service: service}
}

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

func RegisterRoutes(router gin.IRouter, gateway *Gateway) {
	router.GET("/wallet/v1/:chain/fee-rate", gateway.getFeeRate)
	router.GET("/wallet/v1/:chain/address/:address/balance", gateway.getBalance)
	router.GET("/wallet/v1/:chain/address/:address/utxos", gateway.getUTXOs)
}
