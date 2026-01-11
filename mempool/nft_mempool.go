package mempool

import (
	"fmt"
	"strconv"
	"strings"

	indexer "github.com/metaid/utxo_indexer/indexer/contract/meta-contract-nft"

	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/storage"
)

// Ensure NftMempoolManager implements indexer.NftMempoolManager interface
var _ indexer.NftMempoolManager = (*NftMempoolManager)(nil)

// GetNftUTXOsByAddress gets mempool NFT UTXO for specified address
func (m *NftMempoolManager) GetNftUTXOsByAddress(address string, codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Get raw UTXO data
	return m.getRawNftUTXOsByAddress(address, codeHash, genesis)
}

// GetNftUTXOsByCodeHashGenesis gets mempool NFT UTXO for specified codeHash and genesis
func (m *NftMempoolManager) GetNftUTXOsByCodeHashGenesis(codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Get raw UTXO data
	return m.getRawNftUTXOsByCodeHashGenesis(codeHash, genesis)
}

// GetSellNftUTXOsByAddress gets mempool NFT sell UTXO for specified address
func (m *NftMempoolManager) GetSellNftUTXOsByAddress(address string, codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Get raw UTXO data
	return m.getRawSellNftUTXOsByAddress(address, codeHash, genesis)
}

// GetSellNftUTXOsByCodeHashGenesis gets mempool NFT sell UTXO for specified codeHash and genesis
func (m *NftMempoolManager) GetSellNftUTXOsByCodeHashGenesis(codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Get raw UTXO data
	return m.getRawSellNftUTXOsByCodeHashGenesis(codeHash, genesis)
}

func (m *NftMempoolManager) GetNftInfo(codeHash string, genesis string, tokenIndex string) (*common.NftInfoModel, error) {
	return m.getNftInfo(codeHash, genesis, tokenIndex)
}

