package blockchain

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"runtime"
	"strconv"
	"sync"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
)

// 狗狗币网络参数
var (
	// DogeMainNetParams 狗狗币主网参数
	DogeMainNetParams = chaincfg.Params{
		Name:             "dogecoin-mainnet",
		Net:              wire.MainNet,
		PubKeyHashAddrID: 0x1e, // 'D' addresses
		ScriptHashAddrID: 0x16, // '9' or 'A' addresses
		PrivateKeyID:     0x9e, // WIF private keys
		Bech32HRPSegwit:  "dc", // Dogecoin bech32
		HDPrivateKeyID:   [4]byte{0x02, 0xfa, 0xca, 0xfd},
		HDPublicKeyID:    [4]byte{0x02, 0xfa, 0xc3, 0x98},
	}

	// DogeTestNet3Params 狗狗币测试网参数
	DogeTestNet3Params = chaincfg.Params{
		Name:             "dogecoin-testnet",
		Net:              wire.TestNet3,
		PubKeyHashAddrID: 0x71, // 'n' or 'm' addresses
		ScriptHashAddrID: 0xc4, // '2' addresses
		PrivateKeyID:     0xf1, // WIF private keys
		Bech32HRPSegwit:  "td", // Dogecoin testnet bech32
		HDPrivateKeyID:   [4]byte{0x04, 0x35, 0x83, 0x94},
		HDPublicKeyID:    [4]byte{0x04, 0x35, 0x87, 0xcf},
	}

	// DogeRegTestParams 狗狗币回归测试网参数
	DogeRegTestParams = chaincfg.Params{
		Name:             "dogecoin-regtest",
		Net:              wire.TestNet,
		PubKeyHashAddrID: 0x6f, // 'm' or 'n' addresses
		ScriptHashAddrID: 0xc4, // '2' addresses
		PrivateKeyID:     0xef, // WIF private keys
		Bech32HRPSegwit:  "dcrt",
		HDPrivateKeyID:   [4]byte{0x04, 0x35, 0x83, 0x94},
		HDPublicKeyID:    [4]byte{0x04, 0x35, 0x87, 0xcf},
	}
)

// DOGEAdapter DOGE 链适配器
type DOGEAdapter struct {
	rpcClient *rpcclient.Client
	cfg       *config.Config
	params    *chaincfg.Params
}

// NewDOGEAdapter 创建 DOGE 适配器
func NewDOGEAdapter(cfg *config.Config) (*DOGEAdapter, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", cfg.RPC.Host, cfg.RPC.Port),
		User:         cfg.RPC.User,
		Pass:         cfg.RPC.Password,
		HTTPPostMode: true,
		DisableTLS:   true,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DOGE RPC client: %w", err)
	}

	// 根据网络类型选择狗狗币参数
	var params *chaincfg.Params
	switch cfg.Network {
	case "mainnet":
		params = &DogeMainNetParams
	case "testnet":
		params = &DogeTestNet3Params
	case "regtest":
		params = &DogeRegTestParams
	default:
		log.Printf("⚠️ Unknown network '%s', using DOGE mainnet params", cfg.Network)
		params = &DogeMainNetParams
	}

	log.Printf("DOGE adapter using network: %s, PubKeyHashAddrID: 0x%02x", cfg.Network, params.PubKeyHashAddrID)

	// 设置全局 RPC 客户端(兼容现有代码)
	RpcClient = client

	return &DOGEAdapter{
		rpcClient: client,
		cfg:       cfg,
		params:    params,
	}, nil
}

// Connect 连接到 DOGE 节点
func (a *DOGEAdapter) Connect() error {
	_, err := a.rpcClient.GetBlockCount()
	if err != nil {
		return fmt.Errorf("failed to connect to DOGE node: %w", err)
	}
	log.Printf("✓ Connected to DOGE node successfully")
	return nil
}

// Shutdown 关闭连接
func (a *DOGEAdapter) Shutdown() {
	a.rpcClient.Shutdown()
	log.Println("DOGE adapter shutdown")
}

// GetChainName 获取链名称
func (a *DOGEAdapter) GetChainName() string {
	return "doge"
}

// GetChainParams 获取链参数
func (a *DOGEAdapter) GetChainParams() *chaincfg.Params {
	return a.params
}

// GetBlockCount 获取最新区块高度
func (a *DOGEAdapter) GetBlockCount() (int, error) {
	count, err := a.rpcClient.GetBlockCount()
	if err != nil {
		return 0, fmt.Errorf("failed to get block count: %w", err)
	}
	return int(count), nil
}

// GetBlockHash 获取指定高度的区块哈希
func (a *DOGEAdapter) GetBlockHash(height int64) (string, error) {
	hash, err := a.rpcClient.GetBlockHash(height)
	if err != nil {
		return "", fmt.Errorf("failed to get block hash at height %d: %w", height, err)
	}
	return hash.String(), nil
}

