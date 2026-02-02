package indexer

import (
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-colorable"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
	"github.com/metaid/utxo_indexer/syslogs"
	"github.com/schollz/progressbar/v3"
)

// BlockchainClient interface for accessing blockchain data during warmup
type BlockchainClient interface {
	GetBlock(height int64) (*Block, error)
}

type UTXOIndexer struct {
	utxoStore        *storage.PebbleStore
	addressStore     *storage.PebbleStore
	spendStore       *storage.PebbleStore
	metaStore        *storage.MetaStore
	mu               sync.RWMutex
	bar              *progressbar.ProgressBar
	params           config.IndexerParams
	mempoolManager   MempoolManager   // Use interface type instead of interface{}
	blockchainClient BlockchainClient // RPC client for warmup
	// Memory UTXO cache for performance
	memUTXO         sync.Map // key: "txid:index" -> value: "address@amount@blockTime"
	memUTXOCount    int64    // Number of UTXOs in memory
	memHits         int64    // Memory cache hits
	dbHits          int64    // Database query hits
	memUTXOMaxCount int64    // Maximum number of UTXOs to cache (default: 5 million)
}

var workers = 1

var batchSize = 1000
var CleanedHeight int64 // Used to record cleanup height
func NewUTXOIndexer(params config.IndexerParams, utxoStore, addressStore *storage.PebbleStore, metaStore *storage.MetaStore, spendStore *storage.PebbleStore) *UTXOIndexer {
	maxCount := int64(config.GlobalConfig.MemUTXOMaxCount)
	if maxCount <= 0 {
		maxCount = 3000000 // Default: 3M (~480MB)
	}
	return &UTXOIndexer{
		params:          params,
		utxoStore:       utxoStore,
		addressStore:    addressStore,
		metaStore:       metaStore,
		spendStore:      spendStore,
		memUTXOMaxCount: maxCount,
	}
}

// func (i *UTXOIndexer) optimizeConcurrency(dataSize int) int {
// 	// Dynamically adjust concurrency based on data size and system resources
// 	// Use fewer goroutines for small data, limit max concurrency for large data
// 	optimalWorkers := i.workers

// 	if dataSize < 1000 {
// 		// Use fewer goroutines for small data
// 		optimalWorkers = min(i.workers, 2)
// 	} else if dataSize > 10000 {
// 		// Limit max goroutines for large data
// 		maxWorkers := runtime.NumCPU() * 2 // Or set a fixed upper limit
// 		optimalWorkers = min(i.workers, maxWorkers)
// 	}

//		return optimalWorkers
//	}
func (i *UTXOIndexer) InitProgressBar(totalBlocks, startHeight int) {
	remainingBlocks := totalBlocks - startHeight
	if remainingBlocks <= 0 {
		remainingBlocks = 1 // Set to at least 1 to avoid errors
	}
	i.bar = progressbar.NewOptions(remainingBlocks,
		progressbar.OptionSetWriter(colorable.NewColorableStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(50),
		progressbar.OptionSetDescription("Indexing blocks..."),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionSetRenderBlankState(false), // Don't show blank state
		progressbar.OptionShowCount(),                // Show count (e.g., 1/3)
		progressbar.OptionShowIts(),                  // Show iterations
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(colorable.NewColorableStdout(), "\nDone!\n")
		}),
		//progressbar.OptionSetFormat("%s %s %d/%d (%.2f ops/s)"), // Custom format: progress bar + progress + speed
	)
}

// SetMempoolCleanedHeight 设置内存池清理高度
// 只应在处理最新区块时调用，避免在同步历史区块时频繁清理内存池
func (i *UTXOIndexer) SetMempoolCleanedHeight(height int64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	CleanedHeight = height
	i.metaStore.Set([]byte("last_mempool_clean_height"), []byte(strconv.FormatInt(height, 10)))
}

