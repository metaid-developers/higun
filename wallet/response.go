package wallet

type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type StandardBalanceData struct {
	Chain                Chain  `json:"chain"`
	Address              string `json:"address"`
	ConfirmedSatoshi     uint64 `json:"confirmedSatoshi"`
	UnconfirmedSatoshi   int64  `json:"unconfirmedSatoshi"`
	MempoolIncomeSatoshi uint64 `json:"mempoolIncomeSatoshi"`
	MempoolSpendSatoshi  uint64 `json:"mempoolSpendSatoshi"`
	UnsafeSatoshi        uint64 `json:"unsafeSatoshi"`
	SafeSatoshi          uint64 `json:"safeSatoshi"`
	UTXOCount            uint64 `json:"utxoCount"`
	Confirmed            string `json:"confirmed"`
	Unconfirmed          string `json:"unconfirmed"`
	MempoolIncome        string `json:"mempoolIncome"`
	MempoolSpend         string `json:"mempoolSpend"`
	Unsafe               string `json:"unsafe"`
	Safe                 string `json:"safe"`
}

type StandardUTXOData struct {
	Chain         Chain              `json:"chain"`
	Address       string             `json:"address"`
	ConfirmedOnly bool               `json:"confirmedOnly"`
	Sort          string             `json:"sort"`
	Total         int                `json:"total"`
	UTXOs         []StandardUTXOItem `json:"utxos"`
}

type StandardUTXOItem struct {
	TxID      string `json:"txid"`
	Vout      int    `json:"vout"`
	Outpoint  string `json:"outpoint"`
	Satoshi   uint64 `json:"satoshi"`
	Amount    string `json:"amount"`
	Confirmed bool   `json:"confirmed"`
	Mempool   bool   `json:"mempool"`
	Height    *int64 `json:"height,omitempty"`
}

type StandardFeeRateData struct {
	Chain   Chain  `json:"chain"`
	Source  string `json:"source"`
	Unit    string `json:"unit"`
	Slow    int64  `json:"slow"`
	Normal  int64  `json:"normal"`
	Fast    int64  `json:"fast"`
	Default string `json:"default"`
}

type StandardBroadcastData struct {
	Chain    Chain  `json:"chain"`
	TxID     string `json:"txid"`
	Accepted bool   `json:"accepted"`
}

type StandardTxDetailData struct {
	Chain         Chain                  `json:"chain"`
	TxID          string                 `json:"txid"`
	Confirmed     bool                   `json:"confirmed"`
	Mempool       bool                   `json:"mempool"`
	Confirmations uint64                 `json:"confirmations"`
	Height        *int64                 `json:"height"`
	BlockHash     string                 `json:"blockHash"`
	BlockTime     *int64                 `json:"blockTime"`
	Inputs        []StandardTxInputItem  `json:"inputs"`
	Outputs       []StandardTxOutputItem `json:"outputs"`
	FeeSatoshi    *uint64                `json:"feeSatoshi"`
	Fee           *string                `json:"fee"`
	Size          int32                  `json:"size"`
	Vsize         int32                  `json:"vsize"`
}

type StandardTxInputItem struct {
	TxID     string  `json:"txid"`
	Vout     uint32  `json:"vout"`
	Address  string  `json:"address"`
	Satoshi  *uint64 `json:"satoshi,omitempty"`
	Amount   string  `json:"amount,omitempty"`
	Coinbase string  `json:"coinbase,omitempty"`
}

type StandardTxOutputItem struct {
	Vout    uint32 `json:"vout"`
	Address string `json:"address"`
	Satoshi uint64 `json:"satoshi"`
	Amount  string `json:"amount"`
}

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
	Confirmations *uint64 `json:"confirmations"`
	Height        *int64  `json:"height"`
	Timestamp     int64   `json:"timestamp"`
	Time          string  `json:"time"`
}

func Success(data any) Envelope {
	return Envelope{Code: CodeSuccess, Message: "success", Data: data}
}

func ErrorEnvelope(err *WalletError) Envelope {
	if err == nil {
		return Envelope{Code: CodeInternal, Message: "internal wallet error", Data: nil}
	}
	return Envelope{Code: err.Code, Message: err.Message, Data: nil}
}

