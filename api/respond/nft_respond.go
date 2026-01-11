package respond

import (
	nft "github.com/metaid/utxo_indexer/indexer/contract/meta-contract-nft"
)

// NftUTXOsResponse NFT UTXO list response
type NftUTXOsResponse struct {
	Address    string         `json:"address"`
	UTXOs      []*nft.NftUTXO `json:"utxos"`
	Total      int            `json:"total"`
	Cursor     int            `json:"cursor"`
	NextCursor int            `json:"nextCursor"`
	Size       int            `json:"size"`
}

// NftGenesisUTXOsResponse NFT genesis UTXO list response
type NftGenesisUTXOsResponse struct {
	CodeHash string         `json:"codeHash"`
	Genesis  string         `json:"genesis"`
	UTXOs    []*nft.NftUTXO `json:"utxos"`
	Total    int            `json:"total"`
}

// NftSellUTXOsResponse NFT sell UTXO list response
type NftSellUTXOsResponse struct {
	Address    string             `json:"address"`
	UTXOs      []*nft.NftSellUTXO `json:"utxos"`
	Total      int                `json:"total"`
	Cursor     int                `json:"cursor"`
	NextCursor int                `json:"nextCursor"`
	Size       int                `json:"size"`
}

// NftGenesisSellUTXOsResponse NFT genesis sell UTXO list response
type NftGenesisSellUTXOsResponse struct {
	CodeHash string             `json:"codeHash"`
	Genesis  string             `json:"genesis"`
	UTXOs    []*nft.NftSellUTXO `json:"utxos"`
	Total    int                `json:"total"`
}

// NftAddressUtxoCountResponse NFT address UTXO count response
type NftAddressUtxoCountResponse struct {
	Address string `json:"address"`
	Count   int    `json:"count"`
}

// NftAddressSummaryResponse NFT address summary response
type NftAddressSummaryResponse struct {
	Address    string            `json:"address"`
	Summary    []*nft.NftSummary `json:"summary"`
	Total      int               `json:"total"`
	Cursor     int               `json:"cursor"`
	NextCursor int               `json:"nextCursor"`
	Size       int               `json:"size"`
}

// NftSummaryResponse NFT summary response
type NftSummaryResponse struct {
	Summary    []*nft.NftInfo `json:"summary"`
	Total      int            `json:"total"`
	Cursor     int            `json:"cursor"`
	NextCursor int            `json:"nextCursor"`
	Size       int            `json:"size"`
}

// NftUtxoByTxResponse NFT UTXO by transaction response
type NftUtxoByTxResponse struct {
	UTXOs string `json:"utxos"`
}

// NftGenesisInfoResponse NFT genesis information response
type NftGenesisInfoResponse struct {
	CodeHash    string `json:"codeHash"`
	Genesis     string `json:"genesis"`
	SensibleId  string `json:"sensibleId"`
	Txid        string `json:"txid"`
	TxIndex     int64  `json:"txIndex"`
	TokenSupply uint64 `json:"tokenSupply"`
	TokenIndex  uint64 `json:"tokenIndex"`
	ValueString string `json:"valueString"`
	Height      int64  `json:"height"`
}

// NftOwnersResponse NFT owners list response
type NftOwnersResponse struct {
	List       []*nft.NftOwner `json:"list"`
	Total      int             `json:"total"`
	Cursor     int             `json:"cursor"`
	NextCursor int             `json:"nextCursor"`
	Size       int             `json:"size"`
}

// NftIncomeValidResponse NFT valid income response
type NftIncomeValidResponse struct {
	Address    string   `json:"address"`
	IncomeData []string `json:"income_data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination"`
}

// NftUncheckOutpointResponse Used to return unchecked NFT outpoint data
type NftUncheckOutpointResponse struct {
	Data       map[string]string `json:"data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}

// NftUsedIncomeResponse Used to return used NFT income data
type NftUsedIncomeResponse struct {
	Data       map[string]string `json:"data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}

// NftGenesisResponse Used to return NFT Genesis data
type NftGenesisResponse struct {
	Data       map[string]string `json:"data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}

// NftGenesisOutputResponse Used to return NFT Genesis Output data
type NftGenesisOutputResponse struct {
	Data       map[string][]string `json:"data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}

// NftAddressNftIncomeMapResponse NFT address income data response
type NftAddressNftIncomeMapResponse struct {
	Address   string            `json:"address"`
	IncomeMap map[string]string `json:"incomeMap"`
}

// NftAddressNftIncomeValidMapResponse NFT address valid income data response
type NftAddressNftIncomeValidMapResponse struct {
	Address        string            `json:"address"`
	IncomeValidMap map[string]string `json:"incomeValidMap"`
}

// NftSpendMapResponse NFT spend data response
type NftSpendMapResponse struct {
	Address  string            `json:"address"`
	SpendMap map[string]string `json:"spendMap"`
}

// NftAllSellIncomeResponse Used to return all NFT sell income data
type NftAllSellIncomeResponse struct {
	IncomeData map[string]string `json:"income_data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}

// NftAllSellSpendResponse Used to return all NFT sell spend data
type NftAllSellSpendResponse struct {
	SpendData  map[string]string `json:"spend_data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}

// NftCodeHashGenesisSellIncomeResponse Used to return NFT sell income data by codeHash@genesis
type NftCodeHashGenesisSellIncomeResponse struct {
	IncomeData map[string]string `json:"income_data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}

// NftCodeHashGenesisSellSpendResponse Used to return NFT sell spend data by codeHash@genesis
type NftCodeHashGenesisSellSpendResponse struct {
	SpendData  map[string]string `json:"spend_data"`
	Pagination struct {
		CurrentPage int `json:"current_page"`
		PageSize    int `json:"page_size"`
		Total       int `json:"total"`
		TotalPages  int `json:"total_pages"`
	} `json:"pagination,omitempty"`
}
