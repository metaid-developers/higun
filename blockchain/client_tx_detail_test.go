package blockchain

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	bsvchainhash "github.com/bitcoinsv/bsvd/chaincfg/chainhash"
	bsvwire "github.com/bitcoinsv/bsvd/wire"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/metaid/utxo_indexer/config"
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

func TestTxDetailJSONIncludesInputVoutZero(t *testing.T) {
	detail := &TxDetail{
		TxID:   strings.Repeat("a", 64),
		Inputs: []TxInput{{TxID: strings.Repeat("b", 64), Vout: 0}},
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(body), `"vout":0`) {
		t.Fatalf("body missing input vout zero: %s", string(body))
	}
}

func TestTxDetailFromRawJSONPreservesLargeOutputSatoshis(t *testing.T) {
	body := []byte(`{
		"txid":"` + strings.Repeat("a", 64) + `",
		"size":225,
		"vsize":225,
		"confirmations":1,
		"vin":[{"txid":"` + strings.Repeat("b", 64) + `","vout":0}],
		"vout":[{"n":0,"value":92233720368.54775907,"scriptPubKey":{"address":"addr-out"}}]
	}`)

	got, err := txDetailFromRawJSON(body)
	if err != nil {
		t.Fatalf("txDetailFromRawJSON: %v", err)
	}
	if len(got.Outputs) != 1 {
		t.Fatalf("outputs len = %d, want 1", len(got.Outputs))
	}
	if got.Outputs[0].Satoshi != 9223372036854775907 {
		t.Fatalf("satoshi = %d, want 9223372036854775907", got.Outputs[0].Satoshi)
	}
}

func TestTxDetailRPCErrorDoesNotMatchGenericMethodNotFound(t *testing.T) {
	for _, err := range []error{
		errors.New("Method not found"),
		errors.New("rpc: method not found"),
	} {
		if isTransactionNotFoundRPCError(err) {
			t.Fatalf("isTransactionNotFoundRPCError(%q) = true, want false", err.Error())
		}
	}
}

func TestMVCAdapterIndexesPublicTxIDWithNodeAlias(t *testing.T) {
	tx := newMVCVersion10TestTx(t)
	publicTxID, err := GetNewHash2(tx)
	if err != nil {
		t.Fatalf("GetNewHash2: %v", err)
	}
	nodeTxID := tx.TxHash().String()
	if publicTxID == nodeTxID {
		t.Fatalf("test tx should have different MVC public and node txids: %s", publicTxID)
	}

	got := (&MVCAdapter{}).convertMVCTxToIndexerTx(tx)
	if got.ID != publicTxID {
		t.Fatalf("ID = %s, want public txid %s", got.ID, publicTxID)
	}
	if got.NodeID != nodeTxID {
		t.Fatalf("NodeID = %s, want node txid %s", got.NodeID, nodeTxID)
	}
}

func TestMVCTxDetailResolvesPublicTxIDViaAlias(t *testing.T) {
	publicTxID := strings.Repeat("a", 64)
	nodeTxID := strings.Repeat("b", 64)
	var calls []string
	server := newTxDetailRPCServer(t, func(txid string) (any, *testRPCError) {
		calls = append(calls, txid)
		if txid == publicTxID {
			return nil, &testRPCError{Code: -5, Message: "No such mempool or blockchain transaction"}
		}
		if txid != nodeTxID {
			t.Fatalf("rpc txid = %s, want %s", txid, nodeTxID)
		}
		return map[string]any{
			"txid":          nodeTxID,
			"confirmations": 2,
			"vin":           []any{},
			"vout":          []any{},
		}, nil
	})
	defer server.Close()

	client := newTestTxDetailClient(t, server.URL, config.ChainMVC)
	client.SetTxIDAliasResolver(staticTxIDAliasResolver{aliases: map[string]string{publicTxID: nodeTxID}})

	got, err := client.GetTransactionDetail(publicTxID)
	if err != nil {
		t.Fatalf("GetTransactionDetail: %v", err)
	}
	if got.TxID != publicTxID {
		t.Fatalf("TxID = %s, want requested public txid %s", got.TxID, publicTxID)
	}
	if !got.Confirmed || got.Confirmations != 2 {
		t.Fatalf("unexpected confirmation fields: %+v", got)
	}
	if strings.Join(calls, ",") != publicTxID+","+nodeTxID {
		t.Fatalf("rpc calls = %v, want public then node txid", calls)
	}
}

