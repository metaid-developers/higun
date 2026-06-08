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
	clients := make(map[Chain]*CoreClient, len(cfg.Chains))
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
	router.GET("/wallet/v1/:chain/address/:address/balance", gateway.getBalance)
	router.GET("/wallet/v1/:chain/address/:address/utxos", gateway.getUTXOs)
}
