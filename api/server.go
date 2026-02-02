package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/mempool"
	"github.com/metaid/utxo_indexer/storage"
)

type Server struct {
	indexer     *indexer.UTXOIndexer
	Router      *gin.Engine
	mempoolMgr  *mempool.MempoolManager
	bcClient    *blockchain.Client
	metaStore   *storage.MetaStore
	stopCh      <-chan struct{}
	mempoolInit bool // Whether the mempool has been initialized
}

func NewServer(indexer *indexer.UTXOIndexer, metaStore *storage.MetaStore, stopCh <-chan struct{}) *Server {
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	server := &Server{
		indexer:     indexer,
		Router:      gin.Default(),
		mempoolInit: false,
		metaStore:   metaStore,
		stopCh:      stopCh,
	}

	server.setupRoutes()
	return server
}

// Set the mempool manager and blockchain client
func (s *Server) SetMempoolManager(mempoolMgr *mempool.MempoolManager, bcClient *blockchain.Client) {
	s.mempoolMgr = mempoolMgr
	s.bcClient = bcClient
}

func (s *Server) setupRoutes() {
	s.setupWebRoutes()
	s.Router.GET("/balance", s.getBalance)
	s.Router.GET("/utxos", s.getUTXOs)
	s.Router.GET("/utxos/spend", s.getSpendUTXOs)
	s.Router.GET("/utxo/db", s.getUtxoByTx)
	s.Router.POST("/tx/btc-utxo/check", s.checkUtxo)
	s.Router.POST("/utxo/check", s.checkUtxo)
	s.Router.GET("/mempool/utxos", s.getMempoolUTXOs)
	s.Router.GET("/cleanedHeight/get", s.getCleanedHeight)
	s.Router.GET("/utxos/history", s.getHistoryUTXOs)
	// Add API to start the mempool
	s.Router.GET("/mempool/start", s.startMempool)
	// Mempool rebuild API
	s.Router.GET("/mempool/rebuild", s.rebuildMempool)
	// Reindex blocks API
	s.Router.GET("/blocks/reindex", s.reindexBlocks)
}

func (s *Server) StartMempoolCore() error {
	if s.mempoolMgr == nil || s.bcClient == nil {
		return fmt.Errorf("Mempool manager or blockchain client not configured")
	}
	if s.mempoolInit {
		return nil // Already started
	}

	log.Println("Starting ZMQ and mempool listener via API...")
	if err := s.mempoolMgr.Start(); err != nil {
		return fmt.Errorf("Failed to start mempool: %w", err)
	}
	s.mempoolInit = true
	log.Println("Mempool manager started via API, listening for new transactions...")

	// Initialize mempool data (load existing mempool transactions)
	go func() {
		log.Println("Starting to initialize mempool data...")
		s.mempoolMgr.InitializeMempool(s.bcClient)
		log.Println("Mempool data initialization complete")
	}()
	return nil
}

// Mempool cleaning goroutine
func (s *Server) startMempoolCleaner() {
	cleanInterval := 10 * time.Second
	for {
		select {
		case <-s.stopCh:
			return
		case <-time.After(cleanInterval):
			lastCleanHeight := 0
			lastCleanHeightBytes, err := s.metaStore.Get([]byte("last_mempool_clean_height"))
			if err == nil {
				lastCleanHeight, _ = strconv.Atoi(string(lastCleanHeightBytes))
			}
			lastIndexedHeight := 0
			lastIndexedHeightBytes, err := s.metaStore.Get([]byte("last_indexed_height"))
			if err == nil {
				lastIndexedHeight, _ = strconv.Atoi(string(lastIndexedHeightBytes))
			}
			if lastIndexedHeight > lastCleanHeight {
				log.Printf("Performing mempool cleaning from height %d to %d", lastCleanHeight+1, lastIndexedHeight)
				for height := lastCleanHeight + 1; height <= lastIndexedHeight; height++ {
					err := s.mempoolMgr.CleanByHeight(height, s.bcClient)
					if err != nil {
						log.Printf("Failed to clean height %d: %v", height, err)
					}
				}
				err := s.metaStore.Set([]byte("last_mempool_clean_height"), []byte(strconv.Itoa(lastIndexedHeight)))
				if err != nil {
					log.Printf("Failed to update last cleaned height: %v", err)
				}
			}
		}
	}
}

