package mempool

import (
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// NftMempoolManager manages NFT mempool transactions
type NftMempoolManager struct {
	contractNftUtxoStore                 *storage.PebbleStore // Stores contract data key: txID, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
	mempoolAddressNftIncomeDB            *storage.SimpleDB    // Mempool income database key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
	mempoolAddressNftSpendDB             *storage.SimpleDB    // Mempool spend database key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp@usedTxId
	mempoolCodeHashGenesisNftIncomeStore *storage.SimpleDB    // Mempool codeHash@genesis NFT income database key: outpoint+codeHashGenesis, value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
	mempoolCodeHashGenesisNftSpendStore  *storage.SimpleDB    // Mempool codeHash@genesis NFT spend database key: outpoint+codeHashGenesis, value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp@usedTxId

	mempoolAddressSellNftIncomeStore         *storage.SimpleDB // Mempool address sell NFT income database key: outpoint+address value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
	mempoolAddressSellNftSpendStore          *storage.SimpleDB // Mempool address sell NFT spend database key: outpoint+address value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp@usedTxId
	mempoolCodeHashGenesisSellNftIncomeStore *storage.SimpleDB // Mempool codeHash@genesis sell NFT income database key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
	mempoolCodeHashGenesisSellNftSpendStore  *storage.SimpleDB // Mempool codeHash@genesis sell NFT spend database key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp@usedTxId

	contractNftInfoStore                 *storage.PebbleStore // Stores contract info key: codeHash@genesis@TokenIndex, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
	contractNftSummaryInfoStore          *storage.PebbleStore // Stores contract summary info key: codeHash@genesis, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
	contractNftGenesisStore              *storage.PebbleStore // Stores contract genesis info key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@MetaTxId@MetaOutputIndex
	contractNftGenesisOutputStore        *storage.PebbleStore // Stores used contract genesis output info key: usedOutpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
	contractNftGenesisUtxoStore          *storage.PebbleStore // Stores contract genesis UTXO info key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}
	mempoolContractNftInfoStore          *storage.SimpleDB    // Mempool contract info database
	mempoolContractNftSummaryInfoStore   *storage.SimpleDB    // Mempool contract summary info database
	mempoolContractNftGenesisStore       *storage.SimpleDB    // Mempool contract genesis info database key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@MetaTxId@MetaOutputIndex
	mempoolContractNftGenesisOutputStore *storage.SimpleDB    // Mempool contract genesis output info database key: usedOutpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
	mempoolContractNftGenesisUtxoStore   *storage.SimpleDB    // Mempool contract genesis UTXO info database key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}

	mempoolAddressNftIncomeValidStore         *storage.SimpleDB // Mempool contract data database key: outpoint+address value: CodeHash@Genesis@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
	mempoolCodeHashGenesisNftIncomeValidStore *storage.SimpleDB // Mempool contract data database key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
	mempoolUncheckNftOutpointStore            *storage.SimpleDB // Mempool unchecked NFT outpoint database key: outpoint value: NftAddress@codeHash@genesis@sensibleId@TokenIndex@index@value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
	mempoolUsedNftIncomeStore                 *storage.SimpleDB // Mempool used NFT income database key: usedTxId, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...

	mempoolVerifyTxStore *storage.SimpleDB // key: txId, value: ""
	chainCfg             *chaincfg.Params
	zmqClient            *ZMQClient
	basePath             string // Data directory base path
}

// NewNftMempoolManager creates a new NFT mempool manager
func NewNftMempoolManager(basePath string,
	contractNftUtxoStore *storage.PebbleStore,
	contractNftInfoStore *storage.PebbleStore,
	contractNftSummaryInfoStore *storage.PebbleStore,
	contractNftGenesisStore *storage.PebbleStore,
	contractNftGenesisOutputStore *storage.PebbleStore,
	contractNftGenesisUtxoStore *storage.PebbleStore,
	chainCfg *chaincfg.Params, zmqAddress string) *NftMempoolManager {
	// Create mempool databases
	mempoolAddressNftIncomeDB, err := storage.NewSimpleDB(basePath + "/mempool_address_nft_income")
	if err != nil {
		log.Printf("Failed to create NFT mempool income database: %v", err)
		return nil
	}

	mempoolAddressNftSpendDB, err := storage.NewSimpleDB(basePath + "/mempool_address_nft_spend")
	if err != nil {
		log.Printf("Failed to create NFT mempool spend database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		return nil
	}
	mempoolCodeHashGenesisNftIncomeStore, err := storage.NewSimpleDB(basePath + "/mempool_codehash_genesis_nft_income")
	if err != nil {
		log.Printf("Failed to create NFT mempool codeHash genesis income database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		return nil
	}
	mempoolCodeHashGenesisNftSpendStore, err := storage.NewSimpleDB(basePath + "/mempool_codehash_genesis_nft_spend")
	if err != nil {
		log.Printf("Failed to create NFT mempool codeHash genesis spend database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		return nil
	}
	mempoolAddressSellNftIncomeStore, err := storage.NewSimpleDB(basePath + "/mempool_address_sell_nft_income")
	if err != nil {
		log.Printf("Failed to create NFT mempool address sell NFT income database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		return nil
	}
	mempoolAddressSellNftSpendStore, err := storage.NewSimpleDB(basePath + "/mempool_address_sell_nft_spend")
	if err != nil {
		log.Printf("Failed to create NFT mempool address sell NFT spend database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		return nil
	}
	mempoolCodeHashGenesisSellNftIncomeStore, err := storage.NewSimpleDB(basePath + "/mempool_codehash_genesis_sell_nft_income")
	if err != nil {
		log.Printf("Failed to create NFT mempool codeHash genesis sell NFT income database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		return nil
	}
	mempoolCodeHashGenesisSellNftSpendStore, err := storage.NewSimpleDB(basePath + "/mempool_codehash_genesis_sell_nft_spend")
	if err != nil {
		log.Printf("Failed to create NFT mempool codeHash genesis sell NFT spend database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		return nil
	}

	mempoolContractNftInfoStore, err := storage.NewSimpleDB(basePath + "/mempool_contract_nft_info")
	if err != nil {
		log.Printf("Failed to create NFT mempool info database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolContractNftSummaryInfoStore, err := storage.NewSimpleDB(basePath + "/mempool_contract_nft_summary_info")
	if err != nil {
		log.Printf("Failed to create NFT mempool summary info database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolContractNftGenesisStore, err := storage.NewSimpleDB(basePath + "/mempool_contract_nft_genesis")
	if err != nil {
		log.Printf("Failed to create NFT mempool genesis database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolContractNftGenesisOutputStore, err := storage.NewSimpleDB(basePath + "/mempool_contract_nft_genesis_output")
	if err != nil {
		log.Printf("Failed to create NFT mempool genesis output database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolContractNftGenesisUtxoStore, err := storage.NewSimpleDB(basePath + "/mempool_contract_nft_genesis_utxo")
	if err != nil {
		log.Printf("Failed to create NFT mempool genesis UTXO database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolAddressNftIncomeValidStore, err := storage.NewSimpleDB(basePath + "/mempool_address_nft_income_valid")
	if err != nil {
		log.Printf("Failed to create NFT mempool income valid database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolCodeHashGenesisNftIncomeValidStore, err := storage.NewSimpleDB(basePath + "/mempool_codehash_genesis_nft_income_valid")
	if err != nil {
		log.Printf("Failed to create NFT mempool codeHash genesis income valid database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolUncheckNftOutpointStore, err := storage.NewSimpleDB(basePath + "/mempool_uncheck_nft_outpoint")
	if err != nil {
		log.Printf("Failed to create NFT mempool unchecked NFT outpoint database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		mempoolCodeHashGenesisNftIncomeValidStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolUsedNftIncomeStore, err := storage.NewSimpleDB(basePath + "/mempool_used_nft_income")
	if err != nil {
		log.Printf("Failed to create NFT mempool used NFT income database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		mempoolCodeHashGenesisNftIncomeValidStore.Close()
		mempoolUncheckNftOutpointStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	mempoolVerifyTxStore, err := storage.NewSimpleDB(basePath + "/mempool_nft_verify_tx")
	if err != nil {
		log.Printf("Failed to create NFT mempool verify Tx database: %v", err)
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		mempoolCodeHashGenesisNftIncomeValidStore.Close()
		mempoolUncheckNftOutpointStore.Close()
		mempoolUsedNftIncomeStore.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		return nil
	}

	m := &NftMempoolManager{
		contractNftUtxoStore:                      contractNftUtxoStore,
		mempoolAddressNftIncomeDB:                 mempoolAddressNftIncomeDB,
		mempoolAddressNftSpendDB:                  mempoolAddressNftSpendDB,
		mempoolCodeHashGenesisNftIncomeStore:      mempoolCodeHashGenesisNftIncomeStore,
		mempoolCodeHashGenesisNftSpendStore:       mempoolCodeHashGenesisNftSpendStore,
		mempoolAddressSellNftIncomeStore:          mempoolAddressSellNftIncomeStore,
		mempoolAddressSellNftSpendStore:           mempoolAddressSellNftSpendStore,
		mempoolCodeHashGenesisSellNftIncomeStore:  mempoolCodeHashGenesisSellNftIncomeStore,
		mempoolCodeHashGenesisSellNftSpendStore:   mempoolCodeHashGenesisSellNftSpendStore,
		mempoolContractNftInfoStore:               mempoolContractNftInfoStore,
		mempoolContractNftSummaryInfoStore:        mempoolContractNftSummaryInfoStore,
		mempoolContractNftGenesisStore:            mempoolContractNftGenesisStore,
		mempoolContractNftGenesisOutputStore:      mempoolContractNftGenesisOutputStore,
		mempoolContractNftGenesisUtxoStore:        mempoolContractNftGenesisUtxoStore,
		contractNftInfoStore:                      contractNftInfoStore,
		contractNftSummaryInfoStore:               contractNftSummaryInfoStore,
		contractNftGenesisStore:                   contractNftGenesisStore,
		contractNftGenesisOutputStore:             contractNftGenesisOutputStore,
		contractNftGenesisUtxoStore:               contractNftGenesisUtxoStore,
		mempoolAddressNftIncomeValidStore:         mempoolAddressNftIncomeValidStore,
		mempoolCodeHashGenesisNftIncomeValidStore: mempoolCodeHashGenesisNftIncomeValidStore,
		mempoolUncheckNftOutpointStore:            mempoolUncheckNftOutpointStore,
		mempoolUsedNftIncomeStore:                 mempoolUsedNftIncomeStore,
		mempoolVerifyTxStore:                      mempoolVerifyTxStore,
		chainCfg:                                  chainCfg,
		basePath:                                  basePath,
	}

	// Create ZMQ client
	m.zmqClient = NewZMQClient([]string{zmqAddress}, nil)[0]

	// Add "rawtx" topic listener
	m.zmqClient.AddTopic("rawtx", m.HandleRawTransaction)

	return m
}

// Start starts the NFT mempool manager
func (m *NftMempoolManager) Start() error {
	return m.zmqClient.Start()
}

// Stop stops the NFT mempool manager
func (m *NftMempoolManager) Stop() {
	m.zmqClient.Stop()
	if m.mempoolAddressNftIncomeDB != nil {
		m.mempoolAddressNftIncomeDB.Close()
	}
	if m.mempoolAddressNftSpendDB != nil {
		m.mempoolAddressNftSpendDB.Close()
	}
	if m.mempoolCodeHashGenesisNftIncomeStore != nil {
		m.mempoolCodeHashGenesisNftIncomeStore.Close()
	}
	if m.mempoolCodeHashGenesisNftSpendStore != nil {
		m.mempoolCodeHashGenesisNftSpendStore.Close()
	}
	if m.mempoolAddressSellNftIncomeStore != nil {
		m.mempoolAddressSellNftIncomeStore.Close()
	}
	if m.mempoolAddressSellNftSpendStore != nil {
		m.mempoolAddressSellNftSpendStore.Close()
	}
	if m.mempoolCodeHashGenesisSellNftIncomeStore != nil {
		m.mempoolCodeHashGenesisSellNftIncomeStore.Close()
	}
	if m.mempoolCodeHashGenesisSellNftSpendStore != nil {
		m.mempoolCodeHashGenesisSellNftSpendStore.Close()
	}
	if m.mempoolContractNftInfoStore != nil {
		m.mempoolContractNftInfoStore.Close()
	}
	if m.mempoolContractNftSummaryInfoStore != nil {
		m.mempoolContractNftSummaryInfoStore.Close()
	}
	if m.mempoolContractNftGenesisStore != nil {
		m.mempoolContractNftGenesisStore.Close()
	}
	if m.mempoolContractNftGenesisOutputStore != nil {
		m.mempoolContractNftGenesisOutputStore.Close()
	}
	if m.mempoolContractNftGenesisUtxoStore != nil {
		m.mempoolContractNftGenesisUtxoStore.Close()
	}
	if m.mempoolAddressNftIncomeValidStore != nil {
		m.mempoolAddressNftIncomeValidStore.Close()
	}
	if m.mempoolCodeHashGenesisNftIncomeValidStore != nil {
		m.mempoolCodeHashGenesisNftIncomeValidStore.Close()
	}
	if m.mempoolUncheckNftOutpointStore != nil {
		m.mempoolUncheckNftOutpointStore.Close()
	}
	if m.mempoolUsedNftIncomeStore != nil {
		m.mempoolUsedNftIncomeStore.Close()
	}
	if m.mempoolCodeHashGenesisNftIncomeStore != nil {
		m.mempoolCodeHashGenesisNftIncomeStore.Close()
	}
	if m.mempoolCodeHashGenesisNftSpendStore != nil {
		m.mempoolCodeHashGenesisNftSpendStore.Close()
	}
	if m.mempoolVerifyTxStore != nil {
		m.mempoolVerifyTxStore.Close()
	}
}

// HandleRawTransaction handles raw transaction data
func (m *NftMempoolManager) HandleRawTransaction(topic string, data []byte) error {
	now := time.Now().UnixMilli()
	// 1. Parse raw transaction
	tx, err := DeserializeTransaction(data)
	if err != nil {
		return fmt.Errorf("Failed to parse transaction: %w", err)
	}

	txHash := tx.TxHash().String()
	if config.GlobalConfig.RPC.Chain == "mvc" {
		txHash, _ = blockchain.GetNewHash(tx)
	}

	// 2. Process transaction outputs, create new NFT UTXO
	isNftTx, err := m.processNftOutputs(tx, now)
	if err != nil {
		return fmt.Errorf("Failed to process NFT transaction outputs: %w", err)
	}
	if isNftTx {
		fmt.Printf("ZMQ received NFT transaction: %s\n", txHash)
	}

	// 3. Process transaction inputs, mark spent NFT UTXO
	err = m.processNftInputs(tx, now)
	if err != nil {
		return fmt.Errorf("Failed to process NFT transaction inputs: %w", err)
	}

	if isNftTx {
		// 4. Process VerifyTx
		err = m.processVerifyTx(tx)
		if err != nil {
			return fmt.Errorf("Failed to process VerifyTx: %w", err)
		}
	}

	return nil
}

func (m *NftMempoolManager) processVerifyTx(tx *wire.MsgTx) error {
	txHash := tx.TxHash().String()
	if config.GlobalConfig.RPC.Chain == "mvc" {
		txHash, _ = blockchain.GetNewHash(tx)
	}

	// First check if it exists in the main storage
	fmt.Printf("processVerifyTx txHash: %s\n", txHash)
	_, err := m.mempoolVerifyTxStore.GetSimpleRecord(txHash)
	if err != nil {
		if strings.Contains(err.Error(), storage.ErrNotFound.Error()) {
			// Not found in main storage, add to mempool storage
			err = m.mempoolVerifyTxStore.AddSimpleRecord(txHash, []byte(txHash))
			if err != nil {
				return fmt.Errorf("Failed to store VerifyTx: %w", err)
			} else {
				fmt.Printf("Successfully stored VerifyTx: %s\n", txHash)
			}
		} else {
			return fmt.Errorf("Failed to get VerifyTx: %w", err)
		}
	}

	return nil
}

// processNftOutputs processes NFT transaction outputs and creates new UTXO
func (m *NftMempoolManager) processNftOutputs(tx *wire.MsgTx, timestamp int64) (bool, error) {
	txHash := tx.TxHash().String()
	if config.GlobalConfig.RPC.Chain == "mvc" {
		txHash, _ = blockchain.GetNewHash(tx)
	}
	isNftTx := false

	// Process each output
	for i, out := range tx.TxOut {
		pkScriptStr := hex.EncodeToString(out.PkScript)

		outputIndex := strconv.Itoa(i)
		utxoID := txHash + ":" + outputIndex

		nftUtxoInfo, nftSellUtxoInfo, contractTypeStr, err := blockchain.ParseContractNftInfo(pkScriptStr, m.chainCfg)
		if err != nil {
			continue
		}
		if contractTypeStr == "nft" {
			isNftTx = true
			if nftUtxoInfo == nil {
				continue
			}

			// Create NFT UTXO index for each address
			nftAddress := nftUtxoInfo.Address
			valueStr := strconv.FormatInt(out.Value, 10)
			// Store NFT UTXO info, format: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
			tokenIndex := strconv.FormatUint(nftUtxoInfo.TokenIndex, 10)
			tokenSupply := strconv.FormatUint(nftUtxoInfo.TokenSupply, 10)
			mempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftUtxoInfo.CodeHash,
				nftUtxoInfo.Genesis,
				nftUtxoInfo.SensibleId,
				tokenIndex,
				outputIndex,
				valueStr,
				tokenSupply,
				nftUtxoInfo.MetaTxId,
				strconv.FormatUint(nftUtxoInfo.MetaOutputIndex, 10),
				strconv.FormatInt(timestamp, 10),
			}, "@")
			// CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
			err = m.mempoolAddressNftIncomeDB.AddRecord(utxoID, nftAddress, []byte(mempoolNftUtxo))
			if err != nil {
				log.Printf("[Mempool] Failed to store NFT mempool UTXO index %s -> %s: %v", utxoID, nftAddress, err)
				continue
			}

			// Process uncheckNftOutpointStore
			uncheckNftOutpointKey := utxoID
			// NftAddress@codeHash@genesis@sensibleId@TokenIndex@index@value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
			newMempoolNftUtxo := common.ConcatBytesOptimized([]string{nftAddress, mempoolNftUtxo}, "@")
			err = m.mempoolUncheckNftOutpointStore.AddSimpleRecord(uncheckNftOutpointKey, []byte(newMempoolNftUtxo))
			if err != nil {
				log.Printf("[Mempool] Failed to store NFT mempool unchecked UTXO index %s -> %s: %v", utxoID, nftAddress, err)
				continue
			}

			// 1. Process NFT info storage
			nftInfoKey := common.ConcatBytesOptimized([]string{nftUtxoInfo.CodeHash, nftUtxoInfo.Genesis, fmt.Sprintf("%030d", nftUtxoInfo.TokenIndex)}, "@")
			// First check if it exists in the main storage
			_, err = m.contractNftInfoStore.Get([]byte(nftInfoKey))
			if err == storage.ErrNotFound {
				// Not found in main storage, add to mempool storage
				// key: codeHash@genesis@TokenIndex, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
				nftInfoValue := common.ConcatBytesOptimized([]string{
					nftUtxoInfo.SensibleId,
					tokenSupply,
					nftUtxoInfo.MetaTxId,
					strconv.FormatUint(nftUtxoInfo.MetaOutputIndex, 10),
				}, "@")
				err = m.mempoolContractNftInfoStore.AddSimpleRecord(nftInfoKey, []byte(nftInfoValue))
				if err != nil {
					log.Printf("[Mempool] Failed to store NFT mempool info %s: %v", nftInfoKey, err)
				}
			}

			// 2. Process NFT summary info storage
			nftSummaryInfoKey := common.ConcatBytesOptimized([]string{nftUtxoInfo.CodeHash, nftUtxoInfo.Genesis}, "@")
			// First check if it exists in the main storage
			_, err = m.contractNftSummaryInfoStore.Get([]byte(nftSummaryInfoKey))
			if err == storage.ErrNotFound {
				// Not found in main storage, add to mempool storage
				// key: codeHash@genesis, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
				nftSummaryInfoValue := common.ConcatBytesOptimized([]string{
					nftUtxoInfo.SensibleId,
					tokenSupply,
					nftUtxoInfo.MetaTxId,
					strconv.FormatUint(nftUtxoInfo.MetaOutputIndex, 10),
				}, "@")
				err = m.mempoolContractNftSummaryInfoStore.AddSimpleRecord(nftSummaryInfoKey, []byte(nftSummaryInfoValue))
				if err != nil {
					log.Printf("[Mempool] Failed to store NFT mempool summary info %s: %v", nftSummaryInfoKey, err)
				}
			}

			// 3. Process initial genesis info storage
			if nftUtxoInfo.SensibleId == "000000000000000000000000000000000000000000000000000000000000000000000000" {
				genesisKey := common.ConcatBytesOptimized([]string{txHash, outputIndex}, ":")
				// First check if it exists in the main storage
				_, err = m.contractNftGenesisStore.Get([]byte(genesisKey))
				if err == storage.ErrNotFound {
					// Not found in main storage, add to mempool storage
					// key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@MetaTxId@MetaOutputIndex
					genesisValue := common.ConcatBytesOptimized([]string{
						nftUtxoInfo.SensibleId,
						tokenSupply,
						nftUtxoInfo.CodeHash,
						nftUtxoInfo.Genesis,
						nftUtxoInfo.MetaTxId,
						strconv.FormatUint(nftUtxoInfo.MetaOutputIndex, 10),
					}, "@")
					err = m.mempoolContractNftGenesisStore.AddSimpleRecord(genesisKey, []byte(genesisValue))
					if err != nil {
						log.Printf("[Mempool] Failed to store NFT mempool genesis info %s: %v", genesisKey, err)
					}
				}
			}

			// 4. Process new genesis UTXO storage
			if nftUtxoInfo.TokenIndex != 0 && nftUtxoInfo.MetaTxId == "0000000000000000000000000000000000000000000000000000000000000000" {
				genesisUtxoKey := common.ConcatBytesOptimized([]string{txHash, outputIndex}, ":")
				// First check if it exists in the main storage
				_, err = m.contractNftGenesisUtxoStore.Get([]byte(genesisUtxoKey))
				if err == storage.ErrNotFound {
					// Not found in main storage, add to mempool storage
					// key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}
					genesisUtxoValue := common.ConcatBytesOptimized([]string{
						nftUtxoInfo.SensibleId,
						tokenSupply,
						nftUtxoInfo.CodeHash,
						nftUtxoInfo.Genesis,
						tokenIndex,
						outputIndex,
						valueStr,
						nftUtxoInfo.MetaTxId,
						strconv.FormatUint(nftUtxoInfo.MetaOutputIndex, 10),
					}, "@")
					err = m.mempoolContractNftGenesisUtxoStore.AddSimpleRecord(genesisUtxoKey, []byte(genesisUtxoValue))
					if err != nil {
						log.Printf("[Mempool] Failed to store NFT mempool genesis UTXO %s: %v", genesisUtxoKey, err)
					}
				}
			}

			// 5. Process codeHash@genesis NFT income storage
			// key: outpoint+codeHashGenesis, value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
			codeHashGenesisKey := common.ConcatBytesOptimized([]string{nftUtxoInfo.CodeHash, nftUtxoInfo.Genesis}, "@")
			codeHashGenesisMempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftAddress,
				tokenIndex,
				outputIndex,
				valueStr,
				tokenSupply,
				nftUtxoInfo.MetaTxId,
				strconv.FormatUint(nftUtxoInfo.MetaOutputIndex, 10),
				strconv.FormatInt(timestamp, 10),
			}, "@")
			err = m.mempoolCodeHashGenesisNftIncomeStore.AddRecord(utxoID, codeHashGenesisKey, []byte(codeHashGenesisMempoolNftUtxo))
			if err != nil {
				log.Printf("[Mempool] Failed to store NFT mempool codeHash genesis UTXO index %s -> %s: %v", utxoID, codeHashGenesisKey, err)
				continue
			}
		} else if contractTypeStr == "nft_sell" {
			isNftTx = true
			if nftSellUtxoInfo == nil {
				continue
			}

			tokenIndex := strconv.FormatUint(nftSellUtxoInfo.TokenIndex, 10)
			valueStr := strconv.FormatInt(out.Value, 10)

			// key: outpoint+address value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
			addressKey := common.ConcatBytesOptimized([]string{nftSellUtxoInfo.Address}, "@")
			addressMempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftSellUtxoInfo.CodeHash,
				nftSellUtxoInfo.Genesis,
				tokenIndex,
				strconv.FormatUint(nftSellUtxoInfo.Price, 10),
				nftSellUtxoInfo.ContractAddress,
				txHash,
				outputIndex,
				valueStr,
				strconv.FormatInt(timestamp, 10),
			}, "@")
			err = m.mempoolAddressSellNftIncomeStore.AddRecord(utxoID, addressKey, []byte(addressMempoolNftUtxo))
			if err != nil {
				log.Printf("[Mempool] Failed to store NFT mempool address UTXO index %s -> %s: %v", utxoID, addressKey, err)
				continue
			}

			// key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp
			codeHashGenesisKey := common.ConcatBytesOptimized([]string{nftSellUtxoInfo.CodeHash, nftSellUtxoInfo.Genesis}, "@")
			codeHashGenesisMempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftSellUtxoInfo.Address,
				tokenIndex,
				outputIndex,
				strconv.FormatUint(nftSellUtxoInfo.Price, 10),
				nftSellUtxoInfo.ContractAddress,
				txHash,
				outputIndex,
				valueStr,
				strconv.FormatInt(timestamp, 10),
			}, "@")
			err = m.mempoolCodeHashGenesisNftIncomeStore.AddRecord(utxoID, codeHashGenesisKey, []byte(codeHashGenesisMempoolNftUtxo))
			if err != nil {
				log.Printf("[Mempool] Failed to store NFT mempool codeHash genesis UTXO index %s -> %s: %v", utxoID, codeHashGenesisKey, err)
				continue
			}

		}
	}

	return isNftTx, nil
}

// processNftInputs processes NFT transaction inputs
func (m *NftMempoolManager) processNftInputs(tx *wire.MsgTx, timestamp int64) error {
	// Skip coinbase transaction
	if IsCoinbaseTx(tx) {
		return nil
	}

	// Used to store genesis transaction ID and its output info
	var usedGenesisUtxoMap = make(map[string]string)
	var txPointUsedMap = make(map[string]string)
	var allTxPoints []string
	var usedNftIncomeMap = make(map[string][]string)

	txId := tx.TxHash().String()
	if config.GlobalConfig.RPC.Chain == "mvc" {
		txId, _ = blockchain.GetNewHash(tx)
	}

	// First collect all input points
	for _, in := range tx.TxIn {
		prevTxHash := in.PreviousOutPoint.Hash.String()
		prevOutputIndex := strconv.Itoa(int(in.PreviousOutPoint.Index))
		spentUtxoID := prevTxHash + ":" + prevOutputIndex
		allTxPoints = append(allTxPoints, spentUtxoID)

		txPointUsedMap[spentUtxoID] = txId

		var nftUtxoAddress string
		var nftUtxoCodeHash string
		var nftUtxoGenesis string
		var nftUtxoSensibleId string
		var nftUtxoTokenIndex string
		var nftUtxoIndex string
		var nftUtxoValue string
		var nftUtxoTokenSupply string
		var nftUtxoMetaTxId string
		var nftUtxoMetaOutputIndex string
		var nftUtxoContractType string
		var codeHashGenesisKey string
		var nftUtxoPrice string
		var nftUtxoContractAddress string
		var nftUtxoTxID string

		// Get data directly from main UTXO storage
		utxoData, err := m.contractNftUtxoStore.Get([]byte(prevTxHash))
		if err == nil {
			// Handle leading comma
			utxoStr := string(utxoData)
			if len(utxoStr) > 0 && utxoStr[0] == ',' {
				utxoStr = utxoStr[1:]
			}
			parts := strings.Split(utxoStr, ",")
			if len(parts) > int(in.PreviousOutPoint.Index) {
				nftUtxoPart := ""
				for _, part := range parts {
					// value:NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType
					outputInfo := strings.Split(part, "@")
					if len(outputInfo) >= 6 {
						if outputInfo[5] == strconv.Itoa(int(in.PreviousOutPoint.Index)) {
							nftUtxoPart = part
							break
						}
					}
				}
				if nftUtxoPart == "" {
					continue
				}
				outputInfo := strings.Split(nftUtxoPart, "@")

				// Get address
				nftUtxoAddress = outputInfo[0]
				nftUtxoCodeHash = outputInfo[1]
				nftUtxoGenesis = outputInfo[2]
				nftUtxoSensibleId = outputInfo[3]
				nftUtxoTokenIndex = outputInfo[4]
				nftUtxoIndex = outputInfo[5]
				nftUtxoValue = outputInfo[6]
				nftUtxoTokenSupply = outputInfo[7]
				nftUtxoMetaTxId = outputInfo[8]
				nftUtxoMetaOutputIndex = outputInfo[9]
				nftUtxoContractType = outputInfo[11]
			}
		} else if err == storage.ErrNotFound {
			// Not found in main UTXO storage, try to find from mempool income database
			nftUtxoAddress,
				nftUtxoCodeHash,
				nftUtxoGenesis,
				nftUtxoTokenIndex,
				nftUtxoValue,
				nftUtxoTokenSupply,
				nftUtxoMetaTxId,
				nftUtxoMetaOutputIndex,
				nftUtxoIndex, _ = m.mempoolAddressNftIncomeDB.GetByNftUTXO(spentUtxoID)
			if nftUtxoCodeHash == "" {
				nftUtxoAddress,
					nftUtxoCodeHash,
					nftUtxoGenesis, nftUtxoTokenIndex,
					nftUtxoPrice,
					nftUtxoContractAddress,
					nftUtxoTxID,
					nftUtxoIndex,
					nftUtxoValue, _ = m.mempoolAddressSellNftIncomeStore.GetByNftSellUTXO(spentUtxoID)
				if nftUtxoAddress == "" {
					continue
				}
				nftUtxoContractType = "nft_sell"
			} else {
				nftUtxoContractType = "nft"
			}
		} else {
			continue
		}

		if nftUtxoContractType != "nft" && nftUtxoContractType != "nft_sell" {
			continue
		}

		// Record to mempool spend database
		if nftUtxoContractType == "nft" {
			// Process nft spend storage
			recordKey := nftUtxoAddress
			// key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp@usedTxId
			mempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftUtxoCodeHash,
				nftUtxoGenesis,
				nftUtxoSensibleId,
				nftUtxoTokenIndex,
				nftUtxoIndex,
				nftUtxoValue,
				nftUtxoTokenSupply,
				nftUtxoMetaTxId,
				nftUtxoMetaOutputIndex,
				strconv.FormatInt(timestamp, 10),
				txId,
			}, "@")
			err = m.mempoolAddressNftSpendDB.AddRecord(spentUtxoID, recordKey, []byte(mempoolNftUtxo))
			if err != nil {
				continue
			}

			// Process codeHash@genesis NFT spend storage
			// key: outpoint+codeHashGenesis, value: NftAddress@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp@usedTxId
			codeHashGenesisKey = common.ConcatBytesOptimized([]string{nftUtxoCodeHash, nftUtxoGenesis}, "@")
			codeHashGenesisMempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftUtxoAddress,
				nftUtxoTokenIndex,
				nftUtxoIndex,
				nftUtxoValue,
				nftUtxoTokenSupply,
				nftUtxoMetaTxId,
				nftUtxoMetaOutputIndex,
				strconv.FormatInt(timestamp, 10),
				txId,
			}, "@")
			err = m.mempoolCodeHashGenesisNftSpendStore.AddRecord(spentUtxoID, codeHashGenesisKey, []byte(codeHashGenesisMempoolNftUtxo))
			if err != nil {
				continue
			}
		} else if nftUtxoContractType == "nft_sell" {
			// Process nft sell spend storage
			recordKey := nftUtxoAddress
			// key: outpoint+address value: CodeHash@Genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp@usedTxId
			mempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftUtxoCodeHash,
				nftUtxoGenesis,
				nftUtxoTokenIndex,
				nftUtxoPrice,
				nftUtxoContractAddress,
				nftUtxoTxID,
				nftUtxoIndex,
				nftUtxoValue,
				strconv.FormatInt(timestamp, 10),
				txId,
			}, "@")
			err = m.mempoolAddressSellNftSpendStore.AddRecord(spentUtxoID, recordKey, []byte(mempoolNftUtxo))
			if err != nil {
				continue
			}

			// Process codeHash@genesis NFT sell spend storage
			// key: outpoint+codeHashGenesis value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@timestamp@usedTxId
			codeHashGenesisKey := common.ConcatBytesOptimized([]string{nftUtxoCodeHash, nftUtxoGenesis}, "@")
			codeHashGenesisMempoolNftUtxo := common.ConcatBytesOptimized([]string{
				nftUtxoAddress,
				nftUtxoTokenIndex,
				nftUtxoPrice,
				nftUtxoContractAddress,
				nftUtxoTxID,
				nftUtxoIndex,
				nftUtxoValue,
				strconv.FormatInt(timestamp, 10),
				txId,
			}, "@")
			err = m.mempoolCodeHashGenesisSellNftSpendStore.AddRecord(spentUtxoID, codeHashGenesisKey, []byte(codeHashGenesisMempoolNftUtxo))
			if err != nil {
				continue
			}
		} else {
			continue
		}
	}

	usedTxId := txId
	usedNftIncomeMap[usedTxId] = make([]string, 0)
	// Query if all input points exist in contractNftGenesisUtxoStore
	for _, txPoint := range allTxPoints {
		value, err := m.contractNftGenesisUtxoStore.Get([]byte(txPoint))
		if err == nil {
			// Found genesis UTXO, record transaction outpoint
			// key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}
			usedGenesisUtxoMap[txPoint] = string(value)
		}

		// Check if it exists in mempool
		mempoolValue, err := m.mempoolContractNftGenesisUtxoStore.GetSimpleRecord(txPoint)
		if err == nil {
			// If it's a genesis UTXO, record transaction ID
			// key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}
			usedGenesisUtxoMap[txPoint] = string(mempoolValue)
		}

		preTxId := strings.Split(txPoint, ":")[0]
		preTxIndex := strings.Split(txPoint, ":")[1]
		// key: txID, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
		nftUtxoValueListStr, err := m.contractNftUtxoStore.Get([]byte(preTxId))
		if err == nil {
			valueInfoList := strings.Split(string(nftUtxoValueListStr), ",")
			for _, valueInfo := range valueInfoList {
				valueInfoList := strings.Split(valueInfo, "@")
				if len(valueInfoList) < 12 {
					continue
				}
				index := valueInfoList[5]
				if preTxIndex == index {
					newValue := common.ConcatBytesOptimized([]string{
						valueInfoList[0],
						valueInfoList[1],
						valueInfoList[2],
						valueInfoList[3],
						valueInfoList[4],
						preTxId,
						valueInfoList[5],
						valueInfoList[6],
						valueInfoList[7],
						valueInfoList[8],
						valueInfoList[9],
						valueInfoList[10],
					}, "@")
					// key: usedTxId, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
					usedNftIncomeMap[usedTxId] = append(usedNftIncomeMap[usedTxId], newValue)
					break
				}
			}
		}

		// key: outpoint+address value: CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@timestamp
		incomeList, err := m.mempoolAddressNftIncomeDB.GetNftUtxoByOutpoint(txPoint)
		if err == nil {
			for _, utxo := range incomeList {
				if utxo.Index == preTxIndex {
					newValue := common.ConcatBytesOptimized([]string{
						utxo.Address,
						utxo.CodeHash,
						utxo.Genesis,
						utxo.SensibleId,
						utxo.TokenIndex,
						preTxId,
						utxo.Index,
						utxo.Value,
						utxo.TokenSupply,
						utxo.MetaTxId,
						utxo.MetaOutputIndex,
						"-1",
					}, "@")
					// key: usedTxId, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
					usedNftIncomeMap[usedTxId] = append(usedNftIncomeMap[usedTxId], newValue)
					break
				}
			}
		}
	}

	// Process genesis UTXO consumption
	if len(usedGenesisUtxoMap) > 0 {
		for txPoint, value := range usedGenesisUtxoMap {
			// Get original UTXO info and add @IsSpent flag
			spentValue := value + "@1" // Add @IsSpent flag
			if err := m.mempoolContractNftGenesisUtxoStore.AddSimpleRecord(txPoint, []byte(spentValue)); err != nil {
				log.Printf("[Mempool] Failed to update NFT mempool genesis UTXO status %s: %v", txPoint, err)
			}
		}
	}

	if len(usedNftIncomeMap) > 0 {
		for usedTxId, utxoList := range usedNftIncomeMap {
			// value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
			if err := m.mempoolUsedNftIncomeStore.AddSimpleRecord(usedTxId, []byte(strings.Join(utxoList, ","))); err != nil {
				log.Printf("[Mempool] Failed to store NFT mempool used UTXO %s: %v", usedTxId, err)
			}
		}
	}

	// Process genesis output storage
	if len(allTxPoints) > 0 {
		for _, usedTxPoint := range allTxPoints {
			// Collect all output info for this transaction
			var outputs []string
			for i, out := range tx.TxOut {
				pkScriptStr := hex.EncodeToString(out.PkScript)
				nftInfo, nftSellInfo, contractTypeStr, err := blockchain.ParseContractNftInfo(pkScriptStr, m.chainCfg)
				_ = nftSellInfo
				if err != nil || nftInfo == nil {
					continue
				}
				if contractTypeStr != "nft" && contractTypeStr != "nft_sell" {
					continue
				}
				// key: usedOutpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
				outputInfo := common.ConcatBytesOptimized([]string{
					nftInfo.SensibleId,
					strconv.FormatUint(nftInfo.TokenSupply, 10),
					nftInfo.CodeHash,
					nftInfo.Genesis,
					strconv.FormatUint(nftInfo.TokenIndex, 10),
					txId,
					strconv.Itoa(i),
					strconv.FormatInt(out.Value, 10),
					nftInfo.MetaTxId,
					strconv.FormatUint(nftInfo.MetaOutputIndex, 10),
				}, "@")
				outputs = append(outputs, outputInfo)
			}

			if len(outputs) > 0 {
				outputValue := strings.Join(outputs, ",")
				if err := m.mempoolContractNftGenesisOutputStore.AddSimpleRecord(usedTxPoint, []byte(outputValue)); err != nil {
					log.Printf("[Mempool] Failed to store NFT mempool genesis output %s: %v", usedTxPoint, err)
				}
			}
		}
	}

	return nil
}

// ProcessNewBlockTxs processes NFT transactions in new blocks and cleans up mempool records
func (m *NftMempoolManager) ProcessNewBlockTxs(incomeUtxoList []common.NftUtxo, spendOutpointList []string, txList []string) error {
	// Delete VerifyTx
	for _, tx := range txList {
		err := m.mempoolVerifyTxStore.DeleteSimpleRecord(tx)
		if err != nil {
			log.Printf("Failed to delete VerifyTx %s: %v", tx, err)
		}
	}
	if len(incomeUtxoList) == 0 {
		return nil
	}

	// Delete income
	for _, utxo := range incomeUtxoList {
		if utxo.ContractType == "nft" || utxo.ContractType == "nft_sell" {
			fmt.Printf("[ProcessNewBlockTxs]Delete income UTXO: %v\n", utxo)
			err := m.mempoolAddressNftIncomeDB.DeleteRecord(utxo.UtxoId, utxo.Address)
			if err != nil {
				log.Printf("Failed to delete NFT mempool income record %s: %v", utxo.TxID, err)
			}

			// Process uncheckNftOutpointStore
			err = m.mempoolAddressNftIncomeValidStore.DeleteRecord(utxo.UtxoId, utxo.Address)
			if err != nil {
				log.Printf("[Mempool] Failed to delete NFT mempool valid UTXO index %s -> %s: %v", utxo.UtxoId, utxo.Address, err)
			}

			// Delete codeHash@genesis income
			codeHashGenesis := common.ConcatBytesOptimized([]string{utxo.CodeHash, utxo.Genesis}, "@")
			err = m.mempoolCodeHashGenesisNftIncomeStore.DeleteRecord(utxo.UtxoId, codeHashGenesis)
			if err != nil {
				log.Printf("Failed to delete NFT mempool codeHash genesis income record %s: %v", utxo.UtxoId, err)
			}

			err = m.mempoolCodeHashGenesisNftIncomeValidStore.DeleteRecord(utxo.UtxoId, codeHashGenesis)
			if err != nil {
				log.Printf("Failed to delete NFT mempool codeHash genesis income valid record %s: %v", utxo.UtxoId, err)
			}

			//sell nft
			err = m.mempoolAddressSellNftIncomeStore.DeleteRecord(utxo.UtxoId, utxo.Address)
			if err != nil {
				log.Printf("Failed to delete NFT mempool sell income record %s: %v", utxo.UtxoId, err)
			}

			err = m.mempoolCodeHashGenesisSellNftIncomeStore.DeleteRecord(utxo.UtxoId, codeHashGenesis)
			if err != nil {
				log.Printf("Failed to delete NFT mempool codeHash genesis sell income record %s: %v", utxo.UtxoId, err)
			}

			// Check and delete records in NFT info storage
			parts := strings.Split(utxo.UtxoId, ":")
			if len(parts) == 2 {
				outpoint := common.ConcatBytesOptimized([]string{parts[0], parts[1]}, ":")

				// Check and delete records in initial genesis info storage
				genesisKey := outpoint
				_, err = m.mempoolContractNftGenesisStore.GetSimpleRecord(genesisKey)
				if err == nil {
					if err := m.mempoolContractNftGenesisStore.DeleteSimpleRecord(genesisKey); err != nil {
						log.Printf("[Mempool] Failed to delete NFT mempool genesis info %s: %v", genesisKey, err)
					}
				}

				// Check and delete records in new genesis output storage
				_, err = m.mempoolContractNftGenesisUtxoStore.GetSimpleRecord(genesisKey)
				if err == nil {
					if err := m.mempoolContractNftGenesisUtxoStore.DeleteSimpleRecord(genesisKey); err != nil {
						log.Printf("[Mempool] Failed to delete NFT mempool genesis UTXO %s: %v", genesisKey, err)
					}
				}
			}
		}
	}

	for _, utxo := range incomeUtxoList {
		fmt.Printf("[ProcessNewBlockTxs]Delete income UTXO: %v\n", utxo)
		codeHashGenesis := common.ConcatBytesOptimized([]string{utxo.CodeHash, utxo.Genesis}, "@")
		err := m.mempoolContractNftInfoStore.DeleteSimpleRecord(codeHashGenesis)
		if err != nil {
			log.Printf("Failed to delete NFT mempool genesis info %s: %v", utxo.CodeHash+"@"+utxo.Genesis, err)
		}

		err = m.mempoolContractNftSummaryInfoStore.DeleteSimpleRecord(codeHashGenesis)
		if err != nil {
			log.Printf("Failed to delete NFT mempool summary info %s: %v", utxo.CodeHash+"@"+utxo.Genesis, err)
		}
	}

	// Delete spend
	for _, outpoint := range spendOutpointList {
		fmt.Printf("[ProcessNewBlockTxs]Delete spend UTXO: %v\n", outpoint)
		err := m.mempoolAddressNftSpendDB.DeleteNftSpendRecord(outpoint)
		if err != nil {
			log.Printf("Failed to delete NFT mempool nft spend record %s: %v", outpoint, err)
		}
		err = m.mempoolCodeHashGenesisNftSpendStore.DeleteNftSpendRecord(outpoint)
		if err != nil {
			log.Printf("Failed to delete NFT mempool codeHash genesis spend record %s: %v", outpoint, err)
		}

		//sell nft
		err = m.mempoolAddressSellNftSpendStore.DeleteNftSpendRecord(outpoint)
		if err != nil {
			log.Printf("Failed to delete NFT mempool sell spend record %s: %v", outpoint, err)
		}
		err = m.mempoolCodeHashGenesisSellNftSpendStore.DeleteNftSpendRecord(outpoint)
		if err != nil {
			log.Printf("Failed to delete NFT mempool codeHash genesis sell spend record %s: %v", outpoint, err)
		}
	}
	return nil
}

// CleanByHeight cleans NFT mempool records by block height
func (m *NftMempoolManager) CleanByHeight(height int, bcClient interface{}) error {
	log.Printf("Start cleaning NFT mempool, processing to block height: %d", height)

	// Try to assert bcClient as blockchain.NftClient type
	client, ok := bcClient.(*blockchain.NftClient)
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

	txList := make([]string, 0)

	// Extract incomeUtxo list
	var incomeNftUtxoList []common.NftUtxo = make([]common.NftUtxo, 0)
	var spendOutpointList []string
	for _, tx := range block.Tx {
		txList = append(txList, tx.Txid)
		for _, in := range tx.Vin {
			preTxId := in.Txid
			if preTxId == "" {
				preTxId = "0000000000000000000000000000000000000000000000000000000000000000"
			}
			spendOutpointList = append(spendOutpointList, common.ConcatBytesOptimized([]string{preTxId, strconv.Itoa(int(in.Vout))}, ":"))
		}

		for k, out := range tx.Vout {
			address := blockchain.GetAddressFromScript(out.ScriptPubKey.Hex, nil, m.chainCfg, config.GlobalConfig.RPC.Chain)
			amount := strconv.FormatInt(int64(math.Round(out.Value*1e8)), 10)

			// Parse NFT related information
			var nftUtxo *common.NftUtxo
			nftInfo, nftSellInfo, contractTypeStr, err := blockchain.ParseContractNftInfo(out.ScriptPubKey.Hex, m.chainCfg)
			_ = nftSellInfo
			if err != nil {
				continue
			}
			if contractTypeStr == "nft" || contractTypeStr == "nft_sell" {
				nftUtxo = &common.NftUtxo{
					ContractType:    contractTypeStr,
					Index:           strconv.Itoa(k),
					UtxoId:          common.ConcatBytesOptimized([]string{tx.Txid, strconv.Itoa(k)}, ":"),
					TxID:            tx.Txid,
					Address:         address,
					Value:           amount,
					CodeHash:        nftInfo.CodeHash,
					Genesis:         nftInfo.Genesis,
					SensibleId:      nftInfo.SensibleId,
					TokenIndex:      strconv.FormatUint(nftInfo.TokenIndex, 10),
					TokenSupply:     strconv.FormatUint(nftInfo.TokenSupply, 10),
					MetaTxId:        nftInfo.MetaTxId,
					MetaOutputIndex: strconv.FormatUint(nftInfo.MetaOutputIndex, 10),
				}
			} else {
				continue
			}

			if nftInfo == nil {
				continue
			}

			incomeNftUtxoList = append(incomeNftUtxoList, *nftUtxo)
		}
	}

	// Clean mempool records
	return m.ProcessNewBlockTxs(incomeNftUtxoList, spendOutpointList, txList)
}

// InitializeMempool fetches and processes all current NFT mempool transactions from the node at startup
func (m *NftMempoolManager) InitializeMempool(bcClient interface{}) {
	// Use a separate goroutine to avoid blocking the main program
	go func() {
		log.Printf("Starting NFT mempool data initialization...")

		// Assert as blockchain.NftClient
		client, ok := bcClient.(*blockchain.NftClient)
		if !ok {
			log.Printf("Failed to initialize NFT mempool: unsupported blockchain client type")
			return
		}

		// Get all transaction IDs in the mempool
		txids, err := client.GetRawMempool()
		if err != nil {
			log.Printf("Failed to get mempool transaction list: %v", err)
			return
		}

		log.Printf("Fetched %d mempool transactions from node, start processing...", len(txids))

		// Process transactions in batches, 100 per batch to avoid excessive memory usage
		batchSize := 100
		totalBatches := (len(txids) + batchSize - 1) / batchSize

		for batchIdx := 0; batchIdx < totalBatches; batchIdx++ {
			start := batchIdx * batchSize
			end := start + batchSize
			if end > len(txids) {
				end = len(txids)
			}

			// Process current batch
			currentBatch := txids[start:end]
			log.Printf("Processing NFT mempool transaction batch %d/%d (%d transactions)", batchIdx+1, totalBatches, len(currentBatch))

			for _, txid := range currentBatch {
				now := time.Now().UnixMilli()
				// Get transaction details
				txRaw, err := client.GetRawTransactionHex(txid)
				if err != nil {
					fmt.Println("GetRawTransaction error", err)
					continue
				}
				txRawByte, err := hex.DecodeString(txRaw)
				if err != nil {
					fmt.Println("DecodeString error", err)
					continue
				}
				msgTx, err := DeserializeTransaction(txRawByte)
				if err != nil {
					fmt.Println("DeserializeTransaction error", err)
					continue
				}

				// Process outputs first (create new UTXOs)
				isNftTx, err := m.processNftOutputs(msgTx, now)
				if err != nil {
					log.Printf("Failed to process NFT transaction outputs %s: %v", txid, err)
					continue
				}
				if isNftTx {
					fmt.Printf("Mempool received NFT transaction: %s\n", txid)
				}

				// Then process inputs (mark spent UTXOs)
				if err := m.processNftInputs(msgTx, now); err != nil {
					log.Printf("Failed to process NFT transaction inputs %s: %v", txid, err)
					continue
				}

				if isNftTx {
					// Process VerifyTx
					if err := m.processVerifyTx(msgTx); err != nil {
						log.Printf("Failed to process VerifyTx %s: %v", txid, err)
						continue
					}
				}
			}

			// After batch is processed, pause briefly to allow other programs to execute
			time.Sleep(10 * time.Millisecond)
		}

		log.Printf("NFT mempool data initialization complete, processed %d transactions in total", len(txids))
	}()
}

// CleanAllMempool cleans all NFT mempool data for complete rebuild
func (m *NftMempoolManager) CleanAllMempool() error {
	log.Println("Resetting NFT mempool data by deleting physical files...")

	// Save ZMQ address for later reconstruction
	zmqAddress := ""
	if m.zmqClient != nil {
		zmqAddress = m.zmqClient.address
	}

	// Get database file paths using basePath and fixed table names
	incomeDbPath := m.basePath + "/mempool_address_nft_income"
	spendDbPath := m.basePath + "/mempool_address_nft_spend"
	codeHashGenesisNftIncomeDbPath := m.basePath + "/mempool_codehash_genesis_nft_income"
	codeHashGenesisNftSpendDbPath := m.basePath + "/mempool_codehash_genesis_nft_spend"
	sellIncomeDbPath := m.basePath + "/mempool_address_nft_sell_income"
	sellSpendDbPath := m.basePath + "/mempool_address_nft_sell_spend"
	codeHashGenesisSellNftIncomeDbPath := m.basePath + "/mempool_codehash_genesis_nft_sell_income"
	codeHashGenesisSellNftSpendDbPath := m.basePath + "/mempool_codehash_genesis_nft_sell_spend"
	infoDbPath := m.basePath + "/mempool_contract_nft_info"
	summaryInfoDbPath := m.basePath + "/mempool_contract_nft_summary_info"
	genesisDbPath := m.basePath + "/mempool_contract_nft_genesis"
	genesisOutputDbPath := m.basePath + "/mempool_contract_nft_genesis_output"
	genesisUtxoDbPath := m.basePath + "/mempool_contract_nft_genesis_utxo"
	incomeValidDbPath := m.basePath + "/mempool_address_nft_income_valid"
	codeHashGenesisIncomeValidDbPath := m.basePath + "/mempool_codehash_genesis_nft_income_valid"
	uncheckNftOutpointDbPath := m.basePath + "/mempool_uncheck_nft_outpoint"
	usedNftIncomeDbPath := m.basePath + "/mempool_used_nft_income"
	mempoolVerifyTxDbPath := m.basePath + "/mempool_nft_verify_tx"

	// No longer try to detect database status, use defer and recover to handle possible panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Exception caught during cleanup: %v, continuing with file deletion", r)
		}
	}()

	// Safely close database connections
	log.Println("Closing existing NFT mempool database connections...")

	// Close all databases
	closeDB := func(db interface{ Close() error }, name string) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Error occurred while closing %s database: %v", name, r)
			}
		}()
		if db != nil {
			_ = db.Close()
		}
	}

	closeDB(m.mempoolAddressNftIncomeDB, "income")
	closeDB(m.mempoolAddressNftSpendDB, "spend")
	closeDB(m.mempoolCodeHashGenesisNftIncomeStore, "codeHash genesis NFT income")
	closeDB(m.mempoolCodeHashGenesisNftSpendStore, "codeHash genesis NFT spend")
	closeDB(m.mempoolAddressSellNftIncomeStore, "address sell NFT income")
	closeDB(m.mempoolAddressSellNftSpendStore, "address sell NFT spend")
	closeDB(m.mempoolCodeHashGenesisSellNftIncomeStore, "codeHash genesis sell NFT income")
	closeDB(m.mempoolCodeHashGenesisSellNftSpendStore, "codeHash genesis sell NFT spend")
	closeDB(m.mempoolContractNftInfoStore, "info")
	closeDB(m.mempoolContractNftSummaryInfoStore, "summary info")
	closeDB(m.mempoolContractNftGenesisStore, "genesis")
	closeDB(m.mempoolContractNftGenesisOutputStore, "genesis output")
	closeDB(m.mempoolContractNftGenesisUtxoStore, "genesis UTXO")
	closeDB(m.mempoolAddressNftIncomeValidStore, "income valid")
	closeDB(m.mempoolCodeHashGenesisNftIncomeValidStore, "codeHash genesis income valid")
	closeDB(m.mempoolUncheckNftOutpointStore, "unchecked NFT output")
	closeDB(m.mempoolUsedNftIncomeStore, "used income")
	closeDB(m.mempoolVerifyTxStore, "VerifyTx")

	// Delete physical files
	deleteDB := func(path, name string) error {
		log.Printf("Deleting NFT mempool %s database: %s", name, path)
		if err := os.RemoveAll(path); err != nil {
			log.Printf("Failed to delete NFT mempool %s database: %v", name, err)
			return err
		}
		return nil
	}

	if err := deleteDB(incomeDbPath, "income"); err != nil {
		return err
	}
	if err := deleteDB(spendDbPath, "spend"); err != nil {
		return err
	}
	if err := deleteDB(codeHashGenesisNftIncomeDbPath, "codeHash genesis NFT income"); err != nil {
		return err
	}
	if err := deleteDB(codeHashGenesisNftSpendDbPath, "codeHash genesis NFT spend"); err != nil {
		return err
	}
	if err := deleteDB(sellIncomeDbPath, "address sell NFT income"); err != nil {
		return err
	}
	if err := deleteDB(sellSpendDbPath, "address sell NFT spend"); err != nil {
		return err
	}
	if err := deleteDB(codeHashGenesisSellNftIncomeDbPath, "codeHash genesis sell NFT income"); err != nil {
		return err
	}
	if err := deleteDB(codeHashGenesisSellNftSpendDbPath, "codeHash genesis sell NFT spend"); err != nil {
		return err
	}
	if err := deleteDB(infoDbPath, "info"); err != nil {
		return err
	}
	if err := deleteDB(summaryInfoDbPath, "summary info"); err != nil {
		return err
	}
	if err := deleteDB(genesisDbPath, "genesis"); err != nil {
		return err
	}
	if err := deleteDB(genesisOutputDbPath, "genesis output"); err != nil {
		return err
	}
	if err := deleteDB(genesisUtxoDbPath, "genesis UTXO"); err != nil {
		return err
	}
	if err := deleteDB(incomeValidDbPath, "income valid"); err != nil {
		return err
	}
	if err := deleteDB(codeHashGenesisIncomeValidDbPath, "codeHash genesis income valid"); err != nil {
		return err
	}
	if err := deleteDB(uncheckNftOutpointDbPath, "unchecked NFT output"); err != nil {
		return err
	}
	if err := deleteDB(usedNftIncomeDbPath, "used income"); err != nil {
		return err
	}
	if err := deleteDB(mempoolVerifyTxDbPath, "VerifyTx"); err != nil {
		return err
	}

	// Recreate databases
	log.Println("Recreating NFT mempool databases...")
	mempoolAddressNftIncomeDB, err := storage.NewSimpleDB(incomeDbPath)
	if err != nil {
		log.Printf("Failed to recreate NFT mempool income database: %v", err)
		return err
	}

	mempoolAddressNftSpendDB, err := storage.NewSimpleDB(spendDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		log.Printf("Failed to recreate NFT mempool spend database: %v", err)
		return err
	}

	mempoolCodeHashGenesisNftIncomeStore, err := storage.NewSimpleDB(codeHashGenesisNftIncomeDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		log.Printf("Failed to recreate NFT mempool codeHash genesis NFT income database: %v", err)
		return err
	}

	mempoolCodeHashGenesisNftSpendStore, err := storage.NewSimpleDB(codeHashGenesisNftSpendDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		log.Printf("Failed to recreate NFT mempool codeHash genesis NFT spend database: %v", err)
		return err
	}

	mempoolAddressSellNftIncomeStore, err := storage.NewSimpleDB(sellIncomeDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		log.Printf("Failed to recreate NFT mempool address sell NFT income database: %v", err)
		return err
	}

	mempoolAddressSellNftSpendStore, err := storage.NewSimpleDB(sellSpendDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		log.Printf("Failed to recreate NFT mempool address sell NFT spend database: %v", err)
		return err
	}

	mempoolCodeHashGenesisSellNftIncomeStore, err := storage.NewSimpleDB(codeHashGenesisSellNftIncomeDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		log.Printf("Failed to recreate NFT mempool codeHash genesis sell NFT income database: %v", err)
		return err
	}

	mempoolCodeHashGenesisSellNftSpendStore, err := storage.NewSimpleDB(codeHashGenesisSellNftSpendDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		log.Printf("Failed to recreate NFT mempool codeHash genesis sell NFT spend database: %v", err)
		return err
	}

	mempoolContractNftInfoStore, err := storage.NewSimpleDB(infoDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		log.Printf("Failed to recreate NFT mempool info database: %v", err)
		return err
	}

	mempoolContractNftSummaryInfoStore, err := storage.NewSimpleDB(summaryInfoDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		log.Printf("Failed to recreate NFT mempool summary info database: %v", err)
		return err
	}

	mempoolContractNftGenesisStore, err := storage.NewSimpleDB(genesisDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		log.Printf("Failed to recreate NFT mempool genesis database: %v", err)
		return err
	}

	mempoolContractNftGenesisOutputStore, err := storage.NewSimpleDB(genesisOutputDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		log.Printf("Failed to recreate NFT mempool genesis output database: %v", err)
		return err
	}

	mempoolContractNftGenesisUtxoStore, err := storage.NewSimpleDB(genesisUtxoDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		log.Printf("Failed to recreate NFT mempool genesis UTXO database: %v", err)
		return err
	}

	mempoolAddressNftIncomeValidStore, err := storage.NewSimpleDB(incomeValidDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		log.Printf("Failed to recreate NFT mempool income valid database: %v", err)
		return err
	}

	mempoolCodeHashGenesisNftIncomeValidStore, err := storage.NewSimpleDB(codeHashGenesisIncomeValidDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		log.Printf("Failed to recreate NFT mempool codeHash genesis income valid database: %v", err)
		return err
	}

	mempoolUncheckNftOutpointStore, err := storage.NewSimpleDB(uncheckNftOutpointDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		mempoolCodeHashGenesisNftIncomeValidStore.Close()
		log.Printf("Failed to recreate NFT mempool unchecked NFT output database: %v", err)
		return err
	}

	mempoolUsedNftIncomeStore, err := storage.NewSimpleDB(usedNftIncomeDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		mempoolCodeHashGenesisNftIncomeValidStore.Close()
		mempoolUncheckNftOutpointStore.Close()
		log.Printf("Failed to recreate NFT mempool used income database: %v", err)
		return err
	}

	mempoolVerifyTxStore, err := storage.NewSimpleDB(mempoolVerifyTxDbPath)
	if err != nil {
		mempoolAddressNftIncomeDB.Close()
		mempoolAddressNftSpendDB.Close()
		mempoolCodeHashGenesisNftIncomeStore.Close()
		mempoolCodeHashGenesisNftSpendStore.Close()
		mempoolAddressSellNftIncomeStore.Close()
		mempoolAddressSellNftSpendStore.Close()
		mempoolCodeHashGenesisSellNftIncomeStore.Close()
		mempoolCodeHashGenesisSellNftSpendStore.Close()
		mempoolContractNftInfoStore.Close()
		mempoolContractNftSummaryInfoStore.Close()
		mempoolContractNftGenesisStore.Close()
		mempoolContractNftGenesisOutputStore.Close()
		mempoolContractNftGenesisUtxoStore.Close()
		mempoolAddressNftIncomeValidStore.Close()
		mempoolCodeHashGenesisNftIncomeValidStore.Close()
		mempoolUncheckNftOutpointStore.Close()
		mempoolUsedNftIncomeStore.Close()
		log.Printf("Failed to recreate NFT mempool VerifyTx database: %v", err)
		return err
	}

	// Update database references
	m.mempoolAddressNftIncomeDB = mempoolAddressNftIncomeDB
	m.mempoolAddressNftSpendDB = mempoolAddressNftSpendDB
	m.mempoolCodeHashGenesisNftIncomeStore = mempoolCodeHashGenesisNftIncomeStore
	m.mempoolCodeHashGenesisNftSpendStore = mempoolCodeHashGenesisNftSpendStore
	m.mempoolAddressSellNftIncomeStore = mempoolAddressSellNftIncomeStore
	m.mempoolAddressSellNftSpendStore = mempoolAddressSellNftSpendStore
	m.mempoolCodeHashGenesisSellNftIncomeStore = mempoolCodeHashGenesisSellNftIncomeStore
	m.mempoolCodeHashGenesisSellNftSpendStore = mempoolCodeHashGenesisSellNftSpendStore
	m.mempoolContractNftInfoStore = mempoolContractNftInfoStore
	m.mempoolContractNftSummaryInfoStore = mempoolContractNftSummaryInfoStore
	m.mempoolContractNftGenesisStore = mempoolContractNftGenesisStore
	m.mempoolContractNftGenesisOutputStore = mempoolContractNftGenesisOutputStore
	m.mempoolContractNftGenesisUtxoStore = mempoolContractNftGenesisUtxoStore
	m.mempoolAddressNftIncomeValidStore = mempoolAddressNftIncomeValidStore
	m.mempoolCodeHashGenesisNftIncomeValidStore = mempoolCodeHashGenesisNftIncomeValidStore
	m.mempoolUncheckNftOutpointStore = mempoolUncheckNftOutpointStore
	m.mempoolUsedNftIncomeStore = mempoolUsedNftIncomeStore
	m.mempoolVerifyTxStore = mempoolVerifyTxStore

	// Recreate ZMQ client
	if zmqAddress != "" {
		log.Println("Recreating ZMQ client...")
		m.zmqClient = NewZMQClient([]string{zmqAddress}, nil)[0]

		// Re-add topic listeners
		log.Println("Re-adding ZMQ topic listeners...")
		m.zmqClient.AddTopic("rawtx", m.HandleRawTransaction)
	}

	log.Println("NFT mempool data completely reset successfully")
	return nil
}

// GetBasePath returns the base path for NFT mempool data
func (m *NftMempoolManager) GetBasePath() string {
	return m.basePath
}

// GetZmqAddress returns the ZMQ server address
func (m *NftMempoolManager) GetZmqAddress() string {
	if m.zmqClient != nil {
		return m.zmqClient.address
	}
	return ""
}

// GetMempoolAddressNftIncomeMap gets NFT income data for addresses in mempool
// If address is provided, returns data for that address only; otherwise returns all addresses
// CodeHash@Genesis@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
func (m *NftMempoolManager) GetMempoolAddressNftIncomeMap(address string) map[string]string {
	result := make(map[string]string)

	// If address is provided, get data for that address only
	if address != "" {
		value, err := m.mempoolAddressNftIncomeDB.Get(address)
		if err == nil {
			result[address] = value
		}
		return result
	}

	// If no address provided, get all addresses
	keyValues, err := m.mempoolAddressNftIncomeDB.GetAllKeyValues()
	if err != nil {
		log.Printf("Failed to get all key-value pairs: %v", err)
		return result
	}

	for key, value := range keyValues {
		result[key] = value
	}

	return result
}

// GetMempoolAddressNftIncomeValidMap gets valid NFT income data for addresses in mempool
// If address is provided, returns data for that address only; otherwise returns all addresses
// CodeHash@Genesis@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
func (m *NftMempoolManager) GetMempoolAddressNftIncomeValidMap(address string) map[string]string {
	result := make(map[string]string)

	// If address is provided, get data for that address only
	if address != "" {
		// Use prefix query to get all records with address_outpoint format
		utxoList, err := m.mempoolAddressNftIncomeValidStore.GetAddressNftUtxoByKey(address)
		if err != nil {
			log.Printf("Failed to get valid NFT income for address %s: %v", address, err)
			return result
		}

		// Merge all values into comma-separated string (similar to main indexer format)
		var values []string
		for _, utxo := range utxoList {
			// Format: CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex
			value := fmt.Sprintf("%s@%s@%s@%s@%s@%s@%s@%s@%s",
				utxo.CodeHash, utxo.Genesis, utxo.TokenIndex, utxo.TxID, utxo.Index,
				utxo.Value, utxo.TokenSupply, utxo.MetaTxId, utxo.MetaOutputIndex)
			values = append(values, value)
		}
		if len(values) > 0 {
			result[address] = strings.Join(values, ",")
		}
		return result
	}

	// If no address provided, get all addresses and group by address
	keyValues, err := m.mempoolAddressNftIncomeValidStore.GetAllKeyValues()
	if err != nil {
		log.Printf("Failed to get all key-value pairs: %v", err)
		return result
	}

	// Group values by address
	addressValues := make(map[string][]string)
	for key, value := range keyValues {
		// key format: address_outpoint or outpoint_address
		// We only process keys in format address_outpoint (to avoid duplicates)
		parts := strings.Split(key, "_")
		if len(parts) >= 2 {
			// Check if first part looks like an address (not a transaction hash)
			addr := parts[0]
			// Simple heuristic: transaction hashes are 64 characters
			if len(addr) != 64 {
				addressValues[addr] = append(addressValues[addr], value)
			}
		}
	}

	// Merge values for each address
	for addr, values := range addressValues {
		result[addr] = strings.Join(values, ",")
	}

	return result
}
