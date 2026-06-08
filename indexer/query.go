package indexer

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/storage"
)

type Balance struct {
	ConfirmedBalanceSatoshi uint64  `json:"confirmed_balance_satoshi"`
	ConfirmedBalance        float64 `json:"confirmed_balance"`
	BalanceSatoshi          uint64  `json:"balance_satoshi"`
	Balance                 float64 `json:"balance"`
	UTXOCount               int64   `json:"confirmed_utxo_count"`
	MempoolIncome           int64   `json:"mempool_income_satoshi"`
	MempoolIncomeBTC        float64 `json:"mempool_income"`
	MempoolSpend            int64   `json:"mempool_spend_satoshi"`
	MempoolSpendBTC         float64 `json:"mempool_spend"`
	MempoolUTXOCount        int64   `json:"mempool_utxo_count"`
	UnsafeFeeSatoshi        int64   `json:"unsafe_fee_satoshi"`
	UnsafeFee               float64 `json:"unsafe_fee"`
}

func (i *UTXOIndexer) GetBalance(address string, dustThreshold int64) (balanceResult Balance, err error) {
	mempoolIncomeData := map[string]string(nil)
	mempoolSpendData := map[string]string(nil)
	if i.mempoolManager != nil {
		mempoolIncomeData, mempoolSpendData = i.mempoolManager.GetDataByAddress(address)
	}

	noMempoolActivity := len(mempoolIncomeData) == 0 && len(mempoolSpendData) == 0
	if i.balanceStore != nil && noMempoolActivity {
		row, rowErr := i.getAddressBalanceRow(address)
		if rowErr == nil {
			if i.isBalanceIndexReady() || row.Tracked {
				confirmedBalance := row.BalanceSatoshi
				balanceResult = Balance{
					ConfirmedBalanceSatoshi: uint64(confirmedBalance),
					ConfirmedBalance:        float64(confirmedBalance) / 1e8,
					BalanceSatoshi:          uint64(confirmedBalance),
					Balance:                 float64(confirmedBalance) / 1e8,
					UTXOCount:               row.UTXOCount,
				}
				return
			}
		} else if !errors.Is(rowErr, storage.ErrNotFound) {
			err = rowErr
			return
		}
	}

	balanceResult, err = i.getBalanceFromHistory(address, dustThreshold)
	if err != nil {
		return
	}
	if i.balanceStore != nil && noMempoolActivity && !i.isBalanceIndexReady() {
		if cacheErr := i.putTrackedAddressBalance(address, int64(balanceResult.ConfirmedBalanceSatoshi), balanceResult.UTXOCount); cacheErr != nil {
			log.Printf("[BalanceIndex] failed to cache confirmed balance row for %s: %v", address, cacheErr)
		}
	}
	return
}

