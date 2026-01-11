package indexer

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/contract/meta-contract/decoder"
	"github.com/metaid/utxo_indexer/storage"
)

// NftVerifyManager manages NFT-UTXO verification
type NftVerifyManager struct {
	indexer           *ContractNftIndexer
	verifyInterval    time.Duration // Verification interval
	stopChan          chan struct{} // Stop signal channel
	isRunning         bool
	mu                sync.RWMutex
	verifyBatchSize   int   // Number of verifications per batch
	verifyWorkerCount int   // Number of verification worker goroutines
	verifyCount       int64 // Number of verified UTXOs
}

// NewNftVerifyManager creates a new verification manager
func NewNftVerifyManager(indexer *ContractNftIndexer, verifyInterval time.Duration, batchSize, workerCount int) *NftVerifyManager {
	return &NftVerifyManager{
		indexer:           indexer,
		verifyInterval:    verifyInterval,
		stopChan:          make(chan struct{}),
		verifyBatchSize:   batchSize,
		verifyWorkerCount: workerCount,
	}
}

// Start starts the verification manager
func (m *NftVerifyManager) Start() error {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("Verification manager is already running")
	}
	m.isRunning = true
	m.mu.Unlock()

	go m.verifyLoop()
	return nil
}

// Stop stops the verification manager
func (m *NftVerifyManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	close(m.stopChan)
	m.isRunning = false
}

// verifyLoop verification loop
func (m *NftVerifyManager) verifyLoop() {
	ticker := time.NewTicker(m.verifyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			if err := m.verifyNftUtxos(); err != nil {
				log.Printf("Failed to verify NFT-UTXO: %v", err)
			}
		}
	}
}

// verifyNftUtxos verifies NFT-UTXO
func (m *NftVerifyManager) verifyNftUtxos() error {
	// Get unchecked UTXO data
	uncheckData := make(map[string]string)

	// Iterate through all shards
	// key: outpoint, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
	for _, db := range m.indexer.uncheckNftOutpointStore.GetShards() {
		iter, err := db.NewIter(&pebble.IterOptions{})
		if err != nil {
			return fmt.Errorf("Failed to create iterator: %w", err)
		}
		defer iter.Close()

		// Collect a batch of data
		count := 0
		for iter.First(); iter.Valid(); iter.Next() {
			//key: outpoint
			//value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
			key := string(iter.Key())
			value := string(iter.Value())
			uncheckData[key] = value
			count++

			if count >= m.verifyBatchSize {
				break
			}
		}

		if count >= m.verifyBatchSize {
			break
		}
	}

	if len(uncheckData) == 0 {
		return nil
	}

	// Create work channels
	utxoChan := make(chan struct {
		key   string
		value string
	}, len(uncheckData))
	resultChan := make(chan error, len(uncheckData))

	// Start worker goroutines
	for i := 0; i < m.verifyWorkerCount; i++ {
		go m.verifyWorker(utxoChan, resultChan)
	}

	// Send UTXOs to work channel
	for key, value := range uncheckData {
		utxoChan <- struct {
			key   string
			value string
		}{key, value}
	}
	close(utxoChan)

	// Collect results
	var errs []error
	for i := 0; i < len(uncheckData); i++ {
		if err := <-resultChan; err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("Errors occurred during verification: %v", errs)
	}
	return nil
}

// verifyWorker verification worker goroutine
func (m *NftVerifyManager) verifyWorker(utxoChan <-chan struct {
	key   string
	value string
}, resultChan chan<- error) {
	for utxo := range utxoChan {
		if err := m.verifyUtxo(utxo.key, utxo.value); err != nil {
			resultChan <- fmt.Errorf("Failed to verify UTXO: %w", err)
			continue
		}
		resultChan <- nil
	}
}

