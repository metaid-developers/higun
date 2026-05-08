package blockchain

import (
	"fmt"
	"time"

	"github.com/metaid/utxo_indexer/syslogs"
)

type reorgDetection struct {
	lastSameHeight int
	startHeight    int
	endHeight      int
	reorgHash      string
	newHash        string
	reorgSize      int
}

type reorgMismatch struct {
	height    int
	localHash string
	chainHash string
}

func detectReorgFromLogs(logs []syslogs.IndexerLog, getChainHash func(height int) (string, error)) (reorgDetection, bool) {
	if len(logs) == 0 {
		return reorgDetection{}, false
	}

	endHeight := logs[0].Height
	mismatches := make([]reorgMismatch, 0)

	for _, block := range logs {
		chainHash, err := getChainHash(block.Height)
		if err != nil {
			continue
		}

		if chainHash == block.BlockHash {
			if len(mismatches) == 0 {
				continue
			}

			divergent := mismatches[len(mismatches)-1]
			return reorgDetection{
				lastSameHeight: block.Height,
				startHeight:    block.Height + 1,
				endHeight:      endHeight,
				reorgHash:      divergent.localHash,
				newHash:        divergent.chainHash,
				reorgSize:      len(mismatches),
			}, true
		}

		mismatches = append(mismatches, reorgMismatch{
			height:    block.Height,
			localHash: block.BlockHash,
			chainHash: chainHash,
		})
	}

	// If we cannot find a common ancestor within the scan window, skip automatic
	// rollback rather than rewinding to an unsafe height.
	return reorgDetection{}, false
}

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

	detection, ok := detectReorgFromLogs(data, func(height int) (string, error) {
		chainBlockHash, err := c.GetBlockHash(int64(height))
		if err != nil {
			return "", err
		}
		return chainBlockHash.String(), nil
	})
	if ok {
		log := syslogs.ReorgLog{
			Height:       detection.startHeight,
			EndHeight:    detection.endHeight,
			BlockHash:    detection.reorgHash,
			NewBlockHash: detection.newHash,
			ReorgSize:    detection.reorgSize,
			Timestamp:    time.Now().Unix(),
			Status:       0,
		}
		syslogs.InsertReorgLog(log)
		return detection.lastSameHeight, detection.endHeight
	}
	return -1, -1
}
