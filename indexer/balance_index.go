package indexer

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"runtime"
	"strconv"
	"strings"

	"github.com/metaid/utxo_indexer/storage"
)

type confirmedBalanceRow struct {
	BalanceSatoshi int64 `json:"balance_satoshi"`
	UTXOCount      int64 `json:"utxo_count"`
	Tracked        bool  `json:"tracked,omitempty"`
}

type confirmedBalanceDelta struct {
	BalanceSatoshi int64
	UTXOCount      int64
}

type negativeConfirmedBalanceStateError struct {
	Address        string
	BalanceSatoshi int64
	UTXOCount      int64
}

func (e *negativeConfirmedBalanceStateError) Error() string {
	return fmt.Sprintf(
		"negative confirmed balance state for %s: balance=%d utxo_count=%d",
		e.Address,
		e.BalanceSatoshi,
		e.UTXOCount,
	)
}

const balanceIndexReadyMetaKey = "balance_index_ready"

func balanceRankKey(address string, balanceSatoshi int64) string {
	reversed := uint64(math.MaxInt64 - balanceSatoshi)
	return fmt.Sprintf("%019d:%s", reversed, address)
}

func (i *UTXOIndexer) putAddressBalance(address string, balanceSatoshi, utxoCount int64) error {
	if i.balanceStore == nil {
		return fmt.Errorf("balance store not configured")
	}
	data, err := json.Marshal(confirmedBalanceRow{
		BalanceSatoshi: balanceSatoshi,
		UTXOCount:      utxoCount,
	})
	if err != nil {
		return fmt.Errorf("marshal confirmed balance row: %w", err)
	}
	return i.balanceStore.Set([]byte(address), data)
}

func (i *UTXOIndexer) putTrackedAddressBalance(address string, balanceSatoshi, utxoCount int64) error {
	if i.balanceStore == nil {
		return fmt.Errorf("balance store not configured")
	}
	if balanceSatoshi <= 0 && utxoCount <= 0 {
		return nil
	}
	data, err := json.Marshal(confirmedBalanceRow{
		BalanceSatoshi: balanceSatoshi,
		UTXOCount:      utxoCount,
		Tracked:        true,
	})
	if err != nil {
		return fmt.Errorf("marshal tracked confirmed balance row: %w", err)
	}
	return i.balanceStore.Put([]byte(address), data)
}

func (i *UTXOIndexer) getAddressBalanceRow(address string) (confirmedBalanceRow, error) {
	if i.balanceStore == nil {
		return confirmedBalanceRow{}, storage.ErrNotFound
	}
	data, err := i.balanceStore.Get([]byte(address))
	if err != nil {
		return confirmedBalanceRow{}, err
	}
	var row confirmedBalanceRow
	if err := json.Unmarshal(data, &row); err != nil {
		return confirmedBalanceRow{}, fmt.Errorf("parse confirmed balance row: %w", err)
	}
	return row, nil
}

func (i *UTXOIndexer) putBalanceRank(address string, balanceSatoshi, utxoCount int64) error {
	if i.rankStore == nil {
		return fmt.Errorf("rank store not configured")
	}
	if balanceSatoshi <= 0 {
		return nil
	}
	entry := AddressBalance{
		Address:        address,
		BalanceSatoshi: balanceSatoshi,
		Balance:        float64(balanceSatoshi) / 1e8,
		UTXOCount:      utxoCount,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal rank entry: %w", err)
	}
	return i.rankStore.Set([]byte(balanceRankKey(address, balanceSatoshi)), data)
}

func (i *UTXOIndexer) getRichListFromRankStore(page, pageSize int) (list []AddressBalance, total int64, err error) {
	if i.rankStore == nil {
		err = storage.ErrNotFound
		return
	}
	if !i.isBalanceIndexReady() {
		if i.richListHasIndexedAddresses() {
			err = ErrRichListCacheNotReady
			return
		}
	}

	offset := (page - 1) * pageSize
	end := offset + pageSize
	list = make([]AddressBalance, 0, pageSize)

	iterErr := i.rankStore.IterateShards(func(_, value []byte) bool {
		var entry AddressBalance
		if unmarshalErr := json.Unmarshal(value, &entry); unmarshalErr != nil {
			err = fmt.Errorf("parse balance rank entry: %w", unmarshalErr)
			return false
		}

		if total >= int64(offset) && total < int64(end) {
			list = append(list, entry)
		}
		total++
		return true
	})
	if iterErr != nil {
		err = fmt.Errorf("iterate balance rank store: %w", iterErr)
		return
	}
	if total == 0 && i.richListHasIndexedAddresses() {
		err = ErrRichListCacheNotReady
		return
	}
	if err != nil {
		return
	}
	return
}

