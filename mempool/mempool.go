package mempool

import (
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/indexer"
)

// Ensure MempoolManager implements the indexer.MempoolManager interface
var _ indexer.MempoolManager = (*MempoolManager)(nil)

// UTXOResult defines UTXO query result
type UTXOResult struct {
	Income []struct {
		TxID   string
		Index  string
		Amount uint64
	}
	Spent []struct {
		TxID   string
		Index  string
		Amount uint64
	}
	// Add amount statistics
	TotalIncome uint64 // Total income (satoshis)
	TotalSpent  uint64 // Total spent (satoshis)
}

// getRawUTXOsByAddress internal method, get raw UTXO data, including income and spent UTXOs
func (m *MempoolManager) getRawUTXOsByAddress(address string) (incomeUtxoList []common.Utxo, err error) {

	// Map for deduplication
	incomeMap := make(map[string]struct{})
	//spentMap := make(map[string]struct{})

	// 1. Get income UTXOs
	incomeList, err := m.MempoolIncomeDB.GetUtxoByKey(address)
	if err == nil {
		for _, utxo := range incomeList {
			if _, ok := incomeMap[utxo.TxID]; !ok {
				incomeUtxoList = append(incomeUtxoList, utxo)
				incomeMap[utxo.TxID] = struct{}{}
			}
		}
	}
	// // 2. Get spent UTXOs
	// spendList, err := m.mempoolSpendDB.GetUtxoByKey(address)
	// if err == nil {
	// 	for _, utxo := range spendList {
	// 		if _, ok := spentMap[utxo.TxID]; !ok {
	// 			spendUtxoList = append(spendUtxoList, utxo)
	// 			spentMap[utxo.TxID] = struct{}{}
	// 		}
	// 	}
	// }
	return
}
func (m *MempoolManager) getRawSpendByAddress(address string) (spendUtxoList []common.Utxo, err error) {

	// Map for deduplication
	spentMap := make(map[string]struct{})
	// 2. Get spent UTXOs
	spendList, err := m.MempoolSpendDB.GetUtxoByKey(address)
	if err == nil {
		for _, utxo := range spendList {
			if _, ok := spentMap[utxo.TxID]; !ok {
				spendUtxoList = append(spendUtxoList, utxo)
				spentMap[utxo.TxID] = struct{}{}
			}
		}
	}
	return
}

// GetUTXOsByAddress gets mempool UTXOs for the specified address
func (m *MempoolManager) GetUTXOsByAddress(address string) (incomeUtxoList []common.Utxo, err error) {
	// Get raw UTXO data
	return m.getRawUTXOsByAddress(address)
}
func (m *MempoolManager) GetSpendByAddress(address string) (spendUtxoList []common.Utxo, err error) {
	// Get raw UTXO data
	return m.getRawSpendByAddress(address)
}
func (m *MempoolManager) BatchDeleteIncom(list []string) (err error) {
	return m.MempoolIncomeDB.BatchDeleteMempolRecord(list)
}
func (m *MempoolManager) BatchDeleteSpend(list []string) (err error) {
	return m.MempoolSpendDB.BatchDeleteMempolRecord(list)
}
func (m *MempoolManager) GetSpendUTXOs(txPoints []string) (spendMap map[string]struct{}, err error) {
	list, _ := m.MempoolSpendDB.BatchGetMempolRecord(txPoints)
	spendMap = make(map[string]struct{}, len(list))
	for _, txPoint := range list {
		spendMap[txPoint] = struct{}{}
	}
	return
}

func (m *MempoolManager) DeleteMempool() (err error) {
	return m.RebuildMempool()
}
func (m *MempoolManager) StartMempool() (err error) {
	return m.Start()
}

func (m *MempoolManager) GetDataByAddress(address string) (income map[string]string, spend map[string]string) {
	income, _ = m.MempoolIncomeDB.GetByPrefix(address)
	spend, _ = m.MempoolSpendDB.GetByPrefix(address)
	return income, spend
}
