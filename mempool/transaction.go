package mempool

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// MempoolManager manages mempool transactions
type MempoolManager struct {
	utxoStore       *storage.PebbleStore // Main UTXO storage, using sharding
	MempoolIncomeDB *storage.SimpleDB    // Mempool income database
	MempoolSpendDB  *storage.SimpleDB    // Mempool spend database
	chainCfg        *chaincfg.Params
	zmqClient       []*ZMQClient
	basePath        string // Data directory base path
}

// NewMempoolManager creates a new mempool manager
func NewMempoolManager(basePath string, utxoStore *storage.PebbleStore, chainCfg *chaincfg.Params, zmqAddress []string) *MempoolManager {
	mempoolIncomeDB, err := storage.NewSimpleDB(basePath + "/mempool_income")
	if err != nil {
		log.Printf("Failed to create mempool income database: %v", err)
		return nil
	}

	mempoolSpendDB, err := storage.NewSimpleDB(basePath + "/mempool_spend")
	if err != nil {
		log.Printf("Failed to create mempool spend database: %v", err)
		mempoolIncomeDB.Close()
		return nil
	}

	m := &MempoolManager{
		utxoStore:       utxoStore,
		MempoolIncomeDB: mempoolIncomeDB,
		MempoolSpendDB:  mempoolSpendDB,
		chainCfg:        chainCfg,
		basePath:        basePath,
	}

	// Create ZMQ client, no longer passing db
	m.zmqClient = NewZMQClient(zmqAddress, nil)

	// Add "rawtx" topic monitoring
	for _, client := range m.zmqClient {
		client.AddTopic("rawtx", m.HandleRawTransaction)
	}
	return m
}

// Start starts the mempool manager
func (m *MempoolManager) Start() error {
	for _, client := range m.zmqClient {
		client.Start()
	}
	return nil
}

// Stop stops the mempool manager
func (m *MempoolManager) Stop() {
	//m.zmqClient.Stop()
	for _, client := range m.zmqClient {
		client.Stop()
	}
	if m.MempoolIncomeDB != nil {
		m.MempoolIncomeDB.Close()
	}
	if m.MempoolSpendDB != nil {
		m.MempoolSpendDB.Close()
	}
}

// HandleRawTransaction processes raw transaction data
func (m *MempoolManager) HandleRawTransaction(topic string, data []byte) error {
	timeStr := strconv.FormatInt(time.Now().Unix(), 10)
	// 1. Parse raw transaction
	tx, err := DeserializeTransaction(data)
	if err != nil {
		return fmt.Errorf("Failed to parse transaction: %w", err)
	}
	// 2. Process transaction outputs, create new UTXOs
	err = m.processOutputs(tx, timeStr)
	if err != nil {
		return fmt.Errorf("Failed to process transaction outputs: %w", err)
	}

	// 3. Process transaction inputs, mark spent UTXOs
	err = m.processInputs(tx, timeStr)
	if err != nil {
		return fmt.Errorf("Failed to process transaction inputs: %w", err)
	}

	return nil
}

// processOutputs processes transaction outputs, creates new UTXOs
func (m *MempoolManager) processOutputs(tx *wire.MsgTx, timeStr string) error {
	txHash := tx.TxHash().String()
	if config.GlobalConfig.RPC.Chain == "mvc" {
		txHash, _ = blockchain.GetNewHash(tx)
	}

	// Process each output
	var utxoData []string
	for i, out := range tx.TxOut {
		address := blockchain.GetAddressFromScript("", out.PkScript, m.chainCfg, config.GlobalConfig.RPC.Chain)
		// Create UTXO index for each address
		outputIndex := strconv.Itoa(i)
		utxoID := txHash + ":" + outputIndex
		//fmt.Println("Start processing deposit--------->", utxoID, address, out.Value)
		value := strconv.FormatInt(out.Value, 10)
		// Build storage key: addr1_tx1:0 format, value: UTXO amount
		// Store UTXO -> address mapping to mempool income database
		//err := m.mempoolIncomeDB.AddRecord(utxoID, address, []byte(value))
		key := common.ConcatBytesOptimized([]string{address, utxoID, timeStr}, "_")
		err := m.MempoolIncomeDB.AddMempolRecord(key, []byte(value))
		if err != nil {
			log.Printf("Failed to store mempool UTXO index %s -> %s: %v", utxoID, address, err)
			continue
		}
		//address@amount@timestamp
		utxoData = append(utxoData, common.ConcatBytesOptimized([]string{address, value, timeStr}, "@"))
	}
	// Store to utxoStore
	m.utxoStore.Set([]byte(txHash), []byte(strings.Join(utxoData, ",")))
	return nil
}

