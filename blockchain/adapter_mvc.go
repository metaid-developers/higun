package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	bsvwire "github.com/bitcoinsv/bsvd/wire"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
)

// MVCAdapter MVC 链适配器
type MVCAdapter struct {
	rpcClient *rpcclient.Client
	cfg       *config.Config
	params    *chaincfg.Params
}

// NewMVCAdapter 创建 MVC 适配器
func NewMVCAdapter(cfg *config.Config) (*MVCAdapter, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", cfg.RPC.Host, cfg.RPC.Port),
		User:         cfg.RPC.User,
		Pass:         cfg.RPC.Password,
		HTTPPostMode: true,
		DisableTLS:   true,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create MVC RPC client: %w", err)
	}

	params, err := cfg.GetChainParams()
	if err != nil {
		return nil, err
	}

	// 设置全局 RPC 客户端(兼容现有代码)
	RpcClient = client

	return &MVCAdapter{
		rpcClient: client,
		cfg:       cfg,
		params:    params,
	}, nil
}

// Connect 连接到 MVC 节点
func (a *MVCAdapter) Connect() error {
	_, err := a.rpcClient.GetBlockCount()
	if err != nil {
		return fmt.Errorf("failed to connect to MVC node: %w", err)
	}
	log.Printf("✓ Connected to MVC node successfully")
	return nil
}

// Shutdown 关闭连接
func (a *MVCAdapter) Shutdown() {
	a.rpcClient.Shutdown()
	log.Println("MVC adapter shutdown")
}

// GetChainName 获取链名称
func (a *MVCAdapter) GetChainName() string {
	return "mvc"
}

// GetChainParams 获取链参数
func (a *MVCAdapter) GetChainParams() *chaincfg.Params {
	return a.params
}

// GetBlockCount 获取最新区块高度
func (a *MVCAdapter) GetBlockCount() (int, error) {
	count, err := a.rpcClient.GetBlockCount()
	if err != nil {
		return 0, fmt.Errorf("failed to get block count: %w", err)
	}
	return int(count), nil
}

// GetBlockHash 获取指定高度的区块哈希
func (a *MVCAdapter) GetBlockHash(height int64) (string, error) {
	hash, err := a.rpcClient.GetBlockHash(height)
	if err != nil {
		return "", fmt.Errorf("failed to get block hash at height %d: %w", height, err)
	}
	return hash.String(), nil
}

// GetBlock 获取区块数据(核心方法 - MVC 版本)
func (a *MVCAdapter) GetBlock(height int64) (*indexer.Block, error) {
	// 1. 获取区块哈希
	hashStr, err := a.GetBlockHash(height)
	if err != nil {
		return nil, err
	}

	// 2. 先用 verbosity=1 获取区块元数据（含 size 和 txid 列表），代价远小于拉取原始区块
	verbose1, err := a.getBlockVerbose1(hashStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get block verbose1 at height %d: %w", height, err)
	}

	// 3. 根据区块大小决定处理路径
	threshold := config.GlobalConfig.LargeBlockThresholdBytes
	if threshold <= 0 {
		threshold = 200 * 1024 * 1024 // 默认 200MB
	}

	if verbose1.Size >= threshold {
		// === 大块路径：逐笔 TX 拉取，避免将整块大 hex 字符串装入内存 ===
		log.Printf("[LargeBlock-MVC] Height %d: size=%d bytes (%.1f MB) >= threshold=%d bytes, switching to tx-by-tx mode (%d txs)",
			height, verbose1.Size, float64(verbose1.Size)/1024/1024, threshold, len(verbose1.Tx))
		result, err := a.getBlockByTxHashes(verbose1, int(height))
		if err != nil {
			return nil, fmt.Errorf("large block tx-by-tx fetch failed at height %d: %w", height, err)
		}
		return result, nil
	}

	// === 普通路径：拉取原始 hex 并整体反序列化（与改造前完全一致）===

	// 2. 使用 RawRequest 获取原始区块数据
	resp, err := a.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hashStr)),
		json.RawMessage("0"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get raw block: %w", err)
	}

	var blockHex string
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		return nil, err
	}

	// 3. 解析为 MVC 区块格式 (bsvwire.MsgBlock)
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, err
	}

	msgBlock := &bsvwire.MsgBlock{}
	if err := msgBlock.Deserialize(bytes.NewReader(blockBytes)); err != nil {
		return nil, err
	}

	// 4. 转换为统一的索引器格式
	return a.convertToIndexerBlock(msgBlock, int(height), hashStr, msgBlock.Header.Timestamp.Unix())
}