// Start mempool API
func (s *Server) startMempool(c *gin.Context) {
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
func (s *Server) RebuildMempool() error {
	if s.mempoolMgr == nil {
		return fmt.Errorf("mempool manager not initialized")
	}
	return s.mempoolMgr.RebuildMempool()
}

// Rebuild mempool API
func (s *Server) rebuildMempool(c *gin.Context) {
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

// reindexBlocks reindexes blocks in the specified range
func (s *Server) reindexBlocks(c *gin.Context) {
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
			"error":   "invalid height range, start must be greater than or equal to 0, end must be greater than or equal to start",
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
		//s.indexer.InitProgressBar(endHeight, startHeight-1)

		// Process each block
		for height := startHeight; height <= endHeight; height++ {
			// Use shared block processing function
			// For manual API reindexing, assume current height is the target height
			if err := s.bcClient.ProcessBlock(s.indexer, height, false, height); err != nil {
				log.Printf("Failed to process block at height %d: %v", height, err)
				continue // Continue processing next block instead of terminating entire reindexing process
			}
		}

		log.Printf("Reindexing completed, processed %d blocks, from height %d to %d", blocksToProcess, startHeight, endHeight)
	}()
}

func (s *Server) getBalance(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address parameter is required"})
		return
	}
	dustThresholdStr := c.DefaultQuery("unsafeValue", "600")
	dustThreshold, err := strconv.ParseInt(dustThresholdStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsafeValue parameter must be a valid integer"})
		return
	}
	balance, err := s.indexer.GetBalance(address, dustThreshold)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, balance)
}

func (s *Server) getUTXOs(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address parameter is required"})
		return
	}

	utxos, err := s.indexer.GetUTXOs(address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address": address,
		"utxos":   utxos,
		"count":   len(utxos),
	})
}
func (s *Server) getSpendUTXOs(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address parameter is required"})
		return
	}

	utxos, err := s.indexer.GetSpendUTXOs(address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address": address,
		"utxos":   utxos,
		"count":   len(utxos),
	})
}
func (s *Server) getUtxoByTx(c *gin.Context) {
	tx := c.Query("tx")
	if tx == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tx parameter is required"})
		return
	}

	utxos, err := s.indexer.GetDbUtxoByTx(tx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"utxos": string(utxos),
	})
}
func (s *Server) getCleanedHeight(c *gin.Context) {
	dbHeight, err := s.metaStore.Get([]byte("last_mempool_clean_height"))
	if err != nil {
		dbHeight = []byte("0")
	}
	c.JSON(http.StatusOK, gin.H{
		"CleanedHeight": indexer.CleanedHeight,
		"dbHeight":      string(dbHeight),
	})
}
func (s *Server) getMempoolUTXOs(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address parameter is required"})
		return
	}

	imcome, spend, err := s.indexer.GetMempoolUTXOs(address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address": address,
		"imcome":  imcome,
		"spend":   spend,
		"count":   len(imcome) + len(spend),
	})
}

func (s *Server) Start(addr string) error {
	// Start the server
	err := s.Router.Run(addr)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
		return err
	}
	return nil
}

func (s *Server) getHistoryUTXOs(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "address parameter is required"})
		return
	}
	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "10")
	utxos, total, err := s.indexer.GetHistoryUTXOs(address, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address": address,
		"list":    utxos,
		"count":   len(utxos),
		"total":   total,
	})
}

