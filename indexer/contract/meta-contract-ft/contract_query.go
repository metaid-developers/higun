package indexer

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/contract/meta-contract/decoder"
	"github.com/metaid/utxo_indexer/storage"
)

type FtBalance struct {
	Confirmed                                   int64  `json:"confirmed"`
	ConfirmedString                             string `json:"confirmedString"`
	UnconfirmedIncome                           int64  `json:"unconfirmedIncome"`
	UnconfirmedIncomeString                     string `json:"unconfirmedIncomeString"`
	UnconfirmedSpend                            int64  `json:"unconfirmedSpend"`
	UnconfirmedSpendString                      string `json:"unconfirmedSpendString"`
	UnconfirmedSpendFromConfirmed               int64  `json:"unconfirmedSpendFromConfirmed"`
	UnconfirmedSpendFromConfirmedString         string `json:"unconfirmedSpendFromConfirmedString"`
	UnconfirmedSpendFromUnconfirmedIncome       int64  `json:"unconfirmedSpendFromUnconfirmedIncome"`
	UnconfirmedSpendFromUnconfirmedIncomeString string `json:"unconfirmedSpendFromUnconfirmedIncomeString"`
	Balance                                     int64  `json:"balance"`
	BalanceString                               string `json:"balanceString"`
	UTXOCount                                   int64  `json:"utxoCount"`
	CodeHash                                    string `json:"codeHash"`
	Genesis                                     string `json:"genesis"`
	SensibleId                                  string `json:"sensibleId"`
	Name                                        string `json:"name"`
	Symbol                                      string `json:"symbol"`
	Decimal                                     uint8  `json:"decimal"`
	FtAddress                                   string `json:"ftAddress"`
}

type FtUTXO struct {
	CodeHash      string `json:"codeHash"`
	Genesis       string `json:"genesis"`
	Name          string `json:"name"`
	Symbol        string `json:"symbol"`
	SensibleId    string `json:"sensibleId"`
	Decimal       uint8  `json:"decimal"`
	Txid          string `json:"txid"`
	TxIndex       int64  `json:"txIndex"`
	ValueString   string `json:"valueString"`
	SatoshiString string `json:"satoshiString"`
	Value         int64  `json:"value"`
	Satoshi       int64  `json:"satoshi"`
	Height        int64  `json:"height"`
	Address       string `json:"address"`
	Flag          string `json:"flag"`
}

// FtInfo struct definition
type FtInfo struct {
	CodeHash   string `json:"codeHash"`
	Genesis    string `json:"genesis"`
	SensibleId string `json:"sensibleId"`
	Name       string `json:"name"`
	Symbol     string `json:"symbol"`
	Decimal    uint8  `json:"decimal"`
}

type UniqueFtUtxo struct {
	Txid          string `json:"txid"`
	TxIndex       int64  `json:"txIndex"`
	CodeHash      string `json:"codeHash"`
	Genesis       string `json:"genesis"`
	SensibleId    string `json:"sensibleId"`
	Height        int64  `json:"height"`
	CustomData    string `json:"customData"`
	Satoshi       string `json:"satoshi"`
	SatoshiString string `json:"satoshiString"`
}

type FtGenesisInfo struct {
	CodeHash      string `json:"codeHash"`
	Genesis       string `json:"genesis"`
	SensibleId    string `json:"sensibleId"`
	Name          string `json:"name"`
	Symbol        string `json:"symbol"`
	Decimal       uint8  `json:"decimal"`
	Txid          string `json:"txid"`
	TxIndex       int64  `json:"txIndex"`
	ValueString   string `json:"valueString"`
	SatoshiString string `json:"satoshiString"`
	Height        int64  `json:"height"`
}

type FtSupplyInfo struct {
	Confirmed           string `json:"confirmed"`
	Unconfirmed         string `json:"unconfirmed"`
	AllowIncreaseIssues bool   `json:"allowIncreaseIssues"`
	MaxSupply           string `json:"maxSupply"`
}

type FtOwnerInfo struct {
	Total      int        `json:"total"`
	List       []*FtOwner `json:"list"`
	Cursor     int        `json:"cursor"`
	NextCursor int        `json:"nextCursor"`
	Size       int        `json:"size"`
}
type FtOwner struct {
	CodeHash   string `json:"codeHash"`
	Genesis    string `json:"genesis"`
	SensibleId string `json:"sensibleId"`
	Name       string `json:"name"`
	Symbol     string `json:"symbol"`
	Decimal    uint8  `json:"decimal"`
	Address    string `json:"address"`
	Balance    string `json:"balance"`
}

type FtAddressHistory struct {
	Total      int            `json:"total"`
	List       []*FtAddressTx `json:"list"`
	Cursor     int            `json:"cursor"`
	NextCursor int            `json:"nextCursor"`
	Size       int            `json:"size"`
}
type FtAddressTx struct {
	CodeHash      string `json:"codeHash"`
	Genesis       string `json:"genesis"`
	SensibleId    string `json:"sensibleId"`
	Name          string `json:"name"`
	Symbol        string `json:"symbol"`
	Decimal       uint8  `json:"decimal"`
	Address       string `json:"address"`
	TxId          string `json:"txId"`
	Time          int64  `json:"time"`
	BlockHeight   int64  `json:"blockHeight"`
	IsOutcome     bool   `json:"isOutcome"`
	IsIncome      bool   `json:"isIncome"`
	OutcomeAmount string `json:"outcomeAmount"`
	IncomeAmount  string `json:"incomeAmount"`
}

type FtGenesisHistory struct {
	Total      int            `json:"total"`
	List       []*FtGenesisTx `json:"list"`
	Cursor     int            `json:"cursor"`
	NextCursor int            `json:"nextCursor"`
	Size       int            `json:"size"`
}
type FtGenesisTx struct {
	CodeHash   string `json:"codeHash"`
	Genesis    string `json:"genesis"`
	SensibleId string `json:"sensibleId"`
	TxId       string `json:"txId"`
}

// FtSupplyEntry 代表一条增发记录
type FtSupplyEntry struct {
	SensibleId string `json:"sensibleId"`
	Name       string `json:"name"`
	Symbol     string `json:"symbol"`
	Decimal    uint8  `json:"decimal"`
	CodeHash   string `json:"codeHash"`
	Genesis    string `json:"genesis"`
	Amount     string `json:"amount"`
	TxId       string `json:"txId"`
	Index      string `json:"index"`
	Value      string `json:"value"`
}

type FtSupplyList struct {
	Total      int              `json:"total"`
	List       []*FtSupplyEntry `json:"list"`
	Cursor     int              `json:"cursor"`
	NextCursor int              `json:"nextCursor"`
	Size       int              `json:"size"`
}

// FtBurnEntry 代表一条销毁记录
type FtBurnEntry struct {
	SensibleId string `json:"sensibleId"`
	Name       string `json:"name"`
	Symbol     string `json:"symbol"`
	Decimal    uint8  `json:"decimal"`
	CodeHash   string `json:"codeHash"`
	Genesis    string `json:"genesis"`
	Amount     string `json:"amount"`
	TxId       string `json:"txId"`
	Index      string `json:"index"`
	Value      string `json:"value"`
}

type FtBurnList struct {
	Total      int            `json:"total"`
	List       []*FtBurnEntry `json:"list"`
	Cursor     int            `json:"cursor"`
	NextCursor int            `json:"nextCursor"`
	Size       int            `json:"size"`
}

// FtOwnerTxEntry 代表一条所有者交易记录（income 或 spend）
type FtOwnerTxEntry struct {
	Address string `json:"address"`
	Amount  string `json:"amount"`
	TxId    string `json:"txId"`
	Index   string `json:"index"`
}

// FtOwnerTxData 返回指定地址在指定 FT 的收入和支出数据
type FtOwnerTxData struct {
	CodeHash string            `json:"codeHash"`
	Genesis  string            `json:"genesis"`
	Address  string            `json:"address"`
	Income   []*FtOwnerTxEntry `json:"income"`
	Spend    []*FtOwnerTxEntry `json:"spend"`
}

// FtAddressHistoryDbEntry 代表一条地址历史记录（db 查询返回）
type FtAddressHistoryDbEntry struct {
	TxId   string `json:"txId"`
	Time   int64  `json:"time"`
	TxType string `json:"txType"` // "income" or "outcome"
}

// FtAddressHistoryDbList db 查询返回的地址历史列表
type FtAddressHistoryDbList struct {
	Total      int                        `json:"total"`
	List       []*FtAddressHistoryDbEntry `json:"list"`
	Cursor     int                        `json:"cursor"`
	NextCursor int                        `json:"nextCursor"`
	Size       int                        `json:"size"`
}