func (i *UTXOIndexer) getBalanceFromHistory(address string, dustThreshold int64) (balanceResult Balance, err error) {
	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	var income int64
	var spend int64
	var mempoolIncome int64
	var mempoolSpend int64
	var mempoolUtxoCount int64
	var utxoCount int64
	var unsafeFee int64
	mempoolCheckTxMap := make(map[string]int64)

	spendData, _, err := i.spendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			arr := strings.Split(spendTx, "@")
			if len(arr) < 1 {
				continue
			}
			point := arr[0]
			spendMap[point] = struct{}{}
		}
	}

	// Get with shard info for debugging
	incomeMap := make(map[string]struct{})
	data, _, err := i.addressStore.GetWithShard(addrKey)
	if err == nil {
		parts := strings.Split(string(data), ",")
		for _, part := range parts {
			incomes := strings.Split(part, "@")
			if len(incomes) < 3 {
				continue
			}
			key := incomes[0] + ":" + incomes[1]
			if _, exists := incomeMap[key]; exists {
				continue
			}
			incomeMap[key] = struct{}{}

			in, err := strconv.ParseInt(incomes[2], 10, 64)
			if err != nil {
				continue
			}
			if _, exists := spendMap[key]; exists {
				spend += in
			} else {
				// 统计未花费的小额 UTXO
				if in < dustThreshold {
					unsafeFee += in
				}
				utxoCount += 1
			}
			income += in
			mempoolCheckTxMap[key] = in
		}
	}
	balance := income - spend
	// Convert to BTC unit (1 BTC = 100,000,000 satoshis)
	btcBalance := float64(balance) / 1e8

	// Check if mempool manager is available before using it
	var mempoolIncomeData, mempoolSpendData map[string]string
	var mempoolIncomeList []common.Utxo
	if i.mempoolManager != nil {
		mempoolIncomeData, mempoolSpendData = i.mempoolManager.GetDataByAddress(address)
		mempoolIncomeList = getUtxoFromMempoolIncomeMap(mempoolIncomeData)
	}
	//mempoolIncomeList, err := i.mempoolManager.GetUTXOsByAddress(address)

	for _, utxo := range mempoolIncomeList {
		in, err := strconv.ParseInt(utxo.Amount, 10, 64)
		if err != nil {
			continue
		}
		// Check if mempool income is already in confirmed UTXOs
		if _, exists := incomeMap[utxo.TxID]; exists {
			continue // If confirmed, skip
		}
		//检查内存池的收入是否在已花费的UTXO中
		if _, exists := spendMap[utxo.TxID]; exists {
			continue // 如果已花费，则跳过
		}
		// 如果是内存池的UTXO，则添加到结果中
		mempoolIncome += in // 统计内存池中未花费的小额 UTXO
		if in < dustThreshold {
			unsafeFee += in
		}
		mempoolCheckTxMap[utxo.TxID] = in
	}
	// Check if mempool is spent - only process if mempool manager is available
	//if len(mempoolCheckTxMap) > 0 {
	// var list []string
	// for txPoint := range mempoolCheckTxMap {
	// 	list = append(list, txPoint)
	// }
	// mempoolSpendMap, _ := i.mempoolManager.GetSpendUTXOs(list)
	var mempoolSpendMap map[string]struct{}
	if i.mempoolManager != nil && mempoolSpendData != nil {
		mempoolSpendMap = getUtxoFromMempoolSpendMap(mempoolSpendData)
	} else {
		mempoolSpendMap = make(map[string]struct{})
	}
	for txPoint := range mempoolSpendMap {
		// 检查内存池的花费是否已经在出块的花费中
		if _, exists := spendMap[txPoint]; exists {
			continue // 如果已花费，则跳过
		}
		if _, exists := mempoolCheckTxMap[txPoint]; exists {
			amount := mempoolCheckTxMap[txPoint]
			mempoolSpend += amount
			mempoolUtxoCount += 1
			// 如果花费的是小额 UTXO，从 unsafeFee 中减去
			if amount < dustThreshold {
				unsafeFee -= amount
			}
		}
	}
	//}
	lastBalance := balance + mempoolIncome - mempoolSpend
	balanceResult = Balance{
		ConfirmedBalanceSatoshi: uint64(balance),
		ConfirmedBalance:        btcBalance,
		Balance:                 float64(lastBalance) / 1e8,
		BalanceSatoshi:          uint64(lastBalance),
		UTXOCount:               utxoCount,
		MempoolIncome:           mempoolIncome,
		MempoolIncomeBTC:        float64(mempoolIncome) / 1e8,
		MempoolSpend:            mempoolSpend,
		MempoolSpendBTC:         float64(mempoolSpend) / 1e8,
		MempoolUTXOCount:        mempoolUtxoCount,
		UnsafeFeeSatoshi:        unsafeFee,
		UnsafeFee:               float64(unsafeFee) / 1e8,
	}
	// Clean up memory
	spendMap = nil
	mempoolCheckTxMap = nil
	incomeMap = nil
	return balanceResult, nil
}
func getUtxoFromMempoolIncomeMap(data map[string]string) (mempoolIncomeList []common.Utxo) {
	//eg: data: map[bcrt1q2mvt4fkmp94hd2tx9ruj8g7na53kp4mqrq7n3n_927ba3f10f7003b3bc023cc12d047ec76c0984a669523407af6760afa3153b06:1_1763563228:69999577]
	seen := make(map[string]struct{}, len(data))
	for k, v := range data {
		arr := strings.Split(k, "_")
		if len(arr) < 2 {
			continue
		}
		if _, exists := seen[arr[1]]; exists {
			continue
		}
		seen[arr[1]] = struct{}{}
		mempoolIncomeList = append(mempoolIncomeList, common.Utxo{
			TxID:    arr[1],
			Address: arr[0],
			Amount:  v,
		})
	}
	return
}
func getUtxoFromMempoolSpendMap(data map[string]string) (mempoolSpendMap map[string]struct{}) {
	//eg:map[bcrt1q2mvt4fkmp94hd2tx9ruj8g7na53kp4mqrq7n3n_0cbdc69518369cb83dcb22ceaac08cd2164cc1d2bb391b0eeda8fff375309153:1_1763563228:]
	mempoolSpendMap = make(map[string]struct{})
	for k := range data {
		arr := strings.Split(k, "_")
		if len(arr) < 2 {
			continue
		}
		mempoolSpendMap[arr[1]] = struct{}{}
	}
	return mempoolSpendMap
}
func (i *UTXOIndexer) GetUTXOs(address string) (result []UTXO, err error) {
	// 1. Get confirmed UTXOs
	addrKey := []byte(address)
	spendMap := make(map[string]struct{})
	incomeMap := make(map[string]struct{})
	mempoolCheckTxMap := make(map[string]int64)
	var utxos []UTXO
	var mempoolIncomeData, mempoolSpendData map[string]string
	// 2. Get mempool UTXOs
	if i.mempoolManager != nil {
		mempoolIncomeData, mempoolSpendData = i.mempoolManager.GetDataByAddress(address)
		mempoolIncomeList := getUtxoFromMempoolIncomeMap(mempoolIncomeData)
		//mempoolIncomeList, err := i.mempoolManager.GetUTXOsByAddress(address)
		if err == nil {
			for _, utxo := range mempoolIncomeList {
				txArray := strings.Split(utxo.TxID, ":")
				if len(txArray) < 2 {
					continue
				}
				amount, err := strconv.ParseInt(utxo.Amount, 10, 64)
				if err != nil {
					continue
				}
				utxos = append(utxos, UTXO{
					TxID:      txArray[0],
					Index:     txArray[1],
					Amount:    uint64(amount),
					IsMempool: true,
				})
				incomeMap[utxo.TxID] = struct{}{}
				mempoolCheckTxMap[utxo.TxID] = amount
			}

		}
	}

	data, _, _ := i.addressStore.GetWithShard(addrKey)
	// Get spent UTXOs
	spendData, _, err := i.spendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			arr := strings.Split(spendTx, "@")
			if len(arr) < 1 {
				continue
			}
			point := arr[0]
			spendMap[point] = struct{}{}
		}
	}
	// Process confirmed UTXOs
	if data != nil {
		parts := strings.Split(string(data), ",")
		for _, part := range parts {
			incomes := strings.Split(part, "@")
			if len(incomes) < 3 {
				continue
			}
			key := incomes[0] + ":" + incomes[1]
			if _, exists := incomeMap[key]; exists {
				continue
			}
			incomeMap[key] = struct{}{}

			in, err := strconv.ParseInt(incomes[2], 10, 64)
			if err != nil {
				continue
			}
			if _, exists := spendMap[key]; exists {
				continue
			}
			if in <= 1000 {
				continue
			}
			utxos = append(utxos, UTXO{
				TxID:      incomes[0],
				Index:     incomes[1],
				Amount:    uint64(in),
				IsMempool: false,
			})
			mempoolCheckTxMap[key] = in
		}
	}
	// Check if mempool is spent
	if len(mempoolCheckTxMap) > 0 {
		// var list []string
		// for txPoint := range mempoolCheckTxMap {
		// 	list = append(list, txPoint)
		// }
		// mempoolSpendMap, _ := i.mempoolManager.GetSpendUTXOs(list)
		var mempoolSpendMap map[string]struct{}
		if i.mempoolManager != nil && mempoolSpendData != nil {
			mempoolSpendMap = getUtxoFromMempoolSpendMap(mempoolSpendData)
		} else {
			mempoolSpendMap = make(map[string]struct{})
		}
		for txPoint := range mempoolSpendMap {
			if _, exists := mempoolCheckTxMap[txPoint]; exists {
				spendMap[txPoint] = struct{}{}
			}
		}

	}
	// Final filter
	for _, utxo := range utxos {
		if _, exists := spendMap[utxo.TxID+":"+utxo.Index]; exists {
			continue // If spent, skip
		}
		result = append(result, utxo)
	}
	// Clean up memory
	mempoolCheckTxMap = nil
	spendMap = nil
	incomeMap = nil
	return i.filterConfirmedUTXOsWithValidator(address, result)
}