// mvcBlockVerbose1Result 是 getblock <hash> 1 的简化响应结构
type mvcBlockVerbose1Result struct {
	Hash   string   `json:"hash"`
	Height int64    `json:"height"`
	Size   int64    `json:"size"` // 序列化后的字节数
	Time   int64    `json:"time"`
	Tx     []string `json:"tx"` // 仅 txid 列表
}

// getBlockVerbose1 以 verbosity=1 拉取区块元数据（轻量，仅含 txid 列表和 size）
func (a *MVCAdapter) getBlockVerbose1(hashStr string) (*mvcBlockVerbose1Result, error) {
	resp, err := a.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hashStr)),
		json.RawMessage("1"),
	})
	if err != nil {
		return nil, err
	}
	var result mvcBlockVerbose1Result
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// getBlockByTxHashes 大块专用路径：并发逐笔拉取 TX，避免 OOM
// 要求节点已开启 txindex=1
func (a *MVCAdapter) getBlockByTxHashes(verbose1 *mvcBlockVerbose1Result, height int) (*indexer.Block, error) {
	txids := verbose1.Tx
	txCount := len(txids)

	workers := config.GlobalConfig.LargeBlockFetchWorkers
	if workers <= 0 {
		workers = 20
	}
	if workers > txCount {
		workers = txCount
	}

	allBlock := &indexer.Block{
		Height:     height,
		BlockHash:  verbose1.Hash,
		BlockTime:  verbose1.Time,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}

	results := make([]*indexer.Transaction, txCount)
	errCh := make(chan error, 1)
	sem := make(chan struct{}, workers)

	var wg sync.WaitGroup
	for i, txid := range txids {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			tx, err := a.GetTransaction(id)
			if err != nil {
				select {
				case errCh <- fmt.Errorf("getrawtransaction %s: %w", id, err):
				default:
				}
				return
			}
			results[idx] = tx
		}(i, txid)
	}
	wg.Wait()

	select {
	case err := <-errCh:
		return nil, err
	default:
	}

	allBlock.Transactions = results
	return allBlock, nil
}

// BackfillTxIDAliases stores MVC public txid -> node txid aliases in bounded batches.
func (a *MVCAdapter) BackfillTxIDAliases(height int64, startOffset int, store func([]*indexer.Transaction) error, markOffset func(int) error, stopCh <-chan struct{}) error {
	if store == nil {
		return fmt.Errorf("txid alias store callback is nil")
	}
	if markOffset == nil {
		return fmt.Errorf("txid alias offset callback is nil")
	}

	hashStr, err := a.GetBlockHash(height)
	if err != nil {
		return err
	}
	verbose1, err := a.getBlockVerbose1(hashStr)
	if err != nil {
		return fmt.Errorf("failed to get block verbose1 at height %d: %w", height, err)
	}

	txCount := len(verbose1.Tx)
	if startOffset < 0 {
		return fmt.Errorf("invalid txid alias start offset: %d", startOffset)
	}
	if startOffset > txCount {
		return fmt.Errorf("txid alias start offset %d exceeds tx count %d", startOffset, txCount)
	}
	batchSize, workers, _, _ := mvcTxIDAliasBackfillLimits()
	if batchSize > txCount && txCount > 0 {
		batchSize = txCount
	}
	if batchSize <= 0 {
		batchSize = 1
	}
	if workers > batchSize {
		workers = batchSize
	}
	if workers <= 0 {
		workers = 1
	}

	log.Printf("[TxIDAliasBackfill] MVC height=%d streaming aliases offset=%d txs=%d batch_size=%d workers=%d", height, startOffset, txCount, batchSize, workers)
	for start := startOffset; start < txCount; start += batchSize {
		if txIDAliasBackfillStopRequested(stopCh) {
			return errTxIDAliasBackfillStopped
		}
		end := start + batchSize
		if end > txCount {
			end = txCount
		}

		transactions, err := a.fetchMVCTxIDAliasBatch(verbose1.Tx[start:end], workers, stopCh)
		if err != nil {
			return err
		}
		if err := store(transactions); err != nil {
			return err
		}
		if err := markOffset(end); err != nil {
			return err
		}

		transactions = nil
		if txCount > 400000 {
			runtime.GC()
		}
		if start == 0 || end == txCount || end%100000 == 0 {
			log.Printf("[TxIDAliasBackfill] MVC height=%d streamed aliases %d/%d", height, end, txCount)
		}
	}
	return nil
}