func parseBalanceRankAddress(key []byte) string {
	parts := strings.SplitN(string(key), ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func (i *UTXOIndexer) setBalanceIndexReady(ready bool) error {
	if i.metaStore == nil {
		return nil
	}
	value := "0"
	if ready {
		value = "1"
	}
	return i.metaStore.Set([]byte(balanceIndexReadyMetaKey), []byte(value))
}

func (i *UTXOIndexer) isBalanceIndexReady() bool {
	if i.metaStore == nil {
		return false
	}
	data, err := i.metaStore.Get([]byte(balanceIndexReadyMetaKey))
	if err != nil {
		return false
	}
	return string(data) == "1"
}

func clearStore(store *storage.PebbleStore) error {
	if store == nil {
		return nil
	}
	return store.Clear()
}

func (i *UTXOIndexer) BootstrapConfirmedBalanceIndexesIfNeeded() error {
	if i.balanceStore == nil || i.rankStore == nil {
		return nil
	}
	if i.isBalanceIndexReady() {
		return nil
	}
	return i.BootstrapConfirmedBalanceIndexes()
}

func (i *UTXOIndexer) BootstrapConfirmedBalanceIndexes() error {
	if i.balanceStore == nil || i.rankStore == nil {
		return fmt.Errorf("balance index stores are not configured")
	}

	if err := i.setBalanceIndexReady(false); err != nil {
		return fmt.Errorf("mark balance index not ready: %w", err)
	}

	if err := clearStore(i.balanceStore); err != nil {
		return fmt.Errorf("clear balance store: %w", err)
	}
	if err := clearStore(i.rankStore); err != nil {
		return fmt.Errorf("clear rank store: %w", err)
	}

	const commitBatchSize = 5000
	spendMap := make(map[string]struct{}, 128)
	seen := make(map[string]struct{}, 128)
	balanceBatch := i.balanceStore.NewBatch()
	rankBatch := i.rankStore.NewBatch()
	pending := 0
	totalIndexed := 0

	commit := func() error {
		if pending == 0 {
			return nil
		}
		if err := balanceBatch.Commit(); err != nil {
			return err
		}
		if err := rankBatch.Commit(); err != nil {
			return err
		}
		balanceBatch = i.balanceStore.NewBatch()
		rankBatch = i.rankStore.NewBatch()
		pending = 0
		return nil
	}

	var callbackErr error
	scanErr := i.addressStore.IterateShards(func(rawKey, rawValue []byte) bool {
		if callbackErr != nil {
			return false
		}
		address := string(rawKey)
		clear(spendMap)
		clear(seen)

		if i.spendStore != nil {
			spendData, _, err := i.spendStore.GetWithShard(rawKey)
			if err != nil && !errors.Is(err, storage.ErrNotFound) {
				callbackErr = fmt.Errorf("read spend data for %s: %w", address, err)
				return false
			}
			if err == nil {
				for _, spendTx := range strings.Split(string(spendData), ",") {
					if spendTx == "" {
						continue
					}
					parts := strings.Split(spendTx, "@")
					if len(parts) >= 1 && parts[0] != "" {
						spendMap[parts[0]] = struct{}{}
					}
				}
			}
		}

		var balance int64
		var utxoCount int64
		for _, part := range strings.Split(string(rawValue), ",") {
			fields := strings.Split(part, "@")
			if len(fields) < 3 {
				continue
			}
			outpoint := fields[0] + ":" + fields[1]
			if _, dup := seen[outpoint]; dup {
				continue
			}
			seen[outpoint] = struct{}{}

			amount, err := strconv.ParseInt(fields[2], 10, 64)
			if err != nil {
				continue
			}
			if _, spent := spendMap[outpoint]; spent {
				continue
			}
			balance += amount
			utxoCount++
		}

		if balance <= 0 {
			return true
		}

		rowData, err := json.Marshal(confirmedBalanceRow{
			BalanceSatoshi: balance,
			UTXOCount:      utxoCount,
		})
		if err != nil {
			callbackErr = err
			return false
		}
		if err := balanceBatch.Set([]byte(address), rowData); err != nil {
			callbackErr = err
			return false
		}

		rankData, err := json.Marshal(AddressBalance{
			Address:        address,
			BalanceSatoshi: balance,
			Balance:        float64(balance) / 1e8,
			UTXOCount:      utxoCount,
		})
		if err != nil {
			callbackErr = err
			return false
		}
		if err := rankBatch.Set([]byte(balanceRankKey(address, balance)), rankData); err != nil {
			callbackErr = err
			return false
		}

		totalIndexed++
		pending++
		if pending >= commitBatchSize {
			if err := commit(); err != nil {
				callbackErr = err
				return false
			}
		}
		return true
	})
	if scanErr != nil {
		return fmt.Errorf("iterate income store for bootstrap: %w", scanErr)
	}
	if callbackErr != nil {
		return fmt.Errorf("bootstrap callback failed: %w", callbackErr)
	}
	if err := commit(); err != nil {
		return fmt.Errorf("commit bootstrap batch: %w", err)
	}

	if err := i.setBalanceIndexReady(true); err != nil {
		return fmt.Errorf("mark balance index ready: %w", err)
	}

	log.Printf("[BalanceIndex] Bootstrap completed, indexed %d addresses", totalIndexed)
	return nil
}

func (i *UTXOIndexer) StartBalanceIndexBootstrapIfNeeded() <-chan error {
	doneCh := make(chan error, 1)
	go func() {
		runner := i.bootstrapBalanceIndexFn
		if runner == nil {
			runner = i.BootstrapConfirmedBalanceIndexesIfNeeded
		}
		doneCh <- runner()
	}()
	return doneCh
}

func (i *UTXOIndexer) updateConfirmedBalanceIndexes(deltas map[string]confirmedBalanceDelta) error {
	if len(deltas) == 0 || i.balanceStore == nil {
		return nil
	}
	if !i.isBalanceIndexReady() {
		if err := i.syncTouchedConfirmedBalanceRowsFromHistory(deltas); err != nil {
			log.Printf("[BalanceIndex] skipping touched confirmed balance row sync after history refresh failure: %v", err)
		}
		return nil
	}
	if err := i.applyConfirmedBalanceDeltas(deltas); err != nil {
		var negativeStateErr *negativeConfirmedBalanceStateError
		if errors.As(err, &negativeStateErr) {
			log.Printf(
				"[BalanceIndex] detected delta anomaly for %s (balance=%d utxo_count=%d), rebuilding touched addresses from history",
				negativeStateErr.Address,
				negativeStateErr.BalanceSatoshi,
				negativeStateErr.UTXOCount,
			)
			recoverErr := i.rebuildTouchedConfirmedBalanceIndexesFromHistory(deltas)
			if recoverErr == nil {
				log.Printf("[BalanceIndex] recovered %d touched addresses from history; keeping confirmed balance index ready", len(deltas))
				return nil
			}
			log.Printf("[BalanceIndex] touched-address rebuild failed after delta anomaly: %v", recoverErr)
		} else {
			log.Printf("[BalanceIndex] disabling confirmed balance index after delta update failure: %v", err)
		}

		if setErr := i.setBalanceIndexReady(false); setErr != nil {
			return fmt.Errorf("disable confirmed balance index after update failure: %w", setErr)
		}
	}
	return nil
}

func (i *UTXOIndexer) syncTouchedConfirmedBalanceRowsFromHistory(deltas map[string]confirmedBalanceDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	if i.balanceStore == nil {
		return fmt.Errorf("balance store not configured")
	}
	if i.addressStore == nil {
		return fmt.Errorf("address store not configured")
	}
	if i.spendStore == nil {
		return fmt.Errorf("spend store not configured")
	}

	balanceBatch := i.balanceStore.NewBatch()

	for address := range deltas {
		row, exists, err := i.buildTrackedConfirmedBalanceRowFromHistory(address)
		if err != nil {
			return fmt.Errorf("build tracked confirmed balance row for %s: %w", address, err)
		}
		if !exists {
			if err := balanceBatch.Delete([]byte(address)); err != nil {
				return fmt.Errorf("delete tracked confirmed balance row for %s: %w", address, err)
			}
			continue
		}

		rowData, err := json.Marshal(confirmedBalanceRow{
			BalanceSatoshi: row.BalanceSatoshi,
			UTXOCount:      row.UTXOCount,
			Tracked:        true,
		})
		if err != nil {
			return fmt.Errorf("marshal tracked confirmed balance row for %s: %w", address, err)
		}
		if err := balanceBatch.Set([]byte(address), rowData); err != nil {
			return fmt.Errorf("set tracked confirmed balance row for %s: %w", address, err)
		}
	}

	if err := balanceBatch.Commit(); err != nil {
		return fmt.Errorf("commit tracked confirmed balance batch: %w", err)
	}
	return nil
}

func (i *UTXOIndexer) buildTrackedConfirmedBalanceRowFromHistory(address string) (confirmedBalanceRow, bool, error) {
	addrKey := []byte(address)
	spendMap := make(map[string]struct{})

	spendData, _, err := i.spendStore.GetWithShard(addrKey)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return confirmedBalanceRow{}, false, fmt.Errorf("load spend history: %w", err)
	}
	if err == nil {
		for _, spendTx := range strings.Split(string(spendData), ",") {
			if spendTx == "" {
				continue
			}
			arr := strings.Split(spendTx, "@")
			if len(arr) < 1 || arr[0] == "" {
				continue
			}
			spendMap[arr[0]] = struct{}{}
		}
	}

	data, _, err := i.addressStore.GetWithShard(addrKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return confirmedBalanceRow{}, false, nil
		}
		return confirmedBalanceRow{}, false, fmt.Errorf("load income history: %w", err)
	}

	incomeMap := make(map[string]struct{})
	var balance int64
	var utxoCount int64
	for _, part := range strings.Split(string(data), ",") {
		incomes := strings.Split(part, "@")
		if len(incomes) < 3 {
			continue
		}
		key := incomes[0] + ":" + incomes[1]
		if _, exists := incomeMap[key]; exists {
			continue
		}
		incomeMap[key] = struct{}{}

		amount, err := strconv.ParseInt(incomes[2], 10, 64)
		if err != nil {
			continue
		}
		if _, spent := spendMap[key]; spent {
			continue
		}
		balance += amount
		utxoCount++
	}

	if balance == 0 && utxoCount == 0 {
		return confirmedBalanceRow{}, false, nil
	}

	return confirmedBalanceRow{
		BalanceSatoshi: balance,
		UTXOCount:      utxoCount,
		Tracked:        true,
	}, true, nil
}