func (i *UTXOIndexer) filterConfirmedUTXOsWithValidator(address string, utxos []UTXO) ([]UTXO, error) {
	if !i.validateConfirmedUTXOs || i.utxoValidator == nil || len(utxos) == 0 {
		return utxos, nil
	}

	keep := make([]bool, len(utxos))
	jobs := make(chan int)
	var confirmedCount int
	for idx, utxo := range utxos {
		if utxo.IsMempool {
			keep[idx] = true
			continue
		}
		confirmedCount++
	}
	if confirmedCount == 0 {
		return utxos, nil
	}

	workers := i.utxoValidationWorkers
	if workers <= 0 {
		workers = 8
	}
	if workers > confirmedCount {
		workers = confirmedCount
	}

	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error
	setErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
		})
	}

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				utxo := utxos[idx]
				vout, parseErr := strconv.ParseUint(utxo.Index, 10, 32)
				if parseErr != nil {
					setErr(fmt.Errorf("parse confirmed utxo index %s:%s: %w", utxo.TxID, utxo.Index, parseErr))
					continue
				}
				unspent, validateErr := i.ValidateConfirmedUTXO(utxo.TxID, uint32(vout), address, utxo.Amount)
				if validateErr != nil {
					setErr(fmt.Errorf("validate confirmed utxo %s:%s: %w", utxo.TxID, utxo.Index, validateErr))
					continue
				}
				keep[idx] = unspent
			}
		}()
	}

	for idx, utxo := range utxos {
		if utxo.IsMempool {
			continue
		}
		jobs <- idx
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	filtered := make([]UTXO, 0, len(utxos))
	for idx, utxo := range utxos {
		if keep[idx] {
			filtered = append(filtered, utxo)
		}
	}
	if dropped := len(utxos) - len(filtered); dropped > 0 {
		log.Printf("[UTXO] filtered %d stale confirmed UTXOs via node gettxout validation", dropped)
	}
	return filtered, nil
}

