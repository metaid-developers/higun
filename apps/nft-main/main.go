package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	indexer "github.com/metaid/utxo_indexer/indexer/contract/meta-contract-nft"
	"github.com/metaid/utxo_indexer/mempool"

	"github.com/metaid/utxo_indexer/api"
	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// AppResources manages all application resources
type AppResources struct {
	// Storage resources
	contractNftUtxoStore              *storage.PebbleStore
	addressNftIncomeStore             *storage.PebbleStore
	addressNftSpendStore              *storage.PebbleStore
	codeHashGenesisNftIncomeStore     *storage.PebbleStore
	codeHashGenesisNftSpendStore      *storage.PebbleStore
	addressSellNftIncomeStore         *storage.PebbleStore
	addressSellNftSpendStore          *storage.PebbleStore
	codeHashGenesisSellNftIncomeStore *storage.PebbleStore
	codeHashGenesisSellNftSpendStore  *storage.PebbleStore

	contractNftInfoStore              *storage.PebbleStore
	contractNftSummaryInfoStore       *storage.PebbleStore
	contractNftGenesisStore           *storage.PebbleStore
	contractNftGenesisOutputStore     *storage.PebbleStore
	contractNftGenesisUtxoStore       *storage.PebbleStore
	contractNftOwnersIncomeValidStore *storage.PebbleStore
	contractNftOwnersIncomeStore      *storage.PebbleStore
	contractNftOwnersSpendStore       *storage.PebbleStore
	contractNftAddressHistoryStore    *storage.PebbleStore
	contractNftGenesisHistoryStore    *storage.PebbleStore

	addressNftIncomeValidStore         *storage.PebbleStore
	codeHashGenesisNftIncomeValidStore *storage.PebbleStore
	uncheckNftOutpointStore            *storage.PebbleStore
	usedNftIncomeStore                 *storage.PebbleStore
	invalidNftOutpointStore            *storage.PebbleStore
	metaStore                          *storage.MetaStore

	// Blockchain and other resources
	bcClient             *blockchain.NftClient
	verifyManager        *indexer.NftVerifyManager
	mempoolMgr           *mempool.NftMempoolManager
	mempoolVerifyManager *mempool.NftMempoolVerifier
	server               *api.NftServer
	backupMgr            *storage.BackupManager
}

