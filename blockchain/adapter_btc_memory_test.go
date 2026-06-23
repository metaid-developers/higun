package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/metaid/utxo_indexer/config"
)

func TestConvertToIndexerBlockDoesNotPopulateArchiveMapsWhenDisabled(t *testing.T) {
	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MaxTxPerBatch:     100,
		BlockFilesEnabled: false,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{})
	msgBlock.AddTransaction(&wire.MsgTx{})

	block, err := (&BTCAdapter{params: &chaincfg.MainNetParams}).convertToIndexerBlock(msgBlock, 123, "hash", 1460000000)
	if err != nil {
		t.Fatalf("convertToIndexerBlock: %v", err)
	}

	if block.UtxoData != nil {
		t.Fatalf("expected UtxoData to be nil when block file archiving is disabled")
	}
	if block.IncomeData != nil {
		t.Fatalf("expected IncomeData to be nil when block file archiving is disabled")
	}
	if block.SpendData != nil {
		t.Fatalf("expected SpendData to be nil when block file archiving is disabled")
	}
}

func TestConvertToIndexerBlockPopulatesArchiveMapsWhenEnabled(t *testing.T) {
	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		MaxTxPerBatch:     100,
		BlockFilesEnabled: true,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	msgBlock := wire.NewMsgBlock(&wire.BlockHeader{})
	msgBlock.AddTransaction(&wire.MsgTx{})

	block, err := (&BTCAdapter{params: &chaincfg.MainNetParams}).convertToIndexerBlock(msgBlock, 123, "hash", 1460000000)
	if err != nil {
		t.Fatalf("convertToIndexerBlock: %v", err)
	}

	if block.UtxoData == nil {
		t.Fatalf("expected UtxoData map when block file archiving is enabled")
	}
	if block.IncomeData == nil {
		t.Fatalf("expected IncomeData map when block file archiving is enabled")
	}
	if block.SpendData == nil {
		t.Fatalf("expected SpendData map when block file archiving is enabled")
	}
}
