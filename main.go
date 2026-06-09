package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/metaid/utxo_indexer/api"
	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/diagnostics"
	"github.com/metaid/utxo_indexer/explorer/blockindexer"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/mempool"
	"github.com/metaid/utxo_indexer/storage"
	"github.com/metaid/utxo_indexer/syslogs"
	"github.com/metaid/utxo_indexer/wallet"
)

var ApiServer *api.Server
var newMempoolManagerFn = mempool.NewMempoolManager

// blockchainClientWrapper wraps blockchain.Client to implement indexer.BlockchainClient
type blockchainClientWrapper struct {
	*blockchain.Client
}

func (w *blockchainClientWrapper) GetBlock(height int64) (*indexer.Block, error) {
	return w.GetBlockByHeight(height)
}

func main() {
	fmt.Println("Starting UTXO Indexer...")
	defer func() {
		if r := recover(); r != nil {
			log.Printf("==============>global panic: %v", r)
		}
	}()
	cfg, params := initConfig()
	stopDiagnostics, err := diagnostics.StartServer(cfg.Diagnostics, cfg.MemoryBudget.CgroupRoot)
	if err != nil {
		log.Fatalf("Failed to start diagnostics server: %v", err)
	}
	if stopDiagnostics != nil {
		defer stopDiagnostics()
		log.Printf("[Diagnostics] enabled at http://%s/debug/memory", cfg.Diagnostics.Bind)
	}
	// block info indexer
	if cfg.BlockInfoIndexer {
		startBlockIndexer(cfg)
	}
	utxoStore, addressStore, spendStore, balanceStore, rankStore, bcClient, metaStore, mempoolMgr, err := initDb(cfg, params)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer closeDb(utxoStore, addressStore, spendStore, balanceStore, rankStore, bcClient, metaStore, mempoolMgr)
	// Verify last indexed height
	lastHeight, err := metaStore.Get([]byte("last_indexed_height"))
	if err == nil {
		log.Printf("Resuming index from height %s", lastHeight)
	} else if errors.Is(err, storage.ErrNotFound) {
		log.Println("Starting new index from genesis block")
	} else {
		log.Printf("Error reading last height: %v", err)
	}

	// Force sync meta store to ensure durability
	if err := metaStore.Sync(); err != nil {
		log.Printf("Failed to sync metadata storage: %v", err)
	}
	// Ensure last_mempool_clean_height has initial value
	_, err = metaStore.Get([]byte("last_mempool_clean_height"))
	if errors.Is(err, storage.ErrNotFound) {
		// First run, use start height from config file
		startHeight := strconv.Itoa(cfg.MemPoolCleanStartHeight)
		log.Printf("Initializing mempool cleanup height to: %s", startHeight)
		err = metaStore.Set([]byte("last_mempool_clean_height"), []byte(startHeight))
		if err != nil {
			log.Printf("Failed to initialize mempool cleanup height: %v", err)
		}
	}

	// Create stop signal channel
	stopCh := make(chan struct{})

	// Capture interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Handle interrupt signal
	go func() {
		<-sigCh
		log.Println("Received stop signal, preparing to shutdown...")
		close(stopCh)
	}()

	idx := indexer.NewUTXOIndexer(params, utxoStore, addressStore, metaStore, spendStore)
	idx.SetBalanceStores(balanceStore, rankStore)

	// Set blockchain client for cache warmup
	if bcClient != nil {
		wrapper := &blockchainClientWrapper{Client: bcClient}
		idx.SetBlockchainClient(wrapper)
		idx.SetConfirmedUTXOValidator(bcClient, cfg.UTXOValidationEnabled, cfg.UTXOValidationConcurrency)
	}

	// Set mempool manager so indexer can query mempool UTXOs
	if mempoolMgr != nil {
		idx.SetMempoolManager(mempoolMgr)
	}
	if cfg.Chain == config.ChainMVC {
		if cfg.MVCTxIDAliasBackfillEnabled {
			go func() {
				if err := bcClient.BackfillMVCTxIDAliases(idx, stopCh); err != nil {
					log.Printf("[TxIDAliasBackfill] MVC txid alias backfill failed: %v", err)
				}
			}()
		} else {
			log.Printf("[TxIDAliasBackfill] MVC txid alias backfill disabled by config")
		}
	}
	// Pass mempool manager and blockchain client to API server
	ApiServer = api.NewServer(idx, metaStore, stopCh)
	ApiServer.SetMempoolManager(mempoolMgr, bcClient)
	if cfg.Wallet.Enabled {
		walletCfg := wallet.FromAppConfig(cfg.Wallet)
		if err := ApiServer.EnableWalletGateway(walletCfg); err != nil {
			log.Fatalf("Failed to enable wallet gateway: %v", err)
		}
		log.Printf("Wallet Gateway enabled")
	}
	log.Printf("Starting UTXO indexer API, port: %s", cfg.APIPort)
	blockindexer.SetRouter(ApiServer)
	go ApiServer.Start(fmt.Sprintf(":%s", cfg.APIPort))
	balanceIndexReady := false
	if readyBytes, readyErr := metaStore.Get([]byte("balance_index_ready")); readyErr == nil && string(readyBytes) == "1" {
		balanceIndexReady = true
	}
	if !balanceIndexReady {
		log.Println("[BalanceIndex] Automatic bootstrap disabled on startup; /balance will use history fallback and /rich-list remains unavailable until a manual rebuild is performed")
	}
	// Get current blockchain height
	var bestHeight int
	//for {
	bestHeight, err = bcClient.GetBlockCount()
	if err != nil {
		log.Printf("Failed to get block count: %v, retrying in 3 seconds...", err)
		//time.Sleep(3 * time.Second)
		//continue
	}
	lastCleanHeightInt := int64(0)
	lastCleanHeight, err := metaStore.Get([]byte("last_mempool_clean_height"))
	if err == nil {
		log.Printf("Resuming from mempool cleanup height %s", string(lastCleanHeight))
		lastCleanHeightInt, _ = strconv.ParseInt(string(lastCleanHeight), 10, 64)
	}
	if lastCleanHeightInt == 0 {
		lastCleanHeightInt = int64(bestHeight)
	}

	if int64(cfg.MemPoolCleanStartHeight) > lastCleanHeightInt {
		lastCleanHeightInt = int64(cfg.MemPoolCleanStartHeight)
	}
	indexer.CleanedHeight = lastCleanHeightInt
	indexer.BaseCount.BlockLastHeight = int64(bestHeight)
	//	break
	//}

	lastHeightInt, err := strconv.Atoi(string(lastHeight))
	if err != nil {
		lastHeightInt = 0
		log.Printf("Failed to convert last height, starting from 0: %v", err)
	}
	//lastHeightInt = 121050

	// Warmup memory UTXO cache for better initial performance
	if lastHeightInt > 0 {
		idx.WarmupMemoryUTXO(lastHeightInt)
	}

	// Initialize progress bar
	idx.InitProgressBar(bestHeight, lastHeightInt)
	indexer.BaseCount.LocalLastHeight = int64(lastHeightInt)

	// Interval to check for new blocks
	checkInterval := 10 * time.Second
	idx.InitBaseCount()
	go func() {
		defer log.Println("SyncBaseCount goroutine exited")
		idx.SyncBaseCount()
	}()
	log.Println("Starting block synchronization...")
	//log.Println("Note: Mempool not automatically started, please use API '/mempool/start' to start mempool after block sync is complete")
	go bcClient.CheckReorg(idx)
	log.Println("[RichList] Using balance_rank index mode; periodic full-scan warmup disabled")
	// Use goroutine to start block synchronization, no longer automatically start mempool
	go func() {
		for {
			if err := bcClient.SyncBlocks(idx, checkInterval, stopCh, firstSyncCompleted); err != nil {
				errMsg := syslogs.ErrLog{
					ErrType:      "SyncBlocks",
					Timestamp:    time.Now().Unix(),
					ErrorMessage: err.Error(),
				}
				syslogs.InsertErrLog(errMsg)
				log.Printf("Block synchronization failed: %v, retrying in 15 seconds...", err)
				select {
				case <-stopCh:
					return
				case <-time.After(15 * time.Second):
				}
			} else {
				// Normal exit via stopCh
				return
			}
		}
	}()

	// Wait for stop signal
	<-stopCh
	log.Println("Program is shutting down...")

	// 关闭 mempool 管理器
	// if mempoolMgr != nil {
	// 	mempoolMgr.Stop()
	// 	mempoolMgr = nil
	// }
	// 关闭区块链客户端
	if bcClient != nil {
		bcClient.Shutdown()
		bcClient = nil
	}
	// Program won't execute here unless stop signal is received
	finalHeight, err := idx.GetLastIndexedHeight()
	if err != nil {
		log.Printf("Error getting final indexed height: %v", err)
	} else {
		log.Printf("Final indexed height: %d", finalHeight)
	}
}
func firstSyncCompleted() {
	//return
	log.Println("Initial sync completed, attempting to start mempool")
	err := ApiServer.RebuildMempool()
	if err != nil {
		log.Printf("INFO: Mempool functionality disabled - %v", err)
		log.Println("The indexer will continue running without mempool features")
		return
	}
	err = ApiServer.StartMempoolCore()
	if err != nil {
		log.Printf("Failed to start mempool core: %v", err)
		return
	}
	log.Println("Mempool core started successfully")
}

