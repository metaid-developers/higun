package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const satoshisPerBTC = 100_000_000

type upstreamBalance struct {
	ConfirmedBalanceSatoshi int64   `json:"confirmed_balance_satoshi"`
	ConfirmedBalance        float64 `json:"confirmed_balance"`
	MempoolIncomeSatoshi    int64   `json:"mempool_income_satoshi"`
	MempoolIncome           float64 `json:"mempool_income"`
	MempoolSpendSatoshi     int64   `json:"mempool_spend_satoshi"`
	MempoolSpend            float64 `json:"mempool_spend"`
	UnsafeFeeSatoshi        int64   `json:"unsafe_fee_satoshi"`
	UnsafeFee               float64 `json:"unsafe_fee"`
}

type upstreamUTXOResponse struct {
	Address string         `json:"address"`
	UTXOs   []upstreamUTXO `json:"utxos"`
	Count   int            `json:"count"`
}

type upstreamUTXO struct {
	TxID      string `json:"tx_id"`
	Index     string `json:"index"`
	Amount    uint64 `json:"amount"`
	IsMempool bool   `json:"is_mempool"`
}

type ownRespV3 struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  any    `json:"result"`
}

type ownResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

type btcBalanceInfo struct {
	Balance float64 `json:"balance"`
	Block   struct {
		IncomeFee float64 `json:"incomeFee"`
		SpendFee  float64 `json:"spendFee"`
	} `json:"block"`
	Mempool struct {
		IncomeFee float64 `json:"incomeFee"`
		SpendFee  float64 `json:"spendFee"`
	} `json:"mempool"`
	Unsafe         float64 `json:"unsafe"`
	UnsafeSatoshis int64   `json:"total"`
}

type ownUTXOItem struct {
	Confirmed    bool   `json:"confirmed"`
	Inscriptions any    `json:"inscriptions"`
	Satoshi      int64  `json:"satoshi"`
	TxID         string `json:"txId"`
	Vout         int64  `json:"vout"`
}

type ownUTXOAllItem struct {
	TxID      string `json:"txId"`
	Vout      int    `json:"vout"`
	Value     int64  `json:"value"`
	Confirmed bool   `json:"confirmed"`
}

type ownUTXOAllData struct {
	Cursor  int              `json:"cursor"`
	Limit   int              `json:"limit"`
	Total   int              `json:"total"`
	Balance int64            `json:"balance"`
	UTXO    []ownUTXOAllItem `json:"utxo"`
}

func main() {
	var (
		listen   = flag.String("listen", ":8085", "listen address")
		upstream = flag.String("upstream", "http://127.0.0.1:18067", "upstream api base url")
		timeout  = flag.Duration("timeout", 30*time.Second, "http client timeout")
	)
	flag.Parse()

	upstreamURL, err := url.Parse(*upstream)
	if err != nil {
		log.Fatalf("parse upstream: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = upstreamURL.Host
	}
	proxy.ErrorLog = log.New(log.Writer(), "[compat-proxy] ", log.LstdFlags)

	client := &http.Client{Timeout: *timeout}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if handled, err := tryHandleCompatibility(w, r, client, strings.TrimRight(*upstream, "/")); handled {
			if err != nil {
				writeOwnError(w, http.StatusBadGateway, err)
			}
			return
		}
		proxy.ServeHTTP(w, r)
	})

	log.Printf("starting compat proxy on %s -> %s", *listen, upstreamURL.String())
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