// GetBlockByHash 通过哈希获取区块数据（用于预取优化）
func (a *DOGEAdapter) GetBlockByHash(hashStr string) (*indexer.Block, int64, error) {
	hash, _ := chainhash.NewHashFromStr(hashStr)

	// 获取原始区块数据
	resp, err := a.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hash.String())),
		json.RawMessage("0"), // 0 = 返回原始十六进制
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get raw block: %w", err)
	}

	var blockHex string
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		return nil, 0, err
	}

	// 解析区块
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, 0, err
	}

	msgBlock := &wire.MsgBlock{}
	reader := bytes.NewReader(blockBytes)
	if err := msgBlock.Header.Deserialize(reader); err != nil {
		return nil, 0, fmt.Errorf("failed to deserialize block header: %w", err)
	}

	// 检查是否是 AuxPoW
	isAuxPow := (msgBlock.Header.Version & (1 << 8)) != 0
	if isAuxPow {
		if err := readDogeAuxPow(reader); err != nil {
			return nil, 0, fmt.Errorf("failed to read AuxPoW data: %w", err)
		}
	}

	// 读取交易数量
	txCount, err := wire.ReadVarInt(reader, wire.ProtocolVersion)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read tx count: %w", err)
	}

	// 读取所有交易
	msgBlock.Transactions = make([]*wire.MsgTx, txCount)
	for i := uint64(0); i < txCount; i++ {
		tx := &wire.MsgTx{}
		if err := tx.DeserializeNoWitness(reader); err != nil {
			return nil, 0, fmt.Errorf("failed to deserialize tx %d: %w", i, err)
		}
		msgBlock.Transactions[i] = tx
	}

	blockTime := msgBlock.Header.Timestamp.Unix()
	block, err := a.convertToIndexerBlock(msgBlock, 0, hashStr, blockTime)
	return block, blockTime, err
}

// GetBlock 获取区块数据(核心方法)
func (a *DOGEAdapter) GetBlock(height int64) (*indexer.Block, error) {
	// 1. 获取区块哈希
	hashStr, err := a.GetBlockHash(height)
	if err != nil {
		return nil, err
	}

	hash, _ := chainhash.NewHashFromStr(hashStr)

	// 2. 获取原始区块数据
	resp, err := a.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hash.String())),
		json.RawMessage("0"), // 0 = 返回原始十六进制
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get raw block: %w", err)
	}

	var blockHex string
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		return nil, err
	}

	// 3. 解析区块
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, err
	}

	msgBlock := &wire.MsgBlock{}
	// Dogecoin 不支持 SegWit,使用 DeserializeNoWitness 方法
	// 或者使用自定义读取器来避免 witness 检测
	reader := bytes.NewReader(blockBytes)
	if err := msgBlock.Header.Deserialize(reader); err != nil {
		return nil, fmt.Errorf("failed to deserialize block header: %w", err)
	}

	// 检查是否是 AuxPoW
	// Dogecoin AuxPoW 版本位通常是 (1 << 8) = 256
	isAuxPow := (msgBlock.Header.Version & (1 << 8)) != 0
	if isAuxPow {
		if err := readDogeAuxPow(reader); err != nil {
			return nil, fmt.Errorf("failed to read AuxPoW data: %w", err)
		}
	}

	// 读取交易数量
	txCount, err := wire.ReadVarInt(reader, wire.ProtocolVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to read tx count: %w", err)
	}

	// 读取所有交易(不使用 witness)
	msgBlock.Transactions = make([]*wire.MsgTx, txCount)
	for i := uint64(0); i < txCount; i++ {
		tx := &wire.MsgTx{}
		// 使用 BtcDecode 而不是 Deserialize,并指定不支持 witness
		if err := tx.DeserializeNoWitness(reader); err != nil {
			return nil, fmt.Errorf("failed to deserialize tx %d: %w", i, err)
		}
		msgBlock.Transactions[i] = tx
	}

	// 4. 转换为统一的索引器格式
	return a.convertToIndexerBlock(msgBlock, int(height), hashStr, msgBlock.Header.Timestamp.Unix())
}

// GetTransaction 获取单笔交易
func (a *DOGEAdapter) GetTransaction(txid string) (*indexer.Transaction, error) {
	txHash, err := chainhash.NewHashFromStr(txid)
	if err != nil {
		return nil, err
	}

	tx, err := a.rpcClient.GetRawTransaction(txHash)
	if err != nil {
		return nil, err
	}

	return a.convertDOGETxToIndexerTx(tx.MsgTx()), nil
}