func configuredZMQAddresses(addresses []string) []string {
	filtered := make([]string, 0, len(addresses))
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		filtered = append(filtered, address)
	}
	return filtered
}

func hasConfiguredZMQAddresses(addresses []string) bool {
	return len(configuredZMQAddresses(addresses)) > 0
}

func newMempoolManagerIfConfigured(cfg *config.Config, utxoStore *storage.PebbleStore, chainCfg *chaincfg.Params) *mempool.MempoolManager {
	addresses := configuredZMQAddresses(cfg.ZMQAddress)
	if len(addresses) == 0 {
		log.Printf("Mempool disabled: no zmq_address configured")
		return nil
	}
	return newMempoolManagerFn(cfg.DataDir, utxoStore, chainCfg, addresses)
}

func initConfig() (cfg *config.Config, params config.IndexerParams) {
	// Load config
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if cfg.MemoryBudget.ForceCgroupLimitBytes <= 0 && cfg.MemoryBudget.UseCgroupLimit {
		cfg.MemoryBudget.ForceCgroupLimitBytes = config.DetectCgroupLimitBytes(cfg.MemoryBudget.CgroupRoot)
	}
	budget := config.ChooseMemoryBudget(cfg.MemoryGB, cfg.MemoryBudget)
	config.ApplyGoMemoryLimit(budget)

	syslogs.InitIndexerLogDB(cfg.DataDir + "/higun.db")
	config.GlobalConfig = cfg
	config.GlobalNetwork, _ = cfg.GetChainParams()

	// Create auto configuration
	effectiveMemoryGB := int(budget.EffectiveMemoryBytes >> 30)
	if effectiveMemoryGB < 1 {
		effectiveMemoryGB = 1
	}
	params = config.AutoConfigure(config.SystemResources{
		CPUCores:   cfg.CPUCores,
		MemoryGB:   effectiveMemoryGB,
		HighPerf:   cfg.HighPerf,
		ShardCount: cfg.ShardCount,
	})
	params.MemoryBudget = budget
	params.MaxTxPerBatch = config.GlobalConfig.MaxTxPerBatch

	return
}
func startBlockIndexer(cfg *config.Config) {
	// Execute block info indexing
	fmt.Println("Initializing block info index...")
	blockindexer.IndexerInit("blockinfo_data", cfg)
	go blockindexer.DoBlockInfoIndex()
	go blockindexer.SaveBlockInfoData()
	fmt.Println("blockindexer.IndexerInit success")
}
func initDb(cfg *config.Config, params config.IndexerParams) (utxoStore *storage.PebbleStore, addressStore *storage.PebbleStore, spendStore *storage.PebbleStore, balanceStore *storage.PebbleStore, rankStore *storage.PebbleStore, bcClient *blockchain.Client, metaStore *storage.MetaStore, mempoolMgr *mempool.MempoolManager, err error) {
	common.InitBytePool(params.BytePoolSizeKB)
	log.Println("common.InitBytePool success")
	storage.DbInit(params)
	log.Println("storage.DbInit success")
	// Initialize storage
	utxoStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeUTXO, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize UTXO storage: %v", err)
	}
	//defer utxoStore.Close()
	log.Println("storage.NewPebbleStore utxoStore success")
	addressStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeIncome, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize address storage: %v", err)
	}
	log.Println("storage.NewPebbleStore addressStore success")
	//defer addressStore.Close()

	spendStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeSpend, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize spend storage: %v", err)
	}
	//defer spendStore.Close()
	log.Println("storage.NewPebbleStore spendStore success")
	balanceStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeAddressBalance, cfg.ShardCount)
	if err != nil {
		log.Fatalf("Failed to initialize address_balance storage: %v", err)
	}
	log.Println("storage.NewPebbleStore balanceStore success")
	rankStore, err = storage.NewPebbleStore(params, cfg.DataDir, storage.StoreTypeBalanceRank, 1)
	if err != nil {
		log.Fatalf("Failed to initialize balance_rank storage: %v", err)
	}
	log.Println("storage.NewPebbleStore rankStore success")

	// 使用适配器架构创建区块链客户端
	// Create blockchain client using adapter architecture
	log.Printf("Initializing blockchain adapter: chain=%s", cfg.Chain)
	bcClient, err = blockchain.NewClientWithAdapter(cfg)
	if err != nil {
		log.Fatalf("Failed to create blockchain client: %v", err)
	}
	//defer bcClient.Shutdown()
	log.Printf("✓ Blockchain adapter initialized successfully: %s", cfg.Chain)
	// Create metadata storage (create early for mempool cleanup use)
	metaStore, err = storage.NewMetaStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to create metadata storage: %v", err)
	}
	//defer metaStore.Close()
	log.Println("storage.NewMetaStore metaStore success")
	// Get chain parameters
	chainCfg, err := cfg.GetChainParams()
	if err != nil {
		log.Fatalf("Failed to get chain parameters: %v", err)
	}
	log.Printf("DEBUG: Chain parameters obtained successfully, proceeding to mempool initialization")

	// Create mempool manager, but don't start
	log.Printf("Initializing mempool manager, ZMQ address: %s, network: %s", cfg.ZMQAddress, cfg.Network)
	log.Printf("DEBUG: About to call NewMempoolManager with DataDir=%s", cfg.DataDir)
	mempoolMgr = newMempoolManagerIfConfigured(cfg, utxoStore, chainCfg)
	log.Printf("DEBUG: NewMempoolManager returned, mempoolMgr is nil: %v", mempoolMgr == nil)
	if mempoolMgr == nil {
		if hasConfiguredZMQAddresses(cfg.ZMQAddress) {
			log.Printf("WARNING: Failed to create mempool manager. The program will continue but mempool functionality will be disabled.")
			log.Printf("This may be due to insufficient permissions or disk space for mempool database files in: %s", cfg.DataDir+"/mempool_*")
		}
	} else {
		log.Printf("Mempool manager initialized successfully")
	}
	return
}
func closeDb(utxoStore, addressStore, spendStore, balanceStore, rankStore *storage.PebbleStore, bcClient *blockchain.Client, metaStore *storage.MetaStore, mempoolMgr *mempool.MempoolManager) {
	if mempoolMgr != nil {
		log.Println("Close mempoolMgr")
		mempoolMgr.Stop()
	}
	if utxoStore != nil {
		log.Println("Close utxoStore")
		utxoStore.Close()
	}
	if addressStore != nil {
		log.Println("Close addressStore")
		addressStore.Close()
	}
	if spendStore != nil {
		log.Println("Close spendStore")
		spendStore.Close()
	}
	if balanceStore != nil {
		log.Println("Close balanceStore")
		balanceStore.Close()
	}
	if rankStore != nil {
		log.Println("Close rankStore")
		rankStore.Close()
	}
	if bcClient != nil {
		log.Println("Close bcClient")
		bcClient.Shutdown()
	}
	if metaStore != nil {
		log.Println("Close metaStore")
		metaStore.Close()
	}

}
