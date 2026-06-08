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
