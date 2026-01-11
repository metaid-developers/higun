package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strconv"

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

	// MVC 使用 GetNewHash2() 工具函数获取交易ID
	txid, _ := GetNewHash2(tx)
	return &indexer.Transaction{
		ID:      txid,
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