func (a *MVCAdapter) fetchMVCTxIDAliasBatch(txids []string, workers int, stopCh <-chan struct{}) ([]*indexer.Transaction, error) {
	if len(txids) == 0 {
		return nil, nil
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > len(txids) {
		workers = len(txids)
	}

	type result struct {
		idx int
		tx  *indexer.Transaction
		err error
	}
	results := make([]*indexer.Transaction, len(txids))
	jobs := make(chan int)
	resultCh := make(chan result, len(txids))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if txIDAliasBackfillStopRequested(stopCh) {
					resultCh <- result{idx: idx, err: errTxIDAliasBackfillStopped}
					continue
				}
				tx, err := a.getMVCTxIDAliasTransactionWithRetry(txids[idx], stopCh)
				resultCh <- result{idx: idx, tx: tx, err: err}
			}
		}()
	}

	for i := range txids {
		if txIDAliasBackfillStopRequested(stopCh) {
			close(jobs)
			wg.Wait()
			close(resultCh)
			for range resultCh {
			}
			return nil, errTxIDAliasBackfillStopped
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(resultCh)

	var firstErr error
	for item := range resultCh {
		if item.err != nil && firstErr == nil {
			firstErr = item.err
		}
		results[item.idx] = item.tx
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func (a *MVCAdapter) getMVCTxIDAliasTransactionWithRetry(txid string, stopCh <-chan struct{}) (*indexer.Transaction, error) {
	_, _, retryAttempts, retryDelay := mvcTxIDAliasBackfillLimits()
	var tx *indexer.Transaction
	err := retryTxIDAliasBackfillOperation(retryAttempts, retryDelay, stopCh, func() error {
		var err error
		tx, err = a.getMVCTxIDAliasTransaction(txid)
		return err
	})
	return tx, err
}

func mvcTxIDAliasBackfillLimits() (batchSize int, workers int, retryAttempts int, retryDelay time.Duration) {
	batchSize = 1000
	workers = 4
	retryAttempts = 3
	retryDelay = time.Second
	if config.GlobalConfig == nil {
		return batchSize, workers, retryAttempts, retryDelay
	}
	if config.GlobalConfig.MVCTxIDAliasBackfillBatchSize > 0 {
		batchSize = config.GlobalConfig.MVCTxIDAliasBackfillBatchSize
	}
	if config.GlobalConfig.MVCTxIDAliasBackfillWorkers > 0 {
		workers = config.GlobalConfig.MVCTxIDAliasBackfillWorkers
	}
	if config.GlobalConfig.MVCTxIDAliasBackfillRetryAttempts > 0 {
		retryAttempts = config.GlobalConfig.MVCTxIDAliasBackfillRetryAttempts
	}
	if config.GlobalConfig.MVCTxIDAliasBackfillRetryDelayMS >= 0 {
		retryDelay = time.Duration(config.GlobalConfig.MVCTxIDAliasBackfillRetryDelayMS) * time.Millisecond
	}
	return batchSize, workers, retryAttempts, retryDelay
}

func (a *MVCAdapter) getMVCTxIDAliasTransaction(txid string) (*indexer.Transaction, error) {
	nodeTxID := strings.ToLower(strings.TrimSpace(txid))
	txHash, err := chainhash.NewHashFromStr(nodeTxID)
	if err != nil {
		return nil, err
	}

	resp, err := a.rpcClient.RawRequest("getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", txHash.String())),
	})
	if err != nil {
		return nil, err
	}

	var txHex string
	if err := json.Unmarshal(resp, &txHex); err != nil {
		return nil, err
	}
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	msgTx := &bsvwire.MsgTx{}
	if err := msgTx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return nil, err
	}

	nodeTxID = strings.ToLower(strings.TrimSpace(msgTx.TxHash().String()))
	publicTxID, _ := GetNewHash2(msgTx)
	publicTxID = strings.ToLower(strings.TrimSpace(publicTxID))
	if publicTxID == "" {
		publicTxID = nodeTxID
	}

	nodeAlias := ""
	if publicTxID != nodeTxID {
		nodeAlias = nodeTxID
	}
	return &indexer.Transaction{ID: publicTxID, NodeID: nodeAlias}, nil
}

