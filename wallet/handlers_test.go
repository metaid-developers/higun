package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeWalletService struct {
	balance      WalletBalance
	utxos        []WalletUTXO
	feeRate      FeeRate
	err          error
	balanceCalls int
	utxoCalls    int
}

func (s *fakeWalletService) GetBalance(ctx context.Context, chain Chain, address string) (WalletBalance, error) {
	s.balanceCalls++
	if s.err != nil {
		return WalletBalance{}, s.err
	}
	balance := s.balance
	balance.Chain = chain
	balance.Address = address
	return balance, nil
}

func (s *fakeWalletService) GetUTXOs(ctx context.Context, chain Chain, address string) ([]WalletUTXO, error) {
	s.utxoCalls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]WalletUTXO, 0, len(s.utxos))
	for _, utxo := range s.utxos {
		utxo.Chain = chain
		utxo.Address = address
		out = append(out, utxo)
	}
	return out, nil
}

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

func newHandlerTestRouter(service WalletService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, NewGateway(service))
	return router
}

func TestBalanceHandlerStandard(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{balance: WalletBalance{
		ConfirmedSatoshi: 135758,
		UnsafeSatoshi:    134862,
	}})

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/balance")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"code":0`,
		`"message":"success"`,
		`"safeSatoshi":896`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
}

func TestBalanceHandlerMetalet(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{balance: WalletBalance{
		ConfirmedSatoshi: 135758,
		UnsafeSatoshi:    134862,
	}})

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/balance?format=metalet")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"safeBalance":0.00000896`) {
		t.Fatalf("metalet response missing safeBalance: %s", w.Body.String())
	}
}

func TestBalanceHandlerSupportsAllFirstVersionChains(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{balance: WalletBalance{ConfirmedSatoshi: 1}})
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

func TestUTXOHandlerDefaultsToMempoolIncluded(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{utxos: []WalletUTXO{
		{TxID: "confirmed-small", Vout: 0, Satoshi: 100, Confirmed: true, Mempool: false, Height: int64Ptr(100)},
		{TxID: "mempool-large", Vout: 1, Satoshi: 200, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	}})

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/utxos")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"confirmedOnly":false`,
		`"sort":"desc"`,
		`"total":2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
	if strings.Index(body, `"txid":"mempool-large"`) > strings.Index(body, `"txid":"confirmed-small"`) {
		t.Fatalf("default desc sort should place larger mempool utxo first: %s", body)
	}
}

func TestUTXOHandlerConfirmedOnly(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{utxos: []WalletUTXO{
		{TxID: "confirmed", Vout: 0, Satoshi: 100, Confirmed: true, Mempool: false},
		{TxID: "mempool", Vout: 1, Satoshi: 200, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	}})

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/utxos?confirmedOnly=true")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"confirmedOnly":true`,
		`"total":1`,
		`"txid":"confirmed"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"txid":"mempool"`) {
		t.Fatalf("confirmedOnly=true should filter mempool utxos: %s", body)
	}
}