func (i *ContractFtIndexer) GetFtBalance(address, codeHash, genesis string) (balanceResults []*FtBalance, err error) {
	balanceResults = make([]*FtBalance, 0)
	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	mempoolSpendMap := make(map[string]struct{})
	blockIncomeMap := make(map[string]struct{})
	confirmedIncomeOutpointList := make([]string, 0)
	unconfirmedIncomeOutpointList := make([]string, 0)
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
		if mempoolSpendMap != nil {
			mempoolSpendMap = nil
		}
		if blockIncomeMap != nil {
			blockIncomeMap = nil
		}
	}()

	// Get spent FT UTXOs
	spendData, _, err := i.addressFtSpendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			//spendValue: txid@index@codeHash@genesis@sensibleId@amount@value@height@usedTxId,...
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) != 9 {
				continue
			}
			outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
			spendMap[outpoint] = struct{}{}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.FtUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetFtUTXOsByAddress(address, codeHash, genesis)
		if err != nil {
			return nil, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		mempoolSpendMap[key] = struct{}{}
	}

	// Get FT income data
	// data, _, err := i.addressFtIncomeStore.GetWithShard(addrKey)
	data, _, err := i.addressFtIncomeValidStore.GetWithShard(addrKey)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	}

	// Classify and count by codeHash and genesis
	balanceMap := make(map[string]*FtBalance)
	// Map for deduplication
	uniqueUtxoMap := make(map[string]struct{})
	// Map for sorting
	genesisUtxoMap := make(map[string][]string)

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		//CodeHash@Genesis@Amount@TxID@Index@Value@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 7 {
			continue
		}

		// Parse data
		currCodeHash := incomes[0]
		currGenesis := incomes[1]
		currAmount := incomes[2]
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
			continue
		}

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}
		uniqueUtxoMap[key] = struct{}{}
		confirmedIncomeOutpointList = append(confirmedIncomeOutpointList, key)

		// Get or create balance record
		balanceKey := currCodeHash + "@" + currGenesis
		balance, exists := balanceMap[balanceKey]
		if !exists {
			// Get FT info
			ftInfo, err := i.GetFtInfo(currCodeHash + "@" + currGenesis)
			if err != nil {
				continue
			}
			balance = &FtBalance{
				CodeHash:   currCodeHash,
				Genesis:    currGenesis,
				SensibleId: ftInfo.SensibleId,
				Name:       ftInfo.Name,
				Symbol:     ftInfo.Symbol,
				Decimal:    ftInfo.Decimal,
				FtAddress:  address,
			}
			balanceMap[balanceKey] = balance
		}

		// Update balance
		amount, err := strconv.ParseInt(currAmount, 10, 64)
		if err != nil {
			continue
		}
		balance.Confirmed += amount
		balance.ConfirmedString = strconv.FormatInt(balance.Confirmed, 10)
		balance.UTXOCount++

		// Add to sorting map
		genesisUtxoMap[balanceKey] = append(genesisUtxoMap[balanceKey], key)
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		// If codeHash and genesis are specified, only process matching ones
		if codeHash != "" && codeHash != utxo.CodeHash {
			continue
		}
		if genesis != "" && genesis != utxo.Genesis {
			continue
		}

		// Check if already spent
		key := utxo.TxID + ":" + utxo.Index
		// if _, exists := mempoolSpendMap[key]; exists {
		// 	continue
		// }

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}
		uniqueUtxoMap[key] = struct{}{}
		unconfirmedIncomeOutpointList = append(unconfirmedIncomeOutpointList, key)

		// Get or create balance record
		balanceKey := utxo.CodeHash + "@" + utxo.Genesis
		balance, exists := balanceMap[balanceKey]
		if !exists {
			// Get FT info
			ftInfo, err := i.GetFtInfo(balanceKey)
			if err != nil {
				continue
			}
			balance = &FtBalance{
				CodeHash:   utxo.CodeHash,
				Genesis:    utxo.Genesis,
				SensibleId: ftInfo.SensibleId,
				Name:       ftInfo.Name,
				Symbol:     ftInfo.Symbol,
				Decimal:    ftInfo.Decimal,
				FtAddress:  address,
			}
			balanceMap[balanceKey] = balance
		}

		// Update unconfirmed income balance
		amount, err := strconv.ParseInt(utxo.Amount, 10, 64)
		if err != nil {
			continue
		}
		balance.UnconfirmedIncome += amount
		balance.UnconfirmedIncomeString = strconv.FormatInt(balance.UnconfirmedIncome, 10)
		balance.UTXOCount++

		// Add to sorting map
		genesisUtxoMap[balanceKey] = append(genesisUtxoMap[balanceKey], key)
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		// If codeHash and genesis are specified, only process matching ones
		if codeHash != "" && codeHash != utxo.CodeHash {
			continue
		}
		if genesis != "" && genesis != utxo.Genesis {
			continue
		}

		spendOutpoint := utxo.TxID + ":" + utxo.Index
		// Get or create balance record
		balanceKey := utxo.CodeHash + "@" + utxo.Genesis
		balance, exists := balanceMap[balanceKey]
		if !exists {
			// Get FT info
			ftInfo, err := i.GetFtInfo(balanceKey)
			if err != nil {
				continue
			}
			balance = &FtBalance{
				CodeHash:   utxo.CodeHash,
				Genesis:    utxo.Genesis,
				SensibleId: ftInfo.SensibleId,
				Name:       ftInfo.Name,
				Symbol:     ftInfo.Symbol,
				Decimal:    ftInfo.Decimal,
				FtAddress:  address,
			}
			balanceMap[balanceKey] = balance
		}

		// Update unconfirmed spend balance
		amount, err := strconv.ParseInt(utxo.Amount, 10, 64)
		if err != nil {
			continue
		}
		balance.UnconfirmedSpend += amount
		balance.UnconfirmedSpendString = strconv.FormatInt(balance.UnconfirmedSpend, 10)

		// fmt.Println("spendOutpoint:", spendOutpoint)
		// fmt.Println("confirmedIncomeOutpointList:", confirmedIncomeOutpointList)
		// fmt.Println("unconfirmedIncomeOutpointList:", unconfirmedIncomeOutpointList)

		for _, outpoint := range confirmedIncomeOutpointList {
			if outpoint == spendOutpoint {
				// fmt.Println("spendOutpoint from confirmed:", spendOutpoint)
				balance.UnconfirmedSpendFromConfirmed += amount
				balance.UnconfirmedSpendFromConfirmedString = strconv.FormatInt(balance.UnconfirmedSpendFromConfirmed, 10)
				break
			}
		}

		for _, outpoint := range unconfirmedIncomeOutpointList {
			if outpoint == spendOutpoint {
				// fmt.Println("spendOutpoint from unconfirmed income:", spendOutpoint)
				balance.UnconfirmedSpendFromUnconfirmedIncome += amount
				balance.UnconfirmedSpendFromUnconfirmedIncomeString = strconv.FormatInt(balance.UnconfirmedSpendFromUnconfirmedIncome, 10)
				break
			}
		}
	}

	// Sort the outpoint array for each balanceKey and keep only the first element
	for balanceKey, outpoints := range genesisUtxoMap {
		if len(outpoints) > 0 {
			sort.Strings(outpoints)
			genesisUtxoMap[balanceKey] = []string{outpoints[0]}
		}
	}

	// Get all balanceKeys and sort them
	balanceKeys := make([]string, 0, len(genesisUtxoMap))
	for balanceKey := range genesisUtxoMap {
		balanceKeys = append(balanceKeys, balanceKey)
	}

	// Sort based on the first outpoint corresponding to each balanceKey
	sort.Slice(balanceKeys, func(i, j int) bool {
		return genesisUtxoMap[balanceKeys[i]][0] < genesisUtxoMap[balanceKeys[j]][0]
	})

	// Calculate final balance and convert map to slice
	for _, balanceKey := range balanceKeys {
		balance := balanceMap[balanceKey]
		// Calculate total balance: confirmed + unconfirmed income - unconfirmed spend
		balance.Balance = balance.Confirmed + balance.UnconfirmedIncome - balance.UnconfirmedSpend
		balance.BalanceString = strconv.FormatInt(balance.Balance, 10)
		balanceResults = append(balanceResults, balance)
	}

	return balanceResults, nil
}

func (i *ContractFtIndexer) GetFtUTXOs(address, codeHash, genesis string) (utxos []*FtUTXO, err error) {
	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	defer func() {
		if spendMap != nil {
			spendMap = nil
		}
	}()

	// Get spent FT UTXOs
	spendData, _, err := i.addressFtSpendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendValue := range strings.Split(string(spendData), ",") {
			if spendValue == "" {
				continue
			}
			//spendValue: txid@index@codeHash@genesis@sensibleId@amount@value@height@usedTxId,...
			spendValueStrs := strings.Split(spendValue, "@")
			if len(spendValueStrs) != 9 {
				continue
			}
			outpoint := spendValueStrs[0] + ":" + spendValueStrs[1]
			spendMap[outpoint] = struct{}{}
		}
	}

	// Get UTXOs in mempool
	var mempoolIncomeList, mempoolSpendList []common.FtUtxo
	if i.mempoolMgr != nil {
		mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetFtUTXOsByAddress(address, codeHash, genesis)
		if err != nil {
			return nil, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
		}
	}

	// Process spent UTXOs in mempool
	for _, utxo := range mempoolSpendList {
		key := utxo.TxID + ":" + utxo.Index
		spendMap[key] = struct{}{}
	}

	// Get FT income data
	// data, _, err := i.addressFtIncomeStore.GetWithShard(addrKey)
	data, _, err := i.addressFtIncomeValidStore.GetWithShard(addrKey)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	}

	// Map for deduplication
	uniqueUtxoMap := make(map[string]*FtUTXO)

	parts := strings.Split(string(data), ",")
	for _, part := range parts {
		if part == "" {
			continue
		}
		//CodeHash@Genesis@Amount@TxID@Index@Value@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 7 {
			continue
		}

		// Parse data
		currCodeHash := incomes[0]
		currGenesis := incomes[1]
		currAmount := incomes[2]
		currTxID := incomes[3]
		currIndex := incomes[4]
		currValue := incomes[5]
		currHeight := incomes[6]

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

		// Get FT info
		ftInfo, _ := i.GetFtInfo(currCodeHash + "@" + currGenesis)
		if ftInfo == nil {
			outpoint := currTxID + ":" + currIndex
			ftGenesisUtxo, _ := i.GetFtGenesisUtxo(outpoint)
			if ftGenesisUtxo == nil {
				continue
			}
			ftInfo = ftGenesisUtxo
		}

		// Parse amount
		amount, err := strconv.ParseInt(currAmount, 10, 64)
		if err != nil {
			continue
		}
		value, err := strconv.ParseInt(currValue, 10, 64)
		if err != nil {
			continue
		}

		height, err := strconv.ParseInt(currHeight, 10, 64)
		if err != nil {
			continue
		}

		txIndex, err := strconv.ParseInt(currIndex, 10, 64)
		if err != nil {
			continue
		}

		utxos = append(utxos, &FtUTXO{
			Txid:          currTxID,
			TxIndex:       txIndex,
			Value:         amount,
			ValueString:   currAmount,
			Satoshi:       value,
			SatoshiString: currValue,
			CodeHash:      currCodeHash,
			Genesis:       currGenesis,
			SensibleId:    ftInfo.SensibleId,
			Name:          ftInfo.Name,
			Symbol:        ftInfo.Symbol,
			Decimal:       ftInfo.Decimal,
			Address:       address,
			Height:        height, // Confirmed UTXO
			Flag:          fmt.Sprintf("%s_%s", currTxID, currIndex),
		})
	}

	// Process income UTXOs in mempool
	for _, utxo := range mempoolIncomeList {
		// If codeHash and genesis are specified, only process matching ones
		if codeHash != "" && codeHash != utxo.CodeHash {
			continue
		}
		if genesis != "" && genesis != utxo.Genesis {
			continue
		}

		// Check if already spent
		key := utxo.TxID + ":" + utxo.Index
		if _, exists := spendMap[key]; exists {
			continue
		}

		// Check for duplicates
		if _, exists := uniqueUtxoMap[key]; exists {
			continue
		}

		// Get FT info
		ftInfo, _ := i.GetFtInfo(utxo.CodeHash + "@" + utxo.Genesis)
		if ftInfo == nil {
			outpoint := utxo.TxID + ":" + utxo.Index
			ftGenesisUtxo, _ := i.GetFtGenesisUtxo(outpoint)
			if ftGenesisUtxo == nil {
				continue
			}
			ftInfo = ftGenesisUtxo
		}

		// Parse amount
		amount, err := strconv.ParseInt(utxo.Amount, 10, 64)
		if err != nil {
			continue
		}
		value, err := strconv.ParseInt(utxo.Value, 10, 64)
		if err != nil {
			continue
		}
		txIndex, err := strconv.ParseInt(utxo.Index, 10, 64)
		if err != nil {
			continue
		}

		utxos = append(utxos, &FtUTXO{
			Txid:          utxo.TxID,
			TxIndex:       txIndex,
			Value:         amount,
			ValueString:   utxo.Amount,
			Satoshi:       value,
			SatoshiString: utxo.Value,
			CodeHash:      utxo.CodeHash,
			Genesis:       utxo.Genesis,
			SensibleId:    ftInfo.SensibleId,
			Name:          ftInfo.Name,
			Symbol:        ftInfo.Symbol,
			Decimal:       ftInfo.Decimal,
			Address:       address,
			Height:        -1, // UTXO in mempool
			Flag:          fmt.Sprintf("%s_%s", utxo.TxID, utxo.Index),
		})
	}

	return utxos, nil
}

func (i *ContractFtIndexer) GetDbFtUtxoByTx(tx string) ([]byte, error) {
	return i.contractFtUtxoStore.Get([]byte(tx))
}

