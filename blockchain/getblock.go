package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	bsvwire "github.com/bitcoinsv/bsvd/wire"
	"github.com/btcsuite/btcd/wire"
)

func (c *Client) GetBlockMsg(chainName string, height int64) (msgBlock interface{}, txCount int, inTxCount int, outTxCount int, err error) {
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
		log.Printf("Failed to get original block data, height %d: %v, retrying in 3 seconds...", height, err)
		return nil, 0, 0, 0, err
	}
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		log.Printf("Failed to parse original block data, height %d: %v, retrying in 3 seconds...", height, err)
		return nil, 0, 0, 0, err
	}
	// Local block parsing
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return
	}
	msgBlockMvc := &bsvwire.MsgBlock{}
	msgBlockBtc := &wire.MsgBlock{}
	if chainName == "mvc" {
		err = msgBlockMvc.Deserialize(bytes.NewReader(blockBytes))
		if err != nil {
			return
		}
		for _, tx := range msgBlockMvc.Transactions {
			inTxCount += len(tx.TxIn)
			outTxCount += len(tx.TxOut)
		}
		txCount = len(msgBlockMvc.Transactions)
		return msgBlockMvc, txCount, inTxCount, outTxCount, nil
	} else {
		err = msgBlockBtc.Deserialize(bytes.NewReader(blockBytes))
		if err != nil {
			return
		}
		for _, tx := range msgBlockBtc.Transactions {
			inTxCount += len(tx.TxIn)
			outTxCount += len(tx.TxOut)
		}

		txCount = len(msgBlockBtc.Transactions)
		return msgBlockBtc, txCount, inTxCount, outTxCount, nil
	}
}
