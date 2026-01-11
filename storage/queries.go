package storage

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/metaid/utxo_indexer/common"
)

// SimpleDB is a simple single database connection, not using sharding
type SimpleDB struct {
	db   *pebble.DB
	path string
}

// NewSimpleDB creates a new single database connection
func NewSimpleDB(dbPath string) (*SimpleDB, error) {
	db, err := pebble.Open(dbPath, &pebble.Options{Logger: noopLogger})
	if err != nil {
		return nil, fmt.Errorf("Failed to open database: %w", err)
	}
	return &SimpleDB{db: db, path: dbPath}, nil
}

// Close closes the database connection
func (s *SimpleDB) Close() error {
	return s.db.Close()
}

func (s *SimpleDB) Get(key string) (result string, err error) {
	value, _, err := s.db.Get([]byte(key))
	if err != nil {
		return
	}
	result = string(value)
	return
}
func (s *SimpleDB) GetByPrefix(key string) (results map[string]string, err error) {
	results = make(map[string]string)
	prefix := []byte(key)
	// 计算 UpperBound 以优化查询性能
	upperBound := prefixUpperBound(prefix)

	// Create iterator with both bounds for better performance
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		// 复制 value 数据，避免迭代器关闭后失效
		value := append([]byte(nil), iter.Value()...)
		results[string(iter.Key())] = string(value)
	}
	return
}

// prefixUpperBound 计算前缀的上界，用于优化范围查询
func prefixUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil // 前缀全是 0xff，没有上界
}

// GetByUTXO queries addresses associated with UTXO ID
// Example: For key "tx1:0_addr1", query by "tx1:0"
func (s *SimpleDB) GetByUTXO(utxoID string) (address string, amount string, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(utxoID + "_")
	upperBound := prefixUpperBound(prefix)

	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate to find first matching key
	for iter.First(); iter.Valid(); iter.Next() {
		// Extract address part (part after _)
		parts := strings.Split(string(iter.Key()), "_")
		if len(parts) == 2 {
			address = parts[1]
		}
		// Get value corresponding to key (copy data)
		amount = string(append([]byte(nil), iter.Value()...))
		break
	}
	return
}

// GetByAddress queries all related UTXOs by address
// Example: For key "addr1_tx1:0", query by "addr1"
func (s *SimpleDB) GetUtxoByKey(key string) (utxoList []common.Utxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	upperBound := prefixUpperBound(prefix)

	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		utxo := common.Utxo{}
		// Extract address part (part after _)
		parts := strings.Split(string(iter.Key()), "_")
		if len(parts) == 2 {
			utxo.Address = parts[0]
			utxo.TxID = parts[1]
		}
		// Get value corresponding to key (copy data)
		utxo.Amount = string(append([]byte(nil), iter.Value()...))
		utxoList = append(utxoList, utxo)
	}
	return
}

// GetByUTXO queries related addresses through UTXO ID
// Example: For key "tx1:0_addr1", query through "tx1:0"
// value:CodeHash@Genesis@sensibleId@Amount@Index@Value@timestamp
func (s *SimpleDB) GetByFtUTXO(utxoID string) (address string, codeHash string, genesis string, sensibleId string, amount string, value string, index string, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(utxoID + "_")
	upperBound := prefixUpperBound(prefix)

	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate to find first matching key
	for iter.First(); iter.Valid(); iter.Next() {
		// Extract address part (part after _)
		keyParts := strings.Split(string(iter.Key()), "_")
		if len(keyParts) == 2 {
			address = keyParts[1]
		}
		// Get value corresponding to key (copy data)
		//CodeHash@Genesis@sensibleId@Amount@Index@Value
		valueData := string(append([]byte(nil), iter.Value()...))
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 6 {
			codeHash = valueParts[0]
			genesis = valueParts[1]
			sensibleId = valueParts[2]
			amount = valueParts[3]
			index = valueParts[4]
			value = valueParts[5]
		}
		break
	}
	return
}