// GetTransaction 获取单笔交易
func (a *MVCAdapter) GetTransaction(txid string) (*indexer.Transaction, error) {
	// MVC 也使用 chainhash
	txHash, err := chainhash.NewHashFromStr(txid)
	if err != nil {
		return nil, err
	}

	// 获取原始交易
	resp, err := a.rpcClient.RawRequest("getrawtransaction", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", txHash.String())),
	})
	if err != nil {
		return nil, err
	}

	var txHex string
	if err := json.Unmarshal(resp, &txHex); err != nil {
		return nil, err
	}

	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	msgTx := &bsvwire.MsgTx{}
	if err := msgTx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return nil, err
	}

	return a.convertMVCTxToIndexerTx(msgTx), nil
}

// GetRawMempool 获取内存池交易列表
func (a *MVCAdapter) GetRawMempool() ([]string, error) {
	hashes, err := a.rpcClient.GetRawMempool()
	if err != nil {
		return nil, err
	}

	txids := make([]string, len(hashes))
	for i, hash := range hashes {
		txids[i] = hash.String()
	}
	return txids, nil
}

// FindReorgHeight 查找重组高度
func (a *MVCAdapter) FindReorgHeight() (int, int) {
	// TODO: 实现 MVC 特定的重组检测逻辑
	return 0, 0
}

// ========== 私有方法:MVC 特定的转换逻辑 ==========

// convertToIndexerBlock 将 MVC 区块转换为统一格式(批处理)
func (a *MVCAdapter) convertToIndexerBlock(msgBlock *bsvwire.MsgBlock, height int, blockHash string, blockTime int64) (*indexer.Block, error) {
	txCount := len(msgBlock.Transactions)
	maxTxPerBatch := config.GlobalConfig.MaxTxPerBatch

	// 统计预期的输入输出数量
	expectedInTxCount := 0
	expectedOutTxCount := 0
	for _, tx := range msgBlock.Transactions {
		expectedInTxCount += len(tx.TxIn)
		expectedOutTxCount += len(tx.TxOut)
	}

	// 创建完整区块结构
	allBlock := &indexer.Block{
		Height:     height,
		BlockHash:  blockHash,
		BlockTime:  blockTime,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}

	// 批处理交易
	startIdx := 0
	allTransactions := make([]*indexer.Transaction, 0, txCount)

	for startIdx < txCount {
		endIdx := startIdx + maxTxPerBatch
		if endIdx > txCount {
			endIdx = txCount
		}

		// 转换当前批次的交易
		for i := startIdx; i < endIdx; i++ {
			tx := msgBlock.Transactions[i]
			indexerTx := a.convertMVCTxToIndexerTx(tx)
			allTransactions = append(allTransactions, indexerTx)
		}

		startIdx = endIdx
		if txCount > 400000 {
			runtime.GC() // 大区块时强制GC
		}
	}

	allBlock.Transactions = allTransactions

	return allBlock, nil
}

// convertMVCTxToIndexerTx 将 MVC 交易转换为统一格式
func (a *MVCAdapter) convertMVCTxToIndexerTx(tx *bsvwire.MsgTx) *indexer.Transaction {
	// 转换输入
	inputs := make([]*indexer.Input, len(tx.TxIn))
	for i, in := range tx.TxIn {
		prevTxid := in.PreviousOutPoint.Hash.String()
		if prevTxid == "0000000000000000000000000000000000000000000000000000000000000000" {
			prevTxid = "0000000000000000000000000000000000000000000000000000000000000000"
		}
		inputs[i] = &indexer.Input{
			TxPoint: common.ConcatBytesOptimized([]string{prevTxid, strconv.Itoa(int(in.PreviousOutPoint.Index))}, ":"),
		}
	}

	// 转换输出
	outputs := make([]*indexer.Output, len(tx.TxOut))
	for i, out := range tx.TxOut {
		address := a.extractAddress(out.PkScript)
		outputs[i] = &indexer.Output{
			Address: address,
			Amount:  strconv.FormatInt(out.Value, 10),
		}
	}

	nodeTxID := tx.TxHash().String()
	// MVC 使用 GetNewHash2() 工具函数获取交易ID
	txid, _ := GetNewHash2(tx)
	if txid == "" {
		txid = nodeTxID
	}
	nodeAlias := ""
	if txid != nodeTxID {
		nodeAlias = nodeTxID
	}
	return &indexer.Transaction{
		ID:      txid,
		NodeID:  nodeAlias,
		Inputs:  inputs,
		Outputs: outputs,
	}
}

// extractAddress 从脚本中提取 MVC 地址
func (a *MVCAdapter) extractAddress(pkScript []byte) string {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, a.params)
	if err == nil && len(addrs) > 0 {
		return addrs[0].EncodeAddress() // MVC 使用 EncodeAddress() 方法
	}
	return "errAddress"
}