// GetRawMempool 获取内存池交易列表
func (a *DOGEAdapter) GetRawMempool() ([]string, error) {
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
func (a *DOGEAdapter) FindReorgHeight() (int, int) {
	// TODO: 实现 DOGE 特定的重组检测逻辑
	// 可以复用 client.go 中的现有逻辑
	return 0, 0
}

// ========== 私有方法:DOGE 特定的转换逻辑 ==========

// convertToIndexerBlock 将 DOGE 区块转换为统一格式(并发批处理优化)
func (a *DOGEAdapter) convertToIndexerBlock(msgBlock *wire.MsgBlock, height int, blockHash string, blockTime int64) (*indexer.Block, error) {
	txCount := len(msgBlock.Transactions)

	// 创建完整区块结构
	allBlock := &indexer.Block{
		Height:     height,
		BlockHash:  blockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}

	// 预分配交易切片
	allTransactions := make([]*indexer.Transaction, txCount)

	// 根据交易数量决定是否使用并发
	if txCount > 100 {
		// 大区块使用并发处理
		workers := config.GlobalConfig.Workers
		if workers <= 0 {
			workers = 8
		}

		// 计算每个 worker 处理的交易数
		chunkSize := (txCount + workers - 1) / workers

		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			start := w * chunkSize
			end := start + chunkSize
			if end > txCount {
				end = txCount
			}
			if start >= txCount {
				break
			}

			wg.Add(1)
			go func(startIdx, endIdx int) {
				defer wg.Done()
				for i := startIdx; i < endIdx; i++ {
					tx := msgBlock.Transactions[i]
					allTransactions[i] = a.convertDOGETxToIndexerTx(tx)
				}
			}(start, end)
		}
		wg.Wait()
	} else {
		// 小区块串行处理
		for i, tx := range msgBlock.Transactions {
			allTransactions[i] = a.convertDOGETxToIndexerTx(tx)
		}
	}

	allBlock.Transactions = allTransactions

	// 大区块时触发 GC
	if txCount > 400000 {
		runtime.GC()
	}

	return allBlock, nil
}

// convertDOGETxToIndexerTx 将 DOGE 交易转换为统一格式
func (a *DOGEAdapter) convertDOGETxToIndexerTx(tx *wire.MsgTx) *indexer.Transaction {
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

// extractAddress 从脚本中提取 DOGE 地址
// 使用狗狗币特定的地址参数
func (a *DOGEAdapter) extractAddress(pkScript []byte) string {
	// 使用狗狗币参数提取地址
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, a.params)
	if err == nil && len(addrs) > 0 {
		// DOGE 使用 EncodeAddress() 方法（与 MVC 类似）
		return addrs[0].EncodeAddress()
	}
	return "errAddress"
}

// readDogeAuxPow 读取并跳过 AuxPoW 数据
func readDogeAuxPow(r io.Reader) error {
	// 1. CTransaction tx;
	msgTx := &wire.MsgTx{}
	// Parent coinbase might be legacy or segwit. Use generic Deserialize.
	if err := msgTx.Deserialize(r); err != nil {
		return fmt.Errorf("failed to read auxpow tx: %v", err)
	}

	// 2. uint256 hashBlock;
	var hashBlock chainhash.Hash
	if _, err := io.ReadFull(r, hashBlock[:]); err != nil {
		return fmt.Errorf("failed to read hashBlock: %v", err)
	}

	// 3. std::vector<uint256> vMerkleBranch;
	count, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return fmt.Errorf("failed to read vMerkleBranch count: %v", err)
	}
	for i := uint64(0); i < count; i++ {
		var hash chainhash.Hash
		if _, err := io.ReadFull(r, hash[:]); err != nil {
			return fmt.Errorf("failed to read merkle branch item: %v", err)
		}
	}

	// 4. int nIndex;
	var nIndex int32
	if err := binary.Read(r, binary.LittleEndian, &nIndex); err != nil {
		return fmt.Errorf("failed to read nIndex: %v", err)
	}

	// 5. std::vector<uint256> vChainMerkleBranch;
	count, err = wire.ReadVarInt(r, 0)
	if err != nil {
		return fmt.Errorf("failed to read vChainMerkleBranch count: %v", err)
	}
	for i := uint64(0); i < count; i++ {
		var hash chainhash.Hash
		if _, err := io.ReadFull(r, hash[:]); err != nil {
			return fmt.Errorf("failed to read chain merkle branch item: %v", err)
		}
	}

	// 6. int nChainIndex;
	var nChainIndex int32
	if err := binary.Read(r, binary.LittleEndian, &nChainIndex); err != nil {
		return fmt.Errorf("failed to read nChainIndex: %v", err)
	}

	// 7. CBlockHeader parentBlockHeader;
	// 80 bytes
	parentHeader := make([]byte, 80)
	if _, err := io.ReadFull(r, parentHeader); err != nil {
		return fmt.Errorf("failed to read parentBlockHeader: %v", err)
	}

	return nil
}
