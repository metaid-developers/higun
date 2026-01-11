package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/metaid/utxo_indexer/common"
	indexer "github.com/metaid/utxo_indexer/indexer/contract/meta-contract-nft"

	"github.com/gin-gonic/gin"
	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/mempool"
	"github.com/metaid/utxo_indexer/storage"
)

type NftServer struct {
	indexer     *indexer.ContractNftIndexer
	router      *gin.Engine
	mempoolMgr  *mempool.NftMempoolManager
	bcClient    *blockchain.NftClient
	metaStore   *storage.MetaStore
	stopCh      <-chan struct{}
	mempoolInit bool // Whether mempool is initialized
}

func NewNftServer(bcClient *blockchain.NftClient, indexer *indexer.ContractNftIndexer, metaStore *storage.MetaStore, stopCh <-chan struct{}) *NftServer {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	server := &NftServer{
		indexer:     indexer,
		router:      gin.Default(),
		mempoolInit: false,
		metaStore:   metaStore,
		stopCh:      stopCh,
		bcClient:    bcClient,
	}

	server.setupRoutes()
	return server
}

// Set mempool manager and blockchain client
func (s *NftServer) SetMempoolManager(mempoolMgr *mempool.NftMempoolManager, bcClient *blockchain.NftClient) {
	s.mempoolMgr = mempoolMgr
	s.bcClient = bcClient
}

func (s *NftServer) setupRoutes() {
	// NFT API routes
	s.router.GET("/nft/address/utxos", s.getNftAddressUtxos)
	s.router.GET("/nft/genesis/utxos", s.getNftGenesisUtxos)
	s.router.GET("/nft/address/sell-utxos", s.getNftAddressSellUtxos)
	s.router.GET("/nft/genesis/sell-utxos", s.getNftGenesisSellUtxos)
	s.router.GET("/nft/address/utxo-count", s.getNftAddressUtxoCount)
	s.router.GET("/nft/address/summary", s.getNftAddressSummary)
	s.router.GET("/nft/summary", s.getNftSummary)
	s.router.GET("/nft/genesis", s.getNftGenesis)
	s.router.GET("/nft/owners", s.getNftOwners)

	// DB query routes
	s.router.GET("/db/nft/utxo", s.getDbNftUtxoByTx)
	s.router.GET("/db/nft/utxo/all", s.getDbAllNftUtxo)
	s.router.GET("/db/nft/address/income", s.getDbAddressNftIncome)
	s.router.GET("/db/nft/address/income/valid", s.getDbAddressNftIncomeValid)
	s.router.GET("/db/nft/address/spend", s.getDbAddressNftSpend)
	s.router.GET("/db/nft/codehash-genesis/income", s.getDbCodeHashGenesisNftIncome)
	s.router.GET("/db/nft/codehash-genesis/spend", s.getDbCodeHashGenesisNftSpend)
	s.router.GET("/db/nft/address/sell-income", s.getDbAddressSellNftIncome)
	s.router.GET("/db/nft/address/sell-spend", s.getDbAddressSellNftSpend)
	s.router.GET("/db/nft/address/sell-income/all", s.getDbAllAddressSellNftIncome)
	s.router.GET("/db/nft/address/sell-spend/all", s.getDbAllAddressSellNftSpend)
	s.router.GET("/db/nft/codehash-genesis/sell-income", s.getDbCodeHashGenesisSellNftIncome)
	s.router.GET("/db/nft/codehash-genesis/sell-spend", s.getDbCodeHashGenesisSellNftSpend)
	s.router.GET("/db/nft/codehash-genesis/sell-income/all", s.getDbAllCodeHashGenesisSellNftIncome)
	s.router.GET("/db/nft/codehash-genesis/sell-spend/all", s.getDbAllCodeHashGenesisSellNftSpend)
	s.router.GET("/db/nft/info", s.getDbAllNftInfo)
	s.router.GET("/db/nft/genesis", s.getAllDbNftGenesis)
	s.router.GET("/db/nft/genesis/output", s.getAllDbNftGenesisOutput)
	s.router.GET("/db/nft/uncheck/outpoint", s.getAllDbUncheckNftOutpoint)
	s.router.GET("/db/nft/uncheck/outpoint/all", s.getAllDbUncheckNftOutpoint)
	s.router.GET("/db/nft/used/income", s.getAllDbUsedNftIncome)
	s.router.GET("/db/nft/invalid/outpoint", s.getDbInvalidNftOutpoint)

	// Add mempool start API
	s.router.GET("/nft/mempool/start", s.startMempool)
	// Mempool rebuild API
	s.router.GET("/nft/mempool/rebuild", s.rebuildMempool)
	// Reindex blocks API
	s.router.GET("/nft/blocks/reindex", s.reindexBlocks)

	// Add mempool query interfaces
	s.router.GET("/db/nft/mempool/spend", s.getMempoolAddressNftSpendMap)
	s.router.GET("/db/nft/mempool/address/income", s.getMempoolAddressNftIncomeMap)
	s.router.GET("/db/nft/mempool/address/income/valid", s.getMempoolAddressNftIncomeValidMap)
}