// processInputs processes transaction inputs, marks spent UTXOs
func (m *MempoolManager) processInputs(tx *wire.MsgTx, timeStr string) error {
	// Skip mining transactions
	if IsCoinbaseTx(tx) {
		return nil
	}

	txHash := tx.TxHash().String()
	if config.GlobalConfig.RPC.Chain == "mvc" {
		txHash, _ = blockchain.GetNewHash(tx)
	}

	// Process each input
	for _, in := range tx.TxIn {
		// Build spent UTXO ID
		prevTxHash := in.PreviousOutPoint.Hash.String()
		prevOutputIndex := strconv.Itoa(int(in.PreviousOutPoint.Index))
		//find address
		address, err := m.GetUtxoAddress(prevTxHash, in.PreviousOutPoint.Index)
		if err != nil {
			continue
		}
		spentUtxoID := address + "_" + prevTxHash + ":" + prevOutputIndex + "_" + timeStr
		// Store spent UTXO ID to mempool spend database
		m.MempoolSpendDB.AddMempolRecord(spentUtxoID, []byte(txHash))
	}

	return nil
}
func (m *MempoolManager) GetUtxoAddress(txHash string, index uint32) (string, error) {
	utxostr, err := m.utxoStore.Get([]byte(txHash))
	if err != nil {
		return "", err
	}
	// Remove leading comma if present
	str := strings.TrimPrefix(string(utxostr), ",")
	utxos := strings.Split(str, ",")

	if len(utxos) <= int(index) {
		return "", fmt.Errorf("No UTXOs found for transaction %s", txHash)
	}
	address := strings.Split(utxos[index], "@")[0]
	return address, nil
}

// DeserializeTransaction deserializes byte array to transaction
func DeserializeTransaction(data []byte) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(wire.TxVersion)
	err := tx.Deserialize(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// ExtractAddressesFromOutput extracts addresses from transaction output
func ExtractAddressesFromOutput(out *wire.TxOut, chainCfg *chaincfg.Params) ([]string, error) {
	// Parse output script
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(out.PkScript, chainCfg)
	if err != nil {
		return nil, err
	}

	// Convert to string array
	result := make([]string, len(addresses))
	for i, addr := range addresses {
		result[i] = addr.String()
	}

	return result, nil
}

// IsCoinbaseTx checks if transaction is a mining transaction
func IsCoinbaseTx(tx *wire.MsgTx) bool {
	// Mining transaction has only one input
	if len(tx.TxIn) != 1 {
		return false
	}

	// Mining transaction's previous output hash is 0
	zeroHash := &wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
	}

	return tx.TxIn[0].PreviousOutPoint.Hash == zeroHash.Hash &&
		tx.TxIn[0].PreviousOutPoint.Index == zeroHash.Index
}

