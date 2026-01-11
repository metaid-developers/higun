package indexer

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/storage"
)

// NftUTXO struct definition
type NftUTXO struct {
	CodeHash        string `json:"codeHash"`
	Genesis         string `json:"genesis"`
	SensibleId      string `json:"sensibleId"`
	TokenIndex      uint64 `json:"tokenIndex"`
	TokenSupply     uint64 `json:"tokenSupply"`
	MetaTxId        string `json:"metaTxId"`
	MetaOutputIndex uint64 `json:"metaOutputIndex"`
	Txid            string `json:"txid"`
	TxIndex         int64  `json:"txIndex"`
	ValueString     string `json:"valueString"`
	Value           int64  `json:"value"`
	Height          int64  `json:"height"`
	Address         string `json:"address"`
	Flag            string `json:"flag"`
}

// NftSellUTXO struct definition for NFT sell UTXO
type NftSellUTXO struct {
	CodeHash        string `json:"codeHash"`
	Genesis         string `json:"genesis"`
	TokenIndex      uint64 `json:"tokenIndex"`
	Price           uint64 `json:"price"`
	ContractAddress string `json:"contractAddress"`
	Txid            string `json:"txid"`
	TxIndex         int64  `json:"txIndex"`
	ValueString     string `json:"valueString"`
	Value           int64  `json:"value"`
	Height          int64  `json:"height"`
	Address         string `json:"address"`
	Flag            string `json:"flag"`
	IsReady         bool   `json:"isReady"`
	SensibleId      string `json:"sensibleId"`
	TokenSupply     uint64 `json:"tokenSupply"`
	MetaTxId        string `json:"metaTxId"`
	MetaOutputIndex uint64 `json:"metaOutputIndex"`
}

// NftInfo struct definition
type NftInfo struct {
	CodeHash        string `json:"codeHash"`
	Genesis         string `json:"genesis"`
	SensibleId      string `json:"sensibleId"`
	TokenSupply     uint64 `json:"tokenSupply"`
	MetaTxId        string `json:"metaTxId"`
	MetaOutputIndex uint64 `json:"metaOutputIndex"`
}

// NftGenesisInfo struct definition for genesis information
type NftGenesisInfo struct {
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

// NftSummary struct definition for address summary
type NftSummary struct {
	CodeHash        string `json:"codeHash"`
	Genesis         string `json:"genesis"`
	SensibleId      string `json:"sensibleId"`
	TokenSupply     uint64 `json:"tokenSupply"`
	MetaTxId        string `json:"metaTxId"`
	MetaOutputIndex uint64 `json:"metaOutputIndex"`
	Count           int    `json:"count"`
	Address         string `json:"address"`
}

// NftOwnerInfo struct definition for NFT owners with pagination
type NftOwnerInfo struct {
	Total      int         `json:"total"`
	List       []*NftOwner `json:"list"`
	Cursor     int         `json:"cursor"`
	NextCursor int         `json:"nextCursor"`
	Size       int         `json:"size"`
}

// NftOwner struct definition for individual NFT owner
type NftOwner struct {
	CodeHash    string `json:"codeHash"`
	Genesis     string `json:"genesis"`
	SensibleId  string `json:"sensibleId"`
	TokenSupply uint64 `json:"tokenSupply"`
	Address     string `json:"address"`
	Count       int    `json:"count"` // Number of NFTs owned
}

// GetNftUTXOsByAddress gets NFT UTXOs by address with pagination
func (i *ContractNftIndexer) GetNftUTXOsByAddress(address, codeHash, genesis string, cursor, size int) (utxos []*NftUTXO, total int, nextCursor int, err error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	if cursor < 0 {
		cursor = 0
	}

	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
	}()

	// Get spent NFT UTXOs
	spendData, _, err := i.addressNftSpendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			// spendValue: txid@index@codeHash@genesis@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId,...
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) >= 2 {
				outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
				spendMap[outpoint] = struct{}{}
			}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.NftUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetNftUTXOsByAddress(address, codeHash, genesis)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
		// fmt.Printf("[QUERY]mempoolIncomeList: %v, mempoolSpendList: %v\n", mempoolIncomeList, mempoolSpendList)
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		spendMap[key] = struct{}{}
	}
	// for key, _ := range spendMap {
	// 	fmt.Printf("[QUERY]spendMap: %s\n", key)
	// }

	// Get NFT income data
	data, _, err := i.addressNftIncomeValidStore.GetWithShard(addrKey)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, 0, 0, err
		}
	}

	// Map for deduplication
	uniqueUtxoMap := make(map[string]*NftUTXO)

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 10 {
			continue
		}

		// Parse data
		currCodeHash := incomes[0]
		currGenesis := incomes[1]
		currTokenIndex := incomes[2]
		currTxID := incomes[3]
		currIndex := incomes[4]

		// If codeHash and genesis are specified, only process matching ones
		if codeHash != "" && codeHash != currCodeHash {
			continue
		}
		if genesis != "" && genesis != currGenesis {
			continue
		}

		// Check if already spent
		key := currTxID + ":" + currIndex
		if _, exists := spendMap[key]; exists {
			// fmt.Printf("[QUERY]already spent: %s\n", key)
			continue
		}

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		// Get NFT info
		nftInfo, _ := i.GetNftInfo(currCodeHash, currGenesis, currTokenIndex)

		// Parse values
		tokenIndex, _ := strconv.ParseUint(currTokenIndex, 10, 64)
		tokenSupply, _ := strconv.ParseUint(incomes[6], 10, 64)
		metaOutputIndex, _ := strconv.ParseUint(incomes[8], 10, 64)
		value, _ := strconv.ParseInt(incomes[5], 10, 64)
		height, _ := strconv.ParseInt(incomes[9], 10, 64)
		txIndex, _ := strconv.ParseInt(currIndex, 10, 64)

		uniqueUtxoMap[key] = &NftUTXO{
			Txid:            currTxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     incomes[5],
			CodeHash:        currCodeHash,
			Genesis:         currGenesis,
			SensibleId:      nftInfo.SensibleId,
			TokenIndex:      tokenIndex,
			TokenSupply:     tokenSupply,
			MetaTxId:        incomes[7],
			MetaOutputIndex: metaOutputIndex,
			Address:         address,
			Height:          height,
			Flag:            fmt.Sprintf("%s_%s", currTxID, currIndex),
		}
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		if codeHash != "" && codeHash != utxo.CodeHash {
			continue
		}
		if genesis != "" && genesis != utxo.Genesis {
			continue
		}

		key := utxo.TxID + ":" + utxo.Index
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		// Get NFT info
		nftInfo, _ := i.GetNftInfo(utxo.CodeHash, utxo.Genesis, utxo.TokenIndex)
		fmt.Printf("[QUERY]mempoolIncomeList: %+v\n", utxo)

		tokenIndex, _ := strconv.ParseUint(utxo.TokenIndex, 10, 64)
		tokenSupply, _ := strconv.ParseUint(utxo.TokenSupply, 10, 64)
		metaOutputIndex, _ := strconv.ParseUint(utxo.MetaOutputIndex, 10, 64)
		value, _ := strconv.ParseInt(utxo.Value, 10, 64)
		txIndex, _ := strconv.ParseInt(utxo.Index, 10, 64)

		uniqueUtxoMap[key] = &NftUTXO{
			Txid:            utxo.TxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     utxo.Value,
			CodeHash:        utxo.CodeHash,
			Genesis:         utxo.Genesis,
			SensibleId:      nftInfo.SensibleId,
			TokenIndex:      tokenIndex,
			TokenSupply:     tokenSupply,
			MetaTxId:        utxo.MetaTxId,
			MetaOutputIndex: metaOutputIndex,
			Address:         address,
			Height:          -1,
			Flag:            fmt.Sprintf("%s_%s", utxo.TxID, utxo.Index),
		}
	}

	// Convert map to slice
	for _, utxo := range uniqueUtxoMap {
		utxos = append(utxos, utxo)
	}

	// Sort by txid and index
	sort.Slice(utxos, func(i, j int) bool {
		if utxos[i].Txid == utxos[j].Txid {
			return utxos[i].TxIndex < utxos[j].TxIndex
		}
		return utxos[i].Txid < utxos[j].Txid
	})

	// Apply pagination
	total = len(utxos)
	startIndex := cursor
	if startIndex > total {
		startIndex = total
	}
	endIndex := startIndex + size
	if endIndex > total {
		endIndex = total
	}

	if startIndex < total {
		utxos = utxos[startIndex:endIndex]
	} else {
		utxos = []*NftUTXO{}
	}

	nextCursor = 0
	if endIndex < total {
		nextCursor = endIndex
	}

	return utxos, total, nextCursor, nil
}