func tryHandleCompatibility(w http.ResponseWriter, r *http.Request, client *http.Client, upstreamBase string) (bool, error) {
	switch {
	case r.URL.Path == "/address/btc-balance":
		address := strings.TrimSpace(r.URL.Query().Get("address"))
		if address == "" {
			writeOwnError(w, http.StatusBadRequest, fmt.Errorf("address parameter is required"))
			return true, nil
		}
		return true, handleBalanceCompatibility(w, r, client, upstreamBase, address)
	case r.URL.Path == "/wallet-v1/address/btc-utxo", r.URL.Path == "/address/btc-utxo":
		address := strings.TrimSpace(r.URL.Query().Get("address"))
		if address == "" {
			writeOwnError(w, http.StatusBadRequest, fmt.Errorf("address parameter is required"))
			return true, nil
		}
		return true, handleUTXOCompatibility(w, r, client, upstreamBase, address)
	case r.URL.Path == "/address/btc-utxo-all":
		address := strings.TrimSpace(r.URL.Query().Get("address"))
		if address == "" {
			writeOwnErrorV1(w, http.StatusBadRequest, "address parameter is required")
			return true, nil
		}
		return true, handleUTXOAllCompatibility(w, r, client, upstreamBase, address)
	default:
		address, kind, ok := matchPathCompatibilityRoute(r.URL.Path)
		if !ok {
			return false, nil
		}
		switch kind {
		case "balance":
			return true, handleBalanceCompatibility(w, r, client, upstreamBase, address)
		case "utxo":
			return true, handleUTXOCompatibility(w, r, client, upstreamBase, address)
		default:
			return false, nil
		}
	}
}

func matchPathCompatibilityRoute(path string) (address string, kind string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "address" || parts[1] == "" {
		return "", "", false
	}
	switch parts[2] {
	case "balance", "utxo":
		return parts[1], parts[2], true
	default:
		return "", "", false
	}
}

func handleBalanceCompatibility(w http.ResponseWriter, r *http.Request, client *http.Client, upstreamBase string, address string) error {
	payload := upstreamBalance{}
	if err := getUpstreamJSON(r, client, upstreamBase+"/balance", address, &payload); err != nil {
		return err
	}

	resp := btcBalanceInfo{
		Balance:        float64(payload.ConfirmedBalanceSatoshi) / satoshisPerBTC,
		Unsafe:         payload.UnsafeFee,
		UnsafeSatoshis: payload.UnsafeFeeSatoshi,
	}
	resp.Block.IncomeFee = float64(payload.ConfirmedBalanceSatoshi) / satoshisPerBTC
	resp.Block.SpendFee = 0
	resp.Mempool.IncomeFee = payload.MempoolIncome
	resp.Mempool.SpendFee = payload.MempoolSpend

	return writeOwnJSON(w, http.StatusOK, ownRespV3{
		Status:  "1",
		Message: "success",
		Result:  resp,
	})
}

func handleUTXOCompatibility(w http.ResponseWriter, r *http.Request, client *http.Client, upstreamBase string, address string) error {
	payload := upstreamUTXOResponse{}
	if err := getUpstreamJSON(r, client, upstreamBase+"/utxos", address, &payload); err != nil {
		return err
	}

	items, err := buildOwnUTXOItems(payload.UTXOs, r.URL.Query().Get("order"), r.URL.Query().Get("unconfirmed"))
	if err != nil {
		return err
	}

	return writeOwnJSON(w, http.StatusOK, ownRespV3{
		Status:  "1",
		Message: "success",
		Result:  items,
	})
}

func handleUTXOAllCompatibility(w http.ResponseWriter, r *http.Request, client *http.Client, upstreamBase string, address string) error {
	payload := upstreamUTXOResponse{}
	if err := getUpstreamJSON(r, client, upstreamBase+"/utxos", address, &payload); err != nil {
		return err
	}

	includeMempool := r.URL.Query().Get("mempool") == "1"
	cursor, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
	limit, err := parsePositiveIntDefault(r.URL.Query().Get("limit"), 50)
	if err != nil {
		return err
	}

	items, totalBalance, err := buildOwnUTXOAllItems(payload.UTXOs, includeMempool)
	if err != nil {
		return err
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(items) {
		cursor = len(items)
	}
	end := cursor + limit
	if end > len(items) {
		end = len(items)
	}

	data := ownUTXOAllData{
		Cursor:  cursor,
		Limit:   limit,
		Total:   len(items),
		Balance: totalBalance,
		UTXO:    items[cursor:end],
	}
	return writeOwnJSON(w, http.StatusOK, ownResp{
		Code: 2000,
		Msg:  "success",
		Data: data,
	})
}

func getUpstreamJSON(r *http.Request, client *http.Client, upstreamURL string, address string, out any) error {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstreamURL, nil)
	if err != nil {
		return err
	}
	query := req.URL.Query()
	query.Set("address", address)
	req.URL.RawQuery = query.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			return fmt.Errorf("upstream returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode upstream response: %w", err)
	}
	return nil
}

