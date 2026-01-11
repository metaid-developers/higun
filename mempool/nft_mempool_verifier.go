package mempool

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/contract/meta-contract/decoder"
	"github.com/metaid/utxo_indexer/storage"
)

// NftMempoolVerifier manages verification of NFT-UTXO in mempool
type NftMempoolVerifier struct {
	mempoolManager    *NftMempoolManager
	verifyInterval    time.Duration // Verification interval
	stopChan          chan struct{} // Stop signal channel
	isRunning         bool
	mu                sync.RWMutex
	verifyBatchSize   int   // Number of verifications per batch
	verifyWorkerCount int   // Number of verification worker goroutines
	verifyCount       int64 // Number of verified UTXOs
}

// NewNftMempoolVerifier creates a new mempool verification manager
func NewNftMempoolVerifier(mempoolManager *NftMempoolManager, verifyInterval time.Duration, batchSize, workerCount int) *NftMempoolVerifier {
	return &NftMempoolVerifier{
		mempoolManager:    mempoolManager,
		verifyInterval:    verifyInterval,
		stopChan:          make(chan struct{}),
		verifyBatchSize:   batchSize,
		verifyWorkerCount: workerCount,
	}
}

// Start starts the verification manager
func (m *NftMempoolVerifier) Start() error {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("verification manager is already running")
	}
	m.isRunning = true
	m.mu.Unlock()

	go m.verifyLoop()
	return nil
}

// Stop stops the verification manager
func (m *NftMempoolVerifier) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	close(m.stopChan)
	m.isRunning = false
}

// verifyLoop verification loop
func (m *NftMempoolVerifier) verifyLoop() {
	ticker := time.NewTicker(m.verifyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			if err := m.verifyMempoolNftUtxos(); err != nil {
				log.Printf("Failed to verify mempool NFT-UTXO: %v", err)
			}
		}
	}
}