func TestUTXOHandlerSupportsAllFirstVersionChains(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{utxos: []WalletUTXO{
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

func TestUTXOHandlerRejectsInvalidConfirmedOnlyBeforeService(t *testing.T) {
	service := &fakeWalletService{}
	router := newHandlerTestRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/wallet/v1/btc/address/addr/utxos?confirmedOnly=maybe", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "confirmedOnly must be true or false") {
		t.Fatalf("invalid confirmedOnly status/body = %d %s", rec.Code, rec.Body.String())
	}
	if service.utxoCalls != 0 {
		t.Fatalf("service calls = %d, want 0", service.utxoCalls)
	}
}

func TestUTXOHandlerRejectsInvalidSortBeforeService(t *testing.T) {
	service := &fakeWalletService{utxos: []WalletUTXO{
		{TxID: "tx", Vout: 0, Satoshi: 100, Confirmed: true},
	}}
	router := newHandlerTestRouter(service)

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/utxos?sort=random")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sort must be asc or desc") {
		t.Fatalf("response missing sort validation message: %s", w.Body.String())
	}
	if service.utxoCalls != 0 {
		t.Fatalf("service calls = %d, want 0", service.utxoCalls)
	}
}

func TestUTXOHandlerNormalizesSortQuery(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "uppercase",
			path: "/wallet/v1/btc/address/addr-btc/utxos?sort=DESC",
			want: `"sort":"desc"`,
		},
		{
			name: "trimmed",
			path: "/wallet/v1/btc/address/addr-btc/utxos?sort=desc%20",
			want: `"sort":"desc"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newHandlerTestRouter(&fakeWalletService{utxos: []WalletUTXO{
				{TxID: "tx", Vout: 0, Satoshi: 100, Confirmed: true},
			}})

			w := performWalletRequest(router, tt.path)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.want) {
				t.Fatalf("response missing normalized sort %s: %s", tt.want, w.Body.String())
			}
		})
	}
}

func TestHandlerRejectsInvalidChainAndFormat(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{})

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "unsupported chain",
			path:       "/wallet/v1/ltc/address/addr/balance",
			wantStatus: http.StatusNotFound,
			wantBody:   "unsupported chain",
		},
		{
			name:       "invalid format",
			path:       "/wallet/v1/btc/address/addr/balance?format=legacy",
			wantStatus: http.StatusBadRequest,
			wantBody:   "format must be standard or metalet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := performWalletRequest(router, tt.path)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Fatalf("response missing %q: %s", tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestHandlerMapsCoreUnavailableToBadGateway(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{
		err: NewHTTPWalletError(http.StatusBadGateway, CodeCoreUnavailable, "dial tcp http://internal-core.local/balance failed"),
	})

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/balance")

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "core unavailable") {
		t.Fatalf("response missing service error: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "internal-core.local") {
		t.Fatalf("response leaked upstream detail: %s", w.Body.String())
	}
}

func TestHandlerMapsInvalidUpstreamToSafeMessage(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{
		err: NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "json parse failed for addr-btc via http://internal-core.local"),
	})

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/balance")

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "invalid upstream response") {
		t.Fatalf("response missing safe upstream message: %s", body)
	}
	for _, leaked := range []string{"json parse failed", "addr-btc", "internal-core.local"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("response leaked %q: %s", leaked, body)
		}
	}
}

func TestGatewayServiceDisabledChainReturnsServiceUnavailable(t *testing.T) {
	service, err := NewGatewayService(Config{
		Timeout: time.Second,
		Chains: map[Chain]ChainConfig{
			ChainBTC: {Enabled: false, CoreURL: "http://example.test"},
		},
	})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}

	_, err = service.GetBalance(context.Background(), ChainBTC, "addr-btc")

	var walletErr *WalletError
	if !errors.As(err, &walletErr) {
		t.Fatalf("error = %v, want WalletError", err)
	}
	if walletErr.HTTPStatus != http.StatusServiceUnavailable || walletErr.Code != CodeCoreUnavailable {
		t.Fatalf("wallet error = %+v, want HTTP 503 code %d", walletErr, CodeCoreUnavailable)
	}
}

func TestHandlerMapsPlainErrorToInternalError(t *testing.T) {
	router := newHandlerTestRouter(&fakeWalletService{err: errors.New("plain failure with http://internal-service.local and addr-btc")})

	w := performWalletRequest(router, "/wallet/v1/btc/address/addr-btc/balance")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "internal wallet error") {
		t.Fatalf("response missing safe internal error: %s", body)
	}
	for _, leaked := range []string{"plain failure", "internal-service.local", "addr-btc"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("response leaked %q: %s", leaked, body)
		}
	}
}

func TestHandlerReturnsServiceUnavailableWhenServiceIsNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, NewGateway(nil))

	for _, path := range []string{
		"/wallet/v1/btc/address/addr-btc/balance",
		"/wallet/v1/btc/address/addr-btc/utxos",
	} {
		t.Run(path, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("handler panicked: %v", recovered)
				}
			}()

			w := performWalletRequest(router, path)

			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "wallet service is not configured") {
				t.Fatalf("response missing nil service message: %s", w.Body.String())
			}
		})
	}
}

func TestGatewayServiceUsesConfiguredCoreClient(t *testing.T) {
	core := newCoreTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/balance" {
			t.Fatalf("path = %s, want /balance", r.URL.Path)
		}
		if got := r.URL.Query().Get("address"); got != "addr-btc" {
			t.Fatalf("address = %s, want addr-btc", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"confirmed_balance_satoshi": uint64(135758),
			"unsafe_fee_satoshi":        uint64(134862),
		})
	})
	defer core.Close()
	service := newGatewayServiceForCore(t, map[Chain]string{ChainBTC: core.URL})

	got, err := service.GetBalance(context.Background(), ChainBTC, "addr-btc")

	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if got.Chain != ChainBTC || got.Address != "addr-btc" || got.SafeSatoshi() != 896 {
		t.Fatalf("unexpected balance: %+v", got)
	}
}

func TestGatewayServiceRoutesEachChainToConfiguredCore(t *testing.T) {
	btcCore := newBalanceCoreForChain(t, ChainBTC)
	defer btcCore.Close()
	mvcCore := newBalanceCoreForChain(t, ChainMVC)
	defer mvcCore.Close()
	dogeCore := newBalanceCoreForChain(t, ChainDOGE)
	defer dogeCore.Close()
	service := newGatewayServiceForCore(t, map[Chain]string{
		ChainBTC:  btcCore.URL,
		ChainMVC:  mvcCore.URL,
		ChainDOGE: dogeCore.URL,
	})

	for _, chain := range []Chain{ChainBTC, ChainMVC, ChainDOGE} {
		got, err := service.GetBalance(context.Background(), chain, "addr-"+string(chain))
		if err != nil {
			t.Fatalf("GetBalance(%s): %v", chain, err)
		}
		if got.Chain != chain || got.Address != "addr-"+string(chain) || got.ConfirmedSatoshi != chainBalanceAmount(chain) {
			t.Fatalf("GetBalance(%s) routed to wrong core: %+v", chain, got)
		}
	}
}

func TestGatewayServiceRoutesEachChainUTXOsToConfiguredCore(t *testing.T) {
	btcCore := newUTXOCoreForChain(t, ChainBTC)
	defer btcCore.Close()
	mvcCore := newUTXOCoreForChain(t, ChainMVC)
	defer mvcCore.Close()
	dogeCore := newUTXOCoreForChain(t, ChainDOGE)
	defer dogeCore.Close()
	service := newGatewayServiceForCore(t, map[Chain]string{
		ChainBTC:  btcCore.URL,
		ChainMVC:  mvcCore.URL,
		ChainDOGE: dogeCore.URL,
	})

	for _, chain := range []Chain{ChainBTC, ChainMVC, ChainDOGE} {
		got, err := service.GetUTXOs(context.Background(), chain, "addr-"+string(chain))
		if err != nil {
			t.Fatalf("GetUTXOs(%s): %v", chain, err)
		}
		if len(got) != 1 {
			t.Fatalf("GetUTXOs(%s) len = %d, want 1", chain, len(got))
		}
		if got[0].Chain != chain || got[0].Address != "addr-"+string(chain) || got[0].TxID != "tx-"+string(chain) {
			t.Fatalf("GetUTXOs(%s) routed to wrong core: %+v", chain, got[0])
		}
	}
}

func performWalletRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(w, req)
	return w
}

func newCoreTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func newGatewayServiceForCore(t *testing.T, urls map[Chain]string) *GatewayService {
	t.Helper()
	chains := make(map[Chain]ChainConfig, len(urls))
	for chain, url := range urls {
		chains[chain] = ChainConfig{Enabled: true, CoreURL: url}
	}
	service, err := NewGatewayService(Config{Timeout: time.Second, Chains: chains})
	if err != nil {
		t.Fatalf("NewGatewayService: %v", err)
	}
	return service
}

func newBalanceCoreForChain(t *testing.T, chain Chain) *httptest.Server {
	t.Helper()
	return newCoreTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/balance" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("address"); got != "addr-"+string(chain) {
			t.Fatalf("%s core received address %s", chain, got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"confirmed_balance_satoshi": chainBalanceAmount(chain),
		})
	})
}

func newUTXOCoreForChain(t *testing.T, chain Chain) *httptest.Server {
	t.Helper()
	return newCoreTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/utxos" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("address"); got != "addr-"+string(chain) {
			t.Fatalf("%s core received address %s", chain, got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"address": "addr-" + string(chain),
			"utxos": []map[string]any{
				{"tx_id": "tx-" + string(chain), "index": "0", "amount": chainBalanceAmount(chain), "is_mempool": false},
			},
		})
	})
}

func chainBalanceAmount(chain Chain) uint64 {
	switch chain {
	case ChainBTC:
		return 101
	case ChainMVC:
		return 202
	case ChainDOGE:
		return 303
	default:
		return 0
	}
}