func (i *ContractFtIndexer) GetDbAddressFtIncome(address string, codeHash string, genesis string) ([]string, error) {
	data, err := i.addressFtIncomeStore.Get([]byte(address))
	if err != nil {
		return nil, err
	}

	if codeHash == "" && genesis == "" {
		return strings.Split(string(data), ","), nil // Return all data
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		//CodeHash@Genesis@Amount@TxID@Index@Value@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 7 {
			continue
		}

		currCodeHash := incomes[0]
		currGenesis := incomes[1]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Recombine data
	return filteredParts, nil
}

func (i *ContractFtIndexer) GetDbAddressFtSpend(address string, codeHash string, genesis string) ([]string, error) {
	data, err := i.addressFtSpendStore.Get([]byte(address))
	if err != nil {
		return nil, err
	}

	if codeHash == "" && genesis == "" {
		return strings.Split(string(data), ","), nil // Return all data
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		//txid@index@codeHash@genesis@amount@value@height@usedTxId
		spends := strings.Split(part, "@")
		if len(spends) < 8 {
			continue
		}

		currCodeHash := spends[2]
		currGenesis := spends[3]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Recombine data
	return filteredParts, nil
}

// GetDbUniqueFtIncome gets unique FT income data by codeHash and genesis
func (i *ContractFtIndexer) GetDbUniqueFtIncome(codeHash string, genesis string) ([]string, error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	// Build query key
	key := codeHash + "@" + genesis

	// Get data from uniqueFtIncomeStore
	data, err := i.uniqueFtIncomeStore.Get([]byte(key))
	if err != nil {
		return nil, err
	}

	// Parse and return data: TxID@Index@Value@sensibleId@customData@height,...
	return strings.Split(string(data), ","), nil
}

// GetDbUniqueFtSpend gets unique FT spend data by codeHash and genesis
func (i *ContractFtIndexer) GetDbUniqueFtSpend(codeHash string, genesis string) ([]string, error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	// Build query key
	key := codeHash + "@" + genesis

	// Get data from uniqueFtSpendStore
	data, err := i.uniqueFtSpendStore.Get([]byte(key))
	if err != nil {
		return nil, err
	}

	// Parse and return data: TxID@Index@usedTxId,...
	return strings.Split(string(data), ","), nil
}

// GetFtInfo gets FT information
func (i *ContractFtIndexer) GetFtInfo(key string) (*FtInfo, error) {
	// Get FT information from contractFtInfoStore
	data, err := i.contractFtInfoStore.Get([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("Failed to get FT info: %w", err)
	}

	// fmt.Println("data", string(data))
	// Parse FT information
	parts := strings.Split(string(data), "@")
	if len(parts) < 4 {
		return nil, fmt.Errorf("FT info format error")
	}
	decimal, err := strconv.ParseUint(parts[3], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse decimal: %w", err)
	}

	return &FtInfo{
		CodeHash:   strings.Split(key, "@")[0],
		Genesis:    strings.Split(key, "@")[1],
		SensibleId: parts[0],
		Name:       parts[1],
		Symbol:     parts[2],
		Decimal:    uint8(decimal),
	}, nil
}

// GetFtGenesisUtxo gets FT genesis utxo information from database or mempool
// key: outpoint, value: sensibleId@name@symbol@decimal@codeHash@genesis@amount@index@value{@IsSpent}
func (i *ContractFtIndexer) GetFtGenesisUtxo(outpoint string) (*FtInfo, error) {
	// First try to get from database
	data, err := i.contractFtGenesisUtxoStore.Get([]byte(outpoint))
	if err != nil {
		// If not found in database, try mempool
		if errors.Is(err, storage.ErrNotFound) && i.mempoolMgr != nil {
			mempoolUtxo, mempoolErr := i.mempoolMgr.GetMempoolGenesisUtxo(outpoint)
			// fmt.Println("mempoolUtxo:", mempoolUtxo)
			// fmt.Println("mempoolErr:", mempoolErr)
			if mempoolErr == nil && mempoolUtxo != nil {
				// Convert FtUtxo to FtInfo
				decimal, _ := strconv.ParseUint(mempoolUtxo.Decimal, 10, 8)
				return &FtInfo{
					CodeHash:   mempoolUtxo.CodeHash,
					Genesis:    mempoolUtxo.Genesis,
					SensibleId: mempoolUtxo.SensibleId,
					Name:       mempoolUtxo.Name,
					Symbol:     mempoolUtxo.Symbol,
					Decimal:    uint8(decimal),
				}, nil
			}
		}
		return nil, fmt.Errorf("Failed to get FT genesis utxo: %w", err)
	}

	// Parse FT information: sensibleId@name@symbol@decimal@codeHash@genesis@amount@index@value{@IsSpent}
	parts := strings.Split(string(data), "@")
	if len(parts) < 9 {
		return nil, fmt.Errorf("FT info format error")
	}
	decimal, err := strconv.ParseUint(parts[3], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse decimal: %w", err)
	}

	return &FtInfo{
		CodeHash:   parts[4],
		Genesis:    parts[5],
		SensibleId: parts[0],
		Name:       parts[1],
		Symbol:     parts[2],
		Decimal:    uint8(decimal),
	}, nil
}

// GetAddressFtBalance gets address FT balance
func (i *ContractFtIndexer) GetAddressFtBalance(address string) ([]*FtBalance, error) {
	return i.GetFtBalance(address, "", "")
}

// GetAddressFtUTXOs gets address FT UTXO list
func (i *ContractFtIndexer) GetAddressFtUTXOs(address string) ([]*FtUTXO, error) {
	return i.GetFtUTXOs(address, "", "")
}

// GetMempoolUTXOs queries UTXOs in mempool for an address
func (i *ContractFtIndexer) GetMempoolFtUTXOs(address string, codeHash string, genesis string) (mempoolIncomeList []common.FtUtxo, mempoolSpendList []common.FtUtxo, err error) {
	// Check if mempool manager is set
	if i.mempoolMgr == nil {
		return nil, nil, fmt.Errorf("Mempool manager not set")
	}

	// Use interface method directly
	mempoolIncomeList, mempoolSpendList, err = i.mempoolMgr.GetFtUTXOsByAddress(address, codeHash, genesis)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to get mempool UTXOs: %w", err)
	}
	return
}

// GetAllDbAddressFtIncome gets all address FT income data
func (i *ContractFtIndexer) GetAllDbAddressFtIncome() (map[string]string, error) {
	result := make(map[string]string)

	// Iterate through all shards
	for _, db := range i.addressFtIncomeStore.GetShards() {
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

// GetAllDbAddressFtSpend gets all address FT spend data
func (i *ContractFtIndexer) GetAllDbAddressFtSpend() (map[string]string, error) {
	result := make(map[string]string)

	// Iterate through all shards
	for _, db := range i.addressFtSpendStore.GetShards() {
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

// GetDbAddressFtIncomeValidByAddress gets valid FT income data for specified address
func (i *ContractFtIndexer) GetDbAddressFtIncomeValid(address string, codeHash string, genesis string) ([]string, error) {
	data, err := i.addressFtIncomeValidStore.Get([]byte(address))
	if err != nil {
		return nil, err
	}

	if codeHash == "" && genesis == "" {
		return strings.Split(string(data), ","), nil // Return all data
	}

	// Split data by comma
	parts := strings.Split(string(data), ",")
	filteredParts := make([]string, 0)

	// Filter data
	for _, part := range parts {
		if part == "" {
			continue
		}
		// FtAddress@CodeHash@Genesis@sensibleId@Amount@TxID@Index@Value@height
		// CodeHash@Genesis@Amount@TxID@Index@Value@height
		incomes := strings.Split(part, "@")
		if len(incomes) < 7 {
			continue
		}

		currCodeHash := incomes[0]
		currGenesis := incomes[1]

		if (codeHash == "" || currCodeHash == codeHash) && (genesis == "" || currGenesis == genesis) {
			filteredParts = append(filteredParts, part)
		}
	}

	// Return filtered data
	return filteredParts, nil
}

// GetAllDbUncheckFtOutpoint gets unchecked FT outpoint data
// If the outpoint parameter is provided, only the corresponding value is returned
// If the outpoint parameter is not provided, all data is returned
func (i *ContractFtIndexer) GetAllDbUncheckFtOutpoint(outpoint string) (map[string]string, error) {
	result := make(map[string]string)

	// If outpoint is provided, get the corresponding value directly
	if outpoint != "" {
		value, err := i.uncheckFtOutpointStore.Get([]byte(outpoint))
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
	for _, db := range i.uncheckFtOutpointStore.GetShards() {
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

// GetAllDbFtGenesis gets all FT Genesis data
func (i *ContractFtIndexer) GetAllDbFtGenesis(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.contractFtGenesisStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get FT Genesis data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.contractFtGenesisStore.GetShards() {
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

// GetAllDbFtGenesisOutput gets all FT Genesis Output data
func (i *ContractFtIndexer) GetAllDbFtGenesisOutput(key string) (map[string][]string, error) {
	result := make(map[string][]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.contractFtGenesisOutputStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get FT Genesis Output data: %w", err)
		}
		result[key] = strings.Split(string(value), ",")
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.contractFtGenesisOutputStore.GetShards() {
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

// GetAllDbUsedFtIncome gets all used FT income data
func (i *ContractFtIndexer) GetAllDbUsedFtIncome(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.usedFtIncomeStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get used FT income data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.usedFtIncomeStore.GetShards() {
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

// GetAllDbFtGenesisUtxo gets all FT Genesis UTXO data
func (i *ContractFtIndexer) GetAllDbFtGenesisUtxo(key string) (map[string]string, error) {
	result := make(map[string]string)

	// If key is provided, get the corresponding value directly
	if key != "" {
		value, err := i.contractFtGenesisUtxoStore.Get([]byte(key))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return result, nil
			}
			return nil, fmt.Errorf("Failed to get FT Genesis UTXO data: %w", err)
		}
		result[key] = string(value)
		return result, nil
	}

	// Iterate through all shards
	for _, db := range i.contractFtGenesisUtxoStore.GetShards() {
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

// GetUncheckFtOutpointTotal gets the total count of unchecked FT outpoints
func (i *ContractFtIndexer) GetUncheckFtOutpointTotal() (int64, error) {
	var total int64 = 0

	// Iterate through all shards
	for _, db := range i.uncheckFtOutpointStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return 0, fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Iterate through all key-value pairs and count
		for iter.First(); iter.Valid(); iter.Next() {
			total++
		}
	}

	return total, nil
}

// GetUniqueFtUTXOs gets unique FT UTXO list
func (i *ContractFtIndexer) GetUniqueFtUTXOs(codeHash, genesis string) (utxos []*UniqueFtUtxo, err error) {
	// Use map to store unique UTXOs
	uniqueUtxos := make(map[string]*UniqueFtUtxo)
	spentUtxos := make(map[string]struct{})

	// First, get spent UTXOs from uniqueFtSpendStore
	if codeHash != "" && genesis != "" {
		// If both codeHash and genesis are specified, get specific key
		key := codeHash + "@" + genesis
		spendData, err := i.uniqueFtSpendStore.Get([]byte(key))
		if err == nil {
			// Parse spent UTXOs: TxID@Index@usedTxId,...
			parts := strings.Split(string(spendData), ",")
			for _, part := range parts {
				if part == "" {
					continue
				}
				spendParts := strings.Split(part, "@")
				if len(spendParts) >= 2 {
					spentKey := spendParts[0] + "@" + spendParts[1] // TxID@Index
					spentUtxos[spentKey] = struct{}{}
				}
			}
		}
	} else {
		// If no filter, iterate through all shards to get all spent UTXOs
		for _, db := range i.uniqueFtSpendStore.GetShards() {
			iter, err := db.NewIter(&pebble.IterOptions{})
			if err != nil {
				return nil, fmt.Errorf("Failed to create iterator for spend store: %w", err)
			}
			defer iter.Close()

			for iter.First(); iter.Valid(); iter.Next() {
				key := string(iter.Key())
				value := string(iter.Value())

				// Check if this key matches our filter
				if codeHash != "" || genesis != "" {
					keyParts := strings.Split(key, "@")
					if len(keyParts) >= 2 {
						currCodeHash := keyParts[0]
						currGenesis := keyParts[1]
						if codeHash != "" && codeHash != currCodeHash {
							continue
						}
						if genesis != "" && genesis != currGenesis {
							continue
						}
					}
				}

				// Parse spent UTXOs: TxID@Index@usedTxId,...
				parts := strings.Split(value, ",")
				for _, part := range parts {
					if part == "" {
						continue
					}
					spendParts := strings.Split(part, "@")
					if len(spendParts) >= 2 {
						spentKey := spendParts[0] + "@" + spendParts[1] // TxID@Index
						spentUtxos[spentKey] = struct{}{}
					}
				}
			}
		}
	}

	// Get mempool spent UTXOs
	if i.mempoolMgr != nil {
		codeHashGenesis := ""
		if codeHash != "" && genesis != "" {
			codeHashGenesis = codeHash + "@" + genesis
		}
		mempoolSpendMap, err := i.mempoolMgr.GetMempoolUniqueFtSpendMap(codeHashGenesis)
		if err == nil {
			for spendKey, _ := range mempoolSpendMap {
				//Key: TxID:Index
				spendKeyParts := strings.Split(spendKey, ":")
				if len(spendKeyParts) == 2 {
					spentKey := spendKeyParts[0] + "@" + spendKeyParts[1] // TxID@Index
					spentUtxos[spentKey] = struct{}{}
				}
			}
		}
	}

	// Then, get income UTXOs from uniqueFtIncomeStore
	if codeHash != "" && genesis != "" {
		// If both codeHash and genesis are specified, get specific key
		key := codeHash + "@" + genesis
		incomeData, err := i.uniqueFtIncomeStore.Get([]byte(key))
		if err == nil {
			// Parse income UTXOs: TxID@Index@Value@sensibleId@customData@height,...
			parts := strings.Split(string(incomeData), ",")
			for _, part := range parts {
				if part == "" {
					continue
				}
				incomeParts := strings.Split(part, "@")
				if len(incomeParts) < 6 {
					continue
				}

				// Parse data: TxID@Index@Value@sensibleId@customData@height
				currTxID := incomeParts[0]
				currIndex := incomeParts[1]
				currValue := incomeParts[2]
				currSensibleId := incomeParts[3]
				currCustomData := incomeParts[4]
				currHeight := incomeParts[5]

				// Check if already spent
				spentKey := currTxID + "@" + currIndex
				if _, exists := spentUtxos[spentKey]; exists {
					continue
				}

				// Create unique key
				uniqueKey := codeHash + "@" + genesis + "@" + currTxID + "@" + currIndex

				// If already exists, skip
				if _, exists := uniqueUtxos[uniqueKey]; exists {
					continue
				}

				// Parse height
				height, err := strconv.ParseInt(currHeight, 10, 64)
				if err != nil {
					continue
				}

				// Parse index
				index, err := strconv.ParseInt(currIndex, 10, 64)
				if err != nil {
					continue
				}

				// Add to unique UTXO list
				uniqueUtxos[uniqueKey] = &UniqueFtUtxo{
					Txid:          currTxID,
					TxIndex:       index,
					CodeHash:      codeHash,
					Genesis:       genesis,
					SensibleId:    currSensibleId,
					Height:        height,
					CustomData:    currCustomData,
					Satoshi:       currValue,
					SatoshiString: currValue,
				}
			}
		}
	} else {
		// If no filter, iterate through all shards to get all income UTXOs
		for _, db := range i.uniqueFtIncomeStore.GetShards() {
			iter, err := db.NewIter(&pebble.IterOptions{})
			if err != nil {
				return nil, fmt.Errorf("Failed to create iterator for income store: %w", err)
			}
			defer iter.Close()

			for iter.First(); iter.Valid(); iter.Next() {
				key := string(iter.Key())
				value := string(iter.Value())

				// Check if this key matches our filter
				if codeHash != "" || genesis != "" {
					keyParts := strings.Split(key, "@")
					if len(keyParts) >= 2 {
						currCodeHash := keyParts[0]
						currGenesis := keyParts[1]
						if codeHash != "" && codeHash != currCodeHash {
							continue
						}
						if genesis != "" && genesis != currGenesis {
							continue
						}
					}
				}

				// Parse income UTXOs: TxID@Index@Value@sensibleId@customData@height,...
				parts := strings.Split(value, ",")
				for _, part := range parts {
					if part == "" {
						continue
					}
					incomeParts := strings.Split(part, "@")
					if len(incomeParts) < 6 {
						continue
					}

					// Parse data: TxID@Index@Value@sensibleId@customData@height
					currTxID := incomeParts[0]
					currIndex := incomeParts[1]
					currValue := incomeParts[2]
					currSensibleId := incomeParts[3]
					currCustomData := incomeParts[4]
					currHeight := incomeParts[5]

					// Check if already spent
					spentKey := currTxID + "@" + currIndex
					if _, exists := spentUtxos[spentKey]; exists {
						continue
					}

					// Create unique key
					uniqueKey := key + "@" + currTxID + "@" + currIndex

					// If already exists, skip
					if _, exists := uniqueUtxos[uniqueKey]; exists {
						continue
					}

					// Parse height
					height, err := strconv.ParseInt(currHeight, 10, 64)
					if err != nil {
						continue
					}

					// Parse index
					index, err := strconv.ParseInt(currIndex, 10, 64)
					if err != nil {
						continue
					}

					// Get codeHash and genesis from key
					keyParts := strings.Split(key, "@")
					if len(keyParts) < 2 {
						continue
					}
					currCodeHash := keyParts[0]
					currGenesis := keyParts[1]

					// Add to unique UTXO list
					uniqueUtxos[uniqueKey] = &UniqueFtUtxo{
						Txid:          currTxID,
						TxIndex:       index,
						CodeHash:      currCodeHash,
						Genesis:       currGenesis,
						SensibleId:    currSensibleId,
						Height:        height,
						CustomData:    currCustomData,
						Satoshi:       currValue,
						SatoshiString: currValue,
					}
				}
			}
		}
	}

	// Get mempool income UTXOs
	if i.mempoolMgr != nil {
		codeHashGenesis := ""
		if codeHash != "" && genesis != "" {
			codeHashGenesis = codeHash + "@" + genesis
		}
		mempoolIncomeMap, err := i.mempoolMgr.GetMempoolUniqueFtIncomeMap(codeHashGenesis)
		if err == nil {
			for outpoint, incomeValue := range mempoolIncomeMap {
				// Parse: CodeHash@Genesis@SensibleId@CustomData@Index@Value
				incomeParts := strings.Split(incomeValue, "@")
				if len(incomeParts) < 6 {
					continue
				}

				// Extract data
				currCodeHash := incomeParts[0]
				currGenesis := incomeParts[1]
				currSensibleId := incomeParts[2]
				currCustomData := incomeParts[3]
				currIndex := incomeParts[4]
				currValue := incomeParts[5]

				// Apply filter if specified
				if codeHash != "" && codeHash != currCodeHash {
					continue
				}
				if genesis != "" && genesis != currGenesis {
					continue
				}

				// Extract TxID from outpoint
				outpointParts := strings.Split(outpoint, ":")
				if len(outpointParts) != 2 {
					continue
				}
				currTxID := outpointParts[0]

				// Check if already spent
				spentKey := currTxID + "@" + currIndex
				if _, exists := spentUtxos[spentKey]; exists {
					continue
				}

				// Create unique key
				uniqueKey := currCodeHash + "@" + currGenesis + "@" + currTxID + "@" + currIndex

				// If already exists, skip
				if _, exists := uniqueUtxos[uniqueKey]; exists {
					continue
				}

				// Parse index
				index, err := strconv.ParseInt(currIndex, 10, 64)
				if err != nil {
					continue
				}

				// Add to unique UTXO list (mempool UTXO has height = -1)
				uniqueUtxos[uniqueKey] = &UniqueFtUtxo{
					Txid:          currTxID,
					TxIndex:       index,
					CodeHash:      currCodeHash,
					Genesis:       currGenesis,
					SensibleId:    currSensibleId,
					Height:        -1, // Mempool UTXO
					CustomData:    currCustomData,
					Satoshi:       currValue,
					SatoshiString: currValue,
				}
			}
		}
	}

	// Convert map to slice
	for _, utxo := range uniqueUtxos {
		utxos = append(utxos, utxo)
	}

	return utxos, nil
}

// GetMempoolAddressFtSpendMap gets FT spend data for address in mempool
func (i *ContractFtIndexer) GetMempoolAddressFtSpendMap(address string) (map[string]string, error) {
	if i.mempoolMgr == nil {
		return nil, fmt.Errorf("Mempool manager not set")
	}
	return i.mempoolMgr.GetMempoolAddressFtSpendMap(address)
}

// GetMempoolUniqueFtSpendMap gets unique FT spend data in mempool
func (i *ContractFtIndexer) GetMempoolUniqueFtSpendMap(codeHashGenesis string) (map[string]string, error) {
	if i.mempoolMgr == nil {
		return nil, fmt.Errorf("Mempool manager not set")
	}
	return i.mempoolMgr.GetMempoolUniqueFtSpendMap(codeHashGenesis)
}

// GetMempoolUniqueFtIncomeMap gets unique FT income data in mempool
func (i *ContractFtIndexer) GetMempoolUniqueFtIncomeMap(codeHashGenesis string) (map[string]string, error) {
	if i.mempoolMgr == nil {
		return nil, fmt.Errorf("Mempool manager not set")
	}
	return i.mempoolMgr.GetMempoolUniqueFtIncomeMap(codeHashGenesis)
}

// GetFtSummary gets all FT information with cursor-based pagination
func (i *ContractFtIndexer) GetFtSummary(cursor, size int) ([]*FtInfo, string, int, error) {
	var ftInfos []*FtInfo
	var nextCursor string

	// If size is not specified or invalid, use default
	if size <= 0 {
		size = 10
	}

	// Collect all FT info keys first for sorting
	var allKeys []string
	for _, db := range i.contractFtInfoStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return nil, "", 0, fmt.Errorf("Failed to create iterator: %w", err)
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
		value, err := i.contractFtInfoStore.Get([]byte(key))
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

	// 按 sensibleId 排序后采用整型 offset 分页
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

	total := len(items)
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
		nextCursor = strconv.Itoa(endIndex)
	}

	for idx := startIndex; idx < endIndex; idx++ {
		key := items[idx].key
		value := items[idx].value

		parts := strings.Split(value, "@")
		if len(parts) < 4 {
			continue
		}
		decimal, err := strconv.ParseUint(parts[3], 10, 8)
		if err != nil {
			continue
		}
		keyParts := strings.Split(key, "@")
		if len(keyParts) < 2 {
			continue
		}
		ftInfo := &FtInfo{
			CodeHash:   keyParts[0],
			Genesis:    keyParts[1],
			SensibleId: parts[0],
			Name:       parts[1],
			Symbol:     parts[2],
			Decimal:    uint8(decimal),
		}
		ftInfos = append(ftInfos, ftInfo)
	}

	return ftInfos, nextCursor, total, nil
}

// GetFtGenesis gets FT information by codeHash and genesis
func (i *ContractFtIndexer) GetFtGenesis(codeHash, genesis string) (*FtGenesisInfo, error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	// Build query key
	key := codeHash + "@" + genesis

	// Get FT information
	ftInfo, err := i.GetFtInfo(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to get FT genesis info: %w", err)
	}
	ftGenesisInfo := &FtGenesisInfo{
		CodeHash:   ftInfo.CodeHash,
		Genesis:    ftInfo.Genesis,
		SensibleId: ftInfo.SensibleId,
		Name:       ftInfo.Name,
		Symbol:     ftInfo.Symbol,
		Decimal:    ftInfo.Decimal,
	}

	// Parse sensibleId to get genesisTxId and index
	genesisTxId, genesisIndex, err := decoder.ParseSensibleId(ftInfo.SensibleId)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse sensibleId: %w", err)
	}

	// Build outpoint
	outpoint := genesisTxId + ":" + strconv.Itoa(int(genesisIndex))

	// key:usedOutpoint, value: sensibleId@name@symbol@decimal@codeHash@genesis@amount@txId@index@value,...
	// Get genesis output list from contractFtGenesisOutputStore
	genesisOutputs, err := i.contractFtGenesisOutputStore.Get([]byte(outpoint))
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("Failed to get genesis outputs: %w", err)
	}

	// Parse genesis outputs: sensibleId@name@symbol@decimal@codeHash@genesis@amount@txId@index@value,...
	outputParts := strings.Split(string(genesisOutputs), ",")

	issusCodeHash := ""
	issusGenesis := ""

	// Count matching outputs
	for _, output := range outputParts {
		if output == "" {
			continue
		}

		parts := strings.Split(output, "@")
		if len(parts) < 10 {
			continue
		}

		// Check if this output matches our codeHash and genesis
		outputCodeHash := parts[4]
		outputGenesis := parts[5]
		amount := parts[6]
		if amount == "0" || amount == "" {
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
		spendData, _, err := i.addressFtSpendStore.GetWithShard([]byte(issueAddress))
		if err == nil {
			for _, spendValue := range strings.Split(string(spendData), ",") {
				if spendValue == "" {
					continue
				}
				// spendValue: txid@index@codeHash@genesis@sensibleId@amount@value@height@usedTxId
				spendValueStrs := strings.Split(spendValue, "@")
				if len(spendValueStrs) != 9 {
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
		incomeData, _, err := i.addressFtIncomeStore.GetWithShard([]byte(issueAddress))
		if err == nil {
			for _, incomeValue := range strings.Split(string(incomeData), ",") {
				if incomeValue == "" {
					continue
				}
				// incomeValue: CodeHash@Genesis@Amount@TxID@Index@Value@height
				incomeValueStrs := strings.Split(incomeValue, "@")
				if len(incomeValueStrs) < 7 {
					continue
				}

				// Check if this income matches our issue codeHash and genesis
				if incomeValueStrs[0] == issusCodeHash && incomeValueStrs[1] == issusGenesis {
					outpoint := incomeValueStrs[3] + ":" + incomeValueStrs[4]

					// Check if this UTXO is not spent
					if _, isSpent := spendMap[outpoint]; !isSpent {
						// This is an unspent UTXO for the issue
						// Parse the data and add to ftGenesisInfo
						txIndex, err := strconv.ParseInt(incomeValueStrs[4], 10, 64)
						if err != nil {
							continue
						}
						height, err := strconv.ParseInt(incomeValueStrs[6], 10, 64)
						if err != nil {
							continue
						}

						ftGenesisInfo.Txid = incomeValueStrs[3]
						ftGenesisInfo.TxIndex = txIndex
						ftGenesisInfo.ValueString = incomeValueStrs[2]
						ftGenesisInfo.SatoshiString = incomeValueStrs[5]
						ftGenesisInfo.Height = height
						break // Take the first unspent UTXO
					}
				}
			}
		}
	}

	return ftGenesisInfo, nil
}

func (i *ContractFtIndexer) GetFtSupply(codeHash, genesis string) (*FtSupplyInfo, error) {
	if codeHash == "" || genesis == "" {
		return nil, fmt.Errorf("codeHash and genesis parameters are required")
	}

	// Build query key
	key := codeHash + "@" + genesis

	// Get FT information
	ftInfo, err := i.GetFtInfo(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to get FT info: %w", err)
	}

	// Parse sensibleId to get genesisTxId and index
	genesisTxId, genesisIndex, err := decoder.ParseSensibleId(ftInfo.SensibleId)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse sensibleId: %w", err)
	}

	// Build outpoint
	outpoint := genesisTxId + ":" + strconv.Itoa(int(genesisIndex))

	// key:usedOutpoint, value: sensibleId@name@symbol@decimal@codeHash@genesis@amount@txId@index@value,...
	// Get genesis output list from contractFtGenesisOutputStore
	genesisOutputs, err := i.contractFtGenesisOutputStore.Get([]byte(outpoint))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// Return empty supply info if not found
			return &FtSupplyInfo{
				Confirmed:           "0",
				Unconfirmed:         "0",
				AllowIncreaseIssues: false,
				MaxSupply:           "0",
			}, nil
		}
		return nil, fmt.Errorf("Failed to get genesis outputs: %w", err)
	}

	// Parse genesis outputs: sensibleId@name@symbol@decimal@codeHash@genesis@amount@txId@index@value,...
	outputParts := strings.Split(string(genesisOutputs), ",")

	var maxSupply string = "0"
	var allowIncreaseIssues bool = false

	if len(outputParts) > 1 {
		allowIncreaseIssues = true
	}

	// Count matching outputs
	for _, output := range outputParts {
		if output == "" {
			continue
		}

		parts := strings.Split(output, "@")
		if len(parts) < 10 {
			continue
		}

		// Check if this output matches our codeHash and genesis
		outputCodeHash := parts[4]
		outputGenesis := parts[5]

		if outputCodeHash == codeHash && outputGenesis == genesis {
			// The amount is the maxSupply
			maxSupply = parts[6]
		}
	}

	// Calculate total supply and burn
	var totalSupply int64 = 0
	var totalBurn int64 = 0

	// Map to track processed txId@index for deduplication
	processedSupply := make(map[string]struct{})
	processedBurn := make(map[string]struct{})

	// Get supply data from contractFtSupplyStore
	supplyData, err := i.contractFtSupplyStore.Get([]byte(key))
	if err == nil {
		// Parse supply data: sensibleId@name@symbol@decimal@codeHash@genesis@amount@txId@index@value,...
		supplyParts := strings.Split(string(supplyData), ",")
		for _, supplyPart := range supplyParts {
			if supplyPart == "" {
				continue
			}
			parts := strings.Split(supplyPart, "@")
			if len(parts) < 10 {
				continue
			}
			// Check if this supply matches our codeHash and genesis
			if parts[4] == codeHash && parts[5] == genesis {
				// Create unique key for deduplication: txId@index
				txId := parts[7]
				index := parts[8]
				uniqueKey := txId + "@" + index

				// Skip if already processed
				if _, exists := processedSupply[uniqueKey]; exists {
					continue
				}
				processedSupply[uniqueKey] = struct{}{}

				amount, err := strconv.ParseInt(parts[6], 10, 64)
				if err == nil {
					totalSupply += amount
				}
			}
		}
	}

	// Get burn data from contractFtBurnStore
	burnData, err := i.contractFtBurnStore.Get([]byte(key))
	if err == nil {
		// Parse burn data: sensibleId@name@symbol@decimal@codeHash@genesis@amount@txId@index@value,...
		burnParts := strings.Split(string(burnData), ",")
		for _, burnPart := range burnParts {
			if burnPart == "" {
				continue
			}
			parts := strings.Split(burnPart, "@")
			if len(parts) < 10 {
				continue
			}
			// Check if this burn matches our codeHash and genesis
			if parts[4] == codeHash && parts[5] == genesis {
				// Create unique key for deduplication: txId@index
				txId := parts[7]
				index := parts[8]
				uniqueKey := txId + "@" + index

				// Skip if already processed
				if _, exists := processedBurn[uniqueKey]; exists {
					continue
				}
				processedBurn[uniqueKey] = struct{}{}

				amount, err := strconv.ParseInt(parts[6], 10, 64)
				if err == nil {
					totalBurn += amount
				}
			}
		}
	}

	// Calculate current supply: total supply - total burn
	currentSupply := totalSupply - totalBurn
	if currentSupply < 0 {
		currentSupply = 0
	}

	confirmedSupply := strconv.FormatInt(currentSupply, 10)

	if allowIncreaseIssues {
		maxSupply = ""
	}

	return &FtSupplyInfo{
		Confirmed:           confirmedSupply,
		Unconfirmed:         "0", // TODO: Calculate unconfirmed supply from mempool
		AllowIncreaseIssues: allowIncreaseIssues,
		MaxSupply:           maxSupply,
	}, nil
}

// GetFtOwners gets FT owners list by codeHash and genesis with cursor-based pagination
func (i *ContractFtIndexer) GetFtOwners(codeHash, genesis string, cursor int, size int) (*FtOwnerInfo, error) {
	if codeHash == "" || genesis == "" {
		return &FtOwnerInfo{
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

	sensibleId := ""
	name := ""
	symbol := ""
	decimal := uint8(0)
	ftInfo, _ := i.GetFtInfo(key)
	if ftInfo != nil {
		sensibleId = ftInfo.SensibleId
		name = ftInfo.Name
		symbol = ftInfo.Symbol
		decimal = ftInfo.Decimal
	}

	// Map to store address balances
	ownerBalances := make(map[string]int64)
	// Map to track processed txId:index pairs for deduplication
	processedIncome := make(map[string]struct{})
	processedSpend := make(map[string]struct{})

	// Get income data from contractFtOwnersIncomeStore
	incomeData, err := i.contractFtOwnersIncomeStore.Get([]byte(key))
	if err == nil {
		// Parse income data: address@amount@txId@index,...
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
			amount := parts[1]
			txId := parts[2]
			index := parts[3]

			// Create unique key for deduplication: txId:index
			uniqueKey := txId + ":" + index

			// Skip if already processed
			if _, exists := processedIncome[uniqueKey]; exists {
				continue
			}
			processedIncome[uniqueKey] = struct{}{}

			// Parse amount
			amountInt, err := strconv.ParseInt(amount, 10, 64)
			if err != nil {
				continue
			}

			// Add to balance
			ownerBalances[address] += amountInt
		}
	}

	// Get spend data from contractFtOwnersSpendStore
	spendData, err := i.contractFtOwnersSpendStore.Get([]byte(key))
	if err == nil {
		// Parse spend data: address@amount@txId@index,...
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
			amount := parts[1]
			txId := parts[2]
			index := parts[3]

			// Create unique key for deduplication: txId:index
			uniqueKey := txId + ":" + index

			// Skip if already processed
			if _, exists := processedSpend[uniqueKey]; exists {
				continue
			}
			processedSpend[uniqueKey] = struct{}{}

			// Parse amount
			amountInt, err := strconv.ParseInt(amount, 10, 64)
			if err != nil {
				continue
			}

			// Subtract from balance
			ownerBalances[address] -= amountInt
		}
	}

	// Convert map to slice and filter out zero balances
	var owners []*FtOwner
	for address, balance := range ownerBalances {
		if balance > 0 {
			owners = append(owners, &FtOwner{
				CodeHash:   codeHash,
				Genesis:    genesis,
				SensibleId: sensibleId,
				Name:       name,
				Symbol:     symbol,
				Decimal:    decimal,
				Address:    address,
				Balance:    strconv.FormatInt(balance, 10),
			})
		}
	}

	// Sort by balance in descending order
	sort.Slice(owners, func(i, j int) bool {
		balanceI, _ := strconv.ParseInt(owners[i].Balance, 10, 64)
		balanceJ, _ := strconv.ParseInt(owners[j].Balance, 10, 64)
		return balanceI > balanceJ
	})

	// Get total count
	total := len(owners)

	// Apply cursor-based pagination (cursor is offset)
	var nextCursor int
	var paginatedOwners []*FtOwner

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
		paginatedOwners = []*FtOwner{}
	}

	// Set next cursor if there are more items
	if endIndex < len(owners) {
		nextCursor = endIndex
	} else {
		nextCursor = 0 // No more items
	}

	ownerInfo := &FtOwnerInfo{
		Total:      total,
		List:       paginatedOwners,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}

	return ownerInfo, nil
}

// GetFtAddressHistory gets FT address history by address, codeHash and genesis with cursor-based pagination
func (i *ContractFtIndexer) GetFtAddressHistory(address, codeHash, genesis string, cursor int, size int) (*FtAddressHistory, error) {
	if address == "" {
		return &FtAddressHistory{
			Total:      0,
			List:       nil,
			Cursor:     cursor,
			NextCursor: 0,
			Size:       size,
		}, fmt.Errorf("address parameter is required")
	}
	if codeHash == "" || genesis == "0" {
		return &FtAddressHistory{
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
	if size > 100 {
		size = 100
	}

	// If cursor is negative, set to 0
	if cursor < 0 {
		cursor = 0
	}

	// get UTXO changes from mempool (used for merging with the bottom database and then paginating/counting)
	var memIncomeList, memSpendList []common.FtUtxo
	if i.mempoolMgr != nil {
		memIncomeList, memSpendList, _ = i.mempoolMgr.GetFtUTXOsByAddress(address, codeHash, genesis)
	}

	// Get history data from contractFtAddressHistoryStore
	var historyParts []string
	historyData, err := i.contractFtAddressHistoryStore.Get([]byte(address))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("Failed to get address history: %w", err)
		}
	} else {
		// Parse history data: txId@time@income/outcome,...
		historyParts = strings.Split(string(historyData), ",")
	}

	// Map to store unique transactions by txId and type (income/outcome) for deduplication
	// Key: txId@income or txId@outcome, Value: txId, time, txType
	type TxRecord struct {
		TxId        string
		Time        int64
		TxType      string // "income" or "outcome"
		BlockHeight int64
	}
	uniqueTxMap := make(map[string]*TxRecord)

	for _, historyPart := range historyParts {
		if historyPart == "" {
			continue
		}
		parts := strings.Split(historyPart, "@")
		if len(parts) < 3 {
			continue
		}

		txId := parts[0]
		timeStr := parts[1]
		txType := parts[2] // "income" or "outcome"
		blockHeight := int64(0)
		if len(parts) > 3 {
			blockHeightStr := parts[3]
			blockHeight, _ = strconv.ParseInt(blockHeightStr, 10, 64)
		}
		// Parse time
		timeInt, err := strconv.ParseInt(timeStr, 10, 64)
		if err != nil {
			continue
		}

		// Create unique key for deduplication: txId@income or txId@outcome
		uniqueKey := txId + "@" + txType

		// Skip if already processed
		if _, exists := uniqueTxMap[uniqueKey]; exists {
			continue
		}

		uniqueTxMap[uniqueKey] = &TxRecord{
			TxId:        txId,
			Time:        timeInt,
			TxType:      txType,
			BlockHeight: blockHeight,
		}
	}

	// Convert to FtAddressTx temporarily (without amounts) and merge by txId
	// Key: txId, Value: *FtAddressTx (may have both income and outcome)
	txMap := make(map[string]*FtAddressTx)

	for _, record := range uniqueTxMap {
		txId := record.TxId
		txType := record.TxType

		// Get or create FtAddressTx for this txId
		tx, exists := txMap[txId]
		if !exists {
			tx = &FtAddressTx{
				Address:       address,
				TxId:          txId,
				Time:          record.Time,
				BlockHeight:   record.BlockHeight,
				IsIncome:      false,
				IsOutcome:     false,
				IncomeAmount:  "0",
				OutcomeAmount: "0",
			}
			txMap[txId] = tx
		}

		// Update time if this record has a newer time (use the latest time)
		if record.Time > tx.Time {
			tx.Time = record.Time
		}

		// Set flags based on txType
		if txType == "income" {
			tx.IsIncome = true
		} else if txType == "outcome" {
			tx.IsOutcome = true
		}
	}

	// Merge mempool tx flags BEFORE sorting/pagination so we can paginate combined set
	if len(memIncomeList) > 0 || len(memSpendList) > 0 {
		for _, utxo := range memIncomeList {
			txId := utxo.TxID
			if _, exists := txMap[txId]; !exists {
				txMap[txId] = &FtAddressTx{Address: address, TxId: txId, Time: utxo.Timestamp, BlockHeight: -1}
			}
			tx := txMap[txId]
			tx.IsIncome = true
		}
		for _, utxo := range memSpendList {
			txId := utxo.TxID
			if _, exists := txMap[txId]; !exists {
				txMap[txId] = &FtAddressTx{Address: address, TxId: txId, Time: utxo.Timestamp, BlockHeight: -1}
			}
			tx := txMap[txId]
			tx.IsOutcome = true
		}
	}

	// Convert map to slice for sorting
	var txList []*FtAddressTx
	for _, tx := range txMap {
		txList = append(txList, tx)
	}

	// Sort by time in descending order (newest first), then by txId for same time
	sort.Slice(txList, func(i, j int) bool {
		if txList[i].Time != txList[j].Time {
			return txList[i].Time > txList[j].Time
		}
		return txList[i].TxId < txList[j].TxId
	})

	// Apply cursor-based pagination BEFORE calculating amounts
	total := len(txList)
	startIndex := cursor
	if startIndex > total {
		startIndex = total
	}

	endIndex := startIndex + size
	if endIndex > total {
		endIndex = total
	}

	var paginatedList []*FtAddressTx
	if startIndex < total {
		paginatedList = txList[startIndex:endIndex]
	} else {
		paginatedList = []*FtAddressTx{}
	}

	// Now calculate income and outcome amounts from contractFtUtxoStore and addressFtSpendStore
	// Get FT info cache for codeHash@genesis
	ftInfoCache := make(map[string]*FtInfo)

	// Pre-load spend data once for better performance
	// Format: key: FtAddress, value: txid@index@codeHash@genesis@sensibleId@amount@value@height@usedTxId,...
	spendData, _, _ := i.addressFtSpendStore.GetWithShard([]byte(address))

	// Build mempool grouped amounts once: txId -> ftKey -> amounts
	type memFtAmount struct{ IncomeAmount, OutcomeAmount int64 }
	memTxToFtAmounts := make(map[string]map[string]*memFtAmount)
	if len(memIncomeList) > 0 || len(memSpendList) > 0 {
		for _, utxo := range memIncomeList {
			txId := utxo.TxID
			ftKey := utxo.CodeHash + "@" + utxo.Genesis
			if _, ok := memTxToFtAmounts[txId]; !ok {
				memTxToFtAmounts[txId] = make(map[string]*memFtAmount)
			}
			if _, ok := memTxToFtAmounts[txId][ftKey]; !ok {
				memTxToFtAmounts[txId][ftKey] = &memFtAmount{}
			}
			amt, _ := strconv.ParseInt(utxo.Amount, 10, 64)
			memTxToFtAmounts[txId][ftKey].IncomeAmount += amt
		}
		for _, utxo := range memSpendList {
			txId := utxo.UsedTxId
			if txId == "" {
				continue
			}
			ftKey := utxo.CodeHash + "@" + utxo.Genesis
			if _, ok := memTxToFtAmounts[txId]; !ok {
				memTxToFtAmounts[txId] = make(map[string]*memFtAmount)
			}
			if _, ok := memTxToFtAmounts[txId][ftKey]; !ok {
				memTxToFtAmounts[txId][ftKey] = &memFtAmount{}
			}
			amt, _ := strconv.ParseInt(utxo.Amount, 10, 64)
			memTxToFtAmounts[txId][ftKey].OutcomeAmount += amt
		}
	}

	// Calculate amounts for each transaction
	var finalTxList []*FtAddressTx

	for _, tx := range paginatedList {
		txId := tx.TxId

		// For income, we need to group by codeHash@genesis since one txId may have multiple FTs
		// For outcome, we also need to group by codeHash@genesis
		type FtAmount struct {
			CodeHash, Genesis           string
			IncomeAmount, OutcomeAmount int64
		}
		ftAmountMap := make(map[string]*FtAmount) // key: codeHash@genesis

		// Calculate income amount from contractFtUtxoStore
		// Format: key: txID, value: FtAddress@CodeHash@Genesis@sensibleId@Amount@Index@Value@height@contractType,...
		if tx.IsIncome {
			utxoData, err := i.contractFtUtxoStore.Get([]byte(txId))
			if err == nil {
				utxoParts := strings.Split(string(utxoData), ",")
				for _, utxoPart := range utxoParts {
					if utxoPart == "" {
						continue
					}
					utxoStrs := strings.Split(utxoPart, "@")
					if len(utxoStrs) >= 5 {
						utxoAddress := utxoStrs[0]
						currCodeHash := utxoStrs[1]
						currGenesis := utxoStrs[2]

						// Check if matches address and filter
						if utxoAddress != address {
							continue
						}
						if codeHash != "" && codeHash != currCodeHash {
							continue
						}
						if genesis != "" && genesis != currGenesis {
							continue
						}

						// Group by codeHash@genesis and accumulate amount
						ftKey := currCodeHash + "@" + currGenesis
						if ftAmount, exists := ftAmountMap[ftKey]; exists {
							if len(utxoStrs) >= 5 {
								amount, err := strconv.ParseInt(utxoStrs[4], 10, 64)
								if err == nil {
									ftAmount.IncomeAmount += amount
								}
							}
						} else {
							amount := int64(0)
							if len(utxoStrs) >= 5 {
								amount, _ = strconv.ParseInt(utxoStrs[4], 10, 64)
							}
							ftAmountMap[ftKey] = &FtAmount{
								CodeHash:      currCodeHash,
								Genesis:       currGenesis,
								IncomeAmount:  amount,
								OutcomeAmount: 0,
							}
						}

						// Get FT info (cache it)
						if _, exists := ftInfoCache[ftKey]; !exists {
							ftInfo, err := i.GetFtInfo(ftKey)
							if err == nil {
								ftInfoCache[ftKey] = ftInfo
							}
						}
					}
				}
			}
		}

		// Calculate outcome amount from addressFtSpendStore
		// Format: key: FtAddress, value: txid@index@codeHash@genesis@sensibleId@amount@value@height@usedTxId,...
		if tx.IsOutcome && spendData != nil {
			spendParts := strings.Split(string(spendData), ",")
			for _, spendPart := range spendParts {
				if spendPart == "" {
					continue
				}
				spendStrs := strings.Split(spendPart, "@")
				if len(spendStrs) >= 9 {
					usedTxId := spendStrs[8]

					// Check if usedTxId matches current txId
					if usedTxId != txId {
						continue
					}

					currCodeHash := spendStrs[2]
					currGenesis := spendStrs[3]

					// Apply codeHash and genesis filter if provided
					if codeHash != "" && codeHash != currCodeHash {
						continue
					}
					if genesis != "" && genesis != currGenesis {
						continue
					}

					// Group by codeHash@genesis and accumulate amount
					ftKey := currCodeHash + "@" + currGenesis
					if ftAmount, exists := ftAmountMap[ftKey]; exists {
						if len(spendStrs) >= 6 {
							amount, err := strconv.ParseInt(spendStrs[5], 10, 64)
							if err == nil {
								ftAmount.OutcomeAmount += amount
							}
						}
					} else {
						amount := int64(0)
						if len(spendStrs) >= 6 {
							amount, _ = strconv.ParseInt(spendStrs[5], 10, 64)
						}
						ftAmountMap[ftKey] = &FtAmount{
							CodeHash:      currCodeHash,
							Genesis:       currGenesis,
							IncomeAmount:  0,
							OutcomeAmount: amount,
						}
					}

					// Get FT info (cache it)
					if _, exists := ftInfoCache[ftKey]; !exists {
						ftInfo, err := i.GetFtInfo(ftKey)
						if err == nil {
							ftInfoCache[ftKey] = ftInfo
						}
					}
				}
			}
		}

		// Merge mempool grouped amounts for this txId
		if m, ok := memTxToFtAmounts[txId]; ok {
			for ftKey, amounts := range m {
				if (codeHash != "" && !strings.HasPrefix(ftKey, codeHash+"@")) || (genesis != "" && !strings.HasSuffix(ftKey, "@"+genesis)) {
					continue
				}
				if _, exists := ftAmountMap[ftKey]; !exists {
					// split key to fill CodeHash/Genesis
					parts := strings.Split(ftKey, "@")
					if len(parts) >= 2 {
						ftAmountMap[ftKey] = &FtAmount{CodeHash: parts[0], Genesis: parts[1]}
					} else {
						ftAmountMap[ftKey] = &FtAmount{}
					}
				}
				ftAmountMap[ftKey].IncomeAmount += amounts.IncomeAmount
				ftAmountMap[ftKey].OutcomeAmount += amounts.OutcomeAmount
			}
		}

		// Create transaction records for each codeHash@genesis
		// If filter is provided but no match found, skip
		if len(ftAmountMap) == 0 {
			if codeHash != "" || genesis != "" {
				continue
			}
		}

		// Create records for each FT
		for ftKey, ftAmount := range ftAmountMap {
			// Get FT info
			var ftInfo *FtInfo
			if cached, exists := ftInfoCache[ftKey]; exists {
				ftInfo = cached
			} else {
				ftInfo, err = i.GetFtInfo(ftKey)
				if err == nil {
					ftInfoCache[ftKey] = ftInfo
				} else {
					ftInfo = &FtInfo{}
				}
			}

			// Create transaction record
			newTx := &FtAddressTx{
				CodeHash:      ftInfo.CodeHash,
				Genesis:       ftInfo.Genesis,
				SensibleId:    ftInfo.SensibleId,
				Name:          ftInfo.Name,
				Symbol:        ftInfo.Symbol,
				Decimal:       ftInfo.Decimal,
				Address:       address,
				TxId:          txId,
				Time:          tx.Time,
				BlockHeight:   tx.BlockHeight,
				IsIncome:      tx.IsIncome,
				IsOutcome:     tx.IsOutcome,
				IncomeAmount:  strconv.FormatInt(ftAmount.IncomeAmount, 10),
				OutcomeAmount: strconv.FormatInt(ftAmount.OutcomeAmount, 10),
			}

			finalTxList = append(finalTxList, newTx)
		}
	}

	nextCursor := 0
	if endIndex < total {
		nextCursor = endIndex
	}

	return &FtAddressHistory{
		Total:      total,
		List:       finalTxList,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, nil
}

// GetFtGenesisHistory gets FT genesis history by codeHash and genesis with cursor-based pagination
func (i *ContractFtIndexer) GetFtGenesisHistory(codeHash, genesis string, cursor int, size int) (*FtGenesisHistory, error) {
	if codeHash == "" || genesis == "" {
		return &FtGenesisHistory{
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

	// Get history data from contractFtGenesisHistoryStore
	historyData, err := i.contractFtGenesisHistoryStore.Get([]byte(key))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return &FtGenesisHistory{
				Total:      0,
				List:       []*FtGenesisTx{},
				Cursor:     cursor,
				NextCursor: 0,
				Size:       size,
			}, nil
		}
		return nil, fmt.Errorf("Failed to get genesis history: %w", err)
	}

	// Get FT info
	ftInfo, err := i.GetFtInfo(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to get FT info: %w", err)
	}

	// Parse history data: txId@time@income/outcome,...
	historyParts := strings.Split(string(historyData), ",")

	// Map to store unique transactions by txId and type (income/outcome)
	uniqueTxMap := make(map[string]*FtGenesisTx)

	for _, historyPart := range historyParts {
		if historyPart == "" {
			continue
		}
		parts := strings.Split(historyPart, "@")
		if len(parts) < 3 {
			continue
		}

		txId := parts[0]
		txType := parts[2] // "income" or "outcome"

		// Create unique key for deduplication: txId@income or txId@outcome
		uniqueKey := txId + "@" + txType

		// Skip if already processed
		if _, exists := uniqueTxMap[uniqueKey]; exists {
			continue
		}

		// Create transaction record
		tx := &FtGenesisTx{
			CodeHash:   codeHash,
			Genesis:    genesis,
			SensibleId: ftInfo.SensibleId,
			TxId:       txId,
		}

		uniqueTxMap[uniqueKey] = tx
	}

	// Convert map to slice
	var txList []*FtGenesisTx
	for _, tx := range uniqueTxMap {
		txList = append(txList, tx)
	}

	// Sort by txId (you may want to sort by time if available, but we don't have time in FtGenesisTx)
	sort.Slice(txList, func(i, j int) bool {
		return txList[i].TxId > txList[j].TxId
	})

	// Apply cursor-based pagination
	total := len(txList)
	startIndex := cursor
	if startIndex > total {
		startIndex = total
	}

	endIndex := startIndex + size
	if endIndex > total {
		endIndex = total
	}

	var paginatedList []*FtGenesisTx
	if startIndex < total {
		paginatedList = txList[startIndex:endIndex]
	} else {
		paginatedList = []*FtGenesisTx{}
	}

	nextCursor := 0
	if endIndex < total {
		nextCursor = endIndex
	}

	return &FtGenesisHistory{
		Total:      total,
		List:       paginatedList,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, nil
}

// GetFtSupplyList 从 contractFtSupplyStore 获取增发列表（可选 codeHash/genesis 过滤，cursor/size 分页）
func (i *ContractFtIndexer) GetFtSupplyList(codeHash, genesis string, cursor, size int) (*FtSupplyList, error) {
	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}

	// 先收集排序键与原始记录，稍后再分页解析
	type supplyRecord struct {
		key string // 用于排序：codeHash@genesis@txId@index
		rec string // 原始记录片段（sensibleId@name@...@value）
	}
	var records []supplyRecord
	// Map to track processed txId@index for deduplication
	processedSupply := make(map[string]struct{})

	appendFromValue := func(k string, val string) {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			if p == "" {
				continue
			}
			arr := strings.Split(p, "@")
			if len(arr) < 10 {
				continue
			}
			// 过滤（基于字段位置）
			if (codeHash != "" && arr[4] != codeHash) || (genesis != "" && arr[5] != genesis) {
				continue
			}
			// Create unique key for deduplication: txId@index
			txId := arr[7]
			index := arr[8]
			uniqueKey := txId + "@" + index

			// Skip if already processed
			if _, exists := processedSupply[uniqueKey]; exists {
				continue
			}
			processedSupply[uniqueKey] = struct{}{}

			records = append(records, supplyRecord{
				key: arr[4] + "@" + arr[5] + "@" + arr[7] + "@" + arr[8],
				rec: p,
			})
		}
	}

	if codeHash != "" && genesis != "" {
		key := codeHash + "@" + genesis
		if v, err := i.contractFtSupplyStore.Get([]byte(key)); err == nil {
			appendFromValue(key, string(v))
		} else if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	} else {
		for _, db := range i.contractFtSupplyStore.GetShards() {
			iter, err := db.NewIter(&pebble.IterOptions{})
			if err != nil {
				return nil, fmt.Errorf("Failed to create iterator: %w", err)
			}
			for iter.First(); iter.Valid(); iter.Next() {
				k := string(iter.Key())
				v := string(iter.Value())
				// 如果提供了部分过滤，先用键快速过滤
				if codeHash != "" || genesis != "" {
					kp := strings.Split(k, "@")
					if len(kp) >= 2 {
						if codeHash != "" && kp[0] != codeHash {
							continue
						}
						if genesis != "" && kp[1] != genesis {
							continue
						}
					}
				}
				appendFromValue(k, v)
			}
			iter.Close()
		}
	}

	// 排序
	sort.Slice(records, func(i2, j int) bool { return records[i2].key < records[j].key })

	// 统计去重后的总数
	total := len(records)

	// 按 offset 分页解析
	if cursor < 0 {
		cursor = 0
	}
	start := cursor
	if start > len(records) {
		start = len(records)
	}
	end := start + size
	if end > len(records) {
		end = len(records)
	}
	nextCursor := 0
	if end < len(records) {
		nextCursor = end
	}

	var list []*FtSupplyEntry
	for _, r := range records[start:end] {
		arr := strings.Split(r.rec, "@")
		if len(arr) < 10 {
			continue
		}
		decU, err := strconv.ParseUint(arr[3], 10, 8)
		if err != nil {
			continue
		}
		list = append(list, &FtSupplyEntry{
			SensibleId: arr[0],
			Name:       arr[1],
			Symbol:     arr[2],
			Decimal:    uint8(decU),
			CodeHash:   arr[4],
			Genesis:    arr[5],
			Amount:     arr[6],
			TxId:       arr[7],
			Index:      arr[8],
			Value:      arr[9],
		})
	}

	return &FtSupplyList{
		Total:      total,
		List:       list,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, nil
}

// GetFtBurnList 从 contractFtBurnStore 获取销毁列表（可选 codeHash/genesis 过滤，cursor/size 分页）
func (i *ContractFtIndexer) GetFtBurnList(codeHash, genesis string, cursor, size int) (*FtBurnList, error) {
	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}

	// 先收集排序键与原始记录，稍后再分页解析
	type burnRecord struct {
		key string
		rec string
	}
	var records []burnRecord
	// Map to track processed txId@index for deduplication
	processedBurn := make(map[string]struct{})

	appendFromValue := func(k string, val string) {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			if p == "" {
				continue
			}
			arr := strings.Split(p, "@")
			if len(arr) < 10 {
				continue
			}
			// 过滤
			if (codeHash != "" && arr[4] != codeHash) || (genesis != "" && arr[5] != genesis) {
				continue
			}
			// Create unique key for deduplication: txId@index
			txId := arr[7]
			index := arr[8]
			uniqueKey := txId + "@" + index

			// Skip if already processed
			if _, exists := processedBurn[uniqueKey]; exists {
				continue
			}
			processedBurn[uniqueKey] = struct{}{}

			records = append(records, burnRecord{
				key: arr[4] + "@" + arr[5] + "@" + arr[7] + "@" + arr[8],
				rec: p,
			})
		}
	}

	if codeHash != "" && genesis != "" {
		key := codeHash + "@" + genesis
		if v, err := i.contractFtBurnStore.Get([]byte(key)); err == nil {
			appendFromValue(key, string(v))
		} else if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	} else {
		for _, db := range i.contractFtBurnStore.GetShards() {
			iter, err := db.NewIter(&pebble.IterOptions{})
			if err != nil {
				return nil, fmt.Errorf("Failed to create iterator: %w", err)
			}
			for iter.First(); iter.Valid(); iter.Next() {
				k := string(iter.Key())
				v := string(iter.Value())
				// 如果提供了部分过滤，先用键快速过滤
				if codeHash != "" || genesis != "" {
					kp := strings.Split(k, "@")
					if len(kp) >= 2 {
						if codeHash != "" && kp[0] != codeHash {
							continue
						}
						if genesis != "" && kp[1] != genesis {
							continue
						}
					}
				}
				appendFromValue(k, v)
			}
			iter.Close()
		}
	}

	// 排序
	sort.Slice(records, func(i2, j int) bool { return records[i2].key < records[j].key })

	// 统计去重后的总数
	total := len(records)

	// 按 offset 分页解析
	if cursor < 0 {
		cursor = 0
	}
	start := cursor
	if start > len(records) {
		start = len(records)
	}
	end := start + size
	if end > len(records) {
		end = len(records)
	}
	nextCursor := 0
	if end < len(records) {
		nextCursor = end
	}

	var list []*FtBurnEntry
	for _, r := range records[start:end] {
		arr := strings.Split(r.rec, "@")
		if len(arr) < 10 {
			continue
		}
		decU, err := strconv.ParseUint(arr[3], 10, 8)
		if err != nil {
			continue
		}
		list = append(list, &FtBurnEntry{
			SensibleId: arr[0],
			Name:       arr[1],
			Symbol:     arr[2],
			Decimal:    uint8(decU),
			CodeHash:   arr[4],
			Genesis:    arr[5],
			Amount:     arr[6],
			TxId:       arr[7],
			Index:      arr[8],
			Value:      arr[9],
		})
	}

	return &FtBurnList{
		Total:      total,
		List:       list,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, nil
}

// GetFtOwnerTxData 根据 codeHash、genesis 和 address 获取该地址在指定 FT 的收入和支出数据
func (i *ContractFtIndexer) GetFtOwnerTxData(codeHash, genesis, address string) (*FtOwnerTxData, error) {
	if codeHash == "" || genesis == "" || address == "" {
		return nil, fmt.Errorf("codeHash, genesis and address parameters are required")
	}

	// Build query key
	key := codeHash + "@" + genesis

	result := &FtOwnerTxData{
		CodeHash: codeHash,
		Genesis:  genesis,
		Address:  address,
		Income:   make([]*FtOwnerTxEntry, 0),
		Spend:    make([]*FtOwnerTxEntry, 0),
	}

	// Map to track processed txId:index pairs for deduplication
	processedIncome := make(map[string]struct{})
	processedSpend := make(map[string]struct{})

	// Get income data from contractFtOwnersIncomeStore
	incomeData, err := i.contractFtOwnersIncomeStore.Get([]byte(key))
	if err == nil {
		// Parse income data: address@amount@txId@index,...
		incomeParts := strings.Split(string(incomeData), ",")
		for _, incomePart := range incomeParts {
			if incomePart == "" {
				continue
			}
			parts := strings.Split(incomePart, "@")
			if len(parts) < 4 {
				continue
			}
			incomeAddress := parts[0]
			amount := parts[1]
			txId := parts[2]
			index := parts[3]

			// Filter by address
			if incomeAddress != address {
				continue
			}

			// Create unique key for deduplication: txId:index
			uniqueKey := txId + ":" + index

			// Skip if already processed
			if _, exists := processedIncome[uniqueKey]; exists {
				continue
			}
			processedIncome[uniqueKey] = struct{}{}

			result.Income = append(result.Income, &FtOwnerTxEntry{
				Address: incomeAddress,
				Amount:  amount,
				TxId:    txId,
				Index:   index,
			})
		}
	}

	// Get spend data from contractFtOwnersSpendStore
	spendData, err := i.contractFtOwnersSpendStore.Get([]byte(key))
	if err == nil {
		// Parse spend data: address@amount@txId@index,...
		spendParts := strings.Split(string(spendData), ",")
		for _, spendPart := range spendParts {
			if spendPart == "" {
				continue
			}
			parts := strings.Split(spendPart, "@")
			if len(parts) < 4 {
				continue
			}
			spendAddress := parts[0]
			amount := parts[1]
			txId := parts[2]
			index := parts[3]

			// Filter by address
			if spendAddress != address {
				continue
			}

			// Create unique key for deduplication: txId:index
			uniqueKey := txId + ":" + index

			// Skip if already processed
			if _, exists := processedSpend[uniqueKey]; exists {
				continue
			}
			processedSpend[uniqueKey] = struct{}{}

			result.Spend = append(result.Spend, &FtOwnerTxEntry{
				Address: spendAddress,
				Amount:  amount,
				TxId:    txId,
				Index:   index,
			})
		}
	}

	return result, nil
}

// GetDbAddressHistory 从 contractFtAddressHistoryStore 获取地址历史记录（db 查询方法）
// address: 必填
// codeHash, genesis: 可选，如果提供则进行过滤
// cursor, size: 分页参数
func (i *ContractFtIndexer) GetDbAddressHistory(address, codeHash, genesis string, cursor, size int) (*FtAddressHistoryDbList, error) {
	if address == "" {
		return &FtAddressHistoryDbList{
			Total:      0,
			List:       nil,
			Cursor:     cursor,
			NextCursor: 0,
			Size:       size,
		}, fmt.Errorf("address parameter is required")
	}

	// If size is not specified or invalid, use default
	if size <= 0 {
		size = 10
	}
	if size > 100 {
		size = 100
	}

	// If cursor is negative, set to 0
	if cursor < 0 {
		cursor = 0
	}

	// Get history data from contractFtAddressHistoryStore
	// Format: key: address, value: txId@time@income/outcome,...
	historyData, err := i.contractFtAddressHistoryStore.Get([]byte(address))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return &FtAddressHistoryDbList{
				Total:      0,
				List:       []*FtAddressHistoryDbEntry{},
				Cursor:     cursor,
				NextCursor: 0,
				Size:       size,
			}, nil
		}
		return nil, fmt.Errorf("Failed to get address history: %w", err)
	}

	// Parse history data: txId@time@income/outcome,...
	historyParts := strings.Split(string(historyData), ",")

	// If codeHash and genesis are provided, we need to filter by checking contractFtUtxoStore
	var filteredEntries []*FtAddressHistoryDbEntry
	processedMap := make(map[string]struct{}) // For deduplication: txId@txType

	for _, historyPart := range historyParts {
		if historyPart == "" {
			continue
		}
		parts := strings.Split(historyPart, "@")
		if len(parts) < 3 {
			continue
		}

		txId := parts[0]
		timeStr := parts[1]
		txType := parts[2] // "income" or "outcome"

		// Create unique key for deduplication: txId@txType
		uniqueKey := txId + "@" + txType
		if _, exists := processedMap[uniqueKey]; exists {
			continue
		}
		processedMap[uniqueKey] = struct{}{}

		// If codeHash and genesis are provided, filter by checking contractFtUtxoStore
		if codeHash != "" || genesis != "" {
			// For income: check contractFtUtxoStore (key: txId)
			// For outcome: check addressFtSpendStore -> contractFtUtxoStore via usedTxId
			match := false

			if txType == "income" {
				// Check contractFtUtxoStore for income transactions
				utxoData, err := i.contractFtUtxoStore.Get([]byte(txId))
				if err == nil {
					// Format: FtAddress@CodeHash@Genesis@sensibleId@Amount@Index@Value@height@contractType,...
					utxoParts := strings.Split(string(utxoData), ",")
					for _, utxoPart := range utxoParts {
						if utxoPart == "" {
							continue
						}
						utxoStrs := strings.Split(utxoPart, "@")
						if len(utxoStrs) >= 9 {
							utxoAddress := utxoStrs[0]
							utxoCodeHash := utxoStrs[1]
							utxoGenesis := utxoStrs[2]
							contractType := utxoStrs[8]

							// Check if matches address and codeHash/genesis filter
							if utxoAddress == address && contractType == "ft" {
								if codeHash != "" && utxoCodeHash != codeHash {
									continue
								}
								if genesis != "" && utxoGenesis != genesis {
									continue
								}
								match = true
								break
							}
						}
					}
				}
			} else if txType == "outcome" {
				// For outcome: check addressFtSpendStore for transactions where usedTxId matches txId
				spendData, _, err := i.addressFtSpendStore.GetWithShard([]byte(address))
				if err == nil {
					// Format: txid@index@codeHash@genesis@sensibleId@amount@value@height@usedTxId,...
					spendParts := strings.Split(string(spendData), ",")
					for _, spendPart := range spendParts {
						if spendPart == "" {
							continue
						}
						spendStrs := strings.Split(spendPart, "@")
						if len(spendStrs) >= 9 {
							usedTxId := spendStrs[8]

							// Check if usedTxId matches current txId (the transaction that spent this UTXO)
							if usedTxId == txId {
								spendCodeHash := spendStrs[2]
								spendGenesis := spendStrs[3]

								// Check codeHash and genesis filter
								if codeHash != "" && spendCodeHash != codeHash {
									continue
								}
								if genesis != "" && spendGenesis != genesis {
									continue
								}

								match = true
								break
							}
						}
					}
				}
			}

			// Skip if doesn't match filter
			if !match {
				continue
			}
		}

		// Parse time
		timeInt, err := strconv.ParseInt(timeStr, 10, 64)
		if err != nil {
			continue
		}

		filteredEntries = append(filteredEntries, &FtAddressHistoryDbEntry{
			TxId:   txId,
			Time:   timeInt,
			TxType: txType,
		})
	}

	// Sort by time in descending order (newest first)
	sort.Slice(filteredEntries, func(i, j int) bool {
		return filteredEntries[i].Time > filteredEntries[j].Time
	})

	// Apply cursor-based pagination
	total := len(filteredEntries)
	startIndex := cursor
	if startIndex > total {
		startIndex = total
	}

	endIndex := startIndex + size
	if endIndex > total {
		endIndex = total
	}

	nextCursor := 0
	if endIndex < total {
		nextCursor = endIndex
	}

	var paginatedList []*FtAddressHistoryDbEntry
	if startIndex < total {
		paginatedList = filteredEntries[startIndex:endIndex]
	} else {
		paginatedList = []*FtAddressHistoryDbEntry{}
	}

	return &FtAddressHistoryDbList{
		Total:      total,
		List:       paginatedList,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, nil
}