// verifyUtxo verifies a single UTXO
func (m *NftVerifyManager) verifyUtxo(outpoint, utxoData string) error {
	// Parse UTXO data
	// utxoData: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
	utxoParts := strings.Split(utxoData, "@")
	if len(utxoParts) < 12 {
		return fmt.Errorf("Invalid UTXO data format: %s", utxoData)
	}
	heightStr := utxoParts[11]
	height, err := strconv.Atoi(heightStr)
	if err != nil {
		return fmt.Errorf("Invalid block height: %s", heightStr)
	}

	lastHeight, err := m.indexer.GetLastIndexedHeight()
	if err != nil {
		return fmt.Errorf("Failed to get last indexed height: %w", err)
	}
	if height > lastHeight {
		return nil
	}

	// Parse outpoint to get txId
	txId := strings.Split(outpoint, ":")[0]

	// Get transaction that uses this UTXO from usedNftIncomeStore
	usedData, err := m.indexer.usedNftIncomeStore.Get([]byte(txId))
	if err != nil {
		if err == storage.ErrNotFound {
			err = m.indexer.invalidNftOutpointStore.Set([]byte(outpoint), []byte(utxoData+"@not_used-usedNftIncomeStore_not_found"))
			if err != nil {
				return errors.New("Failed to set invalid UTXO: " + err.Error())
			}
			// If no usage record found, UTXO is unused and can be deleted
			err = m.indexer.uncheckNftOutpointStore.Delete([]byte(outpoint))
			if err != nil {
				return errors.New("Failed to delete unchecked UTXO: " + err.Error())
			}

			return nil
		}
		return errors.New("Failed to get usage record: " + err.Error())
	}

	// fmt.Printf("[BLOCK-VERIFY]usedData: %s\n", string(usedData))

	// Get key information of UTXO
	nftAddress := utxoParts[0]
	codeHash := utxoParts[1]
	genesis := utxoParts[2]
	utxoSensibleId := utxoParts[3]

	//init genesis utxo
	if utxoSensibleId == "000000000000000000000000000000000000000000000000000000000000000000000000" {
		// Match successful, add UTXO to addressNftIncomeValidStore
		if err := m.addToValidStore(nftAddress, utxoData); err != nil {
			return errors.New("Failed to add valid UTXO data: " + err.Error())
		}
		return m.indexer.uncheckNftOutpointStore.Delete([]byte(outpoint))
	}

	genesisTxId, genesisIndex, err := decoder.ParseSensibleId(utxoSensibleId)
	if err != nil {
		return errors.New("Failed to parse sensibleId: " + err.Error())
	}
	tokenHash := ""
	tokenCodeHash := ""
	genesisHash := ""
	genesisCodeHash := ""
	sensibleId := ""

	usedGenesisOutpoint := genesisTxId + ":" + strconv.Itoa(int(genesisIndex))

	// fmt.Printf("[BLOCK-VERIFY]usedGenesisOutpoint: %s\n", usedGenesisOutpoint)
	//Get initial genesis UTXO from contractNftGenesisStore
	genesisUtxo, err := m.indexer.contractNftGenesisStore.Get([]byte(usedGenesisOutpoint))
	if err != nil {
		fmt.Printf("[BLOCK]Failed to get initial genesis UTXO [%s][%s]: %s", outpoint, usedGenesisOutpoint, err.Error())
		return err
	}

	// fmt.Printf("[BLOCK-VERIFY]genesisUtxo: %s\n", string(genesisUtxo))

	//sensibleId@tokenSupply@codeHash@genesis@MetaTxId@MetaOutputIndex
	if len(genesisUtxo) > 0 {
		//If exists, get from contractNftGenesisOutputStore
		// key:usedOutpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
		genesisOutputs, err := m.indexer.contractNftGenesisOutputStore.Get([]byte(usedGenesisOutpoint))
		if err != nil {
			return errors.New(fmt.Sprintf("Failed to get initial genesis UTXO [%s][%s]: %s", outpoint, usedGenesisOutpoint, err.Error()))
		}
		fmt.Printf("[BLOCK-VERIFY]genesisOutputs: %s\n", string(genesisOutputs))
		if len(genesisOutputs) > 0 {
			genesisOutputParts := strings.Split(string(genesisOutputs), ",")
			for _, genesisOutput := range genesisOutputParts {
				if genesisOutput == "" {
					continue
				}
				//sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex
				genesisOutputParts := strings.Split(genesisOutput, "@")
				if len(genesisOutputParts) < 10 {
					continue
				}

				// if genesisOutputParts[2] == codeHash {

				// }
				tokenIndex := genesisOutputParts[4]

				if tokenIndex == "0" {
					//token
					tokenHash = genesisOutputParts[3]
					tokenCodeHash = genesisOutputParts[2]
					sensibleId = genesisOutputParts[0]
				} else if tokenIndex == "1" {
					//new genesis
					genesisHash = genesisOutputParts[3]
					genesisCodeHash = genesisOutputParts[2]
					sensibleId = genesisOutputParts[0]
				}
			}
		}
	}

	// fmt.Printf("[BLOCK-VERIFY]tokenCodeHash: %s, tokenHash: %s, sensibleId: %s, genesisHash: %s, genesisCodeHash: %s, genesisTxId: %s\n", tokenCodeHash, tokenHash, sensibleId, genesisHash, genesisCodeHash, genesisTxId)

	hasMatch := false
	// Parse usage records
	// value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
	usedList := strings.Split(string(usedData), ",")
	for _, used := range usedList {
		if used == "" {
			continue
		}

		//value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@heigh
		usedParts := strings.Split(used, "@")
		if len(usedParts) < 12 {
			continue
		}

		// Check if codeHash, genesis and sensibleId match
		if usedParts[1] == codeHash && usedParts[2] == genesis && usedParts[3] == utxoSensibleId {
			hasMatch = true
			fmt.Printf("[BLOCK]Successfully matched inputs and output: %s\n", outpoint)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
		if usedParts[1] == tokenCodeHash && usedParts[2] == tokenHash && usedParts[3] == sensibleId {
			hasMatch = true
			fmt.Printf("[BLOCK]Successfully matched inputs and token: %s\n", outpoint)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
		if usedParts[1] == genesisCodeHash && usedParts[2] == genesisHash && usedParts[3] == sensibleId {
			hasMatch = true
			fmt.Printf("[BLOCK]Successfully matched inputs and genesis: %s\n", outpoint)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
		if usedParts[5] == genesisTxId {
			hasMatch = true
			fmt.Printf("[BLOCK]Successfully matched inputs and genesisTxId: %s\n", outpoint)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
	}
	if !hasMatch {
		fmt.Printf("[BLOCK][Failed]Match failed: %s\n", outpoint)
		fmt.Printf("[BLOCK][Failed]codeHash: %s, genesis: %s, utxoSensibleId: %s\n", codeHash, genesis, utxoSensibleId)
		fmt.Printf("[BLOCK][Failed]tokenCodeHash: %s, tokenHash: %s, sensibleId: %s, genesisHash: %s, genesisCodeHash: %s, genesisTxId: %s\n", tokenCodeHash, tokenHash, sensibleId, genesisHash, genesisCodeHash, genesisTxId)
		err = m.indexer.invalidNftOutpointStore.Set([]byte(outpoint), []byte(utxoData+"@not_used-not_match"))
		if err != nil {
			return errors.New("Failed to set invalid UTXO: " + err.Error())
		}
	}

	// Delete verified UTXO
	return m.indexer.uncheckNftOutpointStore.Delete([]byte(outpoint))
}

// addToValidStore adds UTXO to valid storage
// utxoData: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
func (m *NftVerifyManager) addToValidStore(nftAddress, utxoData string) error {
	// Extract txId and index from utxoData
	// utxoData: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@heightt
	utxoParts := strings.Split(utxoData, "@")
	if len(utxoParts) < 12 {
		return fmt.Errorf("Invalid UTXO data format: %s", utxoData)
	}

	//newValue: CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,
	newValue := common.ConcatBytesOptimized(
		[]string{
			utxoParts[1],
			utxoParts[2],
			utxoParts[4],
			utxoParts[5],
			utxoParts[6],
			utxoParts[7],
			utxoParts[8],
			utxoParts[9],
			utxoParts[10],
			utxoParts[11],
		},
		"@",
	)
	outpoint := utxoParts[5] + ":" + utxoParts[6]

	// Use BulkMergeMapConcurrent method to merge data
	mergeMap := make(map[string][]string)
	mergeMap[nftAddress] = []string{newValue}

	err := m.indexer.addressNftIncomeValidStore.BulkMergeMapConcurrent(&mergeMap, 1)
	if err != nil {
		// return errors.New("Failed to merge and update valid UTXO data: " + err.Error())
		fmt.Printf("[BLOCK]Failed to merge and update address valid NFT UTXO data: %s %s\n", nftAddress, outpoint)
	} else {
		fmt.Printf("[BLOCK]Added address valid NFT UTXO: %s %s\n", nftAddress, outpoint)
	}

	// Process codeHashGenesisNftIncomeValidStore
	// codeHash@genesis, value: NftAddress@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
	codeHashGenesisNftIncomeKey := common.ConcatBytesOptimized([]string{utxoParts[1], utxoParts[2]}, "@")
	codeHashGenesisNftIncomeValue := common.ConcatBytesOptimized([]string{
		nftAddress,
		utxoParts[4],
		utxoParts[5],
		utxoParts[6],
		utxoParts[7],
		utxoParts[8],
		utxoParts[9],
		utxoParts[10],
		utxoParts[11],
	}, "@")
	mergeCodeHashGenesisMap := make(map[string][]string)
	mergeCodeHashGenesisMap[codeHashGenesisNftIncomeKey] = []string{codeHashGenesisNftIncomeValue}
	err = m.indexer.codeHashGenesisNftIncomeValidStore.BulkMergeMapConcurrent(&mergeCodeHashGenesisMap, 1)
	if err != nil {
		return errors.New("Failed to merge and update codeHashGenesis valid NFT UTXO data: " + err.Error())
		// fmt.Printf("[BLOCK]Failed to merge and update valid NFT UTXO data: %s %s\n", codeHashGenesisNftIncomeKey, codeHashGenesisNftIncomeValue)
	}
	fmt.Printf("[BLOCK]Added codeHashGenesis valid NFT UTXO: %s %s\n", codeHashGenesisNftIncomeKey, codeHashGenesisNftIncomeValue)

	// Process contractNftOwnersIncomeValidStore
	// key: codeHash@genesis, value: address@tokenIndex@txId@index,...
	contractNftOwnersIncomeKey := common.ConcatBytesOptimized([]string{utxoParts[1], utxoParts[2]}, "@")
	contractNftOwnersIncomeValue := common.ConcatBytesOptimized([]string{
		nftAddress,
		utxoParts[4],
		utxoParts[5],
		utxoParts[6],
	}, "@")
	mergeOwnersMap := make(map[string][]string)
	mergeOwnersMap[contractNftOwnersIncomeKey] = []string{contractNftOwnersIncomeValue}
	err = m.indexer.contractNftOwnersIncomeValidStore.BulkMergeMapConcurrent(&mergeOwnersMap, 1)
	if err != nil {
		return errors.New("Failed to merge and update contractNftOwners valid income data: " + err.Error())
	}
	fmt.Printf("[BLOCK]Added contractNftOwners valid income: %s %s\n", contractNftOwnersIncomeKey, contractNftOwnersIncomeValue)

	return nil
}