func (i *UTXOIndexer) GetSpendUTXOs(address string) (utxos []string, err error) {
	// 1. Get confirmed UTXOs
	addrKey := []byte(address)
	// Get spent UTXOs
	spendData, _, err := i.spendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			utxos = append(utxos, spendTx)
		}
	}

	return utxos, nil
}

type UTXO struct {
	TxID      string `json:"tx_id"`
	Index     string `json:"index"`
	Amount    uint64 `json:"amount"`
	IsMempool bool   `json:"is_mempool"`
}

func (i *UTXOIndexer) GetDbUtxoByTx(tx string) ([]byte, error) {
	return i.utxoStore.Get([]byte(tx))
}

// GetMempoolUTXOs queries the UTXOs of an address in the mempool
func (i *UTXOIndexer) GetMempoolUTXOs(address string) (mempoolIncomeList []common.Utxo, mempoolSpendList []common.Utxo, err error) {
	// Check if mempool manager is set
	if i.mempoolManager == nil {
		return nil, nil, fmt.Errorf("Mempool manager not set")
	}

	// Directly use interface method
	mempoolIncomeList, err = i.mempoolManager.GetUTXOsByAddress(address)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to get mempool UTXO: %w", err)
	}
	return
}

// GetAddressBalance gets the balance of an address
// dustThreshold: 小于此阈值的 UTXO 将被计入 unsafeFee (默认建议 546 或 1000 聪)
func (i *UTXOIndexer) GetAddressBalance(address string, dustThreshold int64) (*Balance, error) {
	// Directly use GetBalance method
	balance, err := i.GetBalance(address, dustThreshold)
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

// GetHistoryUTXOs
func (i *UTXOIndexer) GetHistoryUTXOs(address string, pageStr string, limitStr string) ([]HistoryTx, int64, error) {
	return i.GetHistoryUTXOsWithOptions(address, pageStr, limitStr, "", "desc")
}

func (i *UTXOIndexer) GetHistoryUTXOsWithOptions(address string, pageStr string, limitStr string, confirmedOnlyStr string, sortStr string) ([]HistoryTx, int64, error) {
	txs, err := i.GetHistoryTxList(address)
	if err != nil {
		return nil, 0, err
	}
	options := normalizeHistoryQueryOptions(pageStr, limitStr, confirmedOnlyStr, sortStr)
	page, total := filterSortPaginateHistoryTxs(txs, options)
	return page, total, nil
}

type HistoryUTXO struct {
	TxID      string `json:"tx_id"`
	Index     string `json:"index"`
	Amount    uint64 `json:"amount"`
	Type      string `json:"type"` // "income" or "spend"
	Timestamp string `json:"timestamp"`
	IsMempool bool   `json:"is_mempool"`
}

func (i *UTXOIndexer) GetHistoryUTXOList(address string) (history []HistoryUTXO, err error) {
	addrKey := []byte(address)
	outpointAmountMap := make(map[string]uint64)

	// 1. Get confirmed Income
	data, _, _ := i.addressStore.GetWithShard(addrKey)
	if data != nil {
		parts := strings.Split(string(data), ",")
		for _, part := range parts {
			incomes := strings.Split(part, "@")
			if len(incomes) < 4 { // Expecting txid@index@amount@time
				continue
			}
			amount, err := strconv.ParseUint(incomes[2], 10, 64)
			if err != nil {
				continue
			}
			timestamp, _ := strconv.ParseInt(incomes[3], 10, 64)
			timeStr := time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")

			outpoint := incomes[0] + ":" + incomes[1]
			outpointAmountMap[outpoint] = amount

			history = append(history, HistoryUTXO{
				TxID:      incomes[0],
				Index:     incomes[1],
				Amount:    amount,
				Type:      "income",
				Timestamp: timeStr,
				IsMempool: false,
			})
		}
	}

	// 2. Get Mempool Data
	if i.mempoolManager != nil {
		mempoolIncomeData, mempoolSpendData := i.mempoolManager.GetDataByAddress(address)

		// Mempool Income
		for k, v := range mempoolIncomeData {
			arr := strings.Split(k, "_")
			if len(arr) < 3 {
				continue
			}
			outpoint := arr[1]
			timestamp, _ := strconv.ParseInt(arr[2], 10, 64)
			timeStr := time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
			amount, _ := strconv.ParseUint(v, 10, 64)

			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) != 2 {
				continue
			}

			outpointAmountMap[outpoint] = amount

			history = append(history, HistoryUTXO{
				TxID:      outpointParts[0],
				Index:     outpointParts[1],
				Amount:    amount,
				Type:      "income",
				Timestamp: timeStr,
				IsMempool: true,
			})
		}

		// Mempool Spend
		for k := range mempoolSpendData {
			arr := strings.Split(k, "_")
			if len(arr) < 3 {
				continue
			}
			outpoint := arr[1]
			timestamp, _ := strconv.ParseInt(arr[2], 10, 64)
			timeStr := time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")

			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) != 2 {
				continue
			}

			amount := outpointAmountMap[outpoint] // Might be 0 if not found

			history = append(history, HistoryUTXO{
				TxID:      outpointParts[0],
				Index:     outpointParts[1],
				Amount:    amount,
				Type:      "spend",
				Timestamp: timeStr,
				IsMempool: true,
			})
		}
	}

	// 3. Get confirmed Spend
	spendData, _, err := i.spendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			arr := strings.Split(spendTx, "@")
			if len(arr) < 2 {
				continue
			}
			outpoint := arr[0]
			timestamp, _ := strconv.ParseInt(arr[1], 10, 64)
			timeStr := time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")

			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) != 2 {
				continue
			}

			amount := outpointAmountMap[outpoint]

			history = append(history, HistoryUTXO{
				TxID:      outpointParts[0],
				Index:     outpointParts[1],
				Amount:    amount,
				Type:      "spend",
				Timestamp: timeStr,
				IsMempool: false,
			})
		}
	}

	// 4. Sort
	sort.Slice(history, func(i, j int) bool {
		return history[i].Timestamp > history[j].Timestamp
	})

	return history, nil
}