func TestTxDetailDoesNotUseAliasResolverForBTC(t *testing.T) {
	publicTxID := strings.Repeat("a", 64)
	nodeTxID := strings.Repeat("b", 64)
	var calls []string
	server := newTxDetailRPCServer(t, func(txid string) (any, *testRPCError) {
		calls = append(calls, txid)
		if txid == nodeTxID {
			return map[string]any{
				"txid":          nodeTxID,
				"confirmations": 1,
				"vin":           []any{},
				"vout":          []any{},
			}, nil
		}
		return nil, &testRPCError{Code: -5, Message: "No such mempool or blockchain transaction"}
	})
	defer server.Close()

	client := newTestTxDetailClient(t, server.URL, config.ChainBTC)
	client.SetTxIDAliasResolver(staticTxIDAliasResolver{aliases: map[string]string{publicTxID: nodeTxID}})

	if _, err := client.GetTransactionDetail(publicTxID); !errors.Is(err, ErrTransactionNotFound) {
		t.Fatalf("GetTransactionDetail err = %v, want ErrTransactionNotFound", err)
	}
	if strings.Join(calls, ",") != publicTxID {
		t.Fatalf("rpc calls = %v, want only public txid", calls)
	}
}

type staticTxIDAliasResolver struct {
	aliases map[string]string
}

func (r staticTxIDAliasResolver) ResolveTxIDAlias(txid string) (string, bool, error) {
	resolved, ok := r.aliases[txid]
	return resolved, ok, nil
}

type testRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newTxDetailRPCServer(t *testing.T, handler func(txid string) (any, *testRPCError)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var req struct {
			ID     any               `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode rpc request: %v", err)
		}
		if req.Method != "getrawtransaction" {
			t.Fatalf("method = %s, want getrawtransaction", req.Method)
		}
		if len(req.Params) != 2 {
			t.Fatalf("params len = %d, want 2", len(req.Params))
		}
		var txid string
		if err := json.Unmarshal(req.Params[0], &txid); err != nil {
			t.Fatalf("decode txid param: %v", err)
		}
		result, rpcErr := handler(txid)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     req.ID,
			"result": result,
			"error":  rpcErr,
		})
	}))
}

func newTestTxDetailClient(t *testing.T, rawURL, chain string) *Client {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse rpc url: %v", err)
	}
	rpc, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         parsed.Host,
		User:         "user",
		Pass:         "pass",
		HTTPPostMode: true,
		DisableTLS:   true,
	}, nil)
	if err != nil {
		t.Fatalf("rpcclient.New: %v", err)
	}
	t.Cleanup(rpc.Shutdown)
	return &Client{
		rpcClient: rpc,
		Rpc:       rpc,
		cfg:       &config.Config{Chain: chain},
	}
}

func newMVCVersion10TestTx(t *testing.T) *bsvwire.MsgTx {
	t.Helper()
	prevHash, err := bsvchainhash.NewHashFromStr(strings.Repeat("1", 64))
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}
	tx := bsvwire.NewMsgTx(10)
	tx.AddTxIn(bsvwire.NewTxIn(bsvwire.NewOutPoint(prevHash, 0), []byte{0x01, 0x02}))
	tx.AddTxOut(bsvwire.NewTxOut(1000, []byte{0x51, 0x51, 0x51, 0x51}))
	return tx
}