// Close closes all resources
func (ar *AppResources) Close() {
	log.Println("Starting to close all resources...")

	// Close order is important, close dependent resources first
	if ar.mempoolVerifyManager != nil {
		log.Println("Closing mempool verifier...")
		ar.mempoolVerifyManager.Stop()
		log.Println("Mempool verifier closed successfully")
	}

	if ar.verifyManager != nil {
		log.Println("Closing NFT verification manager...")
		ar.verifyManager.Stop()
		log.Println("NFT verification manager closed successfully")
	}

	if ar.mempoolMgr != nil {
		log.Println("Closing mempool manager...")
		ar.mempoolMgr.Stop()
		log.Println("Mempool manager closed successfully")
	}

	if ar.bcClient != nil {
		log.Println("Closing blockchain client...")
		// Give blockchain sync goroutine more time to complete current operations and stop completely
		time.Sleep(5 * time.Second)
		ar.bcClient.Shutdown()
		log.Println("Blockchain client closed successfully")
	}

	if ar.backupMgr != nil {
		log.Println("Closing backup manager...")
		ar.backupMgr.Stop()
		log.Println("Backup manager closed successfully")
	}

	// Close storage resources
	if ar.metaStore != nil {
		log.Println("[DB]Closing metaStore...")
		if err := ar.metaStore.Close(); err != nil {
			log.Printf("[DB]Failed to close metaStore: %v", err)
		} else {
			log.Println("[DB]metaStore closed successfully")
		}
	}

	if ar.invalidNftOutpointStore != nil {
		log.Println("[DB]Closing invalidNftOutpointStore...")
		if err := ar.invalidNftOutpointStore.Close(); err != nil {
			log.Printf("[DB]Failed to close invalidNftOutpointStore: %v", err)
		} else {
			log.Println("[DB]invalidNftOutpointStore closed successfully")
		}
	}

	if ar.usedNftIncomeStore != nil {
		log.Println("[DB]Closing usedNftIncomeStore...")
		if err := ar.usedNftIncomeStore.Close(); err != nil {
			log.Printf("[DB]Failed to close usedNftIncomeStore: %v", err)
		} else {
			log.Println("[DB]usedNftIncomeStore closed successfully")
		}
	}

	if ar.uncheckNftOutpointStore != nil {
		log.Println("[DB]Closing uncheckNftOutpointStore...")
		if err := ar.uncheckNftOutpointStore.Close(); err != nil {
			log.Printf("[DB]Failed to close uncheckNftOutpointStore: %v", err)
		} else {
			log.Println("[DB]uncheckNftOutpointStore closed successfully")
		}
	}

	if ar.addressNftIncomeValidStore != nil {
		log.Println("[DB]Closing addressNftIncomeValidStore...")
		if err := ar.addressNftIncomeValidStore.Close(); err != nil {
			log.Printf("[DB]Failed to close addressNftIncomeValidStore: %v", err)
		} else {
			log.Println("[DB]addressNftIncomeValidStore closed successfully")
		}
	}

	if ar.codeHashGenesisNftIncomeValidStore != nil {
		log.Println("[DB]Closing codeHashGenesisNftIncomeValidStore...")
		if err := ar.codeHashGenesisNftIncomeValidStore.Close(); err != nil {
			log.Printf("[DB]Failed to close codeHashGenesisNftIncomeValidStore: %v", err)
		} else {
			log.Println("[DB]codeHashGenesisNftIncomeValidStore closed successfully")
		}
	}

	if ar.contractNftGenesisUtxoStore != nil {
		log.Println("[DB]Closing contractNftGenesisUtxoStore...")
		if err := ar.contractNftGenesisUtxoStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftGenesisUtxoStore: %v", err)
		} else {
			log.Println("[DB]contractNftGenesisUtxoStore closed successfully")
		}
	}

	if ar.contractNftGenesisOutputStore != nil {
		log.Println("[DB]Closing contractNftGenesisOutputStore...")
		if err := ar.contractNftGenesisOutputStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftGenesisOutputStore: %v", err)
		} else {
			log.Println("[DB]contractNftGenesisOutputStore closed successfully")
		}
	}

	if ar.contractNftGenesisStore != nil {
		log.Println("[DB]Closing contractNftGenesisStore...")
		if err := ar.contractNftGenesisStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftGenesisStore: %v", err)
		} else {
			log.Println("[DB]contractNftGenesisStore closed successfully")
		}
	}

	if ar.contractNftInfoStore != nil {
		log.Println("[DB]Closing contractNftInfoStore...")
		if err := ar.contractNftInfoStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftInfoStore: %v", err)
		} else {
			log.Println("[DB]contractNftInfoStore closed successfully")
		}
	}

	if ar.contractNftSummaryInfoStore != nil {
		log.Println("[DB]Closing contractNftSummaryInfoStore...")
		if err := ar.contractNftSummaryInfoStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftSummaryInfoStore: %v", err)
		} else {
			log.Println("[DB]contractNftSummaryInfoStore closed successfully")
		}
	}

	if ar.contractNftOwnersIncomeValidStore != nil {
		log.Println("[DB]Closing contractNftOwnersIncomeValidStore...")
		if err := ar.contractNftOwnersIncomeValidStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftOwnersIncomeValidStore: %v", err)
		} else {
			log.Println("[DB]contractNftOwnersIncomeValidStore closed successfully")
		}
	}

	if ar.contractNftOwnersIncomeStore != nil {
		log.Println("[DB]Closing contractNftOwnersIncomeStore...")
		if err := ar.contractNftOwnersIncomeStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftOwnersIncomeStore: %v", err)
		} else {
			log.Println("[DB]contractNftOwnersIncomeStore closed successfully")
		}
	}

	if ar.contractNftOwnersSpendStore != nil {
		log.Println("[DB]Closing contractNftOwnersSpendStore...")
		if err := ar.contractNftOwnersSpendStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftOwnersSpendStore: %v", err)
		} else {
			log.Println("[DB]contractNftOwnersSpendStore closed successfully")
		}
	}

	if ar.addressNftSpendStore != nil {
		log.Println("[DB]Closing addressNftSpendStore...")
		if err := ar.addressNftSpendStore.Close(); err != nil {
			log.Printf("[DB]Failed to close addressNftSpendStore: %v", err)
		} else {
			log.Println("[DB]addressNftSpendStore closed successfully")
		}
	}

	if ar.addressNftIncomeStore != nil {
		log.Println("[DB]Closing addressNftIncomeStore...")
		if err := ar.addressNftIncomeStore.Close(); err != nil {
			log.Printf("[DB]Failed to close addressNftIncomeStore: %v", err)
		} else {
			log.Println("[DB]addressNftIncomeStore closed successfully")
		}
	}

	if ar.contractNftUtxoStore != nil {
		log.Println("[DB]Closing contractNftUtxoStore...")
		if err := ar.contractNftUtxoStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractNftUtxoStore: %v", err)
		} else {
			log.Println("[DB]contractNftUtxoStore closed successfully")
		}
	}

	log.Println("[DB]All resources closed")
}

