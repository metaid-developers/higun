package indexer

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/metaid/utxo_indexer/syslogs"
)

var IsHandleReorg bool

func (idx *UTXOIndexer) DeleteDataByBlockHeight(blockHeight int64) error {
	// Implement the logic to delete data by block height
	//先看看有没有独立文件
	block, err := LoadFBlockPart(blockHeight, "", -1)
	if err == nil {
		// 找到独立文件，进行删除
		if err := idx.DoDelete(block); err != nil {
			return fmt.Errorf("failed to delete block %d: %w", blockHeight, err)
		}
	}
	//再看看有没有分片文件
	for i := 0; i < 10000; i++ {
		block, err := LoadFBlockPart(blockHeight, "utxo", i)
		if err == errors.New("noFile") {
			break
		}
		if err == nil {
			if err := idx.DoDelete(block); err != nil {
				return fmt.Errorf("failed to delete block %d: %w", blockHeight, err)
			}
		}
	}
	for i := 0; i < 10000; i++ {
		block, err := LoadFBlockPart(blockHeight, "spend", i)
		if err == errors.New("noFile") {
			break
		}
		if err == nil {
			if err := idx.DoDelete(block); err != nil {
				return fmt.Errorf("failed to delete block %d: %w", blockHeight, err)
			}
		}
	}
	return nil
}
func (idx *UTXOIndexer) DoDelete(block *Block) error {
	//fmt.Println("--------UTXO Data-------")
	const batchSize = 10000
	utxos := make([]string, 0, batchSize)
	for k := range block.UtxoData {
		utxos = append(utxos, k)
		if len(utxos) >= batchSize {
			if err := idx.utxoStore.BatchDelete(utxos); err != nil {
				return fmt.Errorf("batch delete failed: %w", err)
			}
			utxos = utxos[:0]
		}
	}
	// 删除剩余未满 batch 的
	if len(utxos) > 0 {
		if err := idx.utxoStore.BatchDelete(utxos); err != nil {
			return fmt.Errorf("batch delete failed: %w", err)
		}
	}
	//fmt.Println("--------Income Data-------")
	incomes := make(map[string][]string, batchSize)
	for k, v := range block.IncomeData {
		incomes[k] = v
		if len(incomes) >= batchSize {
			if err := idx.addressStore.BatchDeleteByMap(incomes); err != nil {
				return fmt.Errorf("batch delete failed: %w", err)
			}
			incomes = make(map[string][]string, batchSize)
		}
	}
	// 删除剩余未满 batch 的
	if len(incomes) > 0 {
		if err := idx.addressStore.BatchDeleteByMap(incomes); err != nil {
			return fmt.Errorf("batch delete failed: %w", err)
		}
	}
	//fmt.Println("--------Spend Data-------")
	spends := make(map[string][]string, batchSize)
	for k, v := range block.SpendData {
		spends[k] = v
		if len(spends) >= batchSize {
			if err := idx.addressStore.BatchDeleteByMap(spends); err != nil {
				return fmt.Errorf("batch delete failed: %w", err)
			}
			spends = make(map[string][]string, batchSize)
		}
	}
	// 删除剩余未满 batch 的
	if len(spends) > 0 {
		if err := idx.addressStore.BatchDeleteByMap(spends); err != nil {
			return fmt.Errorf("batch delete failed: %w", err)
		}
	}
	return nil
}
func (idx *UTXOIndexer) HandleReorg(fromHeight, toHeight int64) error {
	IsHandleReorg = true
	for i := fromHeight; i <= toHeight; i++ {
		if err := idx.DeleteDataByBlockHeight(i); err != nil {
			return fmt.Errorf("failed to delete data for block %d: %w", i, err)
		}
	}
	heightStr := strconv.FormatInt(fromHeight-1, 10)
	err := idx.metaStore.Set([]byte("last_indexed_height"), []byte(heightStr))
	if err != nil {
		errMsg := syslogs.ErrLog{
			ErrType:      "ReorgResetLocal",
			Timestamp:    time.Now().Unix(),
			ErrorMessage: err.Error(),
		}
		go syslogs.InsertErrLog(errMsg)
	}
	if err := syslogs.UpdateReorgStatus(fromHeight, 1); err != nil {
		return fmt.Errorf("failed to update reorg status: %w", err)
	}
	if err := syslogs.UpdateIndexerReorg(int(fromHeight), int(toHeight)); err != nil {
		return fmt.Errorf("failed to update indexer reorg: %w", err)
	}
	//重建内存池
	if idx.mempoolManager != nil {
		err = idx.mempoolManager.DeleteMempool()
		if err != nil {
			return fmt.Errorf("failed to rebuild mempool: %w", err)
		}
		err = idx.mempoolManager.StartMempool()
		if err != nil {
			return fmt.Errorf("failed to start mempool: %w", err)
		}
	}

	IsHandleReorg = false
	return nil
}
