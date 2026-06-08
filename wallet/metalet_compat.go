package wallet

type metaletBTCBalanceData struct {
	Balance             float64               `json:"balance"`
	Block               metaletBalanceBlock   `json:"block"`
	Mempool             metaletBalanceMempool `json:"mempool"`
	PendingBalance      float64               `json:"pendingBalance"`
	SafeBalance         float64               `json:"safeBalance"`
	UnSafeBalance       float64               `json:"unSafeBalance"`
	InscriptionsBalance float64               `json:"inscriptionsBalance"`
	RunesBalance        float64               `json:"runesBalance"`
	PinsBalance         float64               `json:"pinsBalance"`
	MRC20UtxosBalance   float64               `json:"mrc20UtxosBalance"`
}

type metaletBalanceBlock struct {
	IncomeFee float64 `json:"incomeFee"`
	SpendFee  float64 `json:"spendFee"`
}

type metaletBalanceMempool struct {
	IncomeFee float64 `json:"incomeFee"`
	SpendFee  float64 `json:"spendFee"`
}

type metaletMVCDOGEBalanceData struct {
	Address     string `json:"address"`
	Confirmed   uint64 `json:"confirmed"`
	Unconfirmed int64  `json:"unconfirmed"`
	UTXOCount   uint64 `json:"utxoCount"`
}

func NewMetaletBTCBalanceResponse(balance WalletBalance) Envelope {
	return Success(metaletBTCBalanceData{
		Balance: uint64ToFloatBTC(balance.ConfirmedSatoshi),
		Block: metaletBalanceBlock{
			IncomeFee: uint64ToFloatBTC(balance.ConfirmedSatoshi),
			SpendFee:  0,
		},
		Mempool: metaletBalanceMempool{
			IncomeFee: uint64ToFloatBTC(balance.MempoolIncomeSatoshi),
			SpendFee:  uint64ToFloatBTC(balance.MempoolSpendSatoshi),
		},
		PendingBalance:      float64(balance.UnconfirmedSatoshi()) / 100000000,
		SafeBalance:         uint64ToFloatBTC(balance.SafeSatoshi()),
		UnSafeBalance:       uint64ToFloatBTC(balance.UnsafeSatoshi),
		InscriptionsBalance: 0,
		RunesBalance:        0,
		PinsBalance:         0,
		MRC20UtxosBalance:   0,
	})
}

func uint64ToFloatBTC(value uint64) float64 {
	return float64(value) / 100000000
}

func NewMetaletMVCDOGEBalanceResponse(balance WalletBalance) Envelope {
	return Success(metaletMVCDOGEBalanceData{
		Address:     balance.Address,
		Confirmed:   balance.ConfirmedSatoshi,
		Unconfirmed: balance.UnconfirmedSatoshi(),
		UTXOCount:   balance.UTXOCount(),
	})
}