func main() {
	// Create resource manager
	resources := &AppResources{}

	// Load configuration
	cfg, err := config.LoadConfig("")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	fmt.Println("cfg", cfg)
	config.GlobalConfig = cfg
	config.GlobalNetwork, _ = cfg.GetChainParams()

	// Create auto configuration
	params := config.AutoConfigure(config.SystemResources{
		CPUCores:   cfg.CPUCores,
		MemoryGB:   cfg.MemoryGB,
		HighPerf:   cfg.HighPerf,
		ShardCount: cfg.ShardCount,
	})
	params.MaxTxPerBatch = cfg.MaxTxPerBatch
	common.InitBytePool(params.BytePoolSizeKB)
	storage.DbInit(params)

	// Initialize storage
	resources.contractNftUtxoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTUTXO, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT storage: %v", err)
	}

	resources.addressNftIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressNFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT address income storage: %v", err)
	}

	resources.addressNftSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressNFTSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT address spend storage: %v", err)
	}

	resources.codeHashGenesisNftIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeCodeHashGenesisNFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT codeHash genesis income storage: %v", err)
	}

	resources.codeHashGenesisNftSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeCodeHashGenesisNFTSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT codeHash genesis spend storage: %v", err)
	}

	resources.addressSellNftIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressSellNFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT address sell income storage: %v", err)
	}

	resources.addressSellNftSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressSellNFTSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT address sell spend storage: %v", err)
	}

	resources.codeHashGenesisSellNftIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeCodeHashGenesisSellNFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT codeHash genesis sell income storage: %v", err)
	}

	resources.codeHashGenesisSellNftSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeCodeHashGenesisSellNFTSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT codeHash genesis sell spend storage: %v", err)
	}

	resources.contractNftInfoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTInfo, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT info storage: %v", err)
	}

	resources.contractNftSummaryInfoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTSummaryInfo, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT summary info storage: %v", err)
	}

	resources.contractNftGenesisStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTGenesis, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT genesis storage: %v", err)
	}

	resources.contractNftGenesisOutputStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTGenesisOutput, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT genesis output storage: %v", err)
	}

	resources.contractNftGenesisUtxoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTGenesisUTXO, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT genesis UTXO storage: %v", err)
	}

	resources.contractNftOwnersIncomeValidStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTOwnersIncomeValid, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT owners income valid storage: %v", err)
	}

	resources.contractNftOwnersIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTOwnersIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT owners income storage: %v", err)
	}

	resources.contractNftOwnersSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTOwnersSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT owners spend storage: %v", err)
	}

	resources.contractNftAddressHistoryStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTAddressHistory, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT address history storage: %v", err)
	}

	resources.contractNftGenesisHistoryStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractNFTGenesisHistory, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT genesis history storage: %v", err)
	}

	resources.addressNftIncomeValidStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressNFTIncomeValid, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT income valid storage: %v", err)
	}

	resources.codeHashGenesisNftIncomeValidStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeCodeHashGenesisNFTIncomeValid, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT codeHash genesis income valid storage: %v", err)
	}

	resources.uncheckNftOutpointStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeUnCheckNftIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize NFT verification storage: %v", err)
	}

	resources.usedNftIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeUsedNFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize used NFT contract UTXO storage: %v", err)
	}

	resources.invalidNftOutpointStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeInvalidNftOutpoint, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize invalid NFT contract UTXO storage: %v", err)
	}

	// Create blockchain client
	resources.bcClient, err = blockchain.NewNftClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create blockchain client: %v", err)
	}

	// Create metadata storage
	resources.metaStore, err = storage.NewMetaStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to create metadata storage: %v", err)
	}

	// Verify last indexed height
	lastHeight, err := resources.metaStore.Get([]byte(common.MetaStoreKeyLastNftIndexedHeight))
	if err == nil {
		log.Printf("Resuming NFT indexing from height %s", lastHeight)
	} else if errors.Is(err, storage.ErrNotFound) {
		log.Println("Starting new NFT indexing from genesis block")
	} else {
		log.Printf("Error reading last NFT height: %v", err)
	}

	// Force sync metadata storage to ensure persistence
	if err := resources.metaStore.Sync(); err != nil {
		log.Printf("Failed to sync metadata storage: %v", err)
	}

	// Create stop signal channel
	stopCh := make(chan struct{})

	// Capture interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Handle interrupt signals
	go func() {
		<-sigCh
		log.Println("Received shutdown signal, starting graceful shutdown...")

		// Close stopCh first to notify all goroutines to stop
		close(stopCh)
	}()

	// Create and start backup manager
	backupDir := filepath.Join(cfg.BackupDir, "backups")
	resources.backupMgr = storage.NewBackupManager(cfg.BackupDir, backupDir, cfg.ShardCount)
	if err := resources.backupMgr.Start(); err != nil {
		log.Printf("Failed to start backup manager: %v", err)
	} else {
		log.Println("Database backup manager started")
	}

	// Register all storage instances to backup manager
	log.Println("Registering all storage instances to backup manager")
	resources.backupMgr.RegisterStore("contract_nft_utxo", resources.contractNftUtxoStore)
	resources.backupMgr.RegisterStore("address_nft_income", resources.addressNftIncomeStore)
	resources.backupMgr.RegisterStore("address_nft_spend", resources.addressNftSpendStore)
	resources.backupMgr.RegisterStore("codehash_genesis_nft_income", resources.codeHashGenesisNftIncomeStore)
	resources.backupMgr.RegisterStore("codehash_genesis_nft_spend", resources.codeHashGenesisNftSpendStore)
	resources.backupMgr.RegisterStore("address_sell_nft_income", resources.addressSellNftIncomeStore)
	resources.backupMgr.RegisterStore("address_sell_nft_spend", resources.addressSellNftSpendStore)
	resources.backupMgr.RegisterStore("codehash_genesis_sell_nft_income", resources.codeHashGenesisSellNftIncomeStore)
	resources.backupMgr.RegisterStore("codehash_genesis_sell_nft_spend", resources.codeHashGenesisSellNftSpendStore)
	resources.backupMgr.RegisterStore("contract_nft_info", resources.contractNftInfoStore)
	resources.backupMgr.RegisterStore("contract_nft_summary_info", resources.contractNftSummaryInfoStore)
	resources.backupMgr.RegisterStore("contract_nft_genesis", resources.contractNftGenesisStore)
	resources.backupMgr.RegisterStore("contract_nft_genesis_output", resources.contractNftGenesisOutputStore)
	resources.backupMgr.RegisterStore("contract_nft_genesis_utxo", resources.contractNftGenesisUtxoStore)
	resources.backupMgr.RegisterStore("contract_nft_owners_income_valid", resources.contractNftOwnersIncomeValidStore)
	resources.backupMgr.RegisterStore("contract_nft_owners_income", resources.contractNftOwnersIncomeStore)
	resources.backupMgr.RegisterStore("contract_nft_owners_spend", resources.contractNftOwnersSpendStore)
	resources.backupMgr.RegisterStore("contract_nft_address_history", resources.contractNftAddressHistoryStore)
	resources.backupMgr.RegisterStore("contract_nft_genesis_history", resources.contractNftGenesisHistoryStore)
	resources.backupMgr.RegisterStore("address_nft_income_valid", resources.addressNftIncomeValidStore)
	resources.backupMgr.RegisterStore("codehash_genesis_nft_income_valid", resources.codeHashGenesisNftIncomeValidStore)
	resources.backupMgr.RegisterStore("uncheck_nft_income", resources.uncheckNftOutpointStore)
	resources.backupMgr.RegisterStore("used_nft_income", resources.usedNftIncomeStore)
	resources.backupMgr.RegisterStore("invalid_nft_outpoint", resources.invalidNftOutpointStore)

	resources.backupMgr.RegisterMetaStore(resources.metaStore)

	// Create NFT indexer
	idx := indexer.NewContractNftIndexer(params,
		resources.contractNftUtxoStore,
		resources.addressNftIncomeStore,
		resources.addressNftSpendStore,
		resources.codeHashGenesisNftIncomeStore,
		resources.codeHashGenesisNftSpendStore,
		resources.addressSellNftIncomeStore,
		resources.addressSellNftSpendStore,
		resources.codeHashGenesisSellNftIncomeStore,
		resources.codeHashGenesisSellNftSpendStore,
		resources.contractNftInfoStore,
		resources.contractNftSummaryInfoStore,
		resources.contractNftGenesisStore,
		resources.contractNftGenesisOutputStore,
		resources.contractNftGenesisUtxoStore,
		resources.contractNftOwnersIncomeValidStore,
		resources.contractNftOwnersIncomeStore,
		resources.contractNftOwnersSpendStore,
		resources.contractNftAddressHistoryStore,
		resources.contractNftGenesisHistoryStore,
		resources.addressNftIncomeValidStore,
		resources.codeHashGenesisNftIncomeValidStore,
		resources.uncheckNftOutpointStore,
		resources.usedNftIncomeStore,
		resources.invalidNftOutpointStore,
		resources.metaStore)

	// Create and start NFT verification manager
	resources.verifyManager = indexer.NewNftVerifyManager(idx, 5*time.Second, 1000, params.WorkerCount)
	if err := resources.verifyManager.Start(); err != nil {
		log.Printf("Failed to start NFT verification manager: %v", err)
	} else {
		log.Println("NFT verification manager started")
	}

	// err = idx.FixContractNftOwners()
	// if err != nil {
	// 	log.Printf("[FIX]Failed to fix contract NFT owners: %v", err)
	// } else {
	// 	log.Println("[FIX]Contract NFT owners fixed")
	// }

	// Create mempool manager but don't start it
	log.Printf("Initializing mempool manager, ZMQ address: %s, network: %s", cfg.ZMQAddress, cfg.Network)
	resources.mempoolMgr = mempool.NewNftMempoolManager(cfg.DataDir,
		resources.contractNftUtxoStore,
		resources.contractNftInfoStore,
		resources.contractNftSummaryInfoStore,
		resources.contractNftGenesisStore,
		resources.contractNftGenesisOutputStore,
		resources.contractNftGenesisUtxoStore,
		config.GlobalNetwork, cfg.ZMQAddress[0])
	if resources.mempoolMgr == nil {
		log.Printf("Failed to create mempool manager")
	}

	// Set mempool manager, so indexer can query mempool UTXO
	if resources.mempoolMgr != nil {
		idx.SetMempoolManager(resources.mempoolMgr)
	}

	// Create and start NFT mempool verification manager
	resources.mempoolVerifyManager = mempool.NewNftMempoolVerifier(resources.mempoolMgr, 2*time.Second, 1000, params.WorkerCount)
	if err := resources.mempoolVerifyManager.Start(); err != nil {
		log.Printf("Failed to start mempool NFT verification manager: %v", err)
	} else {
		log.Println("mempool NFT verification manager started")
	}

	// Get current blockchain height
	bestHeight, err := resources.bcClient.GetBlockCount()
	if err != nil {
		log.Fatalf("Failed to get block count: %v", err)
	}

	// Start API server
	resources.server = api.NewNftServer(resources.bcClient, idx, resources.metaStore, stopCh)
	log.Printf("Starting NFT-UTXO indexer API, port: %s", cfg.APIPort)
	resources.server.SetMempoolManager(resources.mempoolMgr, resources.bcClient)
	go resources.server.Start(fmt.Sprintf(":%s", cfg.APIPort))

	lastHeightInt, err := strconv.Atoi(string(lastHeight))
	if err != nil {
		lastHeightInt = 0
		log.Printf("Failed to convert last height, starting from 0: %v", err)
	}

	// Initialize progress bar
	idx.InitProgressBar(bestHeight, lastHeightInt)

	// Check interval for new blocks
	checkInterval := 10 * time.Second
	log.Printf("Syncing index to %d height\n", lastHeightInt)

	log.Println("Starting NFT block sync...")

	firstSyncCompleted := func() {
		log.Println("Initial sync completed, starting mempool")
		err := resources.server.RebuildMempool()
		if err != nil {
			log.Printf("Failed to rebuild mempool: %v", err)
			return
		}
		err = resources.server.StartMempoolCore()
		if err != nil {
			log.Printf("Failed to start mempool core: %v", err)
			return
		}
		log.Println("mempool core started")
	}

	// Use goroutine to start block sync
	go func() {
		if err := resources.bcClient.SyncBlocks(idx, checkInterval, stopCh, firstSyncCompleted); err != nil {
			log.Printf("Failed to sync NFT blocks: %v", err)
		}
	}()

	// Wait for stop signal
	<-stopCh
	log.Println("Program is shutting down...")

	// Get final indexed height
	finalHeight, err := idx.GetLastIndexedHeight()
	if err != nil {
		log.Printf("Error getting final NFT indexed height: %v", err)
	} else {
		log.Printf("Final NFT indexed height: %d", finalHeight)
	}

	// Close all resources
	resources.Close()
}
