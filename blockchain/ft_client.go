package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	indexer "github.com/metaid/utxo_indexer/indexer/contract/meta-contract-ft"

	"runtime"
	"strconv"
	"time"

	bsvwire "github.com/bitcoinsv/bsvd/wire"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/contract/meta-contract/decoder"
)

type FtClient struct {
	rpcClient *rpcclient.Client
	cfg       *config.Config
	params    *chaincfg.Params
}

func NewFtClient(cfg *config.Config) (*FtClient, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", cfg.RPC.Host, cfg.RPC.Port),
		User:         cfg.RPC.User,
		Pass:         cfg.RPC.Password,
		HTTPPostMode: true,
		DisableTLS:   true,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}

	params, err := cfg.GetChainParams()
	if err != nil {
		return nil, fmt.Errorf("failed to get chain params: %w", err)
	}

	return &FtClient{
		rpcClient: client,
		cfg:       cfg,
		params:    params,
	}, nil
}

func (c *FtClient) GetBlock(hash *chainhash.Hash) (*btcjson.GetBlockVerboseTxResult, error) {
	return c.rpcClient.GetBlockVerboseTx(hash)
}

func (c *FtClient) GetBlockVerbose(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return c.rpcClient.GetBlockVerbose(hash)
}

func (c *FtClient) GetBlockHash(height int64) (*chainhash.Hash, error) {
	hash, err := c.rpcClient.GetBlockHash(height)
	if err != nil {
		return nil, fmt.Errorf("failed to get block hash at height %d: %w", height, err)
	}
	return hash, nil
}

// GetBlockMsg gets raw block message (MVC chain only)
func (c *FtClient) GetBlockMsg(chainName string, height int64) (msgBlock interface{}, txCount int, inTxCount int, outTxCount int, err error) {
	if chainName != "mvc" {
		return nil, 0, 0, 0, fmt.Errorf("ft_client only supports MVC chain")
	}

	hash, err := c.GetBlockHash(int64(height))
	if err != nil {
		log.Printf("Failed to get block hash, height %d: %v", height, err)
		return
	}

	var blockHex string
	// getblock <blockhash> 0
	resp, err := c.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hash.String())),
		json.RawMessage("0"),
	})
	if err != nil {
		log.Printf("Failed to get raw block data, height %d: %v", height, err)
		return nil, 0, 0, 0, err
	}
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		log.Printf("Failed to parse raw block data, height %d: %v", height, err)
		return nil, 0, 0, 0, err
	}

	// Local block parsing
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	msgBlockMvc := &bsvwire.MsgBlock{}
	err = msgBlockMvc.Deserialize(bytes.NewReader(blockBytes))
	if err != nil {
		return nil, 0, 0, 0, err
	}

	for _, tx := range msgBlockMvc.Transactions {
		inTxCount += len(tx.TxIn)
		outTxCount += len(tx.TxOut)
	}
	txCount = len(msgBlockMvc.Transactions)
	return msgBlockMvc, txCount, inTxCount, outTxCount, nil
}

// GetRawMempool gets all transaction IDs in mempool
func (c *FtClient) GetRawMempool() ([]string, error) {
	hashes, err := c.rpcClient.GetRawMempool()
	if err != nil {
		return nil, fmt.Errorf("failed to get mempool transaction list: %w", err)
	}

	// Convert hashes to strings
	txids := make([]string, len(hashes))
	for i, hash := range hashes {
		txids[i] = hash.String()
	}

	return txids, nil
}

func (c *FtClient) GetRawTransaction(txHashStr string) (*btcutil.Tx, error) {
	txHash, err := chainhash.NewHashFromStr(txHashStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction hash %s: %w", txHashStr, err)
	}
	tx, err := c.rpcClient.GetRawTransaction(txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction %s: %w", txHash, err)
	}
	return tx, nil
}

func (c *FtClient) GetRawTransactionHex(txHashStr string) (string, error) {
	txHash, err := chainhash.NewHashFromStr(txHashStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse transaction hash %s: %w", txHashStr, err)
	}

	// Use RawRequest to directly call getrawtransaction RPC command
	params := []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`"%s"`, txHash.String())),
	}
	result, err := c.rpcClient.RawRequest("getrawtransaction", params)
	if err != nil {
		return "", fmt.Errorf("failed to get transaction hex %s: %w", txHash, err)
	}

	var txHex string
	if err := json.Unmarshal(result, &txHex); err != nil {
		return "", fmt.Errorf("failed to unmarshal transaction hex: %w", err)
	}

	return txHex, nil
}

// func (c *FtClient) GetRawTransactionHex(txHashStr string) (*btcutil.Tx, error) {
// 	txHash, err := chainhash.NewHashFromStr(txHashStr)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to parse transaction hash %s: %w", txHashStr, err)
// 	}
// 	tx, err := c.rpcClient.GetRawTransactionVerbose(txHash)
// 	if err != nil {
// 		return nil,  fmt.Errorf("failed to get transaction %s: %w", txHash, err)
// 	}
// 	return tx, nil
// }