// GetFastNftUTXOsByCodeHashGenesis fast query for NFT UTXO by codeHash, genesis and tokenIndex
// This is optimized for single tokenIndex lookup, used for isReady check in sell UTXOs
func (i *ContractNftIndexer) GetFastNftUTXOsByCodeHashGenesis(codeHash, genesis string, tokenIndex uint64) (*NftUTXO, error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	key := common.ConcatBytesOptimized([]string{codeHash, genesis}, "@")

	// Build spend map for this codeHash@genesis
	spendMap := make(map[string]struct{})
	spendData, _, err := i.codeHashGenesisNftSpendStore.GetWithShard([]byte(key))
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) >= 2 {
				outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
				spendMap[outpoint] = struct{}{}
			}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.NftUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetNftUTXOsByCodeHashGenesis(codeHash, genesis)
		if err == nil {
			// Process mempool spend data
			for _, utxo := range mempoolSpendList {
				outpoint := utxo.TxID + ":" + utxo.Index
				spendMap[outpoint] = struct{}{}
			}

			// Check mempool income for matching tokenIndex
			for _, utxo := range mempoolIncomeList {
				utxoTokenIndex, _ := strconv.ParseUint(utxo.TokenIndex, 10, 64)

				// Only process matching tokenIndex
				if utxoTokenIndex != tokenIndex {
					continue
				}

				outpoint := utxo.TxID + ":" + utxo.Index
				// Check if already spent
				if _, exists := spendMap[outpoint]; exists {
					continue
				}

				// Get NFT info
				// nftInfo, _ := i.GetNftInfo(codeHash, genesis, utxo.TokenIndex)

				tokenSupply, _ := strconv.ParseUint(utxo.TokenSupply, 10, 64)
				metaOutputIndex, _ := strconv.ParseUint(utxo.MetaOutputIndex, 10, 64)
				value, _ := strconv.ParseInt(utxo.Value, 10, 64)
				txIndex, _ := strconv.ParseInt(utxo.Index, 10, 64)

				// Return the mempool UTXO immediately
				return &NftUTXO{
					Txid:        utxo.TxID,
					TxIndex:     txIndex,
					Value:       value,
					ValueString: utxo.Value,
					CodeHash:    utxo.CodeHash,
					Genesis:     utxo.Genesis,
					// SensibleId:      nftInfo.SensibleId,
					TokenIndex:      utxoTokenIndex,
					TokenSupply:     tokenSupply,
					MetaTxId:        utxo.MetaTxId,
					MetaOutputIndex: metaOutputIndex,
					Address:         utxo.Address,
					Height:          -1,
					Flag:            fmt.Sprintf("%s_%s", utxo.TxID, utxo.Index),
				}, nil
			}
		}
	}

	// Get NFT income data from confirmed store
	data, _, err := i.codeHashGenesisNftIncomeValidStore.GetWithShard([]byte(key))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		return nil, nil
	}

	// Parse and find matching tokenIndex
	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// NftAddress@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 9 {
			continue
		}

		// Parse tokenIndex
		currTokenIndex, _ := strconv.ParseUint(incomes[1], 10, 64)

		// Only process matching tokenIndex
		if currTokenIndex != tokenIndex {
			continue
		}

		currAddress := incomes[0]
		currTxID := incomes[2]
		currIndex := incomes[3]

		// Check if already spent
		outpoint := currTxID + ":" + currIndex
		if _, exists := spendMap[outpoint]; exists {
			continue
		}

		// Get NFT info
		// nftInfo, _ := i.GetNftInfo(codeHash, genesis, fmt.Sprintf("%d", tokenIndex))

		// Parse values
		tokenSupply, _ := strconv.ParseUint(incomes[5], 10, 64)
		metaOutputIndex, _ := strconv.ParseUint(incomes[7], 10, 64)
		value, _ := strconv.ParseInt(incomes[4], 10, 64)
		height, _ := strconv.ParseInt(incomes[8], 10, 64)
		txIndex, _ := strconv.ParseInt(currIndex, 10, 64)

		// Return the first matching UTXO
		return &NftUTXO{
			Txid:        currTxID,
			TxIndex:     txIndex,
			Value:       value,
			ValueString: incomes[4],
			CodeHash:    codeHash,
			Genesis:     genesis,
			// SensibleId:      nftInfo.SensibleId,
			TokenIndex:      tokenIndex,
			TokenSupply:     tokenSupply,
			MetaTxId:        incomes[6],
			MetaOutputIndex: metaOutputIndex,
			Address:         currAddress,
			Height:          height,
			Flag:            fmt.Sprintf("%s_%s", currTxID, currIndex),
		}, nil
	}

	// Not found, return nil
	return nil, nil
}

