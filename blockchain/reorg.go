package blockchain

import (
	"fmt"
	"time"

	"github.com/metaid/utxo_indexer/syslogs"
)

// 从区块链中查找重组
// 首先获取本地数据库中最新500个区块的hash和height
// 用这些height去区块链上查找对应的区块,并对比本地hash和区块链上的hash
// 找出最后一个相同的区块,这个区块之后的区块就是重组的区块
// 记录重组的区块信息
func (c *Client) FindReorgHeight() (int, int) {
	data, err := syslogs.QueryUnReorgIndexerLogs(500, 0)
	if err != nil || len(data) == 0 {
		fmt.Println(err)
		return 0, 0
	}
	var lastSameHeight int = -1
	reorgHash := ""
	isReorg := false
	newHash := ""
	reorgSize := 0
	endHeight := data[0].Height
	for _, block := range data {
		chainBlockHash, err := c.GetBlockHash(int64(block.Height))
		if err != nil {

			continue
		}
		reorgSize++
		if chainBlockHash.String() == block.BlockHash {
			lastSameHeight = block.Height
		} else {
			isReorg = true
			reorgHash = block.BlockHash
			newHash = chainBlockHash.String()
			if lastSameHeight == -1 {
				lastSameHeight = block.Height - 1
			}
			break
		}
	}
	if isReorg {
		log := syslogs.ReorgLog{
			Height:       lastSameHeight + 1,
			EndHeight:    endHeight,
			BlockHash:    reorgHash,
			NewBlockHash: newHash,
			ReorgSize:    reorgSize,
			Timestamp:    time.Now().Unix(),
			Status:       0,
		}
		syslogs.InsertReorgLog(log)
		return lastSameHeight, endHeight
	}
	return -1, -1
}
