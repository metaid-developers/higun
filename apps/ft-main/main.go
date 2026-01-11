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

	indexer "github.com/metaid/utxo_indexer/indexer/contract/meta-contract-ft"
	"github.com/metaid/utxo_indexer/mempool"

	"github.com/metaid/utxo_indexer/api"
	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
)

// AppResources 统一管理所有应用资源
type AppResources struct {
	// Storage resources
	contractFtUtxoStore          *storage.PebbleStore
	addressFtIncomeStore         *storage.PebbleStore
	addressFtSpendStore          *storage.PebbleStore
	contractFtInfoStore          *storage.PebbleStore
	contractFtGenesisStore       *storage.PebbleStore
	contractFtGenesisOutputStore *storage.PebbleStore
	contractFtGenesisUtxoStore   *storage.PebbleStore

	contractFtInfoSensibleIdStore    *storage.PebbleStore
	contractFtGenesisOutStore        *storage.PebbleStore
	contractFtSupplyStore            *storage.PebbleStore
	contractFtBurnStore              *storage.PebbleStore
	contractFtOwnersIncomeValidStore *storage.PebbleStore
	contractFtOwnersIncomeStore      *storage.PebbleStore
	contractFtOwnersSpendStore       *storage.PebbleStore
	contractFtAddressHistoryStore    *storage.PebbleStore
	contractFtGenesisHistoryStore    *storage.PebbleStore

	addressFtIncomeValidStore *storage.PebbleStore
	uncheckFtOutpointStore    *storage.PebbleStore
	usedFtIncomeStore         *storage.PebbleStore
	uniqueFtIncomeStore       *storage.PebbleStore
	uniqueFtSpendStore        *storage.PebbleStore
	invalidFtOutpointStore    *storage.PebbleStore
	metaStore                 *storage.MetaStore

	// Blockchain and other resources
	bcClient             *blockchain.FtClient
	verifyManager        *indexer.FtVerifyManager
	mempoolMgr           *mempool.FtMempoolManager
	mempoolVerifyManager *mempool.FtMempoolVerifier
	server               *api.FtServer
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
		log.Println("Closing FT verification manager...")
		ar.verifyManager.Stop()
		log.Println("FT verification manager closed successfully")
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

	if ar.invalidFtOutpointStore != nil {
		log.Println("[DB]Closing invalidFtOutpointStore...")
		if err := ar.invalidFtOutpointStore.Close(); err != nil {
			log.Printf("[DB]Failed to close invalidFtOutpointStore: %v", err)
		} else {
			log.Println("[DB]invalidFtOutpointStore closed successfully")
		}
	}

	if ar.uniqueFtSpendStore != nil {
		log.Println("[DB]Closing uniqueFtSpendStore...")
		if err := ar.uniqueFtSpendStore.Close(); err != nil {
			log.Printf("[DB]Failed to close uniqueFtSpendStore: %v", err)
		} else {
			log.Println("[DB]uniqueFtSpendStore closed successfully")
		}
	}

	if ar.uniqueFtIncomeStore != nil {
		log.Println("[DB]Closing uniqueFtIncomeStore...")
		if err := ar.uniqueFtIncomeStore.Close(); err != nil {
			log.Printf("[DB]Failed to close uniqueFtIncomeStore: %v", err)
		} else {
			log.Println("[DB]uniqueFtIncomeStore closed successfully")
		}
	}

	if ar.usedFtIncomeStore != nil {
		log.Println("[DB]Closing usedFtIncomeStore...")
		if err := ar.usedFtIncomeStore.Close(); err != nil {
			log.Printf("[DB]Failed to close usedFtIncomeStore: %v", err)
		} else {
			log.Println("[DB]usedFtIncomeStore closed successfully")
		}
	}

	if ar.uncheckFtOutpointStore != nil {
		log.Println("[DB]Closing uncheckFtOutpointStore...")
		if err := ar.uncheckFtOutpointStore.Close(); err != nil {
			log.Printf("[DB]Failed to close uncheckFtOutpointStore: %v", err)
		} else {
			log.Println("[DB]uncheckFtOutpointStore closed successfully")
		}
	}

	if ar.addressFtIncomeValidStore != nil {
		log.Println("[DB]Closing addressFtIncomeValidStore...")
		if err := ar.addressFtIncomeValidStore.Close(); err != nil {
			log.Printf("[DB]Failed to close addressFtIncomeValidStore: %v", err)
		} else {
			log.Println("[DB]addressFtIncomeValidStore closed successfully")
		}
	}

	if ar.contractFtGenesisUtxoStore != nil {
		log.Println("[DB]Closing contractFtGenesisUtxoStore...")
		if err := ar.contractFtGenesisUtxoStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractFtGenesisUtxoStore: %v", err)
		} else {
			log.Println("[DB]contractFtGenesisUtxoStore closed successfully")
		}
	}

	if ar.contractFtGenesisOutputStore != nil {
		log.Println("[DB]Closing contractFtGenesisOutputStore...")
		if err := ar.contractFtGenesisOutputStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractFtGenesisOutputStore: %v", err)
		} else {
			log.Println("[DB]contractFtGenesisOutputStore closed successfully")
		}
	}

	if ar.contractFtGenesisStore != nil {
		log.Println("[DB]Closing contractFtGenesisStore...")
		if err := ar.contractFtGenesisStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractFtGenesisStore: %v", err)
		} else {
			log.Println("[DB]contractFtGenesisStore closed successfully")
		}
	}

	if ar.contractFtInfoStore != nil {
		log.Println("[DB]Closing contractFtInfoStore...")
		if err := ar.contractFtInfoStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractFtInfoStore: %v", err)
		} else {
			log.Println("[DB]contractFtInfoStore closed successfully")
		}
	}

	if ar.addressFtSpendStore != nil {
		log.Println("[DB]Closing addressFtSpendStore...")
		if err := ar.addressFtSpendStore.Close(); err != nil {
			log.Printf("[DB]Failed to close addressFtSpendStore: %v", err)
		} else {
			log.Println("[DB]addressFtSpendStore closed successfully")
		}
	}

	if ar.addressFtIncomeStore != nil {
		log.Println("[DB]Closing addressFtIncomeStore...")
		if err := ar.addressFtIncomeStore.Close(); err != nil {
			log.Printf("[DB]Failed to close addressFtIncomeStore: %v", err)
		} else {
			log.Println("[DB]addressFtIncomeStore closed successfully")
		}
	}

	if ar.contractFtUtxoStore != nil {
		log.Println("[DB]Closing contractFtUtxoStore...")
		if err := ar.contractFtUtxoStore.Close(); err != nil {
			log.Printf("[DB]Failed to close contractFtUtxoStore: %v", err)
		} else {
			log.Println("[DB]contractFtUtxoStore closed successfully")
		}
	}

	log.Println("[DB]All resources closed")
}