// GetNftUTXOsByCodeHashGenesis gets NFT UTXOs by codeHash and genesis with tokenIndex filter
func (i *ContractNftIndexer) GetNftUTXOsByCodeHashGenesis(codeHash, genesis string, hasTokenIndex bool, tokenIndex uint64, hasTokenIndexMin bool, tokenIndexMin uint64, hasTokenIndexMax bool, tokenIndexMax uint64) (utxos []*NftUTXO, err error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	key := common.ConcatBytesOptimized([]string{codeHash, genesis}, "@")
	spendMap := make(map[string]struct{})
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
	}()

	// Get spent NFT UTXOs
	spendData, _, err := i.codeHashGenesisNftSpendStore.GetWithShard([]byte(key))
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			// spendValue: txid@index@NftAddress@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId,...
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) >= 2 {
				outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
				spendMap[outpoint] = struct{}{}
			}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.NftUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetNftUTXOsByCodeHashGenesis(codeHash, genesis)
		if err != nil {
			return nil, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		spendMap[key] = struct{}{}
	}

	// Get NFT income data
	data, _, err := i.codeHashGenesisNftIncomeValidStore.GetWithShard([]byte(key))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	}

	// Map for deduplication
	uniqueUtxoMap := make(map[string]*NftUTXO)

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// NftAddress@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 9 {
			continue
		}

		// Parse data
		currAddress := incomes[0]
		currTokenIndex := incomes[1]
		currTxID := incomes[2]
		currIndex := incomes[3]

		// Check if already spent
		key := currTxID + ":" + currIndex
		if _, exists := spendMap[key]; exists {
			continue
		}

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		// Get NFT info
		nftInfo, _ := i.GetNftInfo(codeHash, genesis, currTokenIndex)

		// Parse values
		currTokenIndexUint, _ := strconv.ParseUint(currTokenIndex, 10, 64)
		tokenSupply, _ := strconv.ParseUint(incomes[5], 10, 64)
		metaOutputIndex, _ := strconv.ParseUint(incomes[7], 10, 64)
		value, _ := strconv.ParseInt(incomes[4], 10, 64)
		height, _ := strconv.ParseInt(incomes[8], 10, 64)
		txIndex, _ := strconv.ParseInt(currIndex, 10, 64)

		uniqueUtxoMap[key] = &NftUTXO{
			Txid:            currTxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     incomes[4],
			CodeHash:        codeHash,
			Genesis:         genesis,
			SensibleId:      nftInfo.SensibleId,
			TokenIndex:      currTokenIndexUint,
			TokenSupply:     tokenSupply,
			MetaTxId:        incomes[6],
			MetaOutputIndex: metaOutputIndex,
			Address:         currAddress,
			Height:          height,
			Flag:            fmt.Sprintf("%s_%s", currTxID, currIndex),
		}
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		key := utxo.TxID + ":" + utxo.Index
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		// Get NFT info
		nftInfo, _ := i.GetNftInfo(codeHash, genesis, utxo.TokenIndex)

		utxoTokenIndex, _ := strconv.ParseUint(utxo.TokenIndex, 10, 64)
		tokenSupply, _ := strconv.ParseUint(utxo.TokenSupply, 10, 64)
		metaOutputIndex, _ := strconv.ParseUint(utxo.MetaOutputIndex, 10, 64)
		value, _ := strconv.ParseInt(utxo.Value, 10, 64)
		txIndex, _ := strconv.ParseInt(utxo.Index, 10, 64)

		uniqueUtxoMap[key] = &NftUTXO{
			Txid:            utxo.TxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     utxo.Value,
			CodeHash:        utxo.CodeHash,
			Genesis:         utxo.Genesis,
			SensibleId:      nftInfo.SensibleId,
			TokenIndex:      utxoTokenIndex,
			TokenSupply:     tokenSupply,
			MetaTxId:        utxo.MetaTxId,
			MetaOutputIndex: metaOutputIndex,
			Address:         utxo.Address,
			Height:          -1,
			Flag:            fmt.Sprintf("%s_%s", utxo.TxID, utxo.Index),
		}
	}

	// Convert map to slice
	for _, utxo := range uniqueUtxoMap {
		utxos = append(utxos, utxo)
	}

	// Sort by tokenIndex, then txid
	sort.Slice(utxos, func(i, j int) bool {
		if utxos[i].TokenIndex == utxos[j].TokenIndex {
			if utxos[i].Txid == utxos[j].Txid {
				return utxos[i].TxIndex < utxos[j].TxIndex
			}
			return utxos[i].Txid < utxos[j].Txid
		}
		return utxos[i].TokenIndex < utxos[j].TokenIndex
	})

	// Apply tokenIndex filter after sorting
	if hasTokenIndex || hasTokenIndexMin || hasTokenIndexMax {
		filteredUtxos := make([]*NftUTXO, 0)
		for _, utxo := range utxos {
			// Apply tokenIndex filter
			if hasTokenIndex && utxo.TokenIndex != tokenIndex {
				continue
			}
			if hasTokenIndexMin && utxo.TokenIndex < tokenIndexMin {
				continue
			}
			if hasTokenIndexMax && utxo.TokenIndex > tokenIndexMax {
				continue
			}
			filteredUtxos = append(filteredUtxos, utxo)
		}
		utxos = filteredUtxos
	}

	return utxos, nil
}

// GetNftSellUTXOsByAddress gets NFT sell UTXOs by address with pagination
func (i *ContractNftIndexer) GetNftSellUTXOsByAddress(address, codeHash, genesis string, cursor, size int) (utxos []*NftSellUTXO, total int, nextCursor int, err error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	if cursor < 0 {
		cursor = 0
	}

	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
	}()

	// Get spent NFT sell UTXOs
	spendData, _, err := i.addressSellNftSpendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			// spendValue: txid@index@codeHash@genesis@tokenIndex@value@height@usedTxId,...
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) >= 2 {
				outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
				spendMap[outpoint] = struct{}{}
			}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.NftUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetSellNftUTXOsByAddress(address, codeHash, genesis)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		spendMap[key] = struct{}{}
	}

	// Get NFT sell income data
	data, _, err := i.addressSellNftIncomeStore.GetWithShard(addrKey)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, 0, 0, err
		}
	}

	// Map for deduplication
	uniqueUtxoMap := make(map[string]*NftSellUTXO)

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// codeHash@genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 9 {
			continue
		}

		// Parse data
		currCodeHash := incomes[0]
		currGenesis := incomes[1]
		currTokenIndex := incomes[2]
		currTxID := incomes[5]
		currIndex := incomes[6]

		// If codeHash and genesis are specified, only process matching ones
		if codeHash != "" && codeHash != currCodeHash {
			continue
		}
		if genesis != "" && genesis != currGenesis {
			continue
		}

		// Check if already spent
		key := currTxID + ":" + currIndex
		if _, exists := spendMap[key]; exists {
			continue
		}

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		// Parse values
		tokenIndex, _ := strconv.ParseUint(currTokenIndex, 10, 64)
		price, _ := strconv.ParseUint(incomes[3], 10, 64)
		value, _ := strconv.ParseInt(incomes[7], 10, 64)
		height, _ := strconv.ParseInt(incomes[8], 10, 64)
		txIndex, _ := strconv.ParseInt(currIndex, 10, 64)

		uniqueUtxoMap[key] = &NftSellUTXO{
			Txid:            currTxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     incomes[7],
			CodeHash:        currCodeHash,
			Genesis:         currGenesis,
			TokenIndex:      tokenIndex,
			Price:           price,
			ContractAddress: incomes[4],
			Address:         address,
			Height:          height,
			Flag:            fmt.Sprintf("%s_%s", currTxID, currIndex),
		}
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		if codeHash != "" && codeHash != utxo.CodeHash {
			continue
		}
		if genesis != "" && genesis != utxo.Genesis {
			continue
		}

		key := utxo.TxID + ":" + utxo.Index
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		tokenIndex, _ := strconv.ParseUint(utxo.TokenIndex, 10, 64)
		price, _ := strconv.ParseUint("0", 10, 64) // TODO: Get price from mempool utxo
		value, _ := strconv.ParseInt(utxo.Value, 10, 64)
		txIndex, _ := strconv.ParseInt(utxo.Index, 10, 64)

		uniqueUtxoMap[key] = &NftSellUTXO{
			Txid:            utxo.TxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     utxo.Value,
			CodeHash:        utxo.CodeHash,
			Genesis:         utxo.Genesis,
			TokenIndex:      tokenIndex,
			Price:           price,
			ContractAddress: "", // TODO: Get from mempool
			Address:         address,
			Height:          -1,
			Flag:            fmt.Sprintf("%s_%s", utxo.TxID, utxo.Index),
		}
	}

	// Convert map to slice
	for _, utxo := range uniqueUtxoMap {
		//get nft info
		nftInfo, _ := i.GetNftInfo(utxo.CodeHash, utxo.Genesis, fmt.Sprintf("%d", utxo.TokenIndex))
		utxo.SensibleId = nftInfo.SensibleId
		utxo.TokenSupply = nftInfo.TokenSupply
		utxo.MetaTxId = nftInfo.MetaTxId
		utxo.MetaOutputIndex = nftInfo.MetaOutputIndex

		//check if is ready
		// Query the current owner of this NFT (by codeHash, genesis, tokenIndex) using fast method
		// If the current owner's address equals contractAddress, it means the NFT is still in the contract (ready to sell)
		nftUtxo, err := i.GetFastNftUTXOsByCodeHashGenesis(utxo.CodeHash, utxo.Genesis, utxo.TokenIndex)
		if err == nil && nftUtxo != nil {
			// Check if the NFT's current address matches the contract address
			utxo.IsReady = nftUtxo.Address == utxo.ContractAddress
		} else {
			utxo.IsReady = false
		}

		utxos = append(utxos, utxo)
	}

	// Sort by tokenIndex
	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].TokenIndex < utxos[j].TokenIndex
	})

	// Apply pagination
	total = len(utxos)
	startIndex := cursor
	if startIndex > total {
		startIndex = total
	}
	endIndex := startIndex + size
	if endIndex > total {
		endIndex = total
	}

	if startIndex < total {
		utxos = utxos[startIndex:endIndex]
	} else {
		utxos = []*NftSellUTXO{}
	}

	nextCursor = 0
	if endIndex < total {
		nextCursor = endIndex
	}

	return utxos, total, nextCursor, nil
}