// GetByUTXO queries related addresses through UTXO ID
// Example: For key "tx1:0_addr1", query through "tx1:0"
// value:CodeHash@Genesis@sensibleId@customData@Index@Value
func (s *SimpleDB) GetByUniqueFtUTXO(utxoID string) (codehashGenesis string, codeHash string, genesis string, sensibleId string, customData string, value string, index string, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(utxoID + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()
	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		// Check if key starts with prefix
		key := iter.Key()
		if !strings.HasPrefix(string(key), string(prefix)) {
			break // Already beyond prefix range
		}

		// Extract address part (part after _)
		keyParts := strings.Split(string(key), "_")
		if len(keyParts) == 2 {
			codehashGenesis = keyParts[1]
		}
		// Get value corresponding to key
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) == 6 {
			codeHash = valueParts[0]
			genesis = valueParts[1]
			sensibleId = valueParts[2]
			customData = valueParts[3]
			index = valueParts[4]
			value = valueParts[5]
		}
		break
	}
	return
}

// GetByAddress queries all related UTXOs through address
// Example: For key "addr1_tx1:0", query through "addr1"
// value:CodeHash@Genesis@sensibleId@Amount@Index@Value@timestamp@usedTxId
func (s *SimpleDB) GetFtUtxoByKey(key string) (ftUtxoList []common.FtUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()
	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		// Check if key starts with prefix
		key := iter.Key()
		if !strings.HasPrefix(string(key), string(prefix)) {
			break // Already beyond prefix range
		}
		utxo := common.FtUtxo{}
		// Extract address part (part after _)
		keyParts := strings.Split(string(key), "_")
		if len(keyParts) == 2 {
			utxo.Address = keyParts[0]
			utxo.UtxoId = keyParts[1]

			utxoIdStrs := strings.Split(utxo.UtxoId, ":")
			if len(utxoIdStrs) == 2 {
				utxo.TxID = utxoIdStrs[0]
				utxo.Index = utxoIdStrs[1]
			}

		}
		// Get value corresponding to key
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 6 {
			utxo.CodeHash = valueParts[0]
			utxo.Genesis = valueParts[1]
			utxo.SensibleId = valueParts[2]
			utxo.Amount = valueParts[3]
			utxo.Index = valueParts[4]
			utxo.Value = valueParts[5]
			if len(valueParts) > 6 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[6], 10, 64)
			}
			if len(valueParts) > 7 {
				utxo.UsedTxId = valueParts[7]
			}
		}
		ftUtxoList = append(ftUtxoList, utxo)
	}
	return
}

// GetByAddress queries all related UTXOs through address
// Example: For key "outpoint_ftCodehashGenesis", query through "outpoint_ftCodehashGenesis"
// value:codeHash@genesis@sensibleId@customData@Index@Value
func (s *SimpleDB) GetUniqueFtUtxoByKey(key string) (ftUtxoList []common.FtUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()
	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		// Check if key starts with prefix
		key := iter.Key()
		if !strings.HasPrefix(string(key), string(prefix)) {
			break // Already beyond prefix range
		}
		utxo := common.FtUtxo{}
		// Extract address part (part after _)
		keyParts := strings.Split(string(key), "_")
		if len(keyParts) == 2 {
			outpoint := keyParts[1]
			ftCodehashGenesis := keyParts[0]
			ftCodehashGenesisParts := strings.Split(ftCodehashGenesis, "@")
			if len(ftCodehashGenesisParts) == 2 {
				utxo.CodeHash = ftCodehashGenesisParts[0]
				utxo.Genesis = ftCodehashGenesisParts[1]
			}
			utxo.UtxoId = outpoint
			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) == 2 {
				utxo.TxID = outpointParts[0]
				utxo.Index = outpointParts[1]
			}
		}
		// Get value corresponding to key
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 6 {
			utxo.CodeHash = valueParts[0]
			utxo.Genesis = valueParts[1]
			utxo.SensibleId = valueParts[2]
			utxo.CustomData = valueParts[3]
			utxo.Index = valueParts[4]
			utxo.Value = valueParts[5]
		}
		ftUtxoList = append(ftUtxoList, utxo)
	}
	return
}

