package blockchain

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/metaid/utxo_indexer/indexer"
)

// ChainAdapter is the chain adapter interface - all chains must implement this interface
type ChainAdapter interface {
	// Connection management
	Connect() error
	Shutdown()
	GetChainName() string
	GetChainParams() *chaincfg.Params

	// Blockchain queries
	GetBlockCount() (int, error)
	GetBlockHash(height int64) (string, error)

	// Block data (core method)
	// Returns parsed block data in unified format
	GetBlock(height int64) (*indexer.Block, error)

	// Transaction data
	GetTransaction(txid string) (*indexer.Transaction, error)

	// Memory pool
	GetRawMempool() ([]string, error)

	// Block reorganization detection
	FindReorgHeight() (int, int)
}

// BlockHeader is block header information
type BlockHeader struct {
	Hash              string
	Height            int64
	PreviousBlockHash string
	NextBlockHash     string
	Timestamp         int64
	Confirmations     int64
}
