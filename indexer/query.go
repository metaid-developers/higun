package indexer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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
			}
			income += in
			utxoCount += 1
			mempoolCheckTxMap[key] = in
		}
	}
	balance := income - spend
	// Convert to BTC unit (1 BTC = 100,000,000 satoshis)
	btcBalance := float64(balance) / 1e8
	mempoolIncomeData, mempoolSpendData := i.mempoolManager.GetDataByAddress(address)
	mempoolIncomeList := getUtxoFromMempoolIncomeMap(mempoolIncomeData)
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
	// Check if mempool is spent
	//if len(mempoolCheckTxMap) > 0 {
	// var list []string
	// for txPoint := range mempoolCheckTxMap {
	// 	list = append(list, txPoint)
	// }
	// mempoolSpendMap, _ := i.mempoolManager.GetSpendUTXOs(list)
	mempoolSpendMap := getUtxoFromMempoolSpendMap(mempoolSpendData)
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
	for k, v := range data {
		arr := strings.Split(k, "_")
		if len(arr) < 2 {
			continue
		}
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
		mempoolSpendMap := getUtxoFromMempoolSpendMap(mempoolSpendData)
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
	return result, nil
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
	// Directly use GetUTXOs method
	utxos, err := i.GetHistoryTxList(address)
	if err != nil {
		return nil, 0, err
	}
	// Pagination
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 10
	}
	start := (page - 1) * limit
	end := start + limit
	if start >= len(utxos) {
		return []HistoryTx{}, 0, nil
	}
	if end > len(utxos) {
		end = len(utxos)
	}
	return utxos[start:end], int64(len(utxos)), nil
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
	TxID      string `json:"tx_id"`
	Timestamp string `json:"time"`
	Income    uint64 `json:"income"`
	Spend     uint64 `json:"spend"`
	Type      string `json:"type"` // "income", "spend", "mixed"
	IsMempool bool   `json:"is_mempool"`
}

func (i *UTXOIndexer) GetHistoryTxList(address string) (txs []HistoryTx, err error) {
	addrKey := []byte(address)
	txMap := make(map[string]*HistoryTx)
	outpointAmountMap := make(map[string]uint64)

	getTx := func(txid string, ts int64, isMempool bool) *HistoryTx {
		if _, ok := txMap[txid]; !ok {
			txMap[txid] = &HistoryTx{
				TxID:      txid,
				Timestamp: time.Unix(ts, 0).Format("2006-01-02 15:04:05"),
				IsMempool: isMempool,
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

			tx := getTx(incomes[0], timestamp, false)
			tx.Income += amount
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
				tx := getTx(outpointParts[0], timestamp, true)
				tx.Income += amount
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

			if v != "" {
				tx := getTx(v, timestamp, true)
				tx.Spend += amount
			} else {
				fakeTxID := "pending_spend_" + outpoint
				tx := getTx(fakeTxID, timestamp, true)
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
				tx := getTx(spendingTxID, timestamp, false)
				tx.Spend += amount
			} else {
				// Old data without spending TxID
				// Use outpoint as unique ID to list it
				fakeTxID := "spend_" + outpoint
				tx := getTx(fakeTxID, timestamp, false)
				tx.Spend += amount
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

	// Sort
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].Timestamp > txs[j].Timestamp
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