func (s *SimpleDB) GetFtUtxoByOutpoint(outpoint string) (ftUtxoList []common.FtUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(outpoint + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()
	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		// Check if key starts with prefix
		key := iter.Key()
		if !strings.HasPrefix(string(key), string(prefix)) {
			break // Already beyond prefix range
		}
		utxo := common.FtUtxo{}
		// Extract address part (part after _)
		keyParts := strings.Split(string(key), "_")
		if len(keyParts) == 2 {
			utxo.Address = keyParts[1]
			utxo.UtxoId = keyParts[0]

			utxoIdStrs := strings.Split(utxo.UtxoId, ":")
			if len(utxoIdStrs) == 2 {
				utxo.TxID = utxoIdStrs[0]
				utxo.Index = utxoIdStrs[1]
			}

		}
		// Get value corresponding to key
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 6 {
			utxo.CodeHash = valueParts[0]
			utxo.Genesis = valueParts[1]
			utxo.SensibleId = valueParts[2]
			utxo.Amount = valueParts[3]
			utxo.Index = valueParts[4]
			utxo.Value = valueParts[5]
			if len(valueParts) > 6 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[6], 10, 64)
			}
			if len(valueParts) > 7 {
				utxo.UsedTxId = valueParts[7]
			}
		}
		ftUtxoList = append(ftUtxoList, utxo)
	}
	return
}

// GetFtGenesisByKey queries all related genesis information through outpoint
// Example: For key "outpoint_", query through "outpoint"
// key:outpoint, value: sensibleId@name@symbol@decimal@codeHash@genesis
func (s *SimpleDB) GetFtGenesisByKey(key string) ([]byte, error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		// Check if key starts with prefix
		key := iter.Key()
		if !strings.HasPrefix(string(key), string(prefix)) {
			break // Already beyond prefix range
		}

		// Return first matching value
		return iter.Value(), nil
	}

	return nil, nil
}

// GetFtGenesisByKey queries all related genesis information through outpoint
// Example: For key "outpoint_", query through "outpoint"
// key: usedOutpoint, value: sensibleId@name@symbol@decimal@codeHash@genesis@amount@txId@index@value,...
func (s *SimpleDB) GetFtGenesisOutputsByKey(key string) ([]byte, error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Iterate to find all matching keys
	for iter.First(); iter.Valid(); iter.Next() {
		// Check if key starts with prefix
		key := iter.Key()
		if !strings.HasPrefix(string(key), string(prefix)) {
			break // Already beyond prefix range
		}

		// Return first matching value
		return iter.Value(), nil
	}

	return nil, nil
}

func (s *SimpleDB) AddMempolRecord(key string, value []byte) error {
	return s.db.Set([]byte(key), value, pebble.Sync)
}

func (s *SimpleDB) DeleteMempolRecord(key string) error {
	return s.db.Delete([]byte(key), pebble.Sync)
}
func (s *SimpleDB) DeleteMempolRecordByPreKey(preKey string) error {
	if preKey == "" {
		return nil
	}
	prefix := []byte(preKey)
	upperBound := prefixUpperBound(prefix)
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	batch := s.db.NewBatch()
	defer batch.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		if !strings.HasPrefix(string(key), preKey) {
			break
		}
		copiedKey := append([]byte(nil), key...)
		if err := batch.Delete(copiedKey, pebble.Sync); err != nil {
			return err
		}
	}

	return batch.Commit(pebble.Sync)
}
func (s *SimpleDB) BatchDeleteMempolRecord(keys []string) error {
	batch := s.db.NewBatch()
	defer batch.Close()
	for _, preKey := range keys {
		if preKey == "" {
			continue
		}
		prefix := []byte(preKey)
		upperBound := prefixUpperBound(prefix)
		iter, err := s.db.NewIter(&pebble.IterOptions{
			LowerBound: prefix,
			UpperBound: upperBound,
		})
		if err != nil {
			return err
		}

		for iter.First(); iter.Valid(); iter.Next() {
			key := iter.Key()
			if !strings.HasPrefix(string(key), preKey) {
				break
			}
			copiedKey := append([]byte(nil), key...)
			if err := batch.Delete(copiedKey, pebble.Sync); err != nil {
				iter.Close()
				return err
			}
		}
		iter.Close()
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("Failed to commit batch: %w", err)
	}
	return nil
}

func (s *SimpleDB) BatchGetMempolRecord(keys []string) (list []string, err error) {
	for _, key := range keys {
		_, closer, err := s.db.Get([]byte(key))
		if err == nil {
			list = append(list, key)
		}
		if closer != nil {
			closer.Close()
		}
	}
	return
}