func addConfirmedBalanceDelta(deltas map[string]confirmedBalanceDelta, address string, balanceDelta, utxoDelta int64) {
	if deltas == nil || address == "" || address == "errAddress" {
		return
	}
	delta := deltas[address]
	delta.BalanceSatoshi += balanceDelta
	delta.UTXOCount += utxoDelta
	deltas[address] = delta
}

func (i *UTXOIndexer) loadExistingConfirmedBalanceRows(keys []string) (map[string]confirmedBalanceRow, error) {
	existingRows := make(map[string]confirmedBalanceRow, len(keys))

	rows, err := i.balanceStore.BulkQueryMapConcurrent(keys, runtime.NumCPU())
	if err != nil {
		return nil, fmt.Errorf("load existing confirmed balance rows: %w", err)
	}
	for address, data := range rows {
		if len(data) == 0 {
			continue
		}
		var row confirmedBalanceRow
		if err := json.Unmarshal(data, &row); err != nil {
			return nil, fmt.Errorf("parse confirmed balance row for %s: %w", address, err)
		}
		existingRows[address] = row
	}
	return existingRows, nil
}

func (i *UTXOIndexer) purgeRankEntriesForAddresses(addresses map[string]struct{}, rankBatch *storage.Batch) error {
	if i.rankStore == nil || len(addresses) == 0 || rankBatch == nil {
		return nil
	}

	var callbackErr error
	if err := i.rankStore.IterateShards(func(key, _ []byte) bool {
		address := parseBalanceRankAddress(key)
		if address == "" {
			return true
		}
		if _, ok := addresses[address]; !ok {
			return true
		}

		keyCopy := append([]byte(nil), key...)
		if err := rankBatch.Delete(keyCopy); err != nil {
			callbackErr = err
			return false
		}
		return true
	}); err != nil {
		return fmt.Errorf("iterate rank store for purge: %w", err)
	}
	if callbackErr != nil {
		return fmt.Errorf("delete stale rank entry during purge: %w", callbackErr)
	}
	return nil
}