// WarmupMemoryUTXO preloads recent UTXOs into memory cache by fetching recent blocks via RPC
func (i *UTXOIndexer) WarmupMemoryUTXO(currentHeight int) {
	log.Printf("[MemUTXO] Starting memory cache warmup from height %d...", currentHeight)
	startTime := time.Now()

	targetCount := int(i.memUTXOMaxCount)
	if targetCount <= 0 {
		log.Printf("[MemUTXO] Warmup skipped: cache disabled (maxCount=%d)", targetCount)
		return
	}

	if i.blockchainClient == nil {
		log.Printf("[MemUTXO] Warmup skipped: blockchain client not set")
		return
	}

	log.Printf("[MemUTXO] Loading up to %d recent UTXOs via RPC (reverse scan from height %d)...", targetCount, currentHeight)

	loaded := 0
	blocksFetched := 0
	for height := currentHeight; height > 0 && loaded < targetCount; height-- {
		block, err := i.blockchainClient.GetBlock(int64(height))
		if err != nil {
			log.Printf("[MemUTXO] Warning: Failed to fetch block at height %d: %v", height, err)
			break
		}

		blocksFetched++
		// Only process outputs (income), skip inputs (spend) for faster warmup
		for _, tx := range block.Transactions {
			for idx, out := range tx.Outputs {
				if loaded >= targetCount {
					break
				}
				key := common.ConcatBytesOptimized([]string{tx.ID, strconv.Itoa(idx)}, ":")
				value := common.ConcatBytesOptimized([]string{out.Address, out.Amount, strconv.Itoa(block.Height)}, "@")
				i.memUTXO.Store(key, value)
				loaded++
				atomic.AddInt64(&i.memUTXOCount, 1) // 同步更新计数器
			}
			if loaded >= targetCount {
				break
			}
		}

		// Progress log every 100 blocks
		if blocksFetched%100 == 0 {
			log.Printf("[MemUTXO] Warmup progress: %d blocks, %d UTXOs loaded (%.1f%%)",
				blocksFetched, loaded, float64(loaded)*100/float64(targetCount))
		}
	}

	// Count is already updated by atomic operations in the loop above

	elapsed := time.Since(startTime)
	log.Printf("[MemUTXO] Warmup completed: Loaded %d UTXOs from %d recent blocks in %.2fs (%.0f UTXOs/s, %.1f blocks/s)",
		loaded, blocksFetched, elapsed.Seconds(),
		float64(loaded)/elapsed.Seconds(), float64(blocksFetched)/elapsed.Seconds())
	log.Printf("[MemUTXO] Expected initial hit rate: 70-80%% for recent blocks")
}

// PrintMemoryStats prints memory UTXO cache statistics
func (i *UTXOIndexer) PrintMemoryStats() {
	totalQueries := atomic.LoadInt64(&i.memHits) + atomic.LoadInt64(&i.dbHits)
	hitRate := float64(0)
	if totalQueries > 0 {
		hitRate = float64(atomic.LoadInt64(&i.memHits)) * 100 / float64(totalQueries)
	}
	log.Printf("[MemUTXO Stats] Hit rate: %.1f%% (mem:%d, db:%d), Cache size: %d UTXOs",
		hitRate, atomic.LoadInt64(&i.memHits), atomic.LoadInt64(&i.dbHits), atomic.LoadInt64(&i.memUTXOCount))
}