// GetNftSellUTXOsByCodeHashGenesis gets NFT sell UTXOs by codeHash and genesis with tokenIndex filter
func (i *ContractNftIndexer) GetNftSellUTXOsByCodeHashGenesis(codeHash, genesis string, hasTokenIndex bool, tokenIndex uint64, hasTokenIndexMin bool, tokenIndexMin uint64, hasTokenIndexMax bool, tokenIndexMax uint64) (utxos []*NftSellUTXO, err error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	key := common.ConcatBytesOptimized([]string{codeHash, genesis}, "@")
	spendMap := make(map[string]struct{})
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
	}()

	// Get spent NFT sell UTXOs
	spendData, _, err := i.codeHashGenesisSellNftSpendStore.GetWithShard([]byte(key))
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			// spendValue: txid@index@NftAddress@tokenIndex@value@height@usedTxId,...
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) >= 2 {
				outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
				spendMap[outpoint] = struct{}{}
			}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.NftUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetSellNftUTXOsByCodeHashGenesis(codeHash, genesis)
		if err != nil {
			return nil, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		spendMap[key] = struct{}{}
	}

	// Get NFT sell income data
	data, _, err := i.codeHashGenesisSellNftIncomeStore.GetWithShard([]byte(key))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	}

	// Map for deduplication
	uniqueUtxoMap := make(map[string]*NftSellUTXO)

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 8 {
			continue
		}

		// Parse data
		currAddress := incomes[0]
		currTokenIndex := incomes[1]
		currTxID := incomes[4]
		currIndex := incomes[5]

		// Check if already spent
		key := currTxID + ":" + currIndex
		if _, exists := spendMap[key]; exists {
			continue
		}

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		// Parse values
		currTokenIndexUint, _ := strconv.ParseUint(currTokenIndex, 10, 64)
		price, _ := strconv.ParseUint(incomes[2], 10, 64)
		value, _ := strconv.ParseInt(incomes[6], 10, 64)
		height, _ := strconv.ParseInt(incomes[7], 10, 64)
		txIndex, _ := strconv.ParseInt(currIndex, 10, 64)

		uniqueUtxoMap[key] = &NftSellUTXO{
			Txid:            currTxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     incomes[6],
			CodeHash:        codeHash,
			Genesis:         genesis,
			TokenIndex:      currTokenIndexUint,
			Price:           price,
			ContractAddress: incomes[3],
			Address:         currAddress,
			Height:          height,
			Flag:            fmt.Sprintf("%s_%s", currTxID, currIndex),
		}
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		key := utxo.TxID + ":" + utxo.Index
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		utxoTokenIndex, _ := strconv.ParseUint(utxo.TokenIndex, 10, 64)
		price, _ := strconv.ParseUint("0", 10, 64) // TODO: Get price from mempool utxo
		value, _ := strconv.ParseInt(utxo.Value, 10, 64)
		txIndex, _ := strconv.ParseInt(utxo.Index, 10, 64)

		uniqueUtxoMap[key] = &NftSellUTXO{
			Txid:            utxo.TxID,
			TxIndex:         txIndex,
			Value:           value,
			ValueString:     utxo.Value,
			CodeHash:        utxo.CodeHash,
			Genesis:         utxo.Genesis,
			TokenIndex:      utxoTokenIndex,
			Price:           price,
			ContractAddress: "", // TODO: Get from mempool
			Address:         utxo.Address,
			Height:          -1,
			Flag:            fmt.Sprintf("%s_%s", utxo.TxID, utxo.Index),
		}
	}

	// Convert map to slice
	for _, utxo := range uniqueUtxoMap {
		//get nft info
		nftInfo, _ := i.GetNftInfo(utxo.CodeHash, utxo.Genesis, fmt.Sprintf("%d", utxo.TokenIndex))
		utxo.SensibleId = nftInfo.SensibleId
		utxo.TokenSupply = nftInfo.TokenSupply
		utxo.MetaTxId = nftInfo.MetaTxId
		utxo.MetaOutputIndex = nftInfo.MetaOutputIndex

		//check if is ready - using fast method
		// Query the current owner of this NFT (by codeHash, genesis, tokenIndex)
		// If the current owner's address equals contractAddress, it means the NFT is still in the contract (ready to sell)
		nftUtxo, err := i.GetFastNftUTXOsByCodeHashGenesis(utxo.CodeHash, utxo.Genesis, utxo.TokenIndex)
		if err == nil && nftUtxo != nil {
			// Check if the NFT's current address matches the contract address
			utxo.IsReady = nftUtxo.Address == utxo.ContractAddress
		} else {
			utxo.IsReady = false
		}

		utxos = append(utxos, utxo)
	}

	// Sort by tokenIndex
	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].TokenIndex < utxos[j].TokenIndex
	})

	// Apply tokenIndex filter after sorting
	if hasTokenIndex || hasTokenIndexMin || hasTokenIndexMax {
		filteredUtxos := make([]*NftSellUTXO, 0)
		for _, utxo := range utxos {
			// Apply tokenIndex filter
			if hasTokenIndex && utxo.TokenIndex != tokenIndex {
				continue
			}
			if hasTokenIndexMin && utxo.TokenIndex < tokenIndexMin {
				continue
			}
			if hasTokenIndexMax && utxo.TokenIndex > tokenIndexMax {
				continue
			}
			filteredUtxos = append(filteredUtxos, utxo)
		}
		utxos = filteredUtxos
	}

	return utxos, nil
}

// GetNftUtxoCountByAddress gets NFT UTXO count by address
func (i *ContractNftIndexer) GetNftUtxoCountByAddress(address string) (count int, err error) {
	if address == "" {
		return 0, fmt.Errorf("address parameter is required")
	}

	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
	}()

	// Get spent NFT UTXOs
	spendData, _, err := i.addressNftSpendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) >= 2 {
				outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
				spendMap[outpoint] = struct{}{}
			}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.NftUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetNftUTXOsByAddress(address, "", "")
		if err != nil {
			return 0, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		spendMap[key] = struct{}{}
	}

	// Get NFT income data
	data, _, err := i.addressNftIncomeValidStore.GetWithShard(addrKey)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return 0, err
		}
		data = nil
	}

	// Map for deduplication
	uniqueUtxoMap := make(map[string]struct{})

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 5 {
			continue
		}

		currTxID := incomes[3]
		currIndex := incomes[4]

		// Check if already spent
		key := currTxID + ":" + currIndex
		if _, exists := spendMap[key]; exists {
			continue
		}

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}
		uniqueUtxoMap[key] = struct{}{}
		count++
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		key := utxo.TxID + ":" + utxo.Index
		if _, exists := spendMap[key]; exists {
			continue
		}
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}
		uniqueUtxoMap[key] = struct{}{}
		count++
	}

	return count, nil
}

