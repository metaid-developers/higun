package main

import (
	"fmt"
	"log"
	"testing"

	"github.com/metaid/utxo_indexer/storage"
)

func TestDataFunction(t *testing.T) {
	cfg, params := initConfig()
	utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr, err := initDb(cfg, params)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer closeDb(utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr)
	// 查询单个地址数据
	address := "bcrt1q2mvt4fkmp94hd2tx9ruj8g7na53kp4mqrq7n3n"
	address = "bcrt1q7hfafmwd45xjj2sqlul6lk7p5sewlmu5a2atnq"
	fmt.Printf("查找: %s\n", address)
	value, err := addressStore.Get([]byte(address))
	if err != nil {
		if err == storage.ErrNotFound {
			fmt.Printf("addressStore未找到数据\n")
		} else {
			fmt.Printf("查询出错: %v\n", err)
		}
	} else {
		fmt.Println("---------------------------------")
		fmt.Printf("addressStore Value: %s\n", string(value))
	}

	value, err = spendStore.Get([]byte(address))
	if err != nil {
		if err == storage.ErrNotFound {
			fmt.Printf("spendStore未找到数据\n")
		} else {
			fmt.Printf("查询出错: %v\n", err)
		}
	} else {
		fmt.Println("---------------------------------")
		fmt.Printf("spendStore Value: %s\n", string(value))
	}

	mempoolIncome, mempoolSpend := mempoolMgr.GetDataByAddress(address)
	fmt.Println("---------------------------------")
	fmt.Printf("mempoolMgr mempoolIncome: %s\n", mempoolIncome)
	fmt.Println("---------------------------------")
	fmt.Printf("mempoolMgr mempoolSpend: %s\n", mempoolSpend)
	fmt.Println("---------------------------------")
	//mempoolMgr.MempoolIncomeDB
	mempoolMgr.MempoolIncomeDB.TestData()
	mempoolMgr.MempoolSpendDB.TestData()
}

func TestDataFunction2(t *testing.T) {
	cfg, params := initConfig()
	utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr, err := initDb(cfg, params)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer closeDb(utxoStore, addressStore, spendStore, bcClient, metaStore, mempoolMgr)
	s, err := utxoStore.Get([]byte("17fc3f9b89b6734fcb8298a402ab5e7887089be7c10eaccf7e2abb1026fc0fb4"))
	if err != nil {
		fmt.Printf("Error getting data: %v\n", err)
	} else {
		fmt.Printf("Data: %s\n", string(s))
	}
}