type HistoryTx struct {
	TxID          string  `json:"tx_id"`
	Timestamp     string  `json:"time"`
	TimestampUnix int64   `json:"timestamp,omitempty"`
	Income        uint64  `json:"income"`
	Spend         uint64  `json:"spend"`
	Type          string  `json:"type"` // "income", "spend", "mixed"
	IsMempool     bool    `json:"is_mempool"`
	Confirmations *uint64 `json:"confirmations,omitempty"`
	Height        *int64  `json:"height"`
}

type HistoryQueryOptions struct {
	Page          int
	Limit         int
	ConfirmedOnly bool
	Sort          string
}

func normalizeHistoryQueryOptions(pageStr, limitStr, confirmedOnlyStr, sortStr string) HistoryQueryOptions {
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	sortOrder := strings.ToLower(strings.TrimSpace(sortStr))
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	return HistoryQueryOptions{
		Page:          page,
		Limit:         limit,
		ConfirmedOnly: isTruthyHistoryOption(confirmedOnlyStr),
		Sort:          sortOrder,
	}
}

func isTruthyHistoryOption(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func filterSortPaginateHistoryTxs(txs []HistoryTx, options HistoryQueryOptions) ([]HistoryTx, int64) {
	page := options.Page
	if page < 1 {
		page = 1
	}
	limit := options.Limit
	if limit < 1 {
		limit = 20
	}

	filtered := make([]HistoryTx, 0, len(txs))
	for _, tx := range txs {
		if options.ConfirmedOnly && tx.IsMempool {
			continue
		}
		filtered = append(filtered, tx)
	}

	sortOrder := strings.ToLower(options.Sort)
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].TimestampUnix == filtered[j].TimestampUnix {
			return filtered[i].TxID < filtered[j].TxID
		}
		if sortOrder == "asc" {
			return filtered[i].TimestampUnix < filtered[j].TimestampUnix
		}
		return filtered[i].TimestampUnix > filtered[j].TimestampUnix
	})

	total := int64(len(filtered))
	if len(filtered) == 0 {
		return []HistoryTx{}, total
	}
	lastPage := 1 + (len(filtered)-1)/limit
	if page > lastPage {
		return []HistoryTx{}, total
	}
	start := (page - 1) * limit
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total
}