// SetWithIndex
func (s *SimpleDB) AddRecord(utxoID string, address string, value []byte) error {
	// Create batch
	batch := s.db.NewBatch()
	defer batch.Close()

	// 1. Primary keys
	mainKey1 := []byte(utxoID + "_" + address)
	mainKey2 := []byte(address + "_" + utxoID)
	//fmt.Println("mainKey1:", string(mainKey1), "mainKey2:", string(mainKey2))
	// Add set operations to batch
	if err := batch.Set(mainKey1, value, pebble.Sync); err != nil {
		return fmt.Errorf("Failed to add key1 in batch: %w", err)
	}
	if err := batch.Set(mainKey2, value, pebble.Sync); err != nil {
		return fmt.Errorf("Failed to add key2 in batch: %w", err)
	}

	// Commit batch
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("Failed to commit batch: %w", err)
	}

	return nil
}

// SetWithIndex
func (s *SimpleDB) AddSimpleRecord(key string, value []byte) error {
	// Create batch
	batch := s.db.NewBatch()
	defer batch.Close()

	// 1. Primary key
	mainKey := []byte(key)
	// Add set operation to batch
	if err := batch.Set(mainKey, value, pebble.Sync); err != nil {
		return fmt.Errorf("Failed to set key1 in batch: %w", err)
	}

	// Commit batch
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("Failed to commit batch: %w", err)
	}

	return nil
}

func (s *SimpleDB) GetSimpleRecord(key string) ([]byte, error) {
	value, _, err := s.db.Get([]byte(key))
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (s *SimpleDB) DeleteSimpleRecord(key string) error {
	batch := s.db.NewBatch()
	defer batch.Close()

	if err := batch.Delete([]byte(key), pebble.Sync); err != nil {
		return fmt.Errorf("Failed to delete key in batch: %w", err)
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("Failed to commit batch: %w", err)
	}
	return nil
}

// DeleteWithIndex deletes Spend records
func (s *SimpleDB) DeleteRecord(utxoID string, address string) error {
	// Create batch
	batch := s.db.NewBatch()
	defer batch.Close()

	// 1. Delete primary keys
	mainKey1 := []byte(utxoID + "_" + address)
	mainKey2 := []byte(address + "_" + utxoID)

	// Add delete operations to batch
	if err := batch.Delete(mainKey1, pebble.Sync); err != nil {
		return fmt.Errorf("Failed to delete key1 in batch: %w", err)
	}
	if err := batch.Delete(mainKey2, pebble.Sync); err != nil {
		return fmt.Errorf("Failed to delete key2 in batch: %w", err)
	}

	// Commit batch
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("Failed to commit batch: %w", err)
	}

	return nil
}

func (s *SimpleDB) DeleteSpendRecord(utxoID string) error {
	utxoList, err := s.GetUtxoByKey(utxoID)
	if err != nil {
		return fmt.Errorf("Query failed: %w", err)
	}
	for _, utxo := range utxoList {
		s.DeleteRecord(utxo.TxID, utxo.Address)
	}
	return nil
}

func (s *SimpleDB) DeleteFtSpendRecord(utxoID string) error {
	utxoList, err := s.GetFtUtxoByOutpoint(utxoID)
	if err != nil {
		return fmt.Errorf("Query failed: %w", err)
	}
	for _, utxo := range utxoList {
		s.DeleteRecord(utxoID, utxo.Address)
	}
	return nil
}

func (s *SimpleDB) DeleteUniqueSpendRecord(utxoID string) error {
	utxoList, err := s.GetFtUtxoByOutpoint(utxoID)
	if err != nil {
		return fmt.Errorf("Query failed: %w", err)
	}
	for _, utxo := range utxoList {
		s.DeleteRecord(utxoID, utxo.Address)
	}
	return nil
}

// // UpdateRecord updates record, if record exists then update, otherwise set
// func (s *SimpleDB) UpdateRecord(utxoID string, address string, value []byte) error {
// 	// Create batch
// 	batch := s.db.NewBatch()
// 	defer batch.Close()

