package indexer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
)

type CountMsg struct {
	TotalUtxo       uint64
	TotalAddress    uint64
	BlockLastHeight int64
	LocalLastHeight int64
}

var metaStoreMu sync.Mutex
var BaseCount CountMsg

func (i *UTXOIndexer) InitBaseCount() error {
	// 加载地址表的总统计结果
	totalAddress, err := i.LoadTotalCountFromMetaStore("total_address_count")
	if err != nil {
		return fmt.Errorf("failed to load total address count: %w", err)
	}
	BaseCount.TotalAddress = totalAddress

	// 加载 UTXO 表的总统计结果
	totalUtxo, err := i.LoadTotalCountFromMetaStore("total_utxo_count")
	if err != nil {
		return fmt.Errorf("failed to load total UTXO count: %w", err)
	}
	BaseCount.TotalUtxo = totalUtxo

	return nil
}
func (i *UTXOIndexer) SyncBaseCount() {
	for {
		i.TotalKeyCount()
		time.Sleep(20 * time.Minute)
	}
}
func (i *UTXOIndexer) SetSyncCount(localHeight int, bestHeight int) {
	BaseCount.BlockLastHeight = int64(bestHeight)
	BaseCount.LocalLastHeight = int64(localHeight)
}
func (i *UTXOIndexer) TotalKeyCount() {
	// 加载地址表的 lastKeys
	addressLastKeys, err := i.LoadLastKeysFromMetaStore("address_last_keys")
	if err != nil {
		fmt.Println("Failed to load address last keys:", err)
		//return
	}
	// 地址表增量统计
	addressCnt, updatedAddressKeys, err := i.addressStore.IncrementalKeyCount(addressLastKeys)
	if err == nil {
		BaseCount.TotalAddress += addressCnt // 累加增量到总数
		// 保存更新后的 lastKeys
		if err := i.SaveLastKeysToMetaStore("address_last_keys", updatedAddressKeys); err != nil {
			fmt.Println("Failed to save address last keys:", err)
		}
		// 持久化总统计结果
		if err := i.SaveTotalCountToMetaStore("total_address_count", BaseCount.TotalAddress); err != nil {
			fmt.Println("Failed to save total address count:", err)
		}
	} else {
		fmt.Println("Address store error:", err)
	}
	//fmt.Println("addressCnt:", addressCnt)

	// 加载 UTXO 表的 lastKeys
	utxoLastKeys, err := i.LoadLastKeysFromMetaStore("utxo_last_keys")
	if err != nil {
		fmt.Println("Failed to load UTXO last keys:", err)
		//return
	}

	// UTXO 表增量统计
	utxoCnt, updatedUtxoKeys, err := i.utxoStore.IncrementalKeyCount(utxoLastKeys)
	if err == nil {
		BaseCount.TotalUtxo += utxoCnt // 累加增量到总数
		// 保存更新后的 lastKeys
		if err := i.SaveLastKeysToMetaStore("utxo_last_keys", updatedUtxoKeys); err != nil {
			fmt.Println("Failed to save UTXO last keys:", err)
		}
		// 持久化总统计结果
		if err := i.SaveTotalCountToMetaStore("total_utxo_count", BaseCount.TotalUtxo); err != nil {
			fmt.Println("Failed to save total UTXO count:", err)
		}
	} else {
		fmt.Println("UTXO store error:", err)
	}
	//fmt.Println("utxoCnt:", utxoCnt)
}
func (i *UTXOIndexer) SaveTotalCountToMetaStore(storeName string, totalCount uint64) error {
	metaStoreMu.Lock()
	defer metaStoreMu.Unlock()
	if totalCount <= 0 {
		return nil
	}
	serializedData := []byte(fmt.Sprintf("%d", totalCount))
	// 使用 pebble.Sync 强制落盘，避免写入尚未可见
	return i.metaStore.Set([]byte(storeName), serializedData)
}
func (i *UTXOIndexer) LoadTotalCountFromMetaStore(storeName string) (uint64, error) {
	metaStoreMu.Lock()
	defer metaStoreMu.Unlock()

	data, err := i.metaStore.Get([]byte(storeName))
	if err != nil {
		if err == pebble.ErrNotFound {
			return 0, nil // 如果不存在，返回 0
		}
		return 0, fmt.Errorf("failed to load total count: %w", err)
	}

	totalCount, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse total count: %w", err)
	}

	return totalCount, nil
}

func (i *UTXOIndexer) SaveLastKeysToMetaStore(storeName string, lastKeys map[int][]byte) error {
	metaStoreMu.Lock()
	defer metaStoreMu.Unlock()
	if len(lastKeys) == 0 {
		return nil
	}
	data := make(map[string]string)
	for shardIdx, key := range lastKeys {
		data[fmt.Sprintf("%d", shardIdx)] = string(key)
	}

	serializedData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize lastKeys: %w", err)
	}
	// 使用 pebble.Sync 强制落盘
	return i.metaStore.Set([]byte(storeName), serializedData)
}
func (i *UTXOIndexer) LoadLastKeysFromMetaStore(storeName string) (map[int][]byte, error) {
	metaStoreMu.Lock()
	defer metaStoreMu.Unlock()

	data, err := i.metaStore.Get([]byte(storeName))
	if err != nil {
		if err == pebble.ErrNotFound {
			return make(map[int][]byte), nil // 如果不存在，返回空的 lastKeys
		}
		return nil, fmt.Errorf("failed to load lastKeys: %w", err)
	}

	var deserializedData map[string]string
	if err := json.Unmarshal(data, &deserializedData); err != nil {
		return nil, fmt.Errorf("failed to deserialize lastKeys: %w", err)
	}

	lastKeys := make(map[int][]byte)
	for shardIdxStr, key := range deserializedData {
		shardIdx, err := strconv.Atoi(shardIdxStr)
		if err != nil {
			return nil, fmt.Errorf("invalid shard index: %w", err)
		}
		lastKeys[shardIdx] = []byte(key)
	}

	return lastKeys, nil
}