// getRawNftUTXOsByAddress internal method, gets raw NFT UTXO data including income and spent UTXOs
func (m *NftMempoolManager) getRawNftUTXOsByAddress(address string, codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Map for deduplication
	incomeMap := make(map[string]struct{})
	spentMap := make(map[string]struct{})

	// 1. Get income UTXOs
	incomeList, err := m.mempoolAddressNftIncomeValidStore.GetAddressNftUtxoByKey(address)
	if err == nil {
		for _, utxo := range incomeList {
			// Filter by codeHash and genesis if provided
			if codeHash != "" && utxo.CodeHash != codeHash {
				continue
			}
			if genesis != "" && utxo.Genesis != genesis {
				continue
			}
			if _, ok := incomeMap[utxo.UtxoId]; !ok {
				incomeUtxoList = append(incomeUtxoList, utxo)
				incomeMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	// 2. Get spent UTXOs
	spendList, err := m.mempoolAddressNftSpendDB.GetAddressNftUtxoByKey(address)
	if err == nil {
		for _, utxo := range spendList {
			// Filter by codeHash and genesis if provided
			if codeHash != "" && utxo.CodeHash != codeHash {
				continue
			}
			if genesis != "" && utxo.Genesis != genesis {
				continue
			}
			if _, ok := spentMap[utxo.UtxoId]; !ok {
				spendUtxoList = append(spendUtxoList, utxo)
				spentMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	return
}

// getRawNftUTXOsByCodeHashGenesis internal method, gets raw NFT UTXO data by codeHash and genesis
func (m *NftMempoolManager) getRawNftUTXOsByCodeHashGenesis(codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Map for deduplication
	incomeMap := make(map[string]struct{})
	spentMap := make(map[string]struct{})

	// Build key
	key := common.ConcatBytesOptimized([]string{codeHash, genesis}, "@")

	// 1. Get income UTXOs
	incomeList, err := m.mempoolCodeHashGenesisNftIncomeValidStore.GetCodeHashGenesisNftUtxoByKey(key)
	if err == nil {
		for _, utxo := range incomeList {
			if _, ok := incomeMap[utxo.UtxoId]; !ok {
				incomeUtxoList = append(incomeUtxoList, utxo)
				incomeMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	// 2. Get spent UTXOs
	spendList, err := m.mempoolCodeHashGenesisNftSpendStore.GetCodeHashGenesisNftUtxoByKey(key)
	if err == nil {
		for _, utxo := range spendList {
			if _, ok := spentMap[utxo.UtxoId]; !ok {
				spendUtxoList = append(spendUtxoList, utxo)
				spentMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	return
}

func (m *NftMempoolManager) getNftInfo(codeHash string, genesis string, tokenIndex string) (*common.NftInfoModel, error) {
	// Build key: codeHash@genesis@tokenIndex (tokenIndex should be formatted to 30 digits)
	tokenIndexUint, err := strconv.ParseUint(tokenIndex, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid tokenIndex: %w", err)
	}
	key := common.ConcatBytesOptimized([]string{codeHash, genesis, fmt.Sprintf("%030d", tokenIndexUint)}, "@")

	valueInfo, err := m.mempoolContractNftInfoStore.GetSimpleRecord(key)
	if err != nil {
		return nil, err
	}

	// Parse data: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
	parts := strings.Split(string(valueInfo), "@")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid NFT info format")
	}

	nftInfo := &common.NftInfoModel{
		SensibleId:      parts[0],
		TokenSupply:     parts[1],
		MetaTxId:        parts[2],
		MetaOutputIndex: parts[3],
	}
	return nftInfo, nil
}

// GetVerifyTx gets verification transaction information
func (m *NftMempoolManager) GetVerifyTx(txId string, page, pageSize int) ([]string, int, error) {
	if txId != "" {
		// If txId is provided, only return information for that transaction
		value, err := m.mempoolVerifyTxStore.GetSimpleRecord(txId)
		if err != nil {
			if strings.Contains(err.Error(), storage.ErrNotFound.Error()) {
				return []string{}, 0, nil
			}
			return nil, 0, err
		}
		return []string{string(value)}, 1, nil
	}

	// Get all verification transactions
	values, err := m.mempoolVerifyTxStore.GetAll()
	if err != nil {
		return nil, 0, err
	}

	total := len(values)
	if total == 0 {
		return []string{}, 0, nil
	}

	// Calculate pagination
	start := (page - 1) * pageSize
	if start >= total {
		return []string{}, total, nil
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	return values[start:end], total, nil
}

// GetUncheckNftUtxo gets unchecked NFT UTXO list
func (m *NftMempoolManager) GetUncheckNftUtxo() ([]common.NftUtxo, error) {
	return m.mempoolUncheckNftOutpointStore.GetNftUtxo()
}

// GetMempoolAddressNftSpendMap gets NFT spend data for addresses in mempool
func (m *NftMempoolManager) GetMempoolAddressNftSpendMap(address string) (map[string]string, error) {
	// If address is empty, get all data
	if address == "" {
		// Get NFT spend data for all addresses
		allData, err := m.mempoolAddressNftSpendDB.GetAllKeyValues()
		if err != nil {
			return nil, err
		}

		// Convert string array to map
		result := make(map[string]string)
		for key, data := range allData {
			result[key] = data
		}
		return result, nil
	}

	// Get NFT spend data for specified address
	spendList, err := m.mempoolAddressNftSpendDB.GetAddressNftUtxoByKey(address)
	if err != nil {
		return nil, err
	}

	// Convert data to map format
	result := make(map[string]string)
	for _, utxo := range spendList {
		key := utxo.TxID + ":" + utxo.Index
		value := common.ConcatBytesOptimized([]string{
			utxo.CodeHash,
			utxo.Genesis,
			utxo.SensibleId,
			utxo.TokenIndex,
			utxo.Index,
			utxo.Value,
			utxo.TokenSupply,
			utxo.MetaTxId,
			utxo.MetaOutputIndex,
			strconv.FormatInt(utxo.Timestamp, 10),
			utxo.UsedTxId,
		}, "@")
		result[key] = value
	}

	return result, nil
}

// GetMempoolCodeHashGenesisNftSpendMap gets spend data for codeHash@genesis NFT in mempool
func (m *NftMempoolManager) GetMempoolCodeHashGenesisNftSpendMap(codeHashGenesis string) (map[string]string, error) {
	// If codeHashGenesis is empty, get all data
	if codeHashGenesis == "" {
		// Get spend data for all codeHash@genesis NFTs
		allData, err := m.mempoolCodeHashGenesisNftSpendStore.GetAllKeyValues()
		if err != nil {
			return nil, err
		}

		// Convert string array to map
		result := make(map[string]string)
		for _, data := range allData {
			result[data] = data
		}
		return result, nil
	}

	// Get spend data for specified codeHash@genesis NFT
	spendList, err := m.mempoolCodeHashGenesisNftSpendStore.GetCodeHashGenesisNftUtxoByKey(codeHashGenesis)
	if err != nil {
		return nil, err
	}

	// Convert data to map format
	result := make(map[string]string)
	for _, utxo := range spendList {
		key := utxo.TxID + ":" + utxo.Index
		value := common.ConcatBytesOptimized([]string{
			utxo.Address,
			utxo.SensibleId,
			utxo.TokenIndex,
			utxo.Index,
			utxo.Value,
			utxo.TokenSupply,
			utxo.MetaTxId,
			utxo.MetaOutputIndex,
		}, "@")
		result[key] = value
	}

	return result, nil
}

// GetMempoolGenesisUtxo gets genesis UTXO information from mempool
// key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}
func (m *NftMempoolManager) GetMempoolGenesisUtxo(outpoint string) (utxo *common.NftUtxo, err error) {
	// Get data directly from mempool genesis utxo store using key
	data, err := m.mempoolContractNftGenesisUtxoStore.GetSimpleRecord(outpoint)
	if err != nil {
		return nil, fmt.Errorf("Failed to get genesis utxo from mempool: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("Genesis utxo not found in mempool")
	}

	// Parse data: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}
	parts := strings.Split(string(data), "@")
	if len(parts) < 9 {
		return nil, fmt.Errorf("Invalid genesis utxo data format")
	}

	// Extract outpoint parts (txId:index)
	outpointParts := strings.Split(outpoint, ":")
	if len(outpointParts) != 2 {
		return nil, fmt.Errorf("Invalid outpoint format")
	}

	// Create NftUtxo object
	utxo = &common.NftUtxo{
		TxID:            outpointParts[0],
		Index:           parts[5], // index from value
		UtxoId:          outpoint,
		SensibleId:      parts[0],
		TokenSupply:     parts[1],
		CodeHash:        parts[2],
		Genesis:         parts[3],
		TokenIndex:      parts[4],
		Value:           parts[6],
		MetaTxId:        parts[7],
		MetaOutputIndex: parts[8],
	}

	return utxo, nil
}

// getRawSellNftUTXOsByAddress internal method, gets raw NFT sell UTXO data by address
func (m *NftMempoolManager) getRawSellNftUTXOsByAddress(address string, codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Map for deduplication
	incomeMap := make(map[string]struct{})
	spentMap := make(map[string]struct{})

	// 1. Get income UTXOs
	incomeList, err := m.mempoolAddressSellNftIncomeStore.GetAddressSellNftUtxoByKey(address)
	if err == nil {
		for _, utxo := range incomeList {
			// Filter by codeHash and genesis if provided
			if codeHash != "" && utxo.CodeHash != codeHash {
				continue
			}
			if genesis != "" && utxo.Genesis != genesis {
				continue
			}
			if _, ok := incomeMap[utxo.UtxoId]; !ok {
				incomeUtxoList = append(incomeUtxoList, utxo)
				incomeMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	// 2. Get spent UTXOs
	spendList, err := m.mempoolAddressSellNftSpendStore.GetAddressSellNftUtxoByKey(address)
	if err == nil {
		for _, utxo := range spendList {
			// Filter by codeHash and genesis if provided
			if codeHash != "" && utxo.CodeHash != codeHash {
				continue
			}
			if genesis != "" && utxo.Genesis != genesis {
				continue
			}
			if _, ok := spentMap[utxo.UtxoId]; !ok {
				spendUtxoList = append(spendUtxoList, utxo)
				spentMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	return
}

// getRawSellNftUTXOsByCodeHashGenesis internal method, gets raw NFT sell UTXO data by codeHash and genesis
func (m *NftMempoolManager) getRawSellNftUTXOsByCodeHashGenesis(codeHash string, genesis string) (incomeUtxoList []common.NftUtxo, spendUtxoList []common.NftUtxo, err error) {
	// Map for deduplication
	incomeMap := make(map[string]struct{})
	spentMap := make(map[string]struct{})

	// Build key
	key := common.ConcatBytesOptimized([]string{codeHash, genesis}, "@")

	// 1. Get income UTXOs
	incomeList, err := m.mempoolCodeHashGenesisSellNftIncomeStore.GetCodeHashGenesisSellNftUtxoByKey(key)
	if err == nil {
		for _, utxo := range incomeList {
			if _, ok := incomeMap[utxo.UtxoId]; !ok {
				incomeUtxoList = append(incomeUtxoList, utxo)
				incomeMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	// 2. Get spent UTXOs
	spendList, err := m.mempoolCodeHashGenesisSellNftSpendStore.GetCodeHashGenesisSellNftUtxoByKey(key)
	if err == nil {
		for _, utxo := range spendList {
			if _, ok := spentMap[utxo.UtxoId]; !ok {
				spendUtxoList = append(spendUtxoList, utxo)
				spentMap[utxo.UtxoId] = struct{}{}
			}
		}
	}
	return
}