// 	// 1. Primary key
// 	// 1. Delete primary key
// 	mainKey1 := []byte(utxoID + "_" + address)
// 	mainKey2 := []byte(address + "_" + utxoID)

// 	// Check if record exists
// 	_, closer, err := s.db.Get(mainKey1)
// 	if err == nil {
// 		// Record exists, perform update
// 		closer.Close()
// 		if err := batch.Set(mainKey1, value, pebble.NoSync); err != nil {
// 			return fmt.Errorf("Failed to update key1 in batch: %w", err)
// 		}
// 		if err := batch.Set(mainKey2, value, pebble.NoSync); err != nil {
// 			return fmt.Errorf("Failed to update key2 in batch: %w", err)
// 		}
// 	} else if err == pebble.ErrNotFound {
// 		// Record doesn't exist, perform set
// 		if err := batch.Set(mainKey1, value, pebble.NoSync); err != nil {
// 			return fmt.Errorf("Failed to set key1 in batch: %w", err)
// 		}
// 		if err := batch.Set(mainKey2, value, pebble.NoSync); err != nil {
// 			return fmt.Errorf("Failed to set key2 in batch: %w", err)
// 		}
// 	} else {
// 		// Other errors
// 		return fmt.Errorf("Failed to check record existence: %w", err)
// 	}

// 	// Commit batch
// 	if err := batch.Commit(pebble.Sync); err != nil {
// 		return fmt.Errorf("Failed to commit batch: %w", err)
// 	}

// 	return nil
// }

// GetFtUtxo gets all FT UTXO records
func (s *SimpleDB) GetFtUtxo() (ftUtxoList []common.FtUtxo, err error) {
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate through all records
	for iter.First(); iter.Valid(); iter.Next() {
		utxo := common.FtUtxo{}
		// Extract address part (part after _)
		keyParts := strings.Split(string(iter.Key()), ":")
		if len(keyParts) == 2 {
			utxo.TxID = keyParts[0]
			utxo.Index = keyParts[1]
			utxo.UtxoId = string(iter.Key())
		}

		// Get value corresponding to key
		//ftAddress@CodeHash@Genesis@sensibleId@Amount@Index@Value@timestamp
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 7 {
			utxo.Address = valueParts[0]
			utxo.CodeHash = valueParts[1]
			utxo.Genesis = valueParts[2]
			utxo.SensibleId = valueParts[3]
			utxo.Amount = valueParts[4]
			utxo.Index = valueParts[5]
			utxo.Value = valueParts[6]
			if len(valueParts) > 7 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[7], 10, 64)
			}
		}
		ftUtxoList = append(ftUtxoList, utxo)
	}
	return
}

// GetAll gets all records
func (s *SimpleDB) GetAll() ([]string, error) {
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var values []string
	// Iterate through all records
	for iter.First(); iter.Valid(); iter.Next() {
		values = append(values, string(iter.Value()))
	}
	return values, nil
}

func (s *SimpleDB) GetAllKeyValues() (map[string]string, error) {
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	keyValues := make(map[string]string)
	// Iterate through all records
	for iter.First(); iter.Valid(); iter.Next() {
		keyValues[string(iter.Key())] = string(iter.Value())
	}
	return keyValues, nil
}

// GetByNftUTXO queries NFT addresses associated with UTXO ID
// key: outpoint+address value: CodeHash@Genesis@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
func (s *SimpleDB) GetByNftUTXO(utxoID string) (address string, codeHash string, genesis string, tokenIndex string, value string, tokenSupply string, metaTxId string, metaOutputIndex string, index string, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(utxoID + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate, find first matching record
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Check if key starts with prefix
		if strings.HasPrefix(key, utxoID+"_") {
			// Extract address part (part after _)
			keyParts := strings.Split(key, "_")
			if len(keyParts) > 1 {
				address = keyParts[1]
			}

			// Get value corresponding to key
			// CodeHash@Genesis@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
			valueData := string(iter.Value())
			valueParts := strings.Split(valueData, "@")
			if len(valueParts) >= 8 {
				codeHash = valueParts[0]
				genesis = valueParts[1]
				tokenIndex = valueParts[2]
				index = valueParts[3]
				value = valueParts[4]
				tokenSupply = valueParts[5]
				metaTxId = valueParts[6]
				metaOutputIndex = valueParts[7]
			}
			break
		}
	}
	// err = ErrNotFound
	return
}

