package indexer

import (
	"errors"
	"strings"

	"github.com/metaid/utxo_indexer/storage"
)

const txIDAliasMetaPrefix = "tx_alias:"

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
