package common

type Utxo struct {
	TxID    string
	Address string
	Amount  string
}

type FtUtxo struct {
	ContractType string
	UtxoId       string
	Index        string
	TxID         string
	Address      string
	Value        string
	Amount       string
	Decimal      string
	CodeHash     string
	Genesis      string
	SensibleId   string
	CustomData   string
	Name         string
	Symbol       string
	Timestamp    int64
	UsedTxId     string
}

// FtInfo struct definition
type FtInfoModel struct {
	CodeHash   string
	Genesis    string
	SensibleId string
	Name       string
	Symbol     string
	Decimal    uint8
}

type NftUtxo struct {
	ContractType    string
	UtxoId          string
	Index           string
	TxID            string
	Address         string
	Value           string
	CodeHash        string
	Genesis         string
	SensibleId      string
	TokenIndex      string
	TokenSupply     string
	MetaTxId        string
	MetaOutputIndex string
	Timestamp       int64
	UsedTxId        string

	Price           string
	ContractAddress string
}

// NftInfo struct definition
type NftInfoModel struct {
	CodeHash        string
	Genesis         string
	SensibleId      string
	TokenIndex      string
	TokenSupply     string
	MetaTxId        string
	MetaOutputIndex string
}

// NftInfo struct definition
type NftSummaryInfoModel struct {
	CodeHash        string
	Genesis         string
	SensibleId      string
	TokenSupply     string
	MetaTxId        string
	MetaOutputIndex string
}

// CheckUtxoReq UTXO检测请求
type CheckUtxoReq struct {
	OutPoints []string `json:"outPoints"` // UTXO列表，格式：txhash:index
}

// UtxoSpendInfo UTXO花费信息
type UtxoSpendInfo struct {
	SpendTx string `json:"spendTx"` // 花费交易hash
	Height  int    `json:"height"`  // 花费区块高度
	Date    int64  `json:"date"`    // 花费时间戳
	Where   string `json:"where"`   // 位置：block或mempool
	Address string `json:"address"` // 地址
}

// UtxoInfo UTXO详细信息
type UtxoInfo struct {
	IsExist     bool          `json:"isExist"`     // 是否存在
	Height      int           `json:"height"`      // 区块高度
	Date        int64         `json:"date"`        // 时间戳
	Value       int64         `json:"value"`       // 金额（satoshi）
	TxConfirm   bool          `json:"txConfirm"`   // 是否确认
	Where       string        `json:"where"`       // 位置：block或mempool
	Address     string        `json:"address"`     // 地址
	SpendStatus string        `json:"spendStatus"` // 花费状态：unspent或spend
	SpendInfo   UtxoSpendInfo `json:"spendInfo"`   // 花费信息
}