func NewStandardBalanceResponse(balance WalletBalance) Envelope {
	unconfirmedSatoshi := balance.UnconfirmedSatoshi()
	safeSatoshi := balance.SafeSatoshi()
	return Success(StandardBalanceData{
		Chain:                balance.Chain,
		Address:              balance.Address,
		ConfirmedSatoshi:     balance.ConfirmedSatoshi,
		UnconfirmedSatoshi:   unconfirmedSatoshi,
		MempoolIncomeSatoshi: balance.MempoolIncomeSatoshi,
		MempoolSpendSatoshi:  balance.MempoolSpendSatoshi,
		UnsafeSatoshi:        balance.UnsafeSatoshi,
		SafeSatoshi:          safeSatoshi,
		UTXOCount:            balance.UTXOCount(),
		Confirmed:            SatoshiToDecimalString(balance.ConfirmedSatoshi),
		Unconfirmed:          SignedSatoshiToDecimalString(unconfirmedSatoshi),
		MempoolIncome:        SatoshiToDecimalString(balance.MempoolIncomeSatoshi),
		MempoolSpend:         SatoshiToDecimalString(balance.MempoolSpendSatoshi),
		Unsafe:               SatoshiToDecimalString(balance.UnsafeSatoshi),
		Safe:                 SatoshiToDecimalString(safeSatoshi),
	})
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

func NewStandardBroadcastResponse(result BroadcastResult) Envelope {
	return Success(StandardBroadcastData{Chain: result.Chain, TxID: result.TxID, Accepted: result.Accepted})
}

func NewStandardTxDetailResponse(detail WalletTxDetail) Envelope {
	inputs := make([]StandardTxInputItem, 0, len(detail.Inputs))
	for _, in := range detail.Inputs {
		item := StandardTxInputItem{
			TxID:     in.TxID,
			Vout:     in.Vout,
			Address:  in.Address,
			Satoshi:  in.Satoshi,
			Coinbase: in.Coinbase,
		}
		if in.Satoshi != nil {
			item.Amount = SatoshiToDecimalString(*in.Satoshi)
		}
		inputs = append(inputs, item)
	}

	outputs := make([]StandardTxOutputItem, 0, len(detail.Outputs))
	for _, out := range detail.Outputs {
		outputs = append(outputs, StandardTxOutputItem{
			Vout:    out.Vout,
			Address: out.Address,
			Satoshi: out.Satoshi,
			Amount:  SatoshiToDecimalString(out.Satoshi),
		})
	}

	data := StandardTxDetailData{
		Chain:         detail.Chain,
		TxID:          detail.TxID,
		Confirmed:     detail.Confirmed,
		Mempool:       detail.Mempool,
		Confirmations: detail.Confirmations,
		Height:        detail.Height,
		BlockHash:     detail.BlockHash,
		BlockTime:     detail.BlockTime,
		Inputs:        inputs,
		Outputs:       outputs,
		FeeSatoshi:    detail.FeeSatoshi,
		Size:          detail.Size,
		Vsize:         detail.Vsize,
	}
	if detail.FeeSatoshi != nil {
		fee := SatoshiToDecimalString(*detail.FeeSatoshi)
		data.Fee = &fee
	}
	return Success(data)
}

func NewStandardHistoryResponse(page WalletHistoryPage) Envelope {
	items := make([]StandardHistoryItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, StandardHistoryItem{
			TxID:          item.TxID,
			Direction:     item.Direction,
			IncomeSatoshi: item.IncomeSatoshi,
			SpendSatoshi:  item.SpendSatoshi,
			NetSatoshi:    item.NetSatoshi,
			Income:        SatoshiToDecimalString(item.IncomeSatoshi),
			Spend:         SatoshiToDecimalString(item.SpendSatoshi),
			Net:           SignedSatoshiToDecimalString(item.NetSatoshi),
			Confirmed:     item.Confirmed,
			Mempool:       item.Mempool,
			Confirmations: item.Confirmations,
			Height:        item.Height,
			Timestamp:     item.Timestamp,
			Time:          item.Time,
		})
	}
	return Success(StandardHistoryData{
		Chain:         page.Chain,
		Address:       page.Address,
		Page:          page.Page,
		Limit:         page.Limit,
		ConfirmedOnly: page.ConfirmedOnly,
		Sort:          page.Sort,
		Total:         page.Total,
		Items:         items,
	})
}

func NewStandardUTXOResponse(chain Chain, address string, confirmedOnly bool, sortOrder string, utxos []WalletUTXO) Envelope {
	items := make([]StandardUTXOItem, 0, len(utxos))
	for _, utxo := range utxos {
		items = append(items, StandardUTXOItem{
			TxID:      utxo.TxID,
			Vout:      utxo.Vout,
			Outpoint:  utxo.Outpoint(),
			Satoshi:   utxo.Satoshi,
			Amount:    SatoshiToDecimalString(utxo.Satoshi),
			Confirmed: utxo.Confirmed,
			Mempool:   utxo.Mempool,
			Height:    utxo.Height,
		})
	}
	return Success(StandardUTXOData{
		Chain:         chain,
		Address:       address,
		ConfirmedOnly: confirmedOnly,
		Sort:          sortOrder,
		Total:         len(items),
		UTXOs:         items,
	})
}
