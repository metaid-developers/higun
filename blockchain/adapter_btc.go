package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
)

// BTCAdapter BTC 链适配器
type BTCAdapter struct {
	rpcClient *rpcclient.Client
	cfg       *config.Config
	params    *chaincfg.Params
}

// NewBTCAdapter 创建 BTC 适配器
func NewBTCAdapter(cfg *config.Config) (*BTCAdapter, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", cfg.RPC.Host, cfg.RPC.Port),
		User:         cfg.RPC.User,
		Pass:         cfg.RPC.Password,
		HTTPPostMode: true,
		DisableTLS:   true,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create BTC RPC client: %w", err)
	}

	params, err := cfg.GetChainParams()
	if err != nil {
		return nil, err
	}

	// 设置全局 RPC 客户端(兼容现有代码)
	RpcClient = client

	return &BTCAdapter{
		rpcClient: client,
		cfg:       cfg,
		params:    params,
	}, nil
}

// Connect 连接到 BTC 节点
func (a *BTCAdapter) Connect() error {
	_, err := a.rpcClient.GetBlockCount()
	if err != nil {
		return fmt.Errorf("failed to connect to BTC node: %w", err)
	}
	log.Printf("✓ Connected to BTC node successfully")
	return nil
}

// Shutdown 关闭连接
func (a *BTCAdapter) Shutdown() {
	a.rpcClient.Shutdown()
	log.Println("BTC adapter shutdown")
}

// GetChainName 获取链名称
func (a *BTCAdapter) GetChainName() string {
	return "btc"
}

// GetChainParams 获取链参数
func (a *BTCAdapter) GetChainParams() *chaincfg.Params {
	return a.params
}

// GetBlockCount 获取最新区块高度
func (a *BTCAdapter) GetBlockCount() (int, error) {
	count, err := a.rpcClient.GetBlockCount()
	if err != nil {
		return 0, fmt.Errorf("failed to get block count: %w", err)
	}
	return int(count), nil
}

// GetBlockHash 获取指定高度的区块哈希
func (a *BTCAdapter) GetBlockHash(height int64) (string, error) {
	hash, err := a.rpcClient.GetBlockHash(height)
	if err != nil {
		return "", fmt.Errorf("failed to get block hash at height %d: %w", height, err)
	}
	return hash.String(), nil
}

// GetBlock 获取区块数据(核心方法)
func (a *BTCAdapter) GetBlock(height int64) (*indexer.Block, error) {
	// 1. 获取区块哈希
	t1 := time.Now()
	hashStr, err := a.GetBlockHash(height)
	if err != nil {
		return nil, err
	}
	getHashTime := time.Since(t1)

	hash, _ := chainhash.NewHashFromStr(hashStr)

	// 2. 先用 verbosity=1 获取区块元数据（含 size 和 txid 列表），代价远小于拉取原始区块
	t2 := time.Now()
	verbose1, err := a.getBlockVerbose1(hash.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get block verbose1 at height %d: %w", height, err)
	}
	getVerbose1Time := time.Since(t2)

	// 3. 根据区块大小决定处理路径
	threshold := config.GlobalConfig.LargeBlockThresholdBytes
	if threshold <= 0 {
		threshold = 200 * 1024 * 1024 // 默认 200MB
	}

	if verbose1.Size >= threshold {
		// === 大块路径：逐笔 TX 拉取，避免将整块大 hex 字符串装入内存 ===
		log.Printf("[LargeBlock] Height %d: size=%d bytes (%.1f MB) >= threshold=%d bytes, switching to tx-by-tx mode (%d txs)",
			height, verbose1.Size, float64(verbose1.Size)/1024/1024, threshold, len(verbose1.Tx))
		result, err := a.getBlockByTxHashes(verbose1, int(height))
		if err != nil {
			return nil, fmt.Errorf("large block tx-by-tx fetch failed at height %d: %w", height, err)
		}
		log.Printf("[LargeBlock] Height %d: completed tx-by-tx fetch in %.2fs", height, time.Since(t2).Seconds())
		return result, nil
	}

	// === 普通路径：拉取原始 hex 并整体反序列化（与改造前完全一致）===
	t3 := time.Now()
	resp, err := a.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hash.String())),
		json.RawMessage("0"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get raw block: %w", err)
	}
	getRawBlockTime := time.Since(t3)

	var blockHex string
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		return nil, err
	}

	// 4. 解析区块
	t4 := time.Now()
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, err
	}

	msgBlock := &wire.MsgBlock{}
	if err := msgBlock.Deserialize(bytes.NewReader(blockBytes)); err != nil {
		return nil, err
	}
	deserializeTime := time.Since(t4)

	// 5. 转换为统一的索引器格式
	t5 := time.Now()
	result, err := a.convertToIndexerBlock(msgBlock, int(height), hashStr, msgBlock.Header.Timestamp.Unix())
	convertTime := time.Since(t5)

	// 只在RPC总耗时超过0.2秒时打印警告
	totalRpcTime := getHashTime + getVerbose1Time + getRawBlockTime + deserializeTime + convertTime
	if totalRpcTime.Seconds() > 0.2 {
		log.Printf("[Perf-RPC-Slow] Height %d: GetHash=%.3fs, GetVerbose1=%.3fs, GetRawBlock=%.3fs, Deserialize=%.3fs, Convert=%.3fs",
			height, getHashTime.Seconds(), getVerbose1Time.Seconds(), getRawBlockTime.Seconds(), deserializeTime.Seconds(), convertTime.Seconds())
	}

	return result, err
}