func (i *UTXOIndexer) IndexBlock(block *Block, allBlock *Block, updateHeight bool, blockTimeStr string) (inCnt int, outCnt int, addressNum int, err error) {
	if block == nil {
		return 0, 0, 0, fmt.Errorf("cannot index nil block")
	}

	// Set global worker count and batch size
	workers = config.GlobalConfig.Workers
	batchSize = config.GlobalConfig.BatchSize

	// Validate block
	if err := block.Validate(); err != nil {
		return 0, 0, 0, fmt.Errorf("invalid block: %w", err)
	}

	// Since batch processing is already done in the convertBlock stage, complex large block processing logic is no longer needed here
	// Directly process transactions in the current batch

	// Phase 1: Index all outputs
	tIncome := time.Now()
	if cnt, addressCnt, err := i.indexIncome(block, allBlock, blockTimeStr); err != nil {
		errMsg := syslogs.ErrLog{
			Height:       block.Height,
			BlockHash:    block.BlockHash,
			ErrType:      "ProcessIncome",
			Timestamp:    time.Now().Unix(),
			ErrorMessage: err.Error(),
		}
		go syslogs.InsertErrLog(errMsg)
		return 0, 0, 0, fmt.Errorf("failed to index outputs: %w", err)
	} else {
		outCnt = cnt
		addressNum = addressCnt
	}
	incomeTime := time.Since(tIncome)
	//存储utxo归档文件
	SaveBlockFile("utxo", allBlock, true)
	// After phase 1 is complete, some memory can be released
	block.AddressIncome = nil
	//log.Println("==>i.processSpend")
	// Phase 2: Process all inputs
	tSpend := time.Now()
	if cnt, err := i.processSpend(block, allBlock, blockTimeStr); err != nil {
		errMsg := syslogs.ErrLog{
			Height:       block.Height,
			BlockHash:    block.BlockHash,
			ErrType:      "ProcessSpend",
			Timestamp:    time.Now().Unix(),
			ErrorMessage: err.Error(),
		}
		go syslogs.InsertErrLog(errMsg)
		return 0, 0, 0, fmt.Errorf("failed to process inputs: %w", err)
	} else {
		inCnt = cnt
	}
	spendTime := time.Since(tSpend)
	// After phase 2 is complete, release transaction data
	block.Transactions = nil

	//存储spend归档文件
	SaveBlockFile("spend", allBlock, true)

	// If it's a partial batch of a large block, don't update the index height, wait for the last batch
	if !block.IsPartialBlock && updateHeight {
		// 区块处理完成，确保所有数据持久化到磁盘
		tSync := time.Now()

		// 数据一致性策略：每30个区块做一次完整同步（降低IO等待）
		// - Pebble的WAL机制保证：即使不Sync，数据也会写入WAL日志
		// - 崩溃恢复时WAL会重放未落盘的数据
		// - 每30块Sync可减少磁盘IO，降低CPU等待时间
		// - 代价：崩溃后最多重新索引29个区块（可接受）
		shouldSync := block.Height%30 == 0

		if shouldSync {
			// 完整同步：先同步数据Store，确保UTXO/地址/花费数据落盘
			i.utxoStore.Sync()
			i.addressStore.Sync()
			i.spendStore.Sync()
		}

		// 更新索引高度（每个区块都更新，依赖WAL保护）
		heightStr := strconv.Itoa(block.Height)
		err := i.metaStore.Set([]byte("last_indexed_height"), []byte(heightStr))
		if err != nil {
			errMsg := syslogs.ErrLog{
				Height:       block.Height,
				BlockHash:    block.BlockHash,
				ErrType:      "MetaStoreLastIndexed",
				Timestamp:    time.Now().Unix(),
				ErrorMessage: err.Error(),
			}
			go syslogs.InsertErrLog(errMsg)
			return 0, 0, 0, fmt.Errorf("failed to update last indexed height: %w", err)
		}

		// MetaStore也遵循同样策略：每10块Sync一次
		// 注意：这意味着崩溃可能丢失最近9块的进度记录，需要重新索引
		// 但由于WAL的存在，实际数据不会丢失，只是需要重新处理
		if shouldSync {
			i.metaStore.Sync()
		}

		syncTime := time.Since(tSync)

		// 定期强制GC和内存统计（每100个区块）
		if block.Height%100 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Printf("[Memory] Height %d: Alloc=%dMB, TotalAlloc=%dMB, Sys=%dMB, NumGC=%d, MemUTXO=%d",
				block.Height, m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC, atomic.LoadInt64(&i.memUTXOCount))

			// 如果内存超过6GB，强制GC（降低阈值）
			if m.Alloc > 6*1024*1024*1024 {
				log.Printf("[Memory] WARNING: Memory usage exceeds 6GB (%.2fGB), forcing GC...", float64(m.Alloc)/1024/1024/1024)
				runtime.GC()
				runtime.ReadMemStats(&m)
				log.Printf("[Memory] After GC: Alloc=%.2fGB", float64(m.Alloc)/1024/1024/1024)
			}
		}

		// 只在每10个区块才同步metaStore
		if shouldSync {
			i.metaStore.Sync()
		}

		//最后再存储下File（异步执行，不阻塞主流程）
		go SaveBlockFile("utxo", allBlock, false)
		go SaveBlockFile("spend", allBlock, false)

		// Update progress bar
		if i.bar != nil {
			i.bar.Add(1)
		}

		// 只在Spend操作超过1秒时打印详细性能日志
		if spendTime.Seconds() > 1.0 {
			log.Printf("[Perf-Index-Slow] Height %d: Income=%.3fs, Spend=%.3fs, Sync=%.3fs (InCnt=%d, OutCnt=%d, Addr=%d)",
				block.Height, incomeTime.Seconds(), spendTime.Seconds(), syncTime.Seconds(), inCnt, outCnt, addressNum)
		}

		// 只在每100个区块强制GC一次，避免频繁GC影响性能
		if block.Height%100 == 0 {
			log.Printf("[Memory] Height %d: Forcing GC...", block.Height)
			runtime.GC()
		}
	}
	// Finally release the block object
	block = nil
	return inCnt, outCnt, addressNum, nil
}
func SaveBlockFile(fileType string, allBlock *Block, isPart bool) {
	// 检查是否启用区块文件归档
	if !config.GlobalConfig.BlockFilesEnabled {
		return
	}
	if isPart {
		if fileType == "utxo" {
			//log.Println("--->cur utxo size:", len(allBlock.UtxoData), len(allBlock.IncomeData))
			if len(allBlock.UtxoData) <= 200000 {
				return
			}
		}
		if fileType == "spend" {
			cnt := 0
			for _, v := range allBlock.SpendData {
				cnt += len(v)
			}
			//log.Println("--->cur spend size:", cnt)
			if cnt <= 200000 {
				return
			}
		}
	}
	//log.Println("------------> SaveBlockFile", fileType)
	fblock := BlockToFBlock(allBlock, fileType)
	if fileType == "utxo" {
		err := SaveFBlockPart(fblock, fileType, allBlock.UtxoPartIndex)
		if err != nil {
			fmt.Println("<=== SaveFBlock Error ===>", err)
			errMsg := syslogs.ErrLog{
				Height:       allBlock.Height,
				BlockHash:    allBlock.BlockHash,
				ErrType:      "SaveFBlock",
				Timestamp:    time.Now().Unix(),
				ErrorMessage: err.Error(),
			}
			go syslogs.InsertErrLog(errMsg)
		}
		//释放内存
		fblock = nil
		allBlock.UtxoData = make(map[string][]string)
		allBlock.UtxoPartIndex += 1
		allBlock.IncomeData = make(map[string][]string)
	} else if fileType == "spend" {
		err := SaveFBlockPart(fblock, fileType, allBlock.SpendPartIndex)
		if err != nil {
			fmt.Println("<=== SaveFBlock Error ===>", err)
			errMsg := syslogs.ErrLog{
				Height:       allBlock.Height,
				BlockHash:    allBlock.BlockHash,
				ErrType:      "SaveFBlock",
				Timestamp:    time.Now().Unix(),
				ErrorMessage: err.Error(),
			}
			go syslogs.InsertErrLog(errMsg)
		}
		//释放内存
		fblock = nil
		allBlock.SpendData = make(map[string][]string)
		allBlock.SpendPartIndex += 1
	}
}
func (i *UTXOIndexer) indexIncome(block *Block, allBlock *Block, blockTimeStr string) (cnt int, addressNum int, err error) {
	// Set reasonable batch size based on memory conditions
	//const batchSize = 1000
	workers = config.GlobalConfig.Workers
	batchSize = config.GlobalConfig.BatchSize
	// Calculate the number of batches to process
	txCount := len(block.Transactions)
	batchCount := (txCount + batchSize - 1) / batchSize
	blockHeight := int64(block.Height)
	// Process in batches
	for batchIndex := 0; batchIndex < batchCount; batchIndex++ {
		start := batchIndex * batchSize
		end := start + batchSize
		if end > txCount {
			end = txCount
		}

		// Create temporary maps for current batch
		//addressIncomeMap := make(map[string][]string)
		//txMap := make(map[string][]string)
		currBatchSize := end - start
		addressIncomeMap := make(map[string][]string, currBatchSize*3) // Assume each transaction has an average of 2 outputs
		txMap := make(map[string][]string, currBatchSize)

		// Only process transactions in current batch
		var mempoolIncomeKeys []string
		var inCnt = 0
		for i := start; i < end; i++ {
			tx := block.Transactions[i]
			for x, out := range tx.Outputs {
				if out.Address == "" {
					out.Address = "errAddress"
				}
				if out.Amount == "" {
					out.Amount = "0"
				}
				inCnt++
				v := common.ConcatBytesOptimized([]string{out.Address, out.Amount, blockTimeStr}, "@")
				txMap[tx.ID] = append(txMap[tx.ID], v)
				// 只在BlockFilesEnabled时才累积到allBlock（避免内存泄露）
				if config.GlobalConfig.BlockFilesEnabled {
					allBlock.UtxoData[tx.ID] = append(allBlock.UtxoData[tx.ID], v)
				}
				// Use pre-allocated slices to reduce memory reallocation
				if _, exists := addressIncomeMap[out.Address]; !exists {
					// Pre-allocate a slice with reasonable capacity
					addressIncomeMap[out.Address] = make([]string, 0, 4) // Assume most addresses have less than 4 outputs
				}
				if out.Address != "errAddress" {
					v := common.ConcatBytesOptimized([]string{tx.ID, strconv.Itoa(x), out.Amount, blockTimeStr}, "@")
					addressIncomeMap[out.Address] = append(addressIncomeMap[out.Address], v)
					// 只在BlockFilesEnabled时才累积到allBlock（避免内存泄露）
					if config.GlobalConfig.BlockFilesEnabled {
						allBlock.IncomeData[out.Address] = append(allBlock.IncomeData[out.Address], v)
					}
				}
				// Whether to clean up mempool income records
				if blockHeight > CleanedHeight {
					// If it's a partial batch of a large block, record mempool income
					txPoint := common.ConcatBytesOptimized([]string{tx.ID, strconv.Itoa(x)}, ":")
					// if out.Address == "19egopKjkPDphD9THoj6qbqG13Pf5DcCnj" {
					// 	log.Printf("Found special address %s in block %d, txpoint %s", out.Address, blockHeight, txPoint)
					// }
					mempoolIncomeKeys = append(mempoolIncomeKeys, common.ConcatBytesOptimized([]string{out.Address, txPoint}, "_"))
				}
			}
		}

		// Process current batch
		//workers := 1
		if err = i.utxoStore.BulkMergeMapConcurrent(&txMap, workers); err != nil {
			errMsg := syslogs.ErrLog{
				Height:       block.Height,
				BlockHash:    block.BlockHash,
				ErrType:      "UtxoStoreBulkMerge",
				Timestamp:    time.Now().Unix(),
				ErrorMessage: err.Error(),
			}
			go syslogs.InsertErrLog(errMsg)
			return 0, 0, err
		} else {
			cnt = inCnt
		}

		// Store UTXOs in memory cache for fast lookup (with capacity limit)
		currentCount := atomic.LoadInt64(&i.memUTXOCount)

		// Eviction strategy: when cache reaches capacity, evict oldest UTXOs (by block height) to make room
		// This implements FIFO: oldest blocks' UTXOs are removed first
		if currentCount >= i.memUTXOMaxCount {
			evictCount := int64(float64(i.memUTXOMaxCount) * 0.10) // Remove 10%
			log.Printf("[MemUTXO] Cache full (%d/%d), evicting %d oldest entries (FIFO)",
				currentCount, i.memUTXOMaxCount, evictCount)

			// Collect entries with their block heights for sorting
			type utxoEntry struct {
				key    interface{}
				height int64
			}
			var entries []utxoEntry
			currentBlockHeight := int64(block.Height)

			// Sample entries to find oldest ones (avoid full scan for performance)
			// We'll scan more entries than needed to ensure we find old ones
			sampleSize := int(evictCount * 3) // Sample 3x the evict count
			sampledCount := 0
			i.memUTXO.Range(func(key, value interface{}) bool {
				if sampledCount >= sampleSize {
					return false // Stop sampling
				}
				valueStr := value.(string)
				// Parse block height from value: "address@amount@blockHeight"
				parts := strings.Split(valueStr, "@")
				if len(parts) >= 3 {
					height, err := strconv.ParseInt(parts[2], 10, 64)
					if err == nil {
						entries = append(entries, utxoEntry{key: key, height: height})
					}
				}
				sampledCount++
				return true
			})

			// Sort by height (oldest first) and delete the oldest ones
			if len(entries) > 0 {
				// Simple selection of oldest entries without full sort
				var evictedCount int64
				minHeightThreshold := currentBlockHeight - 10000 // Anything older than 10k blocks is definitely old

				// First pass: delete very old entries
				for _, entry := range entries {
					if entry.height < minHeightThreshold {
						i.memUTXO.Delete(entry.key)
						evictedCount++
						if evictedCount >= evictCount {
							break
						}
					}
				}

				// Second pass: if not enough deleted, delete by random sampling from remaining
				if evictedCount < evictCount {
					remaining := evictCount - evictedCount
					evictionProb := float64(remaining) / float64(len(entries)-int(evictedCount))
					for _, entry := range entries {
						if entry.height >= minHeightThreshold && rand.Float64() < evictionProb {
							i.memUTXO.Delete(entry.key)
							evictedCount++
							if evictedCount >= evictCount {
								break
							}
						}
					}
				}

				atomic.AddInt64(&i.memUTXOCount, -evictedCount)
				log.Printf("[MemUTXO] Evicted %d entries (FIFO), new count: %d", evictedCount, atomic.LoadInt64(&i.memUTXOCount))
			}
		}

		// Now store new UTXOs
		for txID, outputs := range txMap {
			for idx := range outputs {
				if atomic.LoadInt64(&i.memUTXOCount) >= i.memUTXOMaxCount {
					break // Stop adding once limit is reached
				}
				memKey := common.ConcatBytesOptimized([]string{txID, strconv.Itoa(idx)}, ":")
				i.memUTXO.Store(memKey, outputs[idx])
				atomic.AddInt64(&i.memUTXOCount, 1)
			}
		}

		if err = i.addressStore.BulkMergeMapConcurrent(&addressIncomeMap, workers); err != nil {
			errMsg := syslogs.ErrLog{
				Height:       block.Height,
				BlockHash:    block.BlockHash,
				ErrType:      "AddressStoreBulkMerge",
				Timestamp:    time.Now().Unix(),
				ErrorMessage: err.Error(),
			}
			go syslogs.InsertErrLog(errMsg)
			return 0, 0, err
		} else {
			addressNum = len(addressIncomeMap)
		}
		if len(mempoolIncomeKeys) > 0 && i.mempoolManager != nil && blockHeight > CleanedHeight {
			//log.Printf("Deleting %d mempool income records for block height %d,first key:%s", len(mempoolIncomeKeys), blockHeight, mempoolIncomeKeys[0])
			err := i.mempoolManager.BatchDeleteIncom(mempoolIncomeKeys)
			if err != nil {
				errMsg := syslogs.ErrLog{
					Height:       block.Height,
					BlockHash:    block.BlockHash,
					ErrType:      "MempoolBatchDeleteIncom",
					Timestamp:    time.Now().Unix(),
					ErrorMessage: err.Error(),
				}
				go syslogs.InsertErrLog(errMsg)
				log.Printf("Failed to delete mempool income records: %v", err)
			}
			mempoolIncomeKeys = nil // Clean up memory
		}

		// Immediately clean up memory for current batch
		for k := range txMap {
			delete(txMap, k)
		}
		for k := range addressIncomeMap {
			delete(addressIncomeMap, k)
		}
		txMap = nil
		addressIncomeMap = nil

		//log.Printf("Indexed finish block %d batch %d/%d: processed %d transactions, total inputs %d, unique addresses %d", block.Height, batchIndex+1, batchCount, currBatchSize, cnt, addressNum)
		// Optional: Force garbage collection
		// runtime.GC()
	}
	//log.Printf("finish income index")
	return cnt, addressNum, nil
}