func historyConfirmations(isMempool bool) *uint64 {
	if !isMempool {
		return nil
	}
	var confirmations uint64
	return &confirmations
}

func validHistoryTxID(txid string) bool {
	if len(txid) != 64 {
		return false
	}
	for _, ch := range txid {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
}

func (i *UTXOIndexer) GetHistoryTxList(address string) (txs []HistoryTx, err error) {
	addrKey := []byte(address)
	txMap := make(map[string]*HistoryTx)
	outpointAmountMap := make(map[string]uint64)

	getTx := func(txid string, ts int64, isMempool bool) *HistoryTx {
		if !validHistoryTxID(txid) {
			return nil
		}
		if _, ok := txMap[txid]; !ok {
			txMap[txid] = &HistoryTx{
				TxID:          txid,
				Timestamp:     time.Unix(ts, 0).Format("2006-01-02 15:04:05"),
				TimestampUnix: ts,
				IsMempool:     isMempool,
				Confirmations: historyConfirmations(isMempool),
			}
		}
		return txMap[txid]
	}

	// 1. Confirmed Income
	data, _, _ := i.addressStore.GetWithShard(addrKey)
	if data != nil {
		parts := strings.Split(string(data), ",")
		for _, part := range parts {
			incomes := strings.Split(part, "@")
			if len(incomes) < 4 {
				continue
			}
			amount, err := strconv.ParseUint(incomes[2], 10, 64)
			if err != nil {
				continue
			}
			timestamp, _ := strconv.ParseInt(incomes[3], 10, 64)

			outpoint := incomes[0] + ":" + incomes[1]
			outpointAmountMap[outpoint] = amount

			if tx := getTx(incomes[0], timestamp, false); tx != nil {
				tx.Income += amount
			}
		}
	}

	// 2. Mempool Data
	if i.mempoolManager != nil {
		mempoolIncomeData, mempoolSpendData := i.mempoolManager.GetDataByAddress(address)

		// Mempool Income
		for k, v := range mempoolIncomeData {
			arr := strings.Split(k, "_")
			if len(arr) < 3 {
				continue
			}
			outpoint := arr[1]
			timestamp, _ := strconv.ParseInt(arr[2], 10, 64)
			amount, _ := strconv.ParseUint(v, 10, 64)

			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) == 2 {
				outpointAmountMap[outpoint] = amount
				if tx := getTx(outpointParts[0], timestamp, true); tx != nil {
					tx.Income += amount
				}
			}
		}

		// Mempool Spend
		for k, v := range mempoolSpendData {
			arr := strings.Split(k, "_")
			if len(arr) < 3 {
				continue
			}
			outpoint := arr[1]
			timestamp, _ := strconv.ParseInt(arr[2], 10, 64)

			amount := outpointAmountMap[outpoint]

			if validHistoryTxID(v) {
				tx := getTx(v, timestamp, true)
				tx.Spend += amount
			}
		}
	}

	// 3. Confirmed Spend
	spendData, _, err := i.spendStore.GetWithShard(addrKey)
	if err == nil {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			arr := strings.Split(spendTx, "@")
			if len(arr) < 2 {
				continue
			}
			outpoint := arr[0]
			timestamp, _ := strconv.ParseInt(arr[1], 10, 64)
			amount := outpointAmountMap[outpoint]

			if len(arr) >= 3 {
				spendingTxID := arr[2]
				if tx := getTx(spendingTxID, timestamp, false); tx != nil {
					tx.Spend += amount
				}
			}
		}
	}

	// Convert map to slice
	for _, tx := range txMap {
		if tx.Income > 0 && tx.Spend > 0 {
			tx.Type = "mixed"
		} else if tx.Income > 0 {
			tx.Type = "income"
		} else {
			tx.Type = "spend"
		}
		txs = append(txs, *tx)
	}

	sort.Slice(txs, func(i, j int) bool {
		if txs[i].TimestampUnix == txs[j].TimestampUnix {
			return txs[i].TxID < txs[j].TxID
		}
		return txs[i].TimestampUnix > txs[j].TimestampUnix
	})

	return txs, nil
}