// btcBlockVerbose1Result 是 getblock <hash> 1 的简化响应结构
type btcBlockVerbose1Result struct {
	Hash   string   `json:"hash"`
	Height int64    `json:"height"`
	Size   int64    `json:"size"` // 序列化后的字节数（含 witness）
	Time   int64    `json:"time"`
	Tx     []string `json:"tx"` // 仅 txid 列表
}

// getBlockVerbose1 以 verbosity=1 拉取区块元数据（轻量，仅含 txid 列表和 size）
func (a *BTCAdapter) getBlockVerbose1(hashStr string) (*btcBlockVerbose1Result, error) {
	resp, err := a.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hashStr)),
		json.RawMessage("1"),
	})
	if err != nil {
		return nil, err
	}
	var result btcBlockVerbose1Result
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// getBlockByTxHashes 大块专用路径：并发逐笔拉取 TX，避免 OOM
// 要求节点已开启 txindex=1
func (a *BTCAdapter) getBlockByTxHashes(verbose1 *btcBlockVerbose1Result, height int) (*indexer.Block, error) {
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

// GetTransaction 获取单笔交易
func (a *BTCAdapter) GetTransaction(txid string) (*indexer.Transaction, error) {
	txHash, err := chainhash.NewHashFromStr(txid)
	if err != nil {
		return nil, err
	}

	tx, err := a.rpcClient.GetRawTransaction(txHash)
	if err != nil {
		return nil, err
	}

	return a.convertBTCTxToIndexerTx(tx.MsgTx()), nil
}

// GetRawMempool 获取内存池交易列表
func (a *BTCAdapter) GetRawMempool() ([]string, error) {
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
func (a *BTCAdapter) FindReorgHeight() (int, int) {
	// TODO: 实现 BTC 特定的重组检测逻辑
	// 可以复用 client.go 中的现有逻辑
	return 0, 0
}

// ========== 私有方法:BTC 特定的转换逻辑 ==========

// convertToIndexerBlock 将 BTC 区块转换为统一格式(批处理)
func (a *BTCAdapter) convertToIndexerBlock(msgBlock *wire.MsgBlock, height int, blockHash string, blockTime int64) (*indexer.Block, error) {
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
			indexerTx := a.convertBTCTxToIndexerTx(tx)
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

// convertBTCTxToIndexerTx 将 BTC 交易转换为统一格式
func (a *BTCAdapter) convertBTCTxToIndexerTx(tx *wire.MsgTx) *indexer.Transaction {
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

	return &indexer.Transaction{
		ID:      tx.TxHash().String(),
		Inputs:  inputs,
		Outputs: outputs,
	}
}

// extractAddress 从脚本中提取 BTC 地址
func (a *BTCAdapter) extractAddress(pkScript []byte) string {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, a.params)
	if err == nil && len(addrs) > 0 {
		return addrs[0].String() // BTC 使用 String() 方法
	}
	return "errAddress"
}

// 计算预期的输入输出数量
func (a *BTCAdapter) countTxInOut(msgBlock *wire.MsgBlock) (int, int) {
	inCount := 0
	outCount := 0
	for _, tx := range msgBlock.Transactions {
		inCount += len(tx.TxIn)
		outCount += len(tx.TxOut)
	}
	return inCount, outCount
}