// GetNftAddressSummary gets NFT address summary (grouped by codeHash@genesis)
func (i *ContractNftIndexer) GetNftAddressSummary(address string, cursor, size int) (summaries []*NftSummary, total int, nextCursor int, err error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	if cursor < 0 {
		cursor = 0
	}

	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
	}()

	// Get spent NFT UTXOs
	spendData, _, err := i.addressNftSpendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) >= 2 {
				outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
				spendMap[outpoint] = struct{}{}
			}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.NftUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetNftUTXOsByAddress(address, "", "")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		spendMap[key] = struct{}{}
	}

	// Get NFT income data
	data, _, err := i.addressNftIncomeValidStore.GetWithShard(addrKey)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, 0, 0, err
		}
		data = nil
	}

	// Map for grouping by codeHash@genesis
	summaryMap := make(map[string]*NftSummary)
	// Map to track unique UTXOs processed
	processedUtxos := make(map[string]struct{})

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 10 {
			continue
		}

		currCodeHash := incomes[0]
		currGenesis := incomes[1]
		currTokenIndex := incomes[2]
		currTxID := incomes[3]
		currIndex := incomes[4]

		// Check if already spent
		key := currTxID + ":" + currIndex
		if _, exists := spendMap[key]; exists {
			continue
		}

		// Check if already processed
		if _, exists := processedUtxos[key]; exists {
			continue
		}
		processedUtxos[key] = struct{}{}

		// Group by codeHash@genesis
		summaryKey := currCodeHash + "@" + currGenesis
		if summary, exists := summaryMap[summaryKey]; exists {
			summary.Count++
		} else {
			// Get NFT info
			nftInfo, _ := i.GetNftInfo(currCodeHash, currGenesis, currTokenIndex)
			tokenSupply, _ := strconv.ParseUint(incomes[6], 10, 64)
			metaOutputIndex, _ := strconv.ParseUint(incomes[8], 10, 64)

			summaryMap[summaryKey] = &NftSummary{
				CodeHash:        currCodeHash,
				Genesis:         currGenesis,
				SensibleId:      nftInfo.SensibleId,
				TokenSupply:     tokenSupply,
				MetaTxId:        incomes[7],
				MetaOutputIndex: metaOutputIndex,
				Count:           1,
				Address:         address,
			}
		}
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		key := utxo.TxID + ":" + utxo.Index
		if _, exists := spendMap[key]; exists {
			continue
		}
		if _, exists := processedUtxos[key]; exists {
			continue
		}
		processedUtxos[key] = struct{}{}

		// Group by codeHash@genesis
		summaryKey := utxo.CodeHash + "@" + utxo.Genesis
		if summary, exists := summaryMap[summaryKey]; exists {
			summary.Count++
		} else {
			// Get NFT info
			nftInfo, _ := i.GetNftInfo(utxo.CodeHash, utxo.Genesis, utxo.TokenIndex)
			tokenSupply, _ := strconv.ParseUint(utxo.TokenSupply, 10, 64)
			metaOutputIndex, _ := strconv.ParseUint(utxo.MetaOutputIndex, 10, 64)

			summaryMap[summaryKey] = &NftSummary{
				CodeHash:        utxo.CodeHash,
				Genesis:         utxo.Genesis,
				SensibleId:      nftInfo.SensibleId,
				TokenSupply:     tokenSupply,
				MetaTxId:        utxo.MetaTxId,
				MetaOutputIndex: metaOutputIndex,
				Count:           1,
				Address:         address,
			}
		}
	}

	// Convert map to slice
	for _, summary := range summaryMap {
		summaries = append(summaries, summary)
	}

	// Sort by sensibleId
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].SensibleId < summaries[j].SensibleId
	})

	// Apply pagination
	total = len(summaries)
	startIndex := cursor
	if startIndex > total {
		startIndex = total
	}
	endIndex := startIndex + size
	if endIndex > total {
		endIndex = total
	}

	if startIndex < total {
		summaries = summaries[startIndex:endIndex]
	} else {
		summaries = []*NftSummary{}
	}

	nextCursor = 0
	if endIndex < total {
		nextCursor = endIndex
	}

	return summaries, total, nextCursor, nil
}

// GetNftSummary gets all NFT summary with cursor-based pagination
func (i *ContractNftIndexer) GetNftSummary(cursor, size int) (nftInfos []*NftInfo, total int, nextCursor int, err error) {
	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	if cursor < 0 {
		cursor = 0
	}

	// Collect all NFT info keys first for sorting
	var allKeys []string
	for _, db := range i.contractNftSummaryInfoStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, 0, 0, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Collect all keys
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			allKeys = append(allKeys, key)
		}
	}

	// Sort keys for consistent pagination
	sort.Strings(allKeys)

	// Build filtered key list (exclude zero sensibleId) and cache values
	const zeroSensibleId = "000000000000000000000000000000000000000000000000000000000000000000000000"
	filteredKeys := make([]string, 0, len(allKeys))
	keyToValue := make(map[string]string)
	for _, key := range allKeys {
		value, err := i.contractNftSummaryInfoStore.Get([]byte(key))
		if err != nil {
			continue
		}
		parts := strings.Split(string(value), "@")
		if len(parts) < 4 {
			continue
		}
		if parts[0] == zeroSensibleId {
			continue
		}
		filteredKeys = append(filteredKeys, key)
		keyToValue[key] = string(value)
	}

	// Sort by sensibleId
	type item struct {
		key        string
		value      string
		sensibleId string
	}
	items := make([]item, 0, len(filteredKeys))
	for _, key := range filteredKeys {
		v := keyToValue[key]
		parts := strings.Split(v, "@")
		if len(parts) < 4 {
			continue
		}
		items = append(items, item{key: key, value: v, sensibleId: parts[0]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].sensibleId < items[j].sensibleId })

	total = len(items)
	startIndex := cursor
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex > total {
		startIndex = total
	}
	endIndex := startIndex + size
	if endIndex > total {
		endIndex = total
	}
	if endIndex < total {
		nextCursor = endIndex
	}

	for idx := startIndex; idx < endIndex; idx++ {
		key := items[idx].key
		value := items[idx].value

		parts := strings.Split(value, "@")
		if len(parts) < 4 {
			continue
		}
		tokenSupply, _ := strconv.ParseUint(parts[1], 10, 64)
		metaOutputIndex, _ := strconv.ParseUint(parts[3], 10, 64)

		keyParts := strings.Split(key, "@")
		if len(keyParts) < 2 {
			continue
		}
		nftInfo := &NftInfo{
			CodeHash:        keyParts[0],
			Genesis:         keyParts[1],
			SensibleId:      parts[0],
			TokenSupply:     tokenSupply,
			MetaTxId:        parts[2],
			MetaOutputIndex: metaOutputIndex,
		}
		nftInfos = append(nftInfos, nftInfo)
	}

	return nftInfos, total, nextCursor, nil
}