// GetIncomeStore 获取addressStore用于查询收入UTXO
func (i *UTXOIndexer) GetIncomeStore() *storage.PebbleStore {
	return i.addressStore
}

// GetSpendStore 获取spendStore用于查询花费UTXO
func (i *UTXOIndexer) GetSpendStore() *storage.PebbleStore {
	return i.spendStore
}

// AddressBalance holds the confirmed balance for one address.
type AddressBalance struct {
	Address        string  `json:"address"`
	BalanceSatoshi int64   `json:"balance_satoshi"`
	Balance        float64 `json:"balance"`
	UTXOCount      int64   `json:"utxo_count"`
}

// richHeap is a min-heap of AddressBalance ordered by BalanceSatoshi (ascending).
// It keeps only the top-N entries during streaming iteration.
type richHeap []AddressBalance

func (h richHeap) Len() int            { return len(h) }
func (h richHeap) Less(i, j int) bool  { return h[i].BalanceSatoshi < h[j].BalanceSatoshi }
func (h richHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *richHeap) Push(x interface{}) { *h = append(*h, x.(AddressBalance)) }
func (h *richHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

const richListDBKey = "rich_list_cache"
const richListLimit = 500

var ErrRichListCacheNotReady = errors.New("rich list cache not ready")

func (i *UTXOIndexer) richListHasIndexedAddresses() bool {
	if i.metaStore == nil {
		return false
	}

	data, err := i.metaStore.Get([]byte("total_address_count"))
	if err != nil {
		return false
	}

	totalAddressCount, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return false
	}
	return totalAddressCount > 0
}