// checkUtxo checks if UTXO is spent
func (s *Server) checkUtxo(c *gin.Context) {
	var req common.CheckUtxoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": -2001, "msg": "request parameter error"})
		return
	}

	// 1. Parse OutPoints, build query txhash list and request mapping
	var txList []string
	var reqMap = make(map[string]struct{})
	for _, outPoint := range req.OutPoints {
		arr := strings.Split(outPoint, ":")
		if len(arr) < 2 {
			continue
		}
		txList = append(txList, arr[0])
		reqMap[outPoint] = struct{}{}
	}

	// 2. Batch query transaction UTXO data, build address mapping
	addressMap := make(map[string]string)
	for _, txHash := range txList {
		utxoData, err := s.indexer.GetDbUtxoByTx(txHash)
		if err == nil && len(utxoData) > 0 {
			addressMap[txHash] = string(utxoData)
		}
	}

	// 3. Extract address list from UTXO data
	var tmpMap = make(map[string]struct{})
	for _, outPoint := range req.OutPoints {
		arr := strings.Split(outPoint, ":")
		if len(arr) < 2 {
			continue
		}
		idx, _ := strconv.Atoi(arr[1])
		if v, ok := addressMap[arr[0]]; ok {
			// utxoStore format: address@value@timestamp,address@value@timestamp
			// Note: data may start with comma, need to remove it first
			dataStr := strings.TrimPrefix(v, ",")
			ls := strings.Split(dataStr, ",")
			if len(ls) <= idx {
				continue
			}
			a := strings.Split(ls[idx], "@")
			if len(a) > 0 {
				tmpMap[a[0]] = struct{}{}
			}
		}
	}

	var addressList []string
	for address := range tmpMap {
		addressList = append(addressList, address)
	}

	// 4. Batch query income and spend UTXOs for addresses
	txInfoMap := make(map[string]common.UtxoInfo)
	spendTxMap := make(map[string]common.UtxoInfo)

	for _, address := range addressList {
		// Query income UTXOs for address
		incomeData, _, err := s.indexer.GetIncomeStore().GetWithShard([]byte(address))
		if err == nil && len(incomeData) > 0 {
			// addressStore format: txhash@index@value@timestamp,txhash@index@value@timestamp
			// Note: data may start with comma, need to filter empty strings
			dataStr := strings.TrimPrefix(string(incomeData), ",")
			for _, income := range strings.Split(dataStr, ",") {
				if income == "" {
					continue
				}
				parts := strings.Split(income, "@")
				if len(parts) < 3 {
					continue
				}
				// Build txPoint: txhash:index
				txPoint := parts[0] + ":" + parts[1]
				if _, ok := reqMap[txPoint]; ok {
					value, _ := strconv.ParseInt(parts[2], 10, 64)
					timestamp := int64(0)
					if len(parts) >= 4 {
						timestamp, _ = strconv.ParseInt(parts[3], 10, 64)
					}
					txInfoMap[txPoint] = common.UtxoInfo{
						IsExist:     true,
						Date:        timestamp,
						Value:       value,
						TxConfirm:   true,
						Where:       "block",
						Address:     address,
						SpendStatus: "unspent",
					}
				}
			}
		}

		// Query spend UTXOs for address
		spendData, _, err := s.indexer.GetSpendStore().GetWithShard([]byte(address))
		if err == nil && len(spendData) > 0 {
			// spendStore format: txhash:index@timestamp@spendTxHash,txhash:index@timestamp@spendTxHash
			// Note: data may start with comma, need to filter empty strings
			dataStr := strings.TrimPrefix(string(spendData), ",")
			for _, spend := range strings.Split(dataStr, ",") {
				if spend == "" {
					continue
				}
				parts := strings.Split(spend, "@")
				if len(parts) < 1 {
					continue
				}
				txPoint := parts[0]
				if _, ok := reqMap[txPoint]; ok {
					timestamp := int64(0)
					spendTxHash := ""
					if len(parts) >= 2 {
						timestamp, _ = strconv.ParseInt(parts[1], 10, 64)
					}
					if len(parts) >= 3 {
						spendTxHash = parts[2]
					}
					spendTxMap[txPoint] = common.UtxoInfo{
						Date:    timestamp,
						Address: address,
						Where:   "block",
						SpendInfo: common.UtxoSpendInfo{
							SpendTx: spendTxHash,
							Date:    timestamp,
							Where:   "block",
							Address: address,
						},
					}
				}
			}
		}
	}

	// 5. Assemble results
	utxoInfoMap := make(map[string]common.UtxoInfo)
	for _, outPoint := range req.OutPoints {
		utxoInfo := common.UtxoInfo{
			SpendStatus: "unspent",
		}

		// Check if exists in blockchain
		if v, ok := txInfoMap[outPoint]; ok {
			utxoInfo = v
		} else {
			// If not found in addressStore, check mempool or utxoStore
			arr := strings.Split(outPoint, ":")
			if len(arr) >= 2 {
				index, _ := strconv.Atoi(arr[1])
				// Get from utxoStore
				if utxoData, ok := addressMap[arr[0]]; ok {
					// utxoStore data may start with comma, need to remove first
					dataStr := strings.TrimPrefix(utxoData, ",")
					ls := strings.Split(dataStr, ",")
					if len(ls) > index {
						parts := strings.Split(ls[index], "@")
						if len(parts) >= 2 {
							utxoInfo.IsExist = true
							utxoInfo.Value, _ = strconv.ParseInt(parts[1], 10, 64)
							utxoInfo.Address = parts[0]
							if len(parts) >= 3 {
								utxoInfo.Date, _ = strconv.ParseInt(parts[2], 10, 64)
							}
							// Check if in mempool
							if s.mempoolMgr != nil {
								prefix := parts[0] + "_" + outPoint
								results, err := s.mempoolMgr.MempoolIncomeDB.GetByPrefix(prefix)
								if err == nil && len(results) > 0 {
									utxoInfo.Where = "mempool"
									utxoInfo.TxConfirm = false
								} else {
									utxoInfo.Where = "block"
									utxoInfo.TxConfirm = true
								}
							} else {
								utxoInfo.Where = "block"
								utxoInfo.TxConfirm = true
							}
						}
					}
				}
			}
		}

		// Check spend status
		spendInfo := common.UtxoSpendInfo{}
		if v, ok := spendTxMap[outPoint]; ok {
			utxoInfo.SpendStatus = "spend"
			spendInfo = v.SpendInfo
		} else if s.mempoolMgr != nil && utxoInfo.Address != "" {
			// Check spend in mempool
			// Build query key: address_txhash:index
			prefix := utxoInfo.Address + "_" + outPoint
			results, err := s.mempoolMgr.MempoolSpendDB.GetByPrefix(prefix)
			if err == nil && len(results) > 0 {
				// Found spend record, value is spending txhash
				for _, spendTxHash := range results {
					spendInfo.SpendTx = string(spendTxHash)
					utxoInfo.SpendStatus = "spend"
					spendInfo.Where = "mempool"
					break
				}
			}
		}

		utxoInfo.SpendInfo = spendInfo
		utxoInfoMap[outPoint] = utxoInfo
	}

	c.JSON(http.StatusOK, gin.H{"code": 2000, "msg": "ok", "data": utxoInfoMap})
}