func (c *FtClient) GetBlockCount() (int, error) {
	count, err := c.rpcClient.GetBlockCount()
	if err != nil {
		return 0, fmt.Errorf("failed to get block count: %w", err)
	}
	return int(count), nil
}

func (c *FtClient) Shutdown() {
	c.rpcClient.Shutdown()
}

// SyncBlocks continuously syncs blocks (modified version)
func (c *FtClient) SyncBlocks(idx *indexer.ContractFtIndexer, checkInterval time.Duration, stopCh <-chan struct{}, onFirstSyncDone func()) error {
	firstSyncComplete := false

	for {
		select {
		case <-stopCh:
			return nil
		default:
		}

		lastHeight, err := idx.GetLastIndexedHeight()
		if err != nil {
			return fmt.Errorf("failed to get last indexed height: %w", err)
		}

		currentHeight, err := c.GetBlockCount()
		if err != nil {
			return fmt.Errorf("failed to get current block height: %w", err)
		}

		if currentHeight <= lastHeight {
			if !firstSyncComplete && onFirstSyncDone != nil {
				fmt.Printf("Currently indexed to latest block, height: %d, first sync completed\n", lastHeight)
				firstSyncComplete = true
				onFirstSyncDone()
			}
			time.Sleep(checkInterval)
			continue
		}

		fmt.Printf("Found new blocks, indexing from height %d to %d\n", lastHeight+1, currentHeight)
		idx.InitProgressBar(currentHeight, lastHeight+1)

		for height := lastHeight + 1; height <= currentHeight; height++ {
			if err := c.ProcessBlock(idx, height, true); err != nil {
				return fmt.Errorf("failed to process block, height %d: %w", height, err)
			}
		}

		fmt.Printf("Successfully indexed to current height %d\n", currentHeight)

		if !firstSyncComplete && onFirstSyncDone != nil {
			fmt.Printf("First block sync completed, now calling callback function\n")
			firstSyncComplete = true
			onFirstSyncDone()
		}

		time.Sleep(checkInterval)
	}
}

// ProcessBlock processes block at specified height (optimized version with local block parsing)
func (c *FtClient) ProcessBlock(idx *indexer.ContractFtIndexer, height int, updateHeight bool) error {
	// Get raw block data and parse locally (MVC chain only)
	chainName := c.cfg.RPC.Chain
	if chainName != "mvc" {
		return fmt.Errorf("ft_client only supports MVC chain, current chain: %s", chainName)
	}

	msgBlockInterface, txCount, _, _, err := c.GetBlockMsg(chainName, int64(height))
	if txCount > 100000 {
		log.Println("Found large block, transaction count:", txCount, "height:", height)
	}
	if err != nil {
		log.Printf("Failed to get block message, height %d: %v", height, err)
		return err
	}
	if msgBlockInterface == nil {
		return fmt.Errorf("block message is nil, height %d", height)
	}

	mvcBlockMsg := msgBlockInterface.(*bsvwire.MsgBlock)
	blockTime := mvcBlockMsg.Header.Timestamp.Unix()

	maxTxPerBatch := c.GetMaxTxPerBatch()

	// Local batch processing
	startIdx := 0
	for startIdx < txCount {
		endIdx := startIdx + maxTxPerBatch
		if endIdx > txCount {
			endIdx = txCount
		}

		// Assemble ContractFtBlock
		blockPart := &indexer.ContractFtBlock{
			Height:            height,
			Timestamp:         blockTime * 1000,
			Transactions:      make([]*indexer.ContractFtTransaction, 0, endIdx-startIdx),
			ContractFtOutputs: make(map[string][]*indexer.ContractFtOutput),
			IsPartialBlock:    endIdx != txCount,
		}

		t0 := time.Now()
		for i := startIdx; i < endIdx; i++ {
			tx := mvcBlockMsg.Transactions[i]
			indexerTx := c.convertMvcTxToContractFtTx(tx, height, blockTime*1000)
			if indexerTx != nil {
				blockPart.Transactions = append(blockPart.Transactions, indexerTx)

				// Merge ContractFtOutputs
				for _, output := range indexerTx.Outputs {
					if output.Address == "errAddress" {
						continue
					}
					blockPart.ContractFtOutputs[output.Address] = append(
						blockPart.ContractFtOutputs[output.Address],
						output,
					)
				}
			}
		}
		log.Printf("[%d]Current batch: %d to %d, transactions: %d, time: %.2fs\n", height,
			startIdx, endIdx, len(blockPart.Transactions), time.Since(t0).Seconds())

		// Index
		if err := idx.IndexBlock(blockPart, updateHeight); err != nil {
			return fmt.Errorf("failed to index block, height %d: %w", height, err)
		}

		// Release memory
		blockPart.Transactions = nil
		blockPart.ContractFtOutputs = nil
		startIdx = endIdx

		if txCount > 400000 {
			runtime.GC() // Force GC for large blocks
		}
	}

	return nil
}