// GetByNftUTXO queries NFT addresses associated with UTXO ID
// key: outpoint+address value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
func (s *SimpleDB) GetByNftSellUTXO(utxoID string) (address string, codeHash string, genesis string, tokenIndex string, price string, contractAddress string, txID string, index string, value string, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(utxoID + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate, find first matching record
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Check if key starts with prefix
		if strings.HasPrefix(key, utxoID+"_") {
			// Extract address part (part after _)
			keyParts := strings.Split(key, "_")
			if len(keyParts) > 1 {
				address = keyParts[1]
			}

			// Get value corresponding to key
			// CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
			valueData := string(iter.Value())
			valueParts := strings.Split(valueData, "@")
			if len(valueParts) >= 8 {
				codeHash = valueParts[0]
				genesis = valueParts[1]
				tokenIndex = valueParts[2]
				price = valueParts[3]
				contractAddress = valueParts[4]
				txID = valueParts[5]
				index = valueParts[6]
				value = valueParts[7]
			}
			break
		}
	}
	// err = ErrNotFound
	return
}

// GetAddressNftUtxoByKey gets NFT UTXO list by key (address)
func (s *SimpleDB) GetAddressNftUtxoByKey(key string) (nftUtxoList []common.NftUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate, collect all matching records
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// fmt.Printf("[GETAddressNftUtxoByKey]key: %s\n", key)
		// Check if key starts with prefix
		if !strings.HasPrefix(key, string(prefix)) {
			break
		}

		utxo := common.NftUtxo{}
		// Extract outpoint (part before _)
		keyParts := strings.Split(key, "_")
		if len(keyParts) > 1 {
			outpoint := keyParts[1]
			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) == 2 {
				utxo.TxID = outpointParts[0]
				utxo.Index = outpointParts[1]
				utxo.UtxoId = outpoint
			}
		}
		// mempoolAddressNftIncomeValidStore - key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
		// mempoolAddressNftSpendDB - key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp@usedTxId

		// Get value corresponding to key
		// CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
		valueData := string(iter.Value())
		// fmt.Printf("[GETAddressNftUtxoByKey]valueData: %s\n", valueData)
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 9 {
			utxo.CodeHash = valueParts[0]
			utxo.Genesis = valueParts[1]
			utxo.SensibleId = valueParts[2]
			utxo.TokenIndex = valueParts[3]
			utxo.Index = valueParts[4]
			utxo.Value = valueParts[5]
			utxo.TokenSupply = valueParts[6]
			utxo.MetaTxId = valueParts[7]
			utxo.MetaOutputIndex = valueParts[8]
			if len(valueParts) > 9 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[9], 10, 64)
			}
			if len(valueParts) > 10 {
				utxo.UsedTxId = valueParts[10]
			}
		}
		// Extract address from key (part after _)
		if len(keyParts) > 1 {
			utxo.Address = keyParts[1]
		}

		nftUtxoList = append(nftUtxoList, utxo)
	}
	return
}

// GetCodeHashGenesisNftUtxoByKey gets NFT UTXO list by key (codeHashGenesis)
func (s *SimpleDB) GetCodeHashGenesisNftUtxoByKey(key string) (nftUtxoList []common.NftUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate, collect all matching records
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Check if key starts with prefix
		if !strings.HasPrefix(key, string(prefix)) {
			break
		}

		utxo := common.NftUtxo{}
		// Extract outpoint (part before _)
		keyParts := strings.Split(key, "_")
		if len(keyParts) > 1 {
			outpoint := keyParts[1]
			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) == 2 {
				utxo.TxID = outpointParts[0]
				utxo.Index = outpointParts[1]
				utxo.UtxoId = outpoint
			}
		}
		// mempoolCodeHashGenesisNftIncomeValidStore - key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
		// mempoolCodeHashGenesisNftSpendStore - key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp@usedTxId

		// Get value corresponding to key
		// NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 7 {
			utxo.Address = valueParts[0]
			utxo.TokenIndex = valueParts[1]
			utxo.Index = valueParts[2]
			utxo.Value = valueParts[3]
			utxo.TokenSupply = valueParts[4]
			utxo.MetaTxId = valueParts[5]
			utxo.MetaOutputIndex = valueParts[6]
			if len(valueParts) > 7 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[7], 10, 64)
			}
			if len(valueParts) > 8 {
				utxo.UsedTxId = valueParts[8]
			}
		}
		// Extract codeHashGenesis from key (part after _)
		codeHashGenesis := keyParts[0]
		codeHashGenesisParts := strings.Split(codeHashGenesis, "@")
		if len(codeHashGenesisParts) == 2 {
			utxo.CodeHash = codeHashGenesisParts[0]
			utxo.Genesis = codeHashGenesisParts[1]
		}

		nftUtxoList = append(nftUtxoList, utxo)
	}
	return
}

