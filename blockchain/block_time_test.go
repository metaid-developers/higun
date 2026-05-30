package blockchain

import (
	"testing"

	bsvwire "github.com/bitcoinsv/bsvd/wire"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
)

func TestBlockTimeStringForIndexingUsesAdapterBlockTime(t *testing.T) {
	got := blockTimeStringForIndexing(&indexer.Block{BlockTime: 1460000000})
	if got != "1460000000" {
		t.Fatalf("expected adapter block time, got %s", got)
	}
}

func TestMVCConvertToIndexerBlockPreservesBlockTime(t *testing.T) {
	withBlockTimeTestConfig(t)

	block, err := (&MVCAdapter{}).convertToIndexerBlock(&bsvwire.MsgBlock{}, 123, "hash", 1460000000)
	if err != nil {
		t.Fatalf("convertToIndexerBlock: %v", err)
	}
	if block.BlockTime != 1460000000 {
		t.Fatalf("expected block time 1460000000, got %d", block.BlockTime)
	}
}

func TestMVCLargeBlockPathPreservesVerboseBlockTime(t *testing.T) {
	withBlockTimeTestConfig(t)

	block, err := (&MVCAdapter{}).getBlockByTxHashes(&mvcBlockVerbose1Result{
		Hash: "hash",
		Time: 1460000000,
		Tx:   nil,
	}, 123)
	if err != nil {
		t.Fatalf("getBlockByTxHashes: %v", err)
	}
	if block.BlockTime != 1460000000 {
		t.Fatalf("expected block time 1460000000, got %d", block.BlockTime)
	}
}

func withBlockTimeTestConfig(t *testing.T) {
	t.Helper()
	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		LargeBlockFetchWorkers: 1,
		MaxTxPerBatch:          100,
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})
}
