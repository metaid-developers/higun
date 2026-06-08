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
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
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
		Chain:                chain,
		Address:              address,
		ConfirmedSatoshi:     payload.ConfirmedBalanceSatoshi,
		MempoolIncomeSatoshi: payload.MempoolIncomeSatoshi,
		MempoolSpendSatoshi:  payload.MempoolSpendSatoshi,
		UnsafeSatoshi:        payload.UnsafeFeeSatoshi,
		ConfirmedUTXOCount:   payload.ConfirmedUTXOCount,
		MempoolUTXOCount:     payload.MempoolUTXOCount,
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
