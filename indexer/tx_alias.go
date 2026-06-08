package indexer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/metaid/utxo_indexer/storage"
)

const (
	txIDAliasMetaPrefix                    = "tx_alias:"
	txIDAliasBackfillProgressMetaKey       = "tx_alias_backfill:mvc:last_height"
	txIDAliasBackfillCompleteHeightMetaKey = "tx_alias_backfill:mvc:complete_height"
)

func txIDAliasMetaKey(txid string) string {
	return txIDAliasMetaPrefix + txid
}

func (i *UTXOIndexer) storeTxIDAliases(transactions []*Transaction) error {
	if i == nil || i.metaStore == nil || len(transactions) == 0 {
		return nil
	}
	aliases := make(map[string]string)
	for _, tx := range transactions {
		if tx == nil {
			continue
		}
		publicTxID := strings.ToLower(strings.TrimSpace(tx.ID))
		nodeTxID := strings.ToLower(strings.TrimSpace(tx.NodeID))
		if publicTxID == "" || nodeTxID == "" || publicTxID == nodeTxID {
			continue
		}
		aliases[txIDAliasMetaKey(publicTxID)] = nodeTxID
	}
	return i.metaStore.BulkSet(aliases)
}

func (i *UTXOIndexer) StoreTxIDAliases(transactions []*Transaction) error {
	return i.storeTxIDAliases(transactions)
}

func (i *UTXOIndexer) ResolveTxIDAlias(txid string) (string, bool, error) {
	if i == nil || i.metaStore == nil {
		return "", false, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(txid))
	if normalized == "" {
		return "", false, nil
	}
	data, err := i.metaStore.Get([]byte(txIDAliasMetaKey(normalized)))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	resolved := strings.ToLower(strings.TrimSpace(string(data)))
	if resolved == "" || resolved == normalized {
		return "", false, nil
	}
	return resolved, true, nil
}

func (i *UTXOIndexer) GetTxIDAliasBackfillProgress() (int, bool, error) {
	return i.getTxIDAliasBackfillHeight(txIDAliasBackfillProgressMetaKey)
}

func (i *UTXOIndexer) SetTxIDAliasBackfillProgress(height int) error {
	return i.setTxIDAliasBackfillHeight(txIDAliasBackfillProgressMetaKey, height)
}

func (i *UTXOIndexer) GetTxIDAliasBackfillCompleteHeight() (int, bool, error) {
	return i.getTxIDAliasBackfillHeight(txIDAliasBackfillCompleteHeightMetaKey)
}

func (i *UTXOIndexer) MarkTxIDAliasBackfillComplete(height int) error {
	return i.setTxIDAliasBackfillHeight(txIDAliasBackfillCompleteHeightMetaKey, height)
}

func (i *UTXOIndexer) getTxIDAliasBackfillHeight(key string) (int, bool, error) {
	if i == nil || i.metaStore == nil {
		return 0, false, nil
	}
	data, err := i.metaStore.Get([]byte(key))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	height, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false, fmt.Errorf("invalid txid alias backfill height %q: %w", string(data), err)
	}
	return height, true, nil
}

func (i *UTXOIndexer) setTxIDAliasBackfillHeight(key string, height int) error {
	if height < 0 {
		return fmt.Errorf("invalid txid alias backfill height: %d", height)
	}
	if i == nil || i.metaStore == nil {
		return nil
	}
	return i.metaStore.Set([]byte(key), []byte(strconv.Itoa(height)))
}