// GetNftInfo gets NFT information
func (i *ContractNftIndexer) GetNftInfo(codeHash, genesis, tokenIndex string) (*NftInfo, error) {
	// Build key
	key := common.ConcatBytesOptimized([]string{codeHash, genesis, fmt.Sprintf("%030d", mustParseTokenIndex(tokenIndex))}, "@")

	// Get NFT information from contractNftInfoStore
	data, err := i.contractNftInfoStore.Get([]byte(key))
	if err != nil {
		// If not found in main storage, try mempool
		if errors.Is(err, storage.ErrNotFound) && i.mempoolMgr != nil {
			mempoolInfo, mempoolErr := i.mempoolMgr.GetNftInfo(codeHash, genesis, tokenIndex)
			if mempoolErr == nil && mempoolInfo != nil {
				tokenSupply, _ := strconv.ParseUint(mempoolInfo.TokenSupply, 10, 64)
				metaOutputIndex, _ := strconv.ParseUint(mempoolInfo.MetaOutputIndex, 10, 64)
				return &NftInfo{
					CodeHash:        codeHash,
					Genesis:         genesis,
					SensibleId:      mempoolInfo.SensibleId,
					TokenSupply:     tokenSupply,
					MetaTxId:        mempoolInfo.MetaTxId,
					MetaOutputIndex: metaOutputIndex,
				}, nil
			}
		}
		return &NftInfo{
			CodeHash: codeHash,
			Genesis:  genesis,
		}, nil
	}

	// Parse NFT information: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
	parts := strings.Split(string(data), "@")
	if len(parts) < 4 {
		return &NftInfo{
			CodeHash: codeHash,
			Genesis:  genesis,
		}, nil
	}

	tokenSupply, _ := strconv.ParseUint(parts[1], 10, 64)
	metaOutputIndex, _ := strconv.ParseUint(parts[3], 10, 64)

	return &NftInfo{
		CodeHash:        codeHash,
		Genesis:         genesis,
		SensibleId:      parts[0],
		TokenSupply:     tokenSupply,
		MetaTxId:        parts[2],
		MetaOutputIndex: metaOutputIndex,
	}, nil
}

func mustParseTokenIndex(tokenIndex string) uint64 {
	idx, err := strconv.ParseUint(tokenIndex, 10, 64)
	if err != nil {
		return 0
	}
	return idx
}

// GetDbNftUtxoByTx gets NFT UTXO by transaction ID
func (i *ContractNftIndexer) GetDbNftUtxoByTx(tx string) ([]byte, error) {
	return i.contractNftUtxoStore.Get([]byte(tx))
}

// GetDbAllNftUtxo gets all NFT UTXO data with pagination
func (i *ContractNftIndexer) GetDbAllNftUtxo(key string, page, pageSize int) (map[string]string, int, int, error) {
	result := make(map[string]string)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.contractNftUtxoStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, 0, 0, nil
			}
			return nil, 0, 0, fmt.Errorf("Failed to get NFT UTXO data: %w", err)
		}
		result[key] = string(value)
		return result, 1, 0, nil
	}

	// Collect all keys
	var allKeys []string
	for _, db := range i.contractNftUtxoStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, 0, 0, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			allKeys = append(allKeys, key)
		}
	}

	// Sort keys
	sort.Strings(allKeys)

	// Calculate pagination
	total := len(allKeys)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	for idx := start; idx < end && idx < len(allKeys); idx++ {
		value, err := i.contractNftUtxoStore.Get([]byte(allKeys[idx]))
		if err == nil {
			result[allKeys[idx]] = string(value)
		}
	}

	return result, total, totalPages, nil
}

// GetDbAddressNftIncomeValid gets valid NFT income data for specified address with pagination
func (i *ContractNftIndexer) GetDbAddressNftIncomeValid(address string, codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	data, err := i.addressNftIncomeValidStore.Get([]byte(address))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		// CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 2 {
			continue
		}

		currCodeHash := incomes[0]
		currGenesis := incomes[1]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetDbAddressNftIncome gets NFT income data for specified address with pagination
func (i *ContractNftIndexer) GetDbAddressNftIncome(address string, codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	data, err := i.addressNftIncomeStore.Get([]byte(address))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		// CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 2 {
			continue
		}

		currCodeHash := incomes[0]
		currGenesis := incomes[1]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetDbAddressNftSpend gets NFT spend data for specified address with pagination
func (i *ContractNftIndexer) GetDbAddressNftSpend(address string, codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	data, err := i.addressNftSpendStore.Get([]byte(address))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		// txid@index@codeHash@genesis@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId
		spends := strings.Split(part, "@")
		if len(spends) < 4 {
			continue
		}

		currCodeHash := spends[2]
		currGenesis := spends[3]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetDbCodeHashGenesisNftIncome gets NFT income data by codeHash and genesis with pagination
func (i *ContractNftIndexer) GetDbCodeHashGenesisNftIncome(codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if codeHash == "" || genesis == "" {
		return nil, 0, 0, fmt.Errorf("codeHash and genesis parameters are required")
	}

	key := codeHash + "@" + genesis
	data, err := i.codeHashGenesisNftIncomeStore.Get([]byte(key))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	var filteredParts []string
	for _, part := range parts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetDbCodeHashGenesisNftSpend gets NFT spend data by codeHash and genesis with pagination
func (i *ContractNftIndexer) GetDbCodeHashGenesisNftSpend(codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if codeHash == "" || genesis == "" {
		return nil, 0, 0, fmt.Errorf("codeHash and genesis parameters are required")
	}

	key := codeHash + "@" + genesis
	data, err := i.codeHashGenesisNftSpendStore.Get([]byte(key))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	var filteredParts []string
	for _, part := range parts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetDbAddressSellNftIncome gets NFT sell income data for specified address with pagination
func (i *ContractNftIndexer) GetDbAddressSellNftIncome(address string, codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	data, err := i.addressSellNftIncomeStore.Get([]byte(address))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		// codeHash@genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 2 {
			continue
		}

		currCodeHash := incomes[0]
		currGenesis := incomes[1]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetAllDbAddressSellNftIncome gets all address NFT sell income data
// If key (address) is provided, returns data for that address only
func (i *ContractNftIndexer) GetAllDbAddressSellNftIncome(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.addressSellNftIncomeStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get NFT sell income data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.addressSellNftIncomeStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = value
		}
	}

	return result, nil
}

// GetDbAddressSellNftSpend gets NFT sell spend data for specified address with pagination
func (i *ContractNftIndexer) GetDbAddressSellNftSpend(address string, codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("address parameter is required")
	}

	data, err := i.addressSellNftSpendStore.Get([]byte(address))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		// txid@index@codeHash@genesis@tokenIndex@value@height@usedTxId
		spends := strings.Split(part, "@")
		if len(spends) < 4 {
			continue
		}

		currCodeHash := spends[2]
		currGenesis := spends[3]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetAllDbAddressSellNftSpend gets all address NFT sell spend data
// If key (address) is provided, returns data for that address only
func (i *ContractNftIndexer) GetAllDbAddressSellNftSpend(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.addressSellNftSpendStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get NFT sell spend data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.addressSellNftSpendStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = value
		}
	}

	return result, nil
}

// GetDbCodeHashGenesisSellNftIncome gets NFT sell income data by codeHash and genesis with pagination
func (i *ContractNftIndexer) GetDbCodeHashGenesisSellNftIncome(codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if codeHash == "" || genesis == "" {
		return nil, 0, 0, fmt.Errorf("codeHash and genesis parameters are required")
	}

	key := codeHash + "@" + genesis
	data, err := i.codeHashGenesisSellNftIncomeStore.Get([]byte(key))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	var filteredParts []string
	for _, part := range parts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetAllDbCodeHashGenesisSellNftIncome gets all NFT sell income data grouped by codeHash@genesis
// If key (codeHash@genesis) is provided, returns data for that key only
func (i *ContractNftIndexer) GetAllDbCodeHashGenesisSellNftIncome(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.codeHashGenesisSellNftIncomeStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get NFT sell income data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.codeHashGenesisSellNftIncomeStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = value
		}
	}

	return result, nil
}

