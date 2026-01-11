package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strconv"
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

	// 2. 获取原始区块数据
	t2 := time.Now()
	resp, err := a.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hash.String())),
		json.RawMessage("0"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get raw block: %w", err)
	}
	getRawBlockTime := time.Since(t2)

	var blockHex string
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		return nil, err
	}

	// 3. 解析区块
	t3 := time.Now()
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, err
	}

	msgBlock := &wire.MsgBlock{}
	if err := msgBlock.Deserialize(bytes.NewReader(blockBytes)); err != nil {
		return nil, err
	}
	deserializeTime := time.Since(t3)

	// 4. 转换为统一的索引器格式
	t4 := time.Now()
	result, err := a.convertToIndexerBlock(msgBlock, int(height), hashStr, msgBlock.Header.Timestamp.Unix())
	convertTime := time.Since(t4)

	// 只在RPC总耗时超过0.2秒时打印警告
	totalRpcTime := getHashTime + getRawBlockTime + deserializeTime + convertTime
	if totalRpcTime.Seconds() > 0.2 {
		log.Printf("[Perf-RPC-Slow] Height %d: GetHash=%.3fs, GetRawBlock=%.3fs, Deserialize=%.3fs, Convert=%.3fs",
			height, getHashTime.Seconds(), getRawBlockTime.Seconds(), deserializeTime.Seconds(), convertTime.Seconds())
	}

	return result, err
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