// GetNftUtxoByOutpoint gets NFT UTXO list by outpoint
// key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
func (s *SimpleDB) GetNftUtxoByOutpoint(outpoint string) (nftUtxoList []common.NftUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(outpoint + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate, collect all matching records
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Check if key starts with prefix
		if !strings.HasPrefix(key, outpoint+"_") {
			break
		}

		utxo := common.NftUtxo{}
		// Extract outpoint
		keyParts := strings.Split(key, "_")
		if len(keyParts) > 0 {
			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) == 2 {
				utxo.TxID = outpointParts[0]
				utxo.Index = outpointParts[1]
				utxo.UtxoId = outpoint
			}
		}

		// Get value corresponding to key
		// Cvalue: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 9 {
			utxo.CodeHash = valueParts[0]
			utxo.Genesis = valueParts[1]
			utxo.SensibleId = valueParts[2]
			utxo.TokenIndex = valueParts[3]
			utxo.Index = valueParts[4]
			utxo.Value = valueParts[5]
			utxo.TokenSupply = valueParts[6]
			utxo.MetaTxId = valueParts[7]
			utxo.MetaOutputIndex = valueParts[8]
			if len(valueParts) > 9 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[9], 10, 64)
			}
		}
		// Extract address from key (part after _)
		if len(keyParts) > 1 {
			utxo.Address = keyParts[1]
		}

		nftUtxoList = append(nftUtxoList, utxo)
	}
	return
}

// GetNftUtxo gets all NFT UTXO
func (s *SimpleDB) GetNftUtxo() (nftUtxoList []common.NftUtxo, err error) {
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate through all records
	for iter.First(); iter.Valid(); iter.Next() {
		utxo := common.NftUtxo{}
		// Extract outpoint
		keyParts := strings.Split(string(iter.Key()), ":")
		if len(keyParts) == 2 {
			utxo.TxID = keyParts[0]
			utxo.Index = keyParts[1]
			utxo.UtxoId = string(iter.Key())
		}

		// Get value corresponding to key
		// NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 10 {
			utxo.Address = valueParts[0]
			utxo.CodeHash = valueParts[1]
			utxo.Genesis = valueParts[2]
			utxo.SensibleId = valueParts[3]
			utxo.TokenIndex = valueParts[4]
			utxo.Index = valueParts[5]
			utxo.Value = valueParts[6]
			utxo.TokenSupply = valueParts[7]
			utxo.MetaTxId = valueParts[8]
			utxo.MetaOutputIndex = valueParts[9]
			if len(valueParts) > 10 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[10], 10, 64)
			}
		}
		nftUtxoList = append(nftUtxoList, utxo)
	}
	return
}

// DeleteNftSpendRecord deletes NFT spend record by outpoint
func (s *SimpleDB) DeleteNftSpendRecord(outpoint string) error {
	// Use prefix query to find all matching keys
	prefix := []byte(outpoint + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	batch := s.db.NewBatch()
	defer batch.Close()

	// Iterate, collect all keys that need to be deleted
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Check if key starts with prefix
		if !strings.HasPrefix(key, outpoint+"_") {
			break
		}
		if err := batch.Delete([]byte(key), nil); err != nil {
			return err
		}
	}

	return batch.Commit(nil)
}