func (i *UTXOIndexer) rebuildTouchedConfirmedBalanceIndexesFromHistory(deltas map[string]confirmedBalanceDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	if i.balanceStore == nil {
		return fmt.Errorf("balance store not configured")
	}

	keys := make([]string, 0, len(deltas))
	addressSet := make(map[string]struct{}, len(deltas))
	for address := range deltas {
		keys = append(keys, address)
		addressSet[address] = struct{}{}
	}

	balanceBatch := i.balanceStore.NewBatch()
	var rankBatch *storage.Batch
	if i.rankStore != nil {
		rankBatch = i.rankStore.NewBatch()
		if err := i.purgeRankEntriesForAddresses(addressSet, rankBatch); err != nil {
			return fmt.Errorf("purge stale rank entries: %w", err)
		}
	}

	for _, address := range keys {
		row, exists, err := i.buildTrackedConfirmedBalanceRowFromHistory(address)
		if err != nil {
			return fmt.Errorf("build tracked confirmed balance row for %s: %w", address, err)
		}
		if !exists {
			if err := balanceBatch.Delete([]byte(address)); err != nil {
				return fmt.Errorf("delete tracked confirmed balance row for %s: %w", address, err)
			}
			continue
		}

		rowData, err := json.Marshal(confirmedBalanceRow{
			BalanceSatoshi: row.BalanceSatoshi,
			UTXOCount:      row.UTXOCount,
			Tracked:        true,
		})
		if err != nil {
			return fmt.Errorf("marshal tracked confirmed balance row for %s: %w", address, err)
		}
		if err := balanceBatch.Set([]byte(address), rowData); err != nil {
			return fmt.Errorf("set tracked confirmed balance row for %s: %w", address, err)
		}

		if rankBatch != nil && row.BalanceSatoshi > 0 {
			entryData, err := json.Marshal(AddressBalance{
				Address:        address,
				BalanceSatoshi: row.BalanceSatoshi,
				Balance:        float64(row.BalanceSatoshi) / 1e8,
				UTXOCount:      row.UTXOCount,
			})
			if err != nil {
				return fmt.Errorf("marshal balance rank entry for %s: %w", address, err)
			}
			if err := rankBatch.Set([]byte(balanceRankKey(address, row.BalanceSatoshi)), entryData); err != nil {
				return fmt.Errorf("set balance rank entry for %s: %w", address, err)
			}
		}
	}

	if err := balanceBatch.Commit(); err != nil {
		return fmt.Errorf("commit tracked confirmed balance batch: %w", err)
	}
	if rankBatch != nil {
		if err := rankBatch.Commit(); err != nil {
			return fmt.Errorf("commit tracked balance rank batch: %w", err)
		}
	}
	return nil
}

