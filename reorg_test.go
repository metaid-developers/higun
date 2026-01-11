package main

import (
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/metaid/utxo_indexer/blockchain"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/syslogs"
)

func getBlockData(blockHeight int) indexer.Block {
	var blockPart indexer.Block
	blockPart.BlockHash = "test_hash3"
	blockPart.Height = blockHeight
	var transactions []*indexer.Transaction
	transactions = append(transactions, &indexer.Transaction{
		ID: "test_tx_hash_1",
		Inputs: []*indexer.Input{
			{
				TxPoint: "tx03:1",
			},
		},
		Outputs: []*indexer.Output{
			{
				Address: "addr1",
				Amount:  "123456",
			},
		},
	})
	transactions = append(transactions, &indexer.Transaction{
		ID: "test_tx_hash_2",
		Inputs: []*indexer.Input{
			{
				TxPoint: "test_tx_hash_2:0",
			},
		},
		Outputs: []*indexer.Output{
			{
				Address: "addr1",
				Amount:  "123456",
			},
		},
	})
	blockPart.AddressIncome = make(map[string][]*indexer.Income)
	blockPart.Transactions = transactions
	blockPart.UtxoData = make(map[string][]string)
	blockPart.UtxoData["test_tx_hash_1:0"] = []string{"addr1:10000", "addr2:10000"}
	blockPart.UtxoData["test_tx_hash_2:0"] = []string{"addr1:20000", "addr2:20000"}
	blockPart.IncomeData = map[string][]string{}
	// IncomeData
	blockPart.IncomeData["addr1"] = []string{"test_tx_hash_1:0"}
	blockPart.IncomeData["addr2"] = []string{"test_tx_hash_2:0"}
	blockPart.SpendData = map[string][]string{}
	// SpendData
	blockPart.SpendData["addr1"] = []string{"test_tx_hash_1:0"}
	blockPart.SpendData["addr2"] = []string{"test_tx_hash_2:0"}
	return blockPart
}
func TestAddBlockData(t *testing.T) {
	// 测试添加区块数据
	blockPart := getBlockData(100003)
	cfg, params := initConfig()
	utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr, err := initDb(cfg, params)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer closeDb(utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr)
	idx := indexer.NewUTXOIndexer(params, utxoStore, addressStore, metaStore, spendStore)
	if mempoolMgr != nil {
		idx.SetMempoolManager(mempoolMgr)
	}
	allBlock := &indexer.Block{
		Height:     blockPart.Height,
		BlockHash:  blockPart.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}
	inCnt, outCnt, addressNum, err := idx.IndexBlock(&blockPart, allBlock, false, "1623456789")
	fmt.Println("inCnt:", inCnt, "outCnt:", outCnt, "addressNum:", addressNum, "err:", err)

}
func TestDelBlockData(t *testing.T) {
	//getall
	cfg, params := initConfig()
	utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr, err := initDb(cfg, params)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer closeDb(utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr)
	idx := indexer.NewUTXOIndexer(params, utxoStore, addressStore, metaStore, spendStore)
	if mempoolMgr != nil {
		idx.SetMempoolManager(mempoolMgr)
	}
	if err := idx.DeleteDataByBlockHeight(100003); err != nil {
		log.Fatalf("Failed to delete block data: %v", err)
	}
}
func TestGetAllCount(t *testing.T) {
	cfg, params := initConfig()
	utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr, err := initDb(cfg, params)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer closeDb(utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr)
	_, utxoData, _ := utxoStore.GetAll()
	spendKey, spendData, _ := spendStore.GetAll()
	addressKey, addressData, _ := addressStore.GetAll()
	for _, key := range spendKey {
		fmt.Println("spend key:", string(key))
	}
	for i, key := range addressKey {
		fmt.Println("address key:", string(key))
		if string(key) == "addr1" {
			fmt.Println("===>address data:", string(addressData[i]))
		}
	}
	fmt.Println("UTXO Data:", len(utxoData), "Spend Data:", len(spendData), "Address Data:", len(addressData))
}
func TestSaveFBlockFile(t *testing.T) {
	config.LoadConfig("config_btc.yaml")
	fmt.Println("===>", config.GlobalConfig.BlockFilesDir)
	blockPart := getBlockData(100003)
	fblock := indexer.BlockToFBlock(&blockPart, "utxo")
	err := indexer.SaveFBlockPart(fblock, "utxo", 1)
	if err != nil {
		t.Fatalf("Failed to save block: %v", err)
	}
	fblock = indexer.BlockToFBlock(&blockPart, "spend")
	err = indexer.SaveFBlockPart(fblock, "spend", 1)
	if err != nil {
		t.Fatalf("Failed to save block: %v", err)
	}
}
func TestLoadFBlockFile(t *testing.T) {
	blockHeight := int64(100003)
	blockHeight = int64(413)
	config.LoadConfig("config_btc.yaml")
	//先看看有没有独立文件
	block, err := indexer.LoadFBlockPart(blockHeight, "", -1)
	if err != nil {
		fmt.Printf("Failed to load single block: %v", err)
	}
	if block == nil {
		fmt.Println("Failed to load single block: no block found")
	}
	if block != nil {
		fmt.Println("Loaded block:", block.Height, block.BlockHash)
		for k, item := range block.IncomeData {
			fmt.Println("Single Income :", k, item)
		}
		for k, item := range block.UtxoData {
			fmt.Println("Single UTXO :", k, item)
		}
		for k, item := range block.SpendData {
			fmt.Println("Single Spend :", k, item)
		}
	}
	//再看看有没有分片文件
	for i := 0; i < 10000; i++ {
		block, err := indexer.LoadFBlockPart(blockHeight, "utxo", i)
		if err == errors.New("noFile") {
			break
		}
		if err == nil {
			fmt.Println("Loaded part block:", block.Height, block.BlockHash, len(block.UtxoData), len(block.IncomeData), len(block.SpendData))
			for k, item := range block.IncomeData {
				fmt.Println("Part Income :", k, item)
			}
			for k, item := range block.UtxoData {
				fmt.Println("Part UTXO :", k, item)
			}
			for k, item := range block.SpendData {
				fmt.Println("Part Spend :", k, item)
			}
		}
	}
	for i := 0; i < 10000; i++ {
		block, err := indexer.LoadFBlockPart(blockHeight, "spend", i)
		if err == errors.New("noFile") {
			break
		}
		if err == nil {
			for k, item := range block.IncomeData {
				fmt.Println("Part Income :", k, item)
			}
			for k, item := range block.UtxoData {
				fmt.Println("Part UTXO :", k, item)
			}
			for k, item := range block.SpendData {
				fmt.Println("Part Spend :", k, item)
			}
		}
	}
}

func TestFind(t *testing.T) {
	cfg, _ := initConfig()
	client, err := blockchain.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create blockchain client: %v", err)
	}
	height, endHeight := client.FindReorgHeight()
	fmt.Println("Reorg from height:", height, "to", endHeight)
}
func TestReorgLog(t *testing.T) {
	initConfig()
	log := syslogs.ReorgLog{
		Height:       1,
		EndHeight:    1,
		BlockHash:    "aa",
		NewBlockHash: "bb",
		ReorgSize:    1,
		Timestamp:    time.Now().Unix(),
		Status:       0,
	}
	syslogs.InsertReorgLog(log)
}
func TestZmqs(t *testing.T) {
	cfg, _ := initConfig()
	fmt.Println(cfg.ZMQAddress)
}