// ProcessNewBlockTxs processes transactions in new blocks, cleans mempool records
func (m *MempoolManager) ProcessNewBlockTxs(incomeUtxoList []common.Utxo, spendTxList []string) error {
	if len(incomeUtxoList) == 0 {
		return nil
	}

	//log.Printf("Processing %d transactions in new block, cleaning mempool records", len(incomeUtxoList))

	// Delete income
	for _, utxo := range incomeUtxoList {
		// 1. Delete related records from mempool income database
		//fmt.Println("delete", utxo.TxID, utxo.Address)

		//err := m.mempoolIncomeDB.DeleteRecord(utxo.TxID, utxo.Address)
		delKey := common.ConcatBytesOptimized([]string{utxo.Address, utxo.TxID}, "_")
		err := m.MempoolIncomeDB.DeleteMempolRecordByPreKey(delKey)
		if err != nil {
			log.Printf("Failed to delete mempool income record %s: %v", utxo.TxID, err)
		}
		//log.Printf("Cleaned mempool record for transaction %s", txid)
	}
	// Delete spend
	for _, txid := range spendTxList {
		parts := strings.Split(txid, ":")
		if len(parts) != 2 {
			continue
		}
		// convert output index string to int
		idx, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Printf("Invalid output index in spend tx %s: %v", txid, err)
			continue
		}
		address, err := m.GetUtxoAddress(parts[0], uint32(idx))
		if err != nil {
			continue
		}
		delKey := common.ConcatBytesOptimized([]string{address, txid}, "_")
		err = m.MempoolSpendDB.DeleteMempolRecordByPreKey(delKey)
		if err != nil {
			log.Printf("Failed to delete mempool spend record %s: %v", txid, err)
		}
	}
	return nil
}

// CleanByHeight cleans mempool records by block height
func (m *MempoolManager) CleanByHeight(height int, bcClient interface{}) error {
	log.Printf("Starting to clean mempool, processing to block height: %d", height)

	// Try to assert bcClient as blockchain.Client type
	client, ok := bcClient.(*blockchain.Client)
	if !ok {
		return fmt.Errorf("Unsupported blockchain client type")
	}

	// Get block hash at this height
	blockHash, err := client.GetBlockHash(int64(height))
	if err != nil {
		return fmt.Errorf("Failed to get block hash: %w", err)
	}

	// Get block details
	block, err := client.GetBlock(blockHash)
	if err != nil {
		return fmt.Errorf("Failed to get block information: %w", err)
	}

	// Extract incomeUtxo list
	var incomeUtxoList []common.Utxo
	var spendTxList []string
	for _, tx := range block.Tx {
		for _, in := range tx.Vin {
			id := in.Txid
			if id == "" {
				id = "0000000000000000000000000000000000000000000000000000000000000000"
			}
			spendTxList = append(spendTxList, common.ConcatBytesOptimized([]string{id, strconv.Itoa(int(in.Vout))}, ":"))
		}

		for k, out := range tx.Vout {
			address := blockchain.GetAddressFromScript(out.ScriptPubKey.Hex, nil, config.GlobalNetwork, config.GlobalConfig.RPC.Chain)
			txId := common.ConcatBytesOptimized([]string{tx.Txid, strconv.Itoa(k)}, ":")
			incomeUtxoList = append(incomeUtxoList, common.Utxo{TxID: txId, Address: address})
		}
	}
	// Clean mempool records
	return m.ProcessNewBlockTxs(incomeUtxoList, spendTxList)
}