// GetDbCodeHashGenesisSellNftSpend gets NFT sell spend data by codeHash and genesis with pagination
func (i *ContractNftIndexer) GetDbCodeHashGenesisSellNftSpend(codeHash, genesis string, page, pageSize int) ([]string, int, int, error) {
	if codeHash == "" || genesis == "" {
		return nil, 0, 0, fmt.Errorf("codeHash and genesis parameters are required")
	}

	key := codeHash + "@" + genesis
	data, err := i.codeHashGenesisSellNftSpendStore.Get([]byte(key))
	if err != nil {
		return nil, 0, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	var filteredParts []string
	for _, part := range parts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	// Calculate pagination
	total := len(filteredParts)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	var currentPageData []string
	if start < total {
		currentPageData = filteredParts[start:end]
	}

	return currentPageData, total, totalPages, nil
}

// GetAllDbCodeHashGenesisSellNftSpend gets all NFT sell spend data grouped by codeHash@genesis
// If key (codeHash@genesis) is provided, returns data for that key only
func (i *ContractNftIndexer) GetAllDbCodeHashGenesisSellNftSpend(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.codeHashGenesisSellNftSpendStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get NFT sell spend data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.codeHashGenesisSellNftSpendStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = value
		}
	}

	return result, nil
}

// GetDbAllNftInfo gets all NFT info data with pagination
func (i *ContractNftIndexer) GetDbAllNftInfo(key string, page, pageSize int) (map[string]string, int, int, error) {
	result := make(map[string]string)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.contractNftInfoStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, 0, 0, nil
			}
			return nil, 0, 0, fmt.Errorf("Failed to get NFT info data: %w", err)
		}
		result[key] = string(value)
		return result, 1, 0, nil
	}

	// Collect all keys
	var allKeys []string
	for _, db := range i.contractNftInfoStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, 0, 0, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			allKeys = append(allKeys, key)
		}
	}

	// Sort keys
	sort.Strings(allKeys)

	// Calculate pagination
	total := len(allKeys)
	totalPages := (total + pageSize - 1) / pageSize
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	for idx := start; idx < end && idx < len(allKeys); idx++ {
		value, err := i.contractNftInfoStore.Get([]byte(allKeys[idx]))
		if err == nil {
			result[allKeys[idx]] = string(value)
		}
	}

	return result, total, totalPages, nil
}

// GetNftGenesis gets NFT genesis information by codeHash and genesis
func (i *ContractNftIndexer) GetNftGenesis(codeHash, genesis string) (*NftGenesisInfo, error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	// Build query key - for NFT we need to query summary info (without tokenIndex)
	key := codeHash + "@" + genesis

	// Get NFT summary information from contractNftSummaryInfoStore
	// value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
	summaryData, err := i.contractNftSummaryInfoStore.Get([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("Failed to get NFT genesis info: %w", err)
	}

	// Parse summary data
	parts := strings.Split(string(summaryData), "@")
	if len(parts) < 4 {
		return nil, fmt.Errorf("NFT summary info format error")
	}

	tokenSupply, _ := strconv.ParseUint(parts[1], 10, 64)

	nftGenesisInfo := &NftGenesisInfo{
		CodeHash:    codeHash,
		Genesis:     genesis,
		SensibleId:  parts[0],
		TokenSupply: tokenSupply,
	}

	// Parse sensibleId to get genesisTxId and index
	genesisTxId, genesisIndex, err := parseSensibleId(nftGenesisInfo.SensibleId)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse sensibleId: %w", err)
	}

	// Build outpoint
	outpoint := genesisTxId + ":" + strconv.Itoa(int(genesisIndex))

	// key:usedOutpoint,
	// value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
	// Get genesis output list from contractNftGenesisOutputStore
	genesisOutputs, err := i.contractNftGenesisOutputStore.Get([]byte(outpoint))
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("Failed to get genesis outputs: %w", err)
	}

	// Parse genesis outputs: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
	outputParts := strings.Split(string(genesisOutputs), ",")

	issusCodeHash := ""
	issusGenesis := ""

	// Find the issue output (tokenIndex == 0)
	for _, output := range outputParts {
		if output == "" {
			continue
		}

		parts := strings.Split(output, "@")
		if len(parts) < 10 {
			continue
		}

		// Check if this output matches our codeHash and genesis
		outputCodeHash := parts[2]
		outputGenesis := parts[3]
		// tokenIndexStr := parts[4]
		// sensibleId := parts[0]
		metaTxId := parts[8]

		// metaTxId == 000000000000000000000000000000000000000000000000000000000000000000000000 means it's the issue output
		if metaTxId == "0000000000000000000000000000000000000000000000000000000000000000" {
			issusCodeHash = outputCodeHash
			issusGenesis = outputGenesis
		}
	}

	// Get issue utxo from 1111111111111111111114oLvT2
	if issusCodeHash != "" && issusGenesis != "" {
		// Get unspent UTXOs for the issue address
		issueAddress := "1111111111111111111114oLvT2"

		// Get spent UTXOs for issue address
		spendMap := make(map[string]struct{})
		spendData, _, err := i.addressNftSpendStore.GetWithShard([]byte(issueAddress))
		if err == nil {
			for _, spendValue := range strings.Split(string(spendData), ",") {
				if spendValue == "" {
					continue
				}
				// spendValue: txid@index@codeHash@genesis@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId
				spendValueStrs := strings.Split(spendValue, "@")
				if len(spendValueStrs) < 12 {
					continue
				}
				// Check if this spend matches our issue codeHash and genesis
				if spendValueStrs[2] == issusCodeHash && spendValueStrs[3] == issusGenesis {
					outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
					spendMap[outpoint] = struct{}{}
				}
			}
		}

		// Get income UTXOs for issue address
		incomeData, _, err := i.addressNftIncomeStore.GetWithShard([]byte(issueAddress))
		if err == nil {
			for _, incomeValue := range strings.Split(string(incomeData), ",") {
				if incomeValue == "" {
					continue
				}
				// incomeValue: CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
				incomeValueStrs := strings.Split(incomeValue, "@")
				if len(incomeValueStrs) < 10 {
					continue
				}

				// Check if this income matches our issue codeHash and genesis
				if incomeValueStrs[0] == issusCodeHash && incomeValueStrs[1] == issusGenesis {
					outpoint := incomeValueStrs[3] + ":" + incomeValueStrs[4]

					// Check if this UTXO is not spent
					if _, isSpent := spendMap[outpoint]; !isSpent {
						// This is an unspent UTXO for the issue
						// Parse the data and add to nftGenesisInfo
						txIndex, err := strconv.ParseInt(incomeValueStrs[4], 10, 64)
						if err != nil {
							continue
						}
						height, err := strconv.ParseInt(incomeValueStrs[9], 10, 64)
						if err != nil {
							continue
						}
						tokenIndex, err := strconv.ParseUint(incomeValueStrs[2], 10, 64)
						if err != nil {
							continue
						}

						nftGenesisInfo.Txid = incomeValueStrs[3]
						nftGenesisInfo.TxIndex = txIndex
						nftGenesisInfo.TokenIndex = tokenIndex
						nftGenesisInfo.ValueString = incomeValueStrs[5]
						nftGenesisInfo.Height = height
						break // Take the first unspent UTXO
					}
				}
			}
		}
	}

	return nftGenesisInfo, nil
}

// parseSensibleId parses sensibleId string for NFT, returns genesisTxId and genesisOutputIndex
func parseSensibleId(sensibleId string) (string, uint32, error) {
	// Convert hex string to byte array
	sensibleIDBuf, err := hex.DecodeString(sensibleId)
	if err != nil {
		return "", 0, err
	}

	// Check if length is sufficient
	if len(sensibleIDBuf) < 36 {
		return "", 0, fmt.Errorf("sensibleId length too short")
	}

	// Get first 32 bytes as genesisTxId and reverse byte order
	genesisTxId := make([]byte, 32)
	copy(genesisTxId, sensibleIDBuf[:32])
	for i, j := 0, len(genesisTxId)-1; i < j; i, j = i+1, j-1 {
		genesisTxId[i], genesisTxId[j] = genesisTxId[j], genesisTxId[i]
	}

	// Get last 4 bytes as genesisOutputIndex (little-endian)
	genesisOutputIndex := binary.LittleEndian.Uint32(sensibleIDBuf[32:36])

	return hex.EncodeToString(genesisTxId), genesisOutputIndex, nil
}

