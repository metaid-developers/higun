package blockchain

import (
	"fmt"
	"log"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
)

func (c *Client) BackfillMVCTxIDAliases(idx *indexer.UTXOIndexer, stopCh <-chan struct{}) error {
	if c == nil || c.cfg == nil || c.cfg.Chain != config.ChainMVC {
		return nil
	}
	if c.adapter == nil {
		log.Printf("[TxIDAliasBackfill] MVC adapter missing; skipping txid alias backfill")
		return nil
	}
	if idx == nil {
		return fmt.Errorf("indexer is nil")
	}

	targetHeight, err := idx.GetLastIndexedHeight()
	if err != nil {
		return fmt.Errorf("get last indexed height: %w", err)
	}
	completeHeight, completeOK, err := idx.GetTxIDAliasBackfillCompleteHeight()
	if err != nil {
		return fmt.Errorf("get txid alias backfill completion height: %w", err)
	}
	if completeOK && completeHeight >= targetHeight {
		log.Printf("[TxIDAliasBackfill] MVC txid alias backfill already complete through height %d", completeHeight)
		return nil
	}

	progressHeight, progressOK, err := idx.GetTxIDAliasBackfillProgress()
	if err != nil {
		return fmt.Errorf("get txid alias backfill progress: %w", err)
	}
	startHeight := 0
	if progressOK {
		startHeight = progressHeight + 1
	}
	if startHeight > targetHeight {
		if err := idx.MarkTxIDAliasBackfillComplete(targetHeight); err != nil {
			return fmt.Errorf("mark txid alias backfill complete: %w", err)
		}
		log.Printf("[TxIDAliasBackfill] MVC txid alias backfill complete through height %d", targetHeight)
		return nil
	}

	log.Printf("[TxIDAliasBackfill] Starting MVC txid alias backfill from height %d to %d", startHeight, targetHeight)
	for height := startHeight; height <= targetHeight; height++ {
		select {
		case <-stopCh:
			log.Printf("[TxIDAliasBackfill] Stop requested at height %d; progress will resume from last saved height", height)
			return nil
		default:
		}

		block, err := c.adapter.GetBlock(int64(height))
		if err != nil {
			return fmt.Errorf("get MVC block %d for txid alias backfill: %w", height, err)
		}
		if block != nil {
			if err := idx.StoreTxIDAliases(block.Transactions); err != nil {
				return fmt.Errorf("store MVC txid aliases for height %d: %w", height, err)
			}
		}
		if err := idx.SetTxIDAliasBackfillProgress(height); err != nil {
			return fmt.Errorf("persist txid alias backfill progress height %d: %w", height, err)
		}
		if height == startHeight || height == targetHeight || height%1000 == 0 {
			log.Printf("[TxIDAliasBackfill] MVC txid alias backfill progress height=%d target=%d", height, targetHeight)
		}
	}

	if err := idx.MarkTxIDAliasBackfillComplete(targetHeight); err != nil {
		return fmt.Errorf("mark txid alias backfill complete: %w", err)
	}
	log.Printf("[TxIDAliasBackfill] MVC txid alias backfill complete through height %d", targetHeight)
	return nil
}