// InitializeMempool fetches and processes all current mempool transactions from the node at startup
// This method runs asynchronously to avoid blocking the main program
func (m *MempoolManager) InitializeMempool(bcClient interface{}) {
	// Use a separate goroutine to avoid blocking the main program
	go func() {
		log.Printf("Starting mempool data initialization...")

		// Assert as blockchain.Client
		client, ok := bcClient.(*blockchain.Client)
		if !ok {
			log.Printf("Failed to initialize mempool: unsupported blockchain client type")
			return
		}

		// Get all transaction IDs in the mempool
		txids, err := client.GetRawMempool()
		if err != nil {
			log.Printf("Failed to get mempool transaction list: %v", err)
			return
		}

		log.Printf("Fetched %d mempool transactions from node, start processing...", len(txids))

		// Process transactions in batches, 500 per batch to avoid excessive memory usage
		batchSize := 500
		totalBatches := (len(txids) + batchSize - 1) / batchSize

		for batchIdx := 0; batchIdx < totalBatches; batchIdx++ {
			start := batchIdx * batchSize
			end := start + batchSize
			if end > len(txids) {
				end = len(txids)
			}

			// Process current batch
			currentBatch := txids[start:end]
			log.Printf("Processing mempool transaction batch %d/%d (%d transactions)", batchIdx+1, totalBatches, len(currentBatch))
			timeStr := strconv.FormatInt(time.Now().Unix(), 10)
			for _, txid := range currentBatch {
				// Get transaction details
				tx, err := client.GetRawTransaction(txid)
				if err != nil {
					log.Printf("Failed to get transaction details %s: %v", txid, err)
					continue
				}

				// Use existing transaction processing methods
				msgTx := tx.MsgTx()

				// Process outputs first (create new UTXOs)
				if err := m.processOutputs(msgTx, timeStr); err != nil {
					log.Printf("Failed to process transaction outputs %s: %v", txid, err)
					continue
				}

				// Then process inputs (mark spent UTXOs)
				if err := m.processInputs(msgTx, timeStr); err != nil {
					log.Printf("Failed to process transaction inputs %s: %v", txid, err)
					continue
				}
			}

			// After batch is processed, pause briefly to allow other programs to execute
			// Avoid sustained high load
			time.Sleep(10 * time.Millisecond)
		}

		log.Printf("Mempool data initialization complete, processed %d transactions in total", len(txids))
	}()
}

// CleanAllMempool cleans all mempool data for complete rebuild
func (m *MempoolManager) CleanAllMempool() error {
	log.Println("Resetting mempool data by deleting physical files...")

	// Save ZMQ address for later reconstruction
	// zmqAddress := ""
	// if m.zmqClient != nil {
	// 	zmqAddress = m.zmqClient.address
	// }

	// Use basePath and fixed table names to get database file paths
	incomeDbPath := m.basePath + "/mempool_income"
	spendDbPath := m.basePath + "/mempool_spend"

	// No longer try to detect database status, directly use defer and recover to handle possible panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Exception caught during cleanup: %v, continuing with file deletion", r)
		}
	}()

	// Safely close database connections
	log.Println("Closing existing mempool database connections...")
	// Use recover to avoid panic from repeated closing
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Error occurred while closing income database: %v", r)
			}
		}()
		if m.MempoolIncomeDB != nil {
			m.MempoolIncomeDB.Close()
		}
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Error occurred while closing spend database: %v", r)
			}
		}()
		if m.MempoolSpendDB != nil {
			m.MempoolSpendDB.Close()
		}
	}()

	// Delete physical files
	log.Printf("Deleting mempool income database: %s", incomeDbPath)
	if err := os.RemoveAll(incomeDbPath); err != nil {
		log.Printf("Failed to delete mempool income database: %v", err)
		// Recreate connection after failure
		newIncomeDB, err := storage.NewSimpleDB(incomeDbPath)
		if err != nil {
			log.Printf("Failed to recreate mempool income database: %v", err)
		} else {
			m.MempoolIncomeDB = newIncomeDB
		}
		return err
	}

	log.Printf("Deleting mempool spend database: %s", spendDbPath)
	if err := os.RemoveAll(spendDbPath); err != nil {
		log.Printf("Failed to delete mempool spend database: %v", err)
		// Recreate connection after failure
		newIncomeDB, err := storage.NewSimpleDB(incomeDbPath)
		if err != nil {
			log.Printf("Failed to recreate mempool income database: %v", err)
		} else {
			m.MempoolIncomeDB = newIncomeDB
		}
		newSpendDB, err := storage.NewSimpleDB(spendDbPath)
		if err != nil {
			log.Printf("Failed to recreate mempool spend database: %v", err)
		} else {
			m.MempoolSpendDB = newSpendDB
		}
		return err
	}

	// Recreate databases
	log.Println("Recreating mempool databases...")
	newIncomeDB, err := storage.NewSimpleDB(incomeDbPath)
	if err != nil {
		log.Printf("Failed to recreate mempool income database: %v", err)
		return err
	}

	newSpendDB, err := storage.NewSimpleDB(spendDbPath)
	if err != nil {
		newIncomeDB.Close()
		log.Printf("Failed to recreate mempool spend database: %v", err)
		return err
	}

	// Update database references
	m.MempoolIncomeDB = newIncomeDB
	m.MempoolSpendDB = newSpendDB
	zmqAddress := config.GlobalConfig.ZMQAddress
	m.zmqClient = NewZMQClient(zmqAddress, nil)
	// Add "rawtx" topic monitoring
	for _, client := range m.zmqClient {
		client.AddTopic("rawtx", m.HandleRawTransaction)
	}

	log.Println("Mempool data completely reset")
	return nil
}