func main() {
	// 创建资源管理器
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
	resources.contractFtUtxoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTUTXO, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT storage: %v", err)
	}

	resources.addressFtIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT address storage: %v", err)
	}

	resources.addressFtSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressFTSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT spend storage: %v", err)
	}

	resources.contractFtInfoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTInfo, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT info storage: %v", err)
	}

	resources.contractFtGenesisStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTGenesis, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT genesis storage: %v", err)
	}

	resources.contractFtGenesisOutputStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTGenesisOutput, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT genesis output storage: %v", err)
	}

	resources.contractFtGenesisUtxoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTGenesisUTXO, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT genesis UTXO storage: %v", err)
	}

	resources.contractFtInfoSensibleIdStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTInfoSensibleId, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT info sensible ID storage: %v", err)
	}

	resources.contractFtSupplyStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTSupply, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT supply storage: %v", err)
	}

	resources.contractFtBurnStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTBurn, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT burn storage: %v", err)
	}

	resources.contractFtOwnersIncomeValidStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTOwnersIncomeValid, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT owners income valid storage: %v", err)
	}

	resources.contractFtOwnersIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTOwnersIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT owners income storage: %v", err)
	}

	resources.contractFtOwnersSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTOwnersSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT owners spend storage: %v", err)
	}

	resources.contractFtAddressHistoryStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTAddressHistory, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT address history storage: %v", err)
	}

	resources.contractFtGenesisHistoryStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeContractFTGenesisHistory, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT genesis history storage: %v", err)
	}

	resources.addressFtIncomeValidStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressFTIncomeValid, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT income valid storage: %v", err)
	}

	resources.uncheckFtOutpointStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeUnCheckFtIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize FT verification storage: %v", err)
	}

	resources.usedFtIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeUsedFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize used FT contract UTXO storage: %v", err)
	}

	resources.uniqueFtIncomeStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeUniqueFTIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize unique contract UTXO storage: %v", err)
	}

	resources.uniqueFtSpendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeUniqueFTSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize unique contract UTXO storage: %v", err)
	}

	resources.invalidFtOutpointStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeInvalidFtOutpoint, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize invalid FT contract UTXO storage: %v", err)
	}

	// Create blockchain client
	resources.bcClient, err = blockchain.NewFtClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create blockchain client: %v", err)
	}

	// Create metadata storage
	resources.metaStore, err = storage.NewMetaStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to create metadata storage: %v", err)
	}

	// Verify last indexed height
	lastHeight, err := resources.metaStore.Get([]byte(common.MetaStoreKeyLastFtIndexedHeight))
	if err == nil {
		log.Printf("Resuming FT indexing from height %s", lastHeight)
	} else if errors.Is(err, storage.ErrNotFound) {
		log.Println("Starting new FT indexing from genesis block")
	} else {
		log.Printf("Error reading last FT height: %v", err)
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
	resources.backupMgr.RegisterStore("contract_ft_utxo", resources.contractFtUtxoStore)
	resources.backupMgr.RegisterStore("address_ft_income", resources.addressFtIncomeStore)
	resources.backupMgr.RegisterStore("address_ft_spend", resources.addressFtSpendStore)
	resources.backupMgr.RegisterStore("contract_ft_info", resources.contractFtInfoStore)
	resources.backupMgr.RegisterStore("contract_ft_genesis", resources.contractFtGenesisStore)
	resources.backupMgr.RegisterStore("contract_ft_genesis_output", resources.contractFtGenesisOutputStore)
	resources.backupMgr.RegisterStore("contract_ft_genesis_utxo", resources.contractFtGenesisUtxoStore)
	resources.backupMgr.RegisterStore("address_ft_income_valid", resources.addressFtIncomeValidStore)
	resources.backupMgr.RegisterStore("uncheck_ft_income", resources.uncheckFtOutpointStore)
	resources.backupMgr.RegisterStore("used_ft_income", resources.usedFtIncomeStore)
	resources.backupMgr.RegisterStore("unique_ft_income", resources.uniqueFtIncomeStore)
	resources.backupMgr.RegisterStore("unique_ft_spend", resources.uniqueFtSpendStore)
	resources.backupMgr.RegisterStore("invalid_ft_outpoint", resources.invalidFtOutpointStore)

	resources.backupMgr.RegisterStore("contract_ft_info_sensible_id", resources.contractFtInfoSensibleIdStore)
	resources.backupMgr.RegisterStore("contract_ft_supply", resources.contractFtSupplyStore)
	resources.backupMgr.RegisterStore("contract_ft_burn", resources.contractFtBurnStore)
	resources.backupMgr.RegisterStore("contract_ft_owners_income_valid", resources.contractFtOwnersIncomeValidStore)
	resources.backupMgr.RegisterStore("contract_ft_owners_income", resources.contractFtOwnersIncomeStore)
	resources.backupMgr.RegisterStore("contract_ft_owners_spend", resources.contractFtOwnersSpendStore)
	resources.backupMgr.RegisterStore("contract_ft_address_history", resources.contractFtAddressHistoryStore)
	resources.backupMgr.RegisterStore("contract_ft_genesis_history", resources.contractFtGenesisHistoryStore)

	resources.backupMgr.RegisterMetaStore(resources.metaStore)

	// Create FT indexer
	idx := indexer.NewContractFtIndexer(params,
		resources.contractFtUtxoStore,
		resources.addressFtIncomeStore,
		resources.addressFtSpendStore,
		resources.contractFtInfoStore,
		resources.contractFtGenesisStore,
		resources.contractFtGenesisOutputStore,
		resources.contractFtGenesisUtxoStore,

		resources.contractFtInfoSensibleIdStore,
		resources.contractFtSupplyStore,
		resources.contractFtBurnStore,
		resources.contractFtOwnersIncomeValidStore,
		resources.contractFtOwnersIncomeStore,
		resources.contractFtOwnersSpendStore,
		resources.contractFtAddressHistoryStore,
		resources.contractFtGenesisHistoryStore,

		resources.addressFtIncomeValidStore,
		resources.uncheckFtOutpointStore,
		resources.usedFtIncomeStore,
		resources.uniqueFtIncomeStore,
		resources.uniqueFtSpendStore,
		resources.invalidFtOutpointStore,
		resources.metaStore)

	// Create and start FT verification manager
	resources.verifyManager = indexer.NewFtVerifyManager(idx, 5*time.Second, 1000, params.WorkerCount)
	if err := resources.verifyManager.Start(); err != nil {
		log.Printf("Failed to start FT verification manager: %v", err)
	} else {
		log.Println("FT verification manager started")
	}

	// err = idx.FixFtGenesisOutputStore()
	// if err != nil {
	// 	log.Printf("[FIX]Failed to fix FT genesis output store: %v", err)
	// } else {
	// 	log.Println("[FIX]FT genesis output store fixed")
	// }
	// err = idx.FixFtSupply()
	// if err != nil {
	// 	log.Printf("[FIX]Failed to fix FT supply: %v", err)
	// } else {
	// 	log.Println("[FIX]FT supply fixed")
	// }
	// err = idx.FixContractFtInfo()
	// if err != nil {
	// 	log.Printf("[FIX]Failed to fix contract FT info: %v", err)
	// } else {
	// 	log.Println("[FIX]Contract FT info fixed")
	// }

	// err = idx.FixContractFtOwners()
	// if err != nil {
	// 	log.Printf("[FIX]Failed to fix contract FT owners: %v", err)
	// } else {
	// 	log.Println("[FIX]Contract FT owners fixed")
	// }

	// err = idx.FixContractUniqueFtUtxo()
	// if err != nil {
	// 	log.Printf("[FIX]Failed to fix contract unique FT UTXO: %v", err)
	// } else {
	// 	log.Println("[FIX]Contract unique FT UTXO fixed")
	// }

	// Create mempool manager but don't start it
	log.Printf("Initializing mempool manager, ZMQ address: %s, network: %s", cfg.ZMQAddress, cfg.Network)
	resources.mempoolMgr = mempool.NewFtMempoolManager(cfg.DataDir,
		resources.contractFtUtxoStore,
		resources.contractFtInfoStore,
		resources.contractFtGenesisStore,
		resources.contractFtGenesisOutputStore,
		resources.contractFtGenesisUtxoStore,
		config.GlobalNetwork, cfg.ZMQAddress[0])
	if resources.mempoolMgr == nil {
		log.Printf("Failed to create mempool manager")
	}

	// Set mempool manager, so indexer can query mempool UTXO
	if resources.mempoolMgr != nil {
		idx.SetMempoolManager(resources.mempoolMgr)
	}

	// Create and start FT verification manager
	resources.mempoolVerifyManager = mempool.NewFtMempoolVerifier(resources.mempoolMgr, 2*time.Second, 1000, params.WorkerCount)
	if err := resources.mempoolVerifyManager.Start(); err != nil {
		log.Printf("Failed to start mempool FT verification manager: %v", err)
	} else {
		log.Println("mempool FT verification manager started")
	}

	// Get current blockchain height
	bestHeight, err := resources.bcClient.GetBlockCount()
	if err != nil {
		log.Fatalf("Failed to get block count: %v", err)
	}

	// Start API server
	resources.server = api.NewFtServer(resources.bcClient, idx, resources.metaStore, stopCh)
	log.Printf("Starting FT-UTXO indexer API, port: %s", cfg.APIPort)
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

	log.Println("Starting FT block sync...")

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
			log.Printf("Failed to sync FT blocks: %v", err)
		}
	}()

	// Wait for stop signal
	<-stopCh
	log.Println("Program is shutting down...")

	// Get final indexed height
	finalHeight, err := idx.GetLastIndexedHeight()
	if err != nil {
		log.Printf("Error getting final FT indexed height: %v", err)
	} else {
		log.Printf("Final FT indexed height: %d", finalHeight)
	}

	// Close all resources
	resources.Close()
}