func (i *UTXOIndexer) processSpend(block *Block, allBlock *Block, blockTimeStr string) (cnt int, err error) {
	workers = config.GlobalConfig.Workers
	batchSize = config.GlobalConfig.BatchSize
	blockHeight := int64(block.Height)
	// Collect all transaction points
	type InputInfo struct {
		Point string
		TxID  string
	}
	var allInputs []InputInfo
	var inCnt = 0
	for _, tx := range block.Transactions {
		for _, in := range tx.Inputs {
			allInputs = append(allInputs, InputInfo{Point: in.TxPoint, TxID: tx.ID})
			inCnt += 1
		}
	}

	// Calculate batches
	totalPoints := len(allInputs)
	//log.Println(">>>[processSpend] totalPoints:", totalPoints)
	batchCount := (totalPoints + batchSize - 1) / batchSize

	// Process in batches
	for batchIndex := 0; batchIndex < batchCount; batchIndex++ {
		start := batchIndex * batchSize
		end := start + batchSize
		if end > totalPoints {
			end = totalPoints
		}

		// Transaction points for current batch
		batchInputs := allInputs[start:end]
		var batchPoints []string
		pointTxMap := make(map[string]string)
		for _, in := range batchInputs {
			batchPoints = append(batchPoints, in.Point)
			pointTxMap[in.Point] = in.TxID
		}

		var deleteKeys []string
		//log.Printf(">>>[processSpend] QueryUTXOAddresses,size: %d", len(batchPoints))
		// Query current batch - first check memory, then database
		tQuery := time.Now()

		// Step 1: Check memory cache first (优化：减少字符串操作)
		addressResult := make(map[string][]string, len(batchPoints)/4) // 预分配
		dbQueryPoints := make([]string, 0, len(batchPoints)/2)         // 预估50%命中率

		for _, point := range batchPoints {
			if value, ok := i.memUTXO.Load(point); ok {
				// Memory hit
				atomic.AddInt64(&i.memHits, 1)
				valueStr := value.(string)

				// 优化：使用strings.IndexByte替代循环查找
				atIdx := strings.IndexByte(valueStr, '@')
				if atIdx > 0 {
					address := valueStr[:atIdx]
					addressResult[address] = append(addressResult[address], point)
				}

				// Delete from memory after spending
				i.memUTXO.Delete(point)
				atomic.AddInt64(&i.memUTXOCount, -1)
			} else {
				// Memory miss, need to query database
				atomic.AddInt64(&i.dbHits, 1)
				dbQueryPoints = append(dbQueryPoints, point)
			}
		}

		tMemQuery := time.Since(tQuery)

		// Step 2: Query database for missed points
		var dbResult map[string][]string
		tDbQuery := time.Now()
		if len(dbQueryPoints) > 0 {
			dbResult, err = i.utxoStore.QueryUTXOAddresses2(&dbQueryPoints)
			if err != nil {
				errMsg := syslogs.ErrLog{
					Height:       block.Height,
					BlockHash:    block.BlockHash,
					ErrType:      "UtxoStoreQueryAddresses",
					Timestamp:    time.Now().Unix(),
					ErrorMessage: err.Error(),
				}
				go syslogs.InsertErrLog(errMsg)
				return 0, fmt.Errorf("failed to query UTXO addresses: %w", err)
			}
			// Merge database results
			for k, v := range dbResult {
				addressResult[k] = append(addressResult[k], v...)
			}
		}
		dbQueryTime := time.Since(tDbQuery)
		queryTime := time.Since(tQuery)
		//add time
		for k, v := range addressResult {
			for idx := range v {
				outpoint := v[idx]
				deleteKeys = append(deleteKeys, common.ConcatBytesOptimized([]string{k, outpoint}, "_"))
				spendingTxID := pointTxMap[outpoint]
				v[idx] = common.ConcatBytesOptimized([]string{outpoint, blockTimeStr, spendingTxID}, "@")
			}
			addressResult[k] = v
		}
		//log.Printf(">>>[processSpend] finish QueryUTXOAddresses")
		// 只在BlockFilesEnabled时才累积到allBlock（避免内存泄露）
		if config.GlobalConfig.BlockFilesEnabled {
			for k, v := range addressResult {
				allBlock.SpendData[k] = append(allBlock.SpendData[k], v...)
			}
		}
		// Process results for current batch
		//workers := 1
		if err := i.spendStore.BulkMergeMapConcurrent(&addressResult, workers); err != nil {
			errMsg := syslogs.ErrLog{
				Height:       block.Height,
				BlockHash:    block.BlockHash,
				ErrType:      "SpendStoreBulkMerge",
				Timestamp:    time.Now().Unix(),
				ErrorMessage: err.Error(),
			}
			go syslogs.InsertErrLog(errMsg)
			return 0, fmt.Errorf("failed to merge address results: %w", err)
		} else {
			cnt = inCnt
		}

		// 只在第一个batch且数据库查询超过0.5秒时打印详细信息
		if batchIndex == 0 && dbQueryTime.Seconds() > 0.5 {
			totalQueries := atomic.LoadInt64(&i.memHits) + atomic.LoadInt64(&i.dbHits)
			hitRate := float64(0)
			if totalQueries > 0 {
				hitRate = float64(atomic.LoadInt64(&i.memHits)) * 100 / float64(totalQueries)
			}
			log.Printf("[Perf-Spend-Slow] Height %d: Query=%.3fs (Mem=%.3fs, DB=%.3fs), DBQueries=%d, MemHit=%.1f%%, MemUTXO=%d",
				block.Height, queryTime.Seconds(), tMemQuery.Seconds(), dbQueryTime.Seconds(),
				len(dbQueryPoints), hitRate, atomic.LoadInt64(&i.memUTXOCount))
		}
		//log.Println(">>>[processSpend] finish update db")
		// Whether to clean up mempool spend records
		if blockHeight > CleanedHeight && i.mempoolManager != nil {
			//log.Printf("Deleting %d mempool spend records for block height %d,CleanHeight:%d", len(batchPoints), blockHeight, CleanedHeight)
			err := i.mempoolManager.BatchDeleteSpend(deleteKeys)
			if err != nil {
				errMsg := syslogs.ErrLog{
					Height:       block.Height,
					BlockHash:    block.BlockHash,
					ErrType:      "BatchDeleteSpend",
					Timestamp:    time.Now().Unix(),
					ErrorMessage: err.Error(),
				}
				go syslogs.InsertErrLog(errMsg)
				log.Printf("Failed to delete mempool spend records: %v", err)
			}
		}
		// Clean up current batch
		for k := range addressResult {
			delete(addressResult, k)
		}
		addressResult = nil

		// This batch is complete, release memory
		batchPoints = nil

	}

	// Clean up all transaction points
	allInputs = nil
	//log.Println("==>finish processing batch")
	return inCnt, nil
}