// GetBasePath returns the base path for mempool data
func (m *MempoolManager) GetBasePath() string {
	return m.basePath
}

// RebuildMempool rebuilds the mempool data (deletes and reinitializes the database and ZMQ listening)
func (m *MempoolManager) RebuildMempool() error {
	log.Println("Resetting mempool data by deleting physical files...")

	// zmqAddress := ""
	// if m.zmqClient != nil {
	// 	zmqAddress = m.zmqClient.address
	// }

	incomeDbPath := m.basePath + "/mempool_income"
	spendDbPath := m.basePath + "/mempool_spend"

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Exception caught during rebuild: %v, continuing with file deletion", r)
		}
	}()

	log.Println("Closing existing mempool database connections...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Error occurred while closing income database: %v", r)
			}
		}()
		if m.MempoolIncomeDB != nil {
			m.MempoolIncomeDB.Close()
		}
	}()
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Error occurred while closing spend database: %v", r)
			}
		}()
		if m.MempoolSpendDB != nil {
			m.MempoolSpendDB.Close()
		}
	}()

	log.Printf("Deleting mempool income database: %s", incomeDbPath)
	if err := os.RemoveAll(incomeDbPath); err != nil {
		log.Printf("Failed to delete mempool income database: %v", err)
		return err
	}

	log.Printf("Deleting mempool spend database: %s", spendDbPath)
	if err := os.RemoveAll(spendDbPath); err != nil {
		log.Printf("Failed to delete mempool spend database: %v", err)
		return err
	}

	log.Println("Recreating mempool databases...")
	newIncomeDB, err := storage.NewSimpleDB(incomeDbPath)
	if err != nil {
		log.Printf("Failed to recreate mempool income database: %v", err)
		return err
	}
	newSpendDB, err := storage.NewSimpleDB(spendDbPath)
	if err != nil {
		newIncomeDB.Close()
		log.Printf("Failed to recreate mempool spend database: %v", err)
		return err
	}
	m.MempoolIncomeDB = newIncomeDB
	m.MempoolSpendDB = newSpendDB

	// if zmqAddress != "" {
	// 	log.Println("Recreating ZMQ client...")
	// 	m.zmqClient = NewZMQClient(zmqAddress, nil)
	// 	log.Println("Re-adding ZMQ listening topics...")
	// 	m.zmqClient.AddTopic("rawtx", m.HandleRawTransaction)
	// }
	zmqAddress := config.GlobalConfig.ZMQAddress
	m.zmqClient = NewZMQClient(zmqAddress, nil)
	// Add "rawtx" topic monitoring
	for _, client := range m.zmqClient {
		client.AddTopic("rawtx", m.HandleRawTransaction)
	}

	log.Println("Mempool data completely rebuilt")
	return nil
}