// GetAddressSellNftUtxoByKey gets NFT sell UTXO list by address key
// value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
func (s *SimpleDB) GetAddressSellNftUtxoByKey(key string) (nftUtxoList []common.NftUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate, collect all matching records
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Check if key starts with prefix
		if !strings.HasPrefix(key, string(prefix)) {
			break
		}

		utxo := common.NftUtxo{}
		// Extract outpoint (part before _)
		keyParts := strings.Split(key, "_")
		if len(keyParts) > 0 {
			outpoint := keyParts[0]
			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) == 2 {
				utxo.TxID = outpointParts[0]
				utxo.Index = outpointParts[1]
				utxo.UtxoId = outpoint
			}
		}

		// Get value corresponding to key
		// CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
		// mempoolAddressSellNftIncomeStore - key: outpoint+address value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
		// mempoolAddressSellNftSpendStore - key: outpoint+address value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp@usedTxId
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 8 {
			utxo.CodeHash = valueParts[0]
			utxo.Genesis = valueParts[1]
			utxo.TokenIndex = valueParts[2]
			utxo.Price = valueParts[3]
			utxo.ContractAddress = valueParts[4]
			utxo.TxID = valueParts[5]
			utxo.Index = valueParts[6]
			utxo.Value = valueParts[7]
			if len(valueParts) > 8 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[8], 10, 64)
			}
			if len(valueParts) > 9 {
				utxo.UsedTxId = valueParts[9]
			}
		}
		// Extract address from key (part after _)
		if len(keyParts) > 1 {
			utxo.Address = keyParts[1]
		}

		nftUtxoList = append(nftUtxoList, utxo)
	}
	return
}

// GetCodeHashGenesisSellNftUtxoByKey gets NFT sell UTXO list by codeHash@genesis key
// value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
func (s *SimpleDB) GetCodeHashGenesisSellNftUtxoByKey(key string) (nftUtxoList []common.NftUtxo, err error) {
	// Use prefix query to find all matching keys
	prefix := []byte(key + "_")
	// Create iterator
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
	})
	if err != nil {
		return
	}
	defer iter.Close()

	// Iterate, collect all matching records
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Check if key starts with prefix
		if !strings.HasPrefix(key, string(prefix)) {
			break
		}

		utxo := common.NftUtxo{}
		// Extract outpoint (part before _)
		keyParts := strings.Split(key, "_")
		if len(keyParts) > 0 {
			outpoint := keyParts[0]
			outpointParts := strings.Split(outpoint, ":")
			if len(outpointParts) == 2 {
				utxo.TxID = outpointParts[0]
				utxo.Index = outpointParts[1]
				utxo.UtxoId = outpoint
			}
		}

		// Get value corresponding to key
		// NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
		// mempoolCodeHashGenesisSellNftIncomeStore - key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
		// mempoolCodeHashGenesisSellNftSpendStore - key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp@usedTxId
		valueData := string(iter.Value())
		valueParts := strings.Split(valueData, "@")
		if len(valueParts) >= 7 {
			utxo.Address = valueParts[0]
			utxo.TokenIndex = valueParts[1]
			utxo.Price = valueParts[2]
			utxo.ContractAddress = valueParts[3]
			utxo.TxID = valueParts[4]
			utxo.Index = valueParts[5]
			utxo.Value = valueParts[6]
			if len(valueParts) > 7 {
				utxo.Timestamp, _ = strconv.ParseInt(valueParts[7], 10, 64)
			}
			if len(valueParts) > 8 {
				utxo.UsedTxId = valueParts[8]
			}
		}
		// Extract codeHash@genesis from key prefix
		if len(keyParts) > 1 {
			codeHashGenesisParts := strings.Split(keyParts[1], "@")
			if len(codeHashGenesisParts) >= 2 {
				utxo.CodeHash = codeHashGenesisParts[0]
				utxo.Genesis = codeHashGenesisParts[1]
			}
		}

		nftUtxoList = append(nftUtxoList, utxo)
	}
	return
}

func (s *SimpleDB) TestData() {
	iter, err := s.db.NewIter(nil)
	if err != nil {
		fmt.Println("Error creating iterator:", err)
		return
	}
	defer iter.Close()
	fmt.Println("遍历所有 key/value：")
	for iter.First(); iter.Valid(); iter.Next() {
		fmt.Printf("key: %s, value: %s\n", string(iter.Key()), string(iter.Value()))
	}
}