// GetRichList reads the rich list from the database and returns a paginated slice.
// The database is updated every 4 hours by the background goroutine started via
// StartRichListWarmup. If no data exists yet, an empty list is returned.
func (i *UTXOIndexer) GetRichList(page, pageSize int, dustThreshold int64) (list []AddressBalance, total int64, err error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	if i.rankStore != nil {
		list, total, err = i.getRichListFromRankStore(page, pageSize)
		if err == nil || errors.Is(err, ErrRichListCacheNotReady) {
			return
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return
		}
	}

	data, dbErr := i.metaStore.Get([]byte(richListDBKey))
	if dbErr != nil {
		if errors.Is(dbErr, storage.ErrNotFound) {
			if i.richListHasIndexedAddresses() {
				err = ErrRichListCacheNotReady
				return
			}
			list = []AddressBalance{}
			return
		}
		err = fmt.Errorf("read rich list from db: %w", dbErr)
		return
	}

	var all []AddressBalance
	if err = json.Unmarshal(data, &all); err != nil {
		err = fmt.Errorf("parse rich list: %w", err)
		return
	}
	if len(all) == 0 && i.richListHasIndexedAddresses() {
		err = ErrRichListCacheNotReady
		return
	}

	total = int64(len(all))
	offset := (page - 1) * pageSize
	if offset >= int(total) {
		list = []AddressBalance{}
		return
	}
	end := offset + pageSize
	if end > int(total) {
		end = int(total)
	}
	list = all[offset:end]
	return
}

// runRichListScan performs a full streaming scan, builds the top-500 list, and
// writes the result as JSON to the metaStore. It never blocks callers.
func (i *UTXOIndexer) runRichListScan() error {
	h := &richHeap{}
	heap.Init(h)

	// Reuse maps across iterations to avoid per-address allocations.
	spendMap := make(map[string]struct{}, 128)
	seen := make(map[string]struct{}, 128)

	scanErr := i.addressStore.IterateShards(func(rawKey, rawValue []byte) bool {
		clear(spendMap)
		clear(seen)

		spendData, _, _ := i.spendStore.GetWithShard(rawKey)
		if len(spendData) > 0 {
			for _, spendTx := range strings.Split(string(spendData), ",") {
				if spendTx == "" {
					continue
				}
				arr := strings.Split(spendTx, "@")
				if len(arr) >= 1 && arr[0] != "" {
					spendMap[arr[0]] = struct{}{}
				}
			}
		}

		var income, spend, utxoCount int64
		for _, part := range strings.Split(string(rawValue), ",") {
			fields := strings.Split(part, "@")
			if len(fields) < 3 {
				continue
			}
			key := fields[0] + ":" + fields[1]
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			in, e := strconv.ParseInt(fields[2], 10, 64)
			if e != nil {
				continue
			}
			utxoCount++
			income += in
			if _, spent := spendMap[key]; spent {
				spend += in
			}
		}
		balance := income - spend
		if balance <= 0 {
			return true
		}

		entry := AddressBalance{
			Address:        string(rawKey),
			BalanceSatoshi: balance,
			Balance:        float64(balance) / 1e8,
			UTXOCount:      utxoCount - int64(len(spendMap)),
		}
		if h.Len() < richListLimit {
			heap.Push(h, entry)
		} else if balance > (*h)[0].BalanceSatoshi {
			heap.Pop(h)
			heap.Push(h, entry)
		}
		return true
	})
	if scanErr != nil {
		return fmt.Errorf("iterate income store: %w", scanErr)
	}

	// Drain heap into descending slice.
	result := make([]AddressBalance, h.Len())
	for idx := h.Len() - 1; idx >= 0; idx-- {
		result[idx] = heap.Pop(h).(AddressBalance)
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal rich list: %w", err)
	}
	return i.metaStore.Set([]byte(richListDBKey), data)
}

// StartRichListWarmup starts a background goroutine that runs an initial scan
// immediately and then repeats every 4 hours. If a scan is still running when
// the timer fires, that tick is skipped. The goroutine exits when stopCh is closed.
func (i *UTXOIndexer) StartRichListWarmup(stopCh <-chan struct{}) {
	go func() {
		i.doRichListRefresh()

		ticker := time.NewTicker(4 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				log.Println("[RichList] Stopped")
				return
			case <-ticker.C:
				i.doRichListRefresh()
			}
		}
	}()
}

// doRichListRefresh runs a scan if none is already in progress; otherwise skips.
func (i *UTXOIndexer) doRichListRefresh() {
	if !i.richListRefreshing.CompareAndSwap(0, 1) {
		log.Println("[RichList] Scan still in progress, skipping")
		return
	}
	defer i.richListRefreshing.Store(0)
	log.Println("[RichList] Scan started")
	if err := i.runRichListScan(); err != nil {
		log.Printf("[RichList] Scan error: %v", err)
	} else {
		log.Println("[RichList] Scan complete, DB updated")
	}
}