func (i *UTXOIndexer) GetLastIndexedHeight() (int, error) {
	// Get last indexed height from meta store
	heightBytes, err := i.metaStore.Get([]byte("last_indexed_height"))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Println("No previous height found, starting from genesis")
			return 0, nil
		}
		log.Printf("Error reading last height: %v", err)
		return 0, err
	}

	height, err := strconv.Atoi(string(heightBytes))
	if err != nil {
		log.Printf("Invalid height format: %s, error: %v", heightBytes, err)
		return 0, fmt.Errorf("invalid height format: %w", err)
	}

	//log.Printf("Successfully read last indexed height: %d", height)
	return height, nil
}

type Block struct {
	Height         int                  `json:"height"`
	BlockHash      string               `json:"block_hash"`
	Transactions   []*Transaction       `json:"transactions"`
	AddressIncome  map[string][]*Income `json:"address_income"`
	UtxoData       map[string][]string  `json:"utxo_data"`
	IncomeData     map[string][]string  `json:"income_data"`
	SpendData      map[string][]string  `json:"spend_data"`
	UtxoPartIndex  int                  `json:"utxo_part_index"`
	SpendPartIndex int                  `json:"spend_part_index"`
	IsPartialBlock bool                 `json:"-"` // Mark whether it is a partial block for batch processing
}

func (b *Block) Validate() error {
	if b.Height < 0 {
		return fmt.Errorf("invalid block height: %d", b.Height)
	}
	return nil
}

type Transaction struct {
	ID      string
	Inputs  []*Input
	Outputs []*Output
}
type Income struct {
	TxID  string
	Index string
	Value string
}
type Input struct {
	TxPoint string
}

type Output struct {
	Address string
	Amount  string
}

// SetMempoolManager sets the mempool manager
func (i *UTXOIndexer) SetMempoolManager(mgr MempoolManager) {
	i.mempoolManager = mgr
}

// SetBlockchainClient sets the blockchain client for warmup
func (i *UTXOIndexer) SetBlockchainClient(client BlockchainClient) {
	i.blockchainClient = client
}

// GetUtxoStore returns the UTXO storage object
func (i *UTXOIndexer) GetUtxoStore() *storage.PebbleStore {
	return i.utxoStore
}