// GetMaxTxPerBatch gets the maximum number of transactions per batch
func (c *FtClient) GetMaxTxPerBatch() int {
	if c.cfg != nil && c.cfg.MaxTxPerBatch > 0 {
		return c.cfg.MaxTxPerBatch
	}
	return 3000 // Default value
}

// ParseFtInfo parses FT information from script
func ParseContractFtInfo(scriptHex string, params *chaincfg.Params) (*decoder.FTUtxoInfo, *decoder.UniqueUtxoInfo, string, error) {
	scriptBytes, err := hex.DecodeString(scriptHex)
	if err != nil {
		return nil, nil, "", err
	}

	contractTypeStr := ""
	contractType := decoder.GetContractType(scriptBytes)
	if contractType == decoder.ContractTypeFT {
		contractTypeStr = "ft"
		ftUtxoInfo, err := decoder.ExtractFTUtxoInfo(scriptBytes, params)
		if err != nil {
			return nil, nil, "", err
		}
		return ftUtxoInfo, nil, contractTypeStr, nil
	} else if contractType == decoder.ContractTypeUnique {
		contractTypeStr = "unique"
		uniqueUtxoInfo, err := decoder.ExtractUniqueUtxoInfo(scriptBytes, params)
		if err != nil {
			return nil, nil, "", err
		}
		return nil, uniqueUtxoInfo, contractTypeStr, nil
	} else {
		contractTypeStr = "unknown"
		return nil, nil, contractTypeStr, nil
	}
}

// convertMvcTxToContractFtTx converts bsvwire.MsgTx to ContractFtTransaction (optimized version)
func (c *FtClient) convertMvcTxToContractFtTx(tx *bsvwire.MsgTx, height int, timestamp int64) *indexer.ContractFtTransaction {
	newHash, _ := GetNewHash2(tx)

	// Process inputs
	inputs := make([]*indexer.ContractFtInput, len(tx.TxIn))
	for j, in := range tx.TxIn {
		prevTxid := in.PreviousOutPoint.Hash.String()
		vout := in.PreviousOutPoint.Index
		id := prevTxid
		if id == "" {
			id = "0000000000000000000000000000000000000000000000000000000000000000"
		}
		inputs[j] = &indexer.ContractFtInput{
			TxPoint: common.ConcatBytesOptimized([]string{id, strconv.Itoa(int(vout))}, ":"),
		}
	}

	// Process outputs, only keep FT-related outputs
	outputs := make([]*indexer.ContractFtOutput, 0)
	hasFtOutput := false

	for k, out := range tx.TxOut {
		scriptHex := hex.EncodeToString(out.PkScript)
		address := GetAddressFromScript(scriptHex, nil, c.params, c.cfg.RPC.Chain)
		amount := strconv.FormatInt(out.Value, 10)

		// Parse FT-related information
		ftInfo, uniqueUtxoInfo, contractTypeStr, err := ParseContractFtInfo(scriptHex, c.params)
		if err != nil {
			continue // Parse failed, skip
		}

		var output *indexer.ContractFtOutput
		if contractTypeStr == "ft" {
			if ftInfo == nil {
				continue
			}
			output = &indexer.ContractFtOutput{
				ContractType: contractTypeStr,
				Address:      address,
				Value:        amount,
				Index:        int64(k),
				Height:       int64(height),
				CodeHash:     ftInfo.CodeHash,
				Genesis:      ftInfo.Genesis,
				SensibleId:   ftInfo.SensibleId,
				Name:         ftInfo.Name,
				Symbol:       ftInfo.Symbol,
				Amount:       strconv.FormatUint(ftInfo.Amount, 10),
				Decimal:      ftInfo.Decimal,
				FtAddress:    ftInfo.Address,
			}
			hasFtOutput = true
		} else if contractTypeStr == "unique" {
			if uniqueUtxoInfo == nil {
				continue
			}
			output = &indexer.ContractFtOutput{
				ContractType: contractTypeStr,
				Address:      address,
				Value:        amount,
				Index:        int64(k),
				Height:       int64(height),
				CodeHash:     uniqueUtxoInfo.CodeHash,
				Genesis:      uniqueUtxoInfo.Genesis,
				SensibleId:   uniqueUtxoInfo.SensibleId,
				CustomData:   uniqueUtxoInfo.CustomData,
			}
			hasFtOutput = true
		} else {
			continue // Not FT/unique type, skip
		}

		if output != nil {
			outputs = append(outputs, output)
		}
	}

	// If no FT-related output, return nil, no need to index this transaction
	if !hasFtOutput {
		return nil
	}

	return &indexer.ContractFtTransaction{
		ID:        newHash,
		Inputs:    inputs,
		Outputs:   outputs,
		Timestamp: timestamp,
	}
}
