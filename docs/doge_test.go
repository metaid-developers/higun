package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// 定义狗狗币的主网参数
var dogeMainNetParams = chaincfg.Params{
	Name:             "dogecoin-mainnet",
	Net:              wire.MainNet,
	PubKeyHashAddrID: 0x1e, // 'D' addresses
	ScriptHashAddrID: 0x16, // '9' or 'A' addresses
	PrivateKeyID:     0x9e, // WIF private keys
	Bech32HRPSegwit:  "bc", // Not used by Dogecoin
	HDPrivateKeyID:   [4]byte{0x02, 0xfa, 0xca, 0xfd},
	HDPublicKeyID:    [4]byte{0x02, 0xfa, 0xc3, 0x98},
}

func main() {
	// 通过网址查询交易原始数据
	//https://api.blockcypher.com/v1/doge/main/txs/d96170578d6c2868cb9cf63ec414c854f39c3e5fadd1e03005e9db54c309935c?includeHex=true
	//交易浏览器
	//https://sochain.com/tx/DOGE/d96170578d6c2868cb9cf63ec414c854f39c3e5fadd1e03005e9db54c309935c
	// 一个干净、标准的狗狗币交易原始数据
	rawTxHex := "0100000001c61fe83ba9a47f13238972e53c6645243dab6c0975f0bacaaa11c5a8e06beac4000000006b483045022100e7a64c6f4be99556a37fb1e0aa3aaaee4b102842d4d91ed4283e46700a54df8e02201a3f3c727cdccb9664beb7a131fa84ebc58eada847a14cce08dd8f8a0202b8c60121021af2709eb9329150658b40d1a7987f484d70a9e9e7d21f2449a29f67e8f64c73fdffffff06e0e347e21b0000001976a91469140ac9abc2016f7a9dc9c67be6b96cccd3c84888ac1436be231d0000001976a914788a64424c2b5206cb59bb7fd3d870829fa0ac9188acfb35be231d0000001976a914e254330131ae32fec4f05a1e18ec74cb0187a7cf88acd535be231d0000001976a914e7526c2aba74a3c3cfeb3fab479f8b2dbbada06988ac0936be231d0000001976a914d474dd385839d6bd4eb7ade3b628f09d9a29efde88ac1935be231d0000001976a914fc79863a37d2f564e96f6a856151e8ba36c99b3088ac00000000"

	// 1. 解码交易
	rawTxBytes, err := hex.DecodeString(rawTxHex)
	if err != nil {
		log.Fatalf("无法解码原始交易: %v", err)
	}

	var msgTx wire.MsgTx
	err = msgTx.Deserialize(bytes.NewReader(rawTxBytes))
	if err != nil {
		log.Fatalf("无法反序列化交易: %v", err)
	}

	fmt.Printf("✅ 交易解析成功!\n")
	fmt.Printf("TXID: %s\n", msgTx.TxHash().String())
	fmt.Printf("输入数量: %d\n", len(msgTx.TxIn))
	fmt.Printf("输出数量: %d\n", len(msgTx.TxOut))
	fmt.Println("---------------------------------")

	// 2. 遍历所有交易输出
	fmt.Println("🔍 正在解析交易输出...")
	for i, txOut := range msgTx.TxOut {
		fmt.Printf("\n--- 输出 %d ---\n", i)

		pkScript := txOut.PkScript

		// 3. 使用狗狗币参数提取地址
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(pkScript, &dogeMainNetParams)
		if err != nil {
			fmt.Printf("无法从脚本中提取地址: %v\n", err)
			continue
		}

		// 转换金额单位
		amount := float64(txOut.Value) / 100000000.0
		fmt.Printf("金额: %.8f DOGE\n", amount)

		if len(addresses) > 0 {
			fmt.Println("地址:")
			for _, addr := range addresses {
				fmt.Printf("  - %s\n", addr.EncodeAddress())
			}
		} else {
			fmt.Println("未找到标准地址。")
		}
	}
}