// verifyMempoolNftUtxos verifies NFT-UTXO in mempool
func (m *NftMempoolVerifier) verifyMempoolNftUtxos() error {
	// Get all UTXOs that need verification
	uncheckData := make(map[string]string)

	if m.mempoolManager == nil {
		return fmt.Errorf("mempoolManager is nil")
	}
	if m.mempoolManager.mempoolUncheckNftOutpointStore == nil {
		return fmt.Errorf("mempoolUncheckNftOutpointStore is nil")
	}
	// Get all unchecked UTXOs
	utxoList, err := m.mempoolManager.mempoolUncheckNftOutpointStore.GetNftUtxo()
	if err != nil {
		return fmt.Errorf("Failed to get unchecked UTXOs: %w", err)
	}

	// Collect a batch of data
	count := 0
	for _, utxo := range utxoList {
		outpoint := utxo.TxID + ":" + utxo.Index
		//value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
		utxoData := common.ConcatBytesOptimized([]string{
			utxo.Address,
			utxo.CodeHash,
			utxo.Genesis,
			utxo.SensibleId,
			utxo.TokenIndex,
			utxo.Index,
			utxo.Value,
			utxo.TokenSupply,
			utxo.MetaTxId,
			utxo.MetaOutputIndex,
		}, "@")
		uncheckData[outpoint] = utxoData
		count++

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
func (m *NftMempoolVerifier) verifyWorker(utxoChan <-chan struct {
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
// utxoData: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
func (m *NftMempoolVerifier) verifyUtxo(outpoint, utxoData string) error {
	// Parse outpoint to get txId
	txId := strings.Split(outpoint, ":")[0]

	_, err := m.mempoolManager.mempoolVerifyTxStore.GetSimpleRecord(txId)
	if err != nil {
		if strings.Contains(err.Error(), storage.ErrNotFound.Error()) {
			return nil
		}
		return errors.New("Failed to get VerifyTx [" + txId + "][" + outpoint + "]: " + err.Error())
	}
	fmt.Printf("[MEMPOOL]Verifying mempool NFT UTXO: %s, %s\n", outpoint, utxoData)

	// Get transaction that uses this UTXO from mempoolUsedNftIncomeStore
	//key: usedTxId, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
	usedData, err := m.mempoolManager.mempoolUsedNftIncomeStore.GetSimpleRecord(txId)
	if err != nil {
		if strings.Contains(err.Error(), storage.ErrNotFound.Error()) {
			// If no usage record found, UTXO is unused and can be deleted
			return m.mempoolManager.mempoolUncheckNftOutpointStore.DeleteSimpleRecord(outpoint)
		}
		return err
	}

	// Parse UTXO data
	// utxoData: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
	utxoParts := strings.Split(utxoData, "@")
	if len(utxoParts) < 10 {
		return fmt.Errorf("Invalid UTXO data format: %s", utxoData)
	}

	// Get key information of UTXO
	nftAddress := utxoParts[0]
	codeHash := utxoParts[1]
	genesis := utxoParts[2]
	utxoSensibleId := utxoParts[3]

	// init genesis utxo
	if utxoSensibleId == "000000000000000000000000000000000000000000000000000000000000000000000000" {
		// Match successful, add UTXO to addressNftIncomeValidStore
		if err := m.addToValidStore(outpoint, nftAddress, utxoData); err != nil {
			return errors.New("Failed to add valid UTXO data: " + err.Error())
		}
		return m.mempoolManager.mempoolUncheckNftOutpointStore.DeleteSimpleRecord(outpoint)
	}

	genesisTxId, genesisIndex, err := decoder.ParseSensibleId(utxoSensibleId)
	if err != nil {
		return err
	}
	tokenHash := ""
	tokenCodeHash := ""
	genesisHash := ""
	genesisCodeHash := ""
	sensibleId := ""

	usedOutpoint := genesisTxId + ":" + strconv.Itoa(int(genesisIndex))

	// Get initial genesis UTXO from contractNftGenesisStore
	genesisUtxo, _ := m.mempoolManager.contractNftGenesisStore.Get([]byte(usedOutpoint))

	// If genesisUtxo has no data, get from mempoolContractNftGenesisStore
	if len(genesisUtxo) == 0 {
		genesisUtxo, err = m.mempoolManager.mempoolContractNftGenesisStore.GetSimpleRecord(usedOutpoint)
		if err != nil {
			fmt.Printf("[MEMPOOL]Failed to get genesisUtxo in mempoolContractNftGenesisStore: %s, %s\n", outpoint, utxoData)
			return err
		}
	}

	if len(genesisUtxo) > 0 {
		// If exists, get from contractNftGenesisOutputStore
		// key:usedOutpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
		genesisOutputs, _ := m.mempoolManager.contractNftGenesisOutputStore.Get([]byte(usedOutpoint))
		if len(genesisOutputs) == 0 {
			genesisOutputs, err = m.mempoolManager.mempoolContractNftGenesisOutputStore.GetSimpleRecord(usedOutpoint)

			if err != nil {
				fmt.Printf("[MEMPOOL]Failed to get genesisOutputs in mempoolContractNftGenesisOutputStore: %s, %s\n", outpoint, utxoData)
				return err
			}
		}
		if len(genesisOutputs) > 0 {
			genesisOutputParts := strings.Split(string(genesisOutputs), ",")
			for _, genesisOutput := range genesisOutputParts {
				if genesisOutput == "" {
					continue
				}
				// sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex
				genesisOutputParts := strings.Split(genesisOutput, "@")
				if len(genesisOutputParts) < 10 {
					continue
				}

				tokenIndex := genesisOutputParts[4]

				if tokenIndex == "0" {
					// token
					tokenHash = genesisOutputParts[3]
					tokenCodeHash = genesisOutputParts[2]
					sensibleId = genesisOutputParts[0]
				} else if tokenIndex == "1" {
					// new genesis
					genesisHash = genesisOutputParts[3]
					genesisCodeHash = genesisOutputParts[2]
					sensibleId = genesisOutputParts[0]
				}
			}
		}
	}

	// Parse usage records
	//
	usedList := strings.Split(string(usedData), ",")
	for _, used := range usedList {
		if used == "" {
			continue
		}
		// value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
		usedParts := strings.Split(used, "@")
		if len(usedParts) < 12 {
			continue
		}

		// Check if codeHash, genesis and sensibleId match
		if usedParts[1] == codeHash && usedParts[2] == genesis && usedParts[3] == utxoSensibleId {
			fmt.Printf("[MEMPOOL]Successfully matched inputs and output: %s, %s\n", outpoint, utxoData)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(outpoint, nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
		if usedParts[1] == tokenCodeHash && usedParts[2] == tokenHash && usedParts[3] == sensibleId {
			fmt.Printf("[MEMPOOL]Successfully matched inputs and token: %s, %s\n", outpoint, utxoData)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(outpoint, nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
		if usedParts[1] == genesisCodeHash && usedParts[2] == genesisHash && usedParts[3] == sensibleId {
			fmt.Printf("[MEMPOOL]Successfully matched inputs and genesis: %s, %s\n", outpoint, utxoData)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(outpoint, nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
		if usedParts[5] == genesisTxId {
			fmt.Printf("[MEMPOOL]Successfully matched inputs and genesisTxId: %s, %s\n", outpoint, utxoData)
			// Match successful, add UTXO to addressNftIncomeValidStore
			if err := m.addToValidStore(outpoint, nftAddress, utxoData); err != nil {
				return err
			}
			break
		}
	}

	// Delete verified UTXO
	return m.mempoolManager.mempoolUncheckNftOutpointStore.DeleteSimpleRecord(outpoint)
}

// addToValidStore adds UTXO to valid storage
// utxoData: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
func (m *NftMempoolVerifier) addToValidStore(outpoint, nftAddress, utxoData string) error {
	now := time.Now().UnixMilli()
	// value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
	utxoParts := strings.Split(utxoData, "@")
	if len(utxoParts) < 10 {
		return fmt.Errorf("Invalid UTXO data format: %s", utxoData)
	}
	// Process address NFT income valid storage
	// key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
	newValue := common.ConcatBytesOptimized([]string{
		utxoParts[1],
		utxoParts[2],
		utxoParts[3],
		utxoParts[4],
		utxoParts[5],
		utxoParts[6],
		utxoParts[7],
		utxoParts[8],
		utxoParts[9],
		strconv.FormatInt(now, 10),
	}, "@")
	err := m.mempoolManager.mempoolAddressNftIncomeValidStore.AddRecord(outpoint, nftAddress, []byte(newValue))
	if err != nil {
		fmt.Printf("[MEMPOOL]Failed to add valid address NFT UTXO data to addressNftIncomeValidStore: %s, %s\n", outpoint, nftAddress)
		// return fmt.Errorf("Failed to add valid UTXO data to addressNftIncomeValidStore: %w", err)
	} else {
		fmt.Printf("[MEMPOOL]Added valid address NFT UTXO data to addressNftIncomeValidStore: %s, %s\n", outpoint, nftAddress)
	}

	// Process codeHashGenesisNftIncomeValidStore
	// key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
	codeHashGenesisNftIncomeKey := common.ConcatBytesOptimized([]string{utxoParts[1], utxoParts[2]}, "@")
	codeHashGenesisNftIncomeValue := common.ConcatBytesOptimized([]string{
		utxoParts[0],
		utxoParts[4],
		utxoParts[5],
		utxoParts[6],
		utxoParts[7],
		utxoParts[8],
		utxoParts[9],
		strconv.FormatInt(now, 10),
	}, "@")
	err = m.mempoolManager.mempoolCodeHashGenesisNftIncomeValidStore.AddRecord(outpoint, codeHashGenesisNftIncomeKey, []byte(codeHashGenesisNftIncomeValue))
	if err != nil {
		// fmt.Printf("[MEMPOOL]Failed to add valid codeHashGenesis NFT UTXO data to codeHashGenesisNftIncomeValidStore: %s, %s\n", codeHashGenesisNftIncomeKey, nftAddress)
		return fmt.Errorf("Failed to add valid codeHashGenesis NFT UTXO data to codeHashGenesisNftIncomeValidStore: %w", err)
	} else {
		fmt.Printf("[MEMPOOL]Added valid codeHashGenesis NFT UTXO data to codeHashGenesisNftIncomeValidStore: %s, %s\n", outpoint, codeHashGenesisNftIncomeKey)
	}
	return nil
}