// GetAllDbNftGenesis gets all NFT Genesis data
func (i *ContractNftIndexer) GetAllDbNftGenesis(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.contractNftGenesisStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get NFT Genesis data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.contractNftGenesisStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = value
		}
	}

	return result, nil
}

// GetAllDbNftGenesisOutput gets all NFT Genesis Output data
func (i *ContractNftIndexer) GetAllDbNftGenesisOutput(key string) (map[string][]string, error) {
	result := make(map[string][]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.contractNftGenesisOutputStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get NFT Genesis Output data: %w", err)
		}
		result[key] = strings.Split(string(value), ",")
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.contractNftGenesisOutputStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = strings.Split(value, ",")
		}
	}

	return result, nil
}

// GetAllDbUsedNftIncome gets all used NFT income data
func (i *ContractNftIndexer) GetAllDbUsedNftIncome(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.usedNftIncomeStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get used NFT income data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.usedNftIncomeStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = value
		}
	}

	return result, nil
}

// GetAllDbUncheckNftOutpoint gets unchecked NFT outpoint data
// If the outpoint parameter is provided, only the corresponding value is returned
// If the outpoint parameter is not provided, all data is returned
func (i *ContractNftIndexer) GetAllDbUncheckNftOutpoint(outpoint string) (map[string]string, error) {
	result := make(map[string]string)

	// If outpoint is provided, get the corresponding value directly
	if outpoint != "" {
		value, err := i.uncheckNftOutpointStore.Get([]byte(outpoint))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get outpoint data: %w", err)
		}
		result[outpoint] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.uncheckNftOutpointStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs
		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			result[key] = value
		}
	}

	return result, nil
}

// GetMempoolAddressNftIncomeMap gets NFT income data for addresses in mempool
// If address is provided, returns data for that address only; otherwise returns all addresses
func (i *ContractNftIndexer) GetMempoolAddressNftIncomeMap(address string) map[string]string {
	if i.mempoolMgr == nil {
		return make(map[string]string)
	}
	return i.mempoolMgr.GetMempoolAddressNftIncomeMap(address)
}

// GetMempoolAddressNftIncomeValidMap gets valid NFT income data for addresses in mempool
// If address is provided, returns data for that address only; otherwise returns all addresses
func (i *ContractNftIndexer) GetMempoolAddressNftIncomeValidMap(address string) map[string]string {
	if i.mempoolMgr == nil {
		return make(map[string]string)
	}
	return i.mempoolMgr.GetMempoolAddressNftIncomeValidMap(address)
}

// GetMempoolAddressNftSpendMap gets NFT spend data for address in mempool
func (i *ContractNftIndexer) GetMempoolAddressNftSpendMap(address string) (map[string]string, error) {
	if i.mempoolMgr == nil {
		return nil, fmt.Errorf("Mempool manager not set")
	}
	return i.mempoolMgr.GetMempoolAddressNftSpendMap(address)
}

// GetNftOwners gets NFT owners list by codeHash and genesis with cursor-based pagination
func (i *ContractNftIndexer) GetNftOwners(codeHash, genesis string, cursor int, size int) (*NftOwnerInfo, error) {
	if codeHash == "" || genesis == "" {
		return &NftOwnerInfo{
			Total:      0,
			List:       nil,
			Cursor:     cursor,
			NextCursor: 0,
			Size:       size,
		}, fmt.Errorf("codeHash and genesis parameters are required")
	}

	// If size is not specified or invalid, use default
	if size <= 0 {
		size = 10
	}

	// If cursor is negative, set to 0
	if cursor < 0 {
		cursor = 0
	}

	// Build query key
	key := codeHash + "@" + genesis

	// Get NFT info
	sensibleId := ""
	tokenSupply := uint64(0)
	nftInfo, _ := i.GetNftInfo(codeHash, genesis, "0")
	if nftInfo != nil {
		sensibleId = nftInfo.SensibleId
		tokenSupply = nftInfo.TokenSupply
	}

	// Map to store address NFT counts
	ownerCounts := make(map[string]int)
	// Map to track processed txId:index pairs for deduplication
	processedIncome := make(map[string]struct{})
	processedSpend := make(map[string]struct{})

	// Get income data from contractNftOwnersIncomeValidStore
	incomeData, err := i.contractNftOwnersIncomeValidStore.Get([]byte(key))
	if err == nil {
		// Parse income data: address@tokenIndex@txId@index,...
		incomeParts := strings.Split(string(incomeData), ",")
		for _, incomePart := range incomeParts {
			if incomePart == "" {
				continue
			}
			parts := strings.Split(incomePart, "@")
			if len(parts) < 4 {
				continue
			}
			address := parts[0]
			// tokenIndex := parts[1]
			txId := parts[2]
			index := parts[3]

			// Create unique key for deduplication: txId:index
			uniqueKey := txId + ":" + index

			// Skip if already processed
			if _, exists := processedIncome[uniqueKey]; exists {
				continue
			}
			processedIncome[uniqueKey] = struct{}{}

			// Increment count for this address
			ownerCounts[address]++
		}
	}

	// Get spend data from contractNftOwnersSpendStore
	spendData, err := i.contractNftOwnersSpendStore.Get([]byte(key))
	if err == nil {
		// Parse spend data: address@tokenIndex@txId@index,...
		spendParts := strings.Split(string(spendData), ",")
		for _, spendPart := range spendParts {
			if spendPart == "" {
				continue
			}
			parts := strings.Split(spendPart, "@")
			if len(parts) < 4 {
				continue
			}
			address := parts[0]
			// tokenIndex := parts[1]
			txId := parts[2]
			index := parts[3]

			// Create unique key for deduplication: txId:index
			uniqueKey := txId + ":" + index

			// Skip if already processed
			if _, exists := processedSpend[uniqueKey]; exists {
				continue
			}
			processedSpend[uniqueKey] = struct{}{}

			// Decrement count for this address
			ownerCounts[address]--
		}
	}

	// Convert map to slice and filter out zero/negative counts
	var owners []*NftOwner
	for address, count := range ownerCounts {
		if count > 0 {
			owners = append(owners, &NftOwner{
				CodeHash:    codeHash,
				Genesis:     genesis,
				SensibleId:  sensibleId,
				TokenSupply: tokenSupply,
				Address:     address,
				Count:       count,
			})
		}
	}

	// Sort by count in descending order
	sort.Slice(owners, func(i, j int) bool {
		return owners[i].Count > owners[j].Count
	})

	// Get total count
	total := len(owners)

	// Apply cursor-based pagination (cursor is offset)
	var nextCursor int
	var paginatedOwners []*NftOwner

	// Calculate start and end indices
	startIndex := cursor
	if startIndex > len(owners) {
		startIndex = len(owners)
	}

	endIndex := startIndex + size
	if endIndex > len(owners) {
		endIndex = len(owners)
	}

	// Extract paginated owners
	if startIndex < len(owners) {
		paginatedOwners = owners[startIndex:endIndex]
	} else {
		paginatedOwners = []*NftOwner{}
	}

	// Set next cursor if there are more items
	if endIndex < len(owners) {
		nextCursor = endIndex
	} else {
		nextCursor = 0 // No more items
	}

	ownerInfo := &NftOwnerInfo{
		Total:      total,
		List:       paginatedOwners,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}

	return ownerInfo, nil
}