// Start mempool API
func (s *NftServer) StartMempoolCore() error {
	// Check if mempool manager is configured
	if s.mempoolMgr == nil || s.bcClient == nil {
		return fmt.Errorf("mempool manager or blockchain client not configured")
	}

	// Check if already initialized
	if s.mempoolInit {
		return nil
	}

	// Start mempool
	log.Println("Starting ZMQ and mempool monitoring via API...")
	err := s.mempoolMgr.Start()
	if err != nil {
		return fmt.Errorf("mempool startup failed: %w", err)
	}

	// Mark as initialized
	s.mempoolInit = true
	log.Println("Mempool manager started via API, monitoring new transactions...")

	// Initialize mempool data (load existing mempool transactions)
	go func() {
		log.Println("Starting NFT mempool data initialization...")
		s.mempoolMgr.InitializeMempool(s.bcClient)
		log.Println("NFT mempool data initialization completed")
	}()

	// Get current index height as cleanup start height
	lastIndexedHeightBytes, err := s.metaStore.Get([]byte(common.MetaStoreKeyLastNftIndexedHeight))
	if err == nil {
		// Set current height as cleanup start height to avoid cleaning historical blocks
		log.Println("Setting mempool cleanup start height to current index height:", string(lastIndexedHeightBytes))
		err = s.metaStore.Set([]byte(common.MetaStoreKeyLastNftMempoolCleanHeight), lastIndexedHeightBytes)
		if err != nil {
			log.Printf("Failed to set mempool cleanup start height: %v", err)
		}
	} else {
		log.Printf("Failed to get current index height: %v", err)
	}

	go s.startMempoolCleaner()
	return nil
}

func (s *NftServer) startMempoolCleaner() {
	// Mempool cleanup interval
	cleanInterval := 10 * time.Second

	for {
		select {
		case <-s.stopCh:
			return
		case <-time.After(cleanInterval):
			// 1. Get last cleaned height
			lastCleanHeight := 0
			lastCleanHeightBytes, err := s.metaStore.Get([]byte(common.MetaStoreKeyLastNftMempoolCleanHeight))
			if err == nil {
				lastCleanHeight, _ = strconv.Atoi(string(lastCleanHeightBytes))
			}

			// 2. Get latest indexed height
			lastIndexedHeight := 0
			lastIndexedHeightBytes, err := s.metaStore.Get([]byte(common.MetaStoreKeyLastNftIndexedHeight))
			if err == nil {
				lastIndexedHeight, _ = strconv.Atoi(string(lastIndexedHeightBytes))
			}

			// 3. If latest indexed height is greater than last cleaned height, perform cleanup
			if lastIndexedHeight > lastCleanHeight {
				log.Printf("Performing mempool cleanup from height %d to %d", lastCleanHeight+1, lastIndexedHeight)

				// Perform cleanup for each new block
				for height := lastCleanHeight + 1; height <= lastIndexedHeight; height++ {
					err := s.mempoolMgr.CleanByHeight(height, s.bcClient)
					if err != nil {
						log.Printf("Failed to clean height %d: %v", height, err)
					}
				}

				// Update last cleaned height
				err := s.metaStore.Set([]byte(common.MetaStoreKeyLastNftMempoolCleanHeight), []byte(strconv.Itoa(lastIndexedHeight)))
				if err != nil {
					log.Printf("Failed to update last cleaned height: %v", err)
				}
			}
		}
	}
}

func (s *NftServer) startMempool(c *gin.Context) {
	err := s.StartMempoolCore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mempool started successfully",
		"status":  "running",
	})
}

func (s *NftServer) RebuildMempool() error {
	return s.mempoolMgr.CleanAllMempool()
}

// Rebuild mempool API
func (s *NftServer) rebuildMempool(c *gin.Context) {
	// Check if mempool manager is configured
	err := s.RebuildMempool()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	err = s.StartMempoolCore()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mempool started successfully",
		"status":  "running",
	})
}

// reindexBlocks reindexes blocks in specified range
func (s *NftServer) reindexBlocks(c *gin.Context) {
	// Check if blockchain client is configured
	if s.bcClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Blockchain client not configured",
		})
		return
	}

	// Parse request parameters
	startHeightStr := c.Query("start")
	endHeightStr := c.Query("end")

	if startHeightStr == "" || endHeightStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "start and end parameters are required",
		})
		return
	}

	startHeight, err := strconv.Atoi(startHeightStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "start parameter must be a valid integer",
		})
		return
	}

	endHeight, err := strconv.Atoi(endHeightStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "end parameter must be a valid integer",
		})
		return
	}

	// Validate height range
	if startHeight < 0 || endHeight < startHeight {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid height range, start must be greater than or equal to 0, end must be greater than or equal to start",
		})
		return
	}

	// Check current latest block height
	currentHeight, err := s.bcClient.GetBlockCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get current block height: " + err.Error(),
		})
		return
	}

	if endHeight > currentHeight {
		endHeight = currentHeight
	}

	// Return response immediately, start reindexing in background
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Starting to reindex blocks, range from %d to %d", startHeight, endHeight),
	})

	// Start reindexing process in background
	go func() {
		log.Printf("Starting to reindex blocks, range from %d to %d", startHeight, endHeight)

		// Set progress bar
		blocksToProcess := endHeight - startHeight + 1

		// Process each block
		for height := startHeight; height <= endHeight; height++ {
			// Use shared block processing function
			if err := s.bcClient.ProcessBlock(s.indexer, height, false); err != nil {
				log.Printf("Failed to process block, height %d: %v", height, err)
				continue // Continue processing next block instead of terminating entire reindex process
			}
		}

		log.Printf("Reindexing completed, processed %d blocks, from height %d to %d", blocksToProcess, startHeight, endHeight)
	}()
}

func (s *NftServer) Start(addr string) error {
	// Start the server
	err := s.router.Run(addr)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
		return err
	}
	return nil
}