func (i *UTXOIndexer) applyConfirmedBalanceDeltas(deltas map[string]confirmedBalanceDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	if i.balanceStore == nil {
		return fmt.Errorf("balance store not configured")
	}

	keys := make([]string, 0, len(deltas))
	for address := range deltas {
		keys = append(keys, address)
	}

	existingRows, err := i.loadExistingConfirmedBalanceRows(keys)
	if err != nil {
		return err
	}

	balanceBatch := i.balanceStore.NewBatch()
	var rankBatch *storage.Batch
	if i.rankStore != nil {
		rankBatch = i.rankStore.NewBatch()
	}

	for address, delta := range deltas {
		row := existingRows[address]
		newBalance := row.BalanceSatoshi + delta.BalanceSatoshi
		newCount := row.UTXOCount + delta.UTXOCount
		if newBalance < 0 || newCount < 0 {
			return &negativeConfirmedBalanceStateError{
				Address:        address,
				BalanceSatoshi: newBalance,
				UTXOCount:      newCount,
			}
		}

		if rankBatch != nil && row.BalanceSatoshi > 0 {
			if err := rankBatch.Delete([]byte(balanceRankKey(address, row.BalanceSatoshi))); err != nil {
				return fmt.Errorf("delete old balance rank for %s: %w", address, err)
			}
		}

		if newBalance == 0 && newCount == 0 {
			if err := balanceBatch.Delete([]byte(address)); err != nil {
				return fmt.Errorf("delete confirmed balance row for %s: %w", address, err)
			}
			continue
		}

		rowData, err := json.Marshal(confirmedBalanceRow{
			BalanceSatoshi: newBalance,
			UTXOCount:      newCount,
		})
		if err != nil {
			return fmt.Errorf("marshal confirmed balance row for %s: %w", address, err)
		}
		if err := balanceBatch.Set([]byte(address), rowData); err != nil {
			return fmt.Errorf("set confirmed balance row for %s: %w", address, err)
		}

		if rankBatch != nil && newBalance > 0 {
			entryData, err := json.Marshal(AddressBalance{
				Address:        address,
				BalanceSatoshi: newBalance,
				Balance:        float64(newBalance) / 1e8,
				UTXOCount:      newCount,
			})
			if err != nil {
				return fmt.Errorf("marshal balance rank entry for %s: %w", address, err)
			}
			if err := rankBatch.Set([]byte(balanceRankKey(address, newBalance)), entryData); err != nil {
				return fmt.Errorf("set balance rank entry for %s: %w", address, err)
			}
		}
	}

	if err := balanceBatch.Commit(); err != nil {
		return fmt.Errorf("commit confirmed balance batch: %w", err)
	}
	if rankBatch != nil {
		if err := rankBatch.Commit(); err != nil {
			return fmt.Errorf("commit balance rank batch: %w", err)
		}
	}
	return nil
}
