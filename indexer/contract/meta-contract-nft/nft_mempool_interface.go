package indexer

import "github.com/metaid/utxo_indexer/common"

// NftMempoolManager defines the interface that NFT mempool manager needs to implement
type NftMempoolManager interface {
	// GetNftUTXOsByAddress gets NFT mempool UTXO for specified address
	GetNftUTXOsByAddress(address string, codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error)

	// GetNftUTXOsByCodeHashGenesis gets NFT mempool UTXO for specified codeHash@genesis
	GetNftUTXOsByCodeHashGenesis(codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error)

	// GetSellNftUTXOsByAddress gets NFT sell mempool UTXO for specified address
	GetSellNftUTXOsByAddress(address string, codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error)

	// GetSellNftUTXOsByCodeHashGenesis gets NFT sell mempool UTXO for specified codeHash@genesis
	GetSellNftUTXOsByCodeHashGenesis(codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error)

	// GetNftInfo gets NFT information through contract code hash, genesis and token index
	GetNftInfo(codeHash string, genesis string, tokenIndex string) (*common.NftInfoModel, error)

	// GetMempoolAddressNftSpendMap gets NFT spend data for addresses in mempool
	GetMempoolAddressNftSpendMap(address string) (map[string]string, error)

	// GetMempoolCodeHashGenesisNftSpendMap gets spend data for codeHash@genesis NFT in mempool
	GetMempoolCodeHashGenesisNftSpendMap(codeHashGenesis string) (map[string]string, error)

	// GetMempoolGenesisUtxo gets genesis UTXO information
	GetMempoolGenesisUtxo(outpoint string) (utxo *common.NftUtxo, err error)

	// GetMempoolAddressNftIncomeMap gets NFT income data for addresses in mempool
	// If address is provided, returns data for that address only; otherwise returns all addresses
	GetMempoolAddressNftIncomeMap(address string) map[string]string

	// GetMempoolAddressNftIncomeValidMap gets valid NFT income data for addresses in mempool
	// If address is provided, returns data for that address only; otherwise returns all addresses
	GetMempoolAddressNftIncomeValidMap(address string) map[string]string
}