func buildOwnUTXOItems(utxos []upstreamUTXO, order, unconfirmed string) ([]ownUTXOItem, error) {
	includeUnconfirmed := unconfirmed != "0"
	items := make([]ownUTXOItem, 0, len(utxos))
	for _, utxo := range utxos {
		if utxo.IsMempool && !includeUnconfirmed {
			continue
		}
		vout, err := strconv.ParseInt(utxo.Index, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse utxo index %q: %w", utxo.Index, err)
		}
		items = append(items, ownUTXOItem{
			Confirmed:    !utxo.IsMempool,
			Inscriptions: nil,
			Satoshi:      int64(utxo.Amount),
			TxID:         utxo.TxID,
			Vout:         vout,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if order == "desc" {
			if items[i].Confirmed != items[j].Confirmed {
				return items[i].Confirmed && !items[j].Confirmed
			}
			if items[i].Satoshi != items[j].Satoshi {
				return items[i].Satoshi > items[j].Satoshi
			}
			if items[i].TxID != items[j].TxID {
				return items[i].TxID > items[j].TxID
			}
			return items[i].Vout > items[j].Vout
		}

		if items[i].Confirmed != items[j].Confirmed {
			return !items[i].Confirmed && items[j].Confirmed
		}
		if items[i].Satoshi != items[j].Satoshi {
			return items[i].Satoshi < items[j].Satoshi
		}
		if items[i].TxID != items[j].TxID {
			return items[i].TxID < items[j].TxID
		}
		return items[i].Vout < items[j].Vout
	})
	return items, nil
}

func buildOwnUTXOAllItems(utxos []upstreamUTXO, includeMempool bool) ([]ownUTXOAllItem, int64, error) {
	items := make([]ownUTXOAllItem, 0, len(utxos))
	var balance int64
	for _, utxo := range utxos {
		if utxo.IsMempool && !includeMempool {
			continue
		}
		vout, err := strconv.Atoi(utxo.Index)
		if err != nil {
			return nil, 0, fmt.Errorf("parse utxo index %q: %w", utxo.Index, err)
		}
		value := int64(utxo.Amount)
		items = append(items, ownUTXOAllItem{
			TxID:      utxo.TxID,
			Vout:      vout,
			Value:     value,
			Confirmed: !utxo.IsMempool,
		})
		balance += value
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Confirmed != items[j].Confirmed {
			return items[i].Confirmed && !items[j].Confirmed
		}
		if items[i].Value != items[j].Value {
			return items[i].Value > items[j].Value
		}
		if items[i].TxID != items[j].TxID {
			return items[i].TxID > items[j].TxID
		}
		return items[i].Vout > items[j].Vout
	})
	return items, balance, nil
}

func parsePositiveIntDefault(raw string, fallback int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", raw)
	}
	if value <= 0 {
		return fallback, nil
	}
	return value, nil
}

func writeOwnJSON(w http.ResponseWriter, status int, payload any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

func writeOwnError(w http.ResponseWriter, status int, err error) {
	_ = writeOwnJSON(w, status, ownRespV3{
		Status:  "0",
		Message: err.Error(),
		Result:  nil,
	})
}

func writeOwnErrorV1(w http.ResponseWriter, status int, msg string) {
	_ = writeOwnJSON(w, status, ownResp{
		Code: 5000,
		Msg:  msg,
		Data: nil,
	})
}
