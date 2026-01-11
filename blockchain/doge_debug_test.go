package blockchain

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
)

func readAuxPow(r io.Reader) error {
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

func TestDebugDogeBlock(t *testing.T) {
	// Configure RPC
	connCfg := &rpcclient.ConnConfig{
		Host:         "47.242.142.8:23116",
		User:         "dxllzyll",
		Pass:         "kjgoep61xjg1i5u3",
		HTTPPostMode: true,
		DisableTLS:   true,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Shutdown()

	// Get block hash
	height := int64(2714381)
	hash, err := client.GetBlockHash(height)
	if err != nil {
		t.Fatalf("Failed to get block hash: %v", err)
	}
	fmt.Printf("Block hash: %s\n", hash)

	// Get block Hex
	resp, err := client.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hash)),
		json.RawMessage("0"),
	})
	if err != nil {
		t.Fatalf("Failed to get block hex: %v", err)
	}

	var blockHex string
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	fmt.Printf("Block Hex Length: %d\n", len(blockHex))

	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	// Try parsing
	msgBlock := &wire.MsgBlock{}
	reader := bytes.NewReader(blockBytes)

	// 1. Parse Header
	if err := msgBlock.Header.Deserialize(reader); err != nil {
		t.Fatalf("Failed to deserialize header: %v", err)
	}
	fmt.Printf("Header parsed. Version: %d\n", msgBlock.Header.Version)

	// Check if is AuxPoW
	// Dogecoin AuxPoW version bit is usually (1 << 8) = 256
	isAuxPow := (msgBlock.Header.Version & (1 << 8)) != 0
	fmt.Printf("Is AuxPoW: %v\n", isAuxPow)

	if isAuxPow {
		fmt.Println("Detected AuxPoW block, attempting to skip AuxPoW data...")
		if err := readAuxPow(reader); err != nil {
			t.Fatalf("Failed to read AuxPoW data: %v", err)
		}
		fmt.Println("AuxPoW data skipped successfully.")
	}

	// 2. Parse transaction count
	txCount, err := wire.ReadVarInt(reader, 0)
	if err != nil {
		t.Fatalf("Failed to read tx count: %v", err)
	}
	fmt.Printf("Tx Count read: %d\n", txCount)

	// 3. Parse transactions one by one
	for i := uint64(0); i < txCount; i++ {
		tx := &wire.MsgTx{}
		// Try using DeserializeNoWitness
		err := tx.DeserializeNoWitness(reader)
		if err != nil {
			fmt.Printf("Failed to deserialize tx %d: %v\n", i, err)
			fmt.Printf("Remaining bytes: %d\n", reader.Len())
			return
		}
		// fmt.Printf("Tx %d parsed. Hash: %s\n", i, tx.TxHash())
		if i%100 == 0 {
			fmt.Printf("Parsed %d txs...\n", i)
		}
	}
	fmt.Println("All transactions parsed successfully!")
}
