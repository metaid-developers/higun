package indexer

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-colorable"
	"github.com/metaid/utxo_indexer/common"
	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/storage"
	"github.com/schollz/progressbar/v3"
)

var (
	// Record the last time logs were printed
	lastLogTime time.Time
)

type ContractNftIndexer struct {
	contractNftUtxoStore          *storage.PebbleStore // Store contract Utxo data key: txID, value:NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
	addressNftIncomeStore         *storage.PebbleStore // Store address-related NFT contract Utxo data key: NftAddress, value: CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
	addressNftSpendStore          *storage.PebbleStore // Store used NFT contract Utxo data key: NftAddress, value: txid@index@codeHash@genesis@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId,...
	codeHashGenesisNftIncomeStore *storage.PebbleStore // Store codeHash@genesis, value: NftAddress@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
	codeHashGenesisNftSpendStore  *storage.PebbleStore // Store codeHash@genesis, value: txid@index@NftAddress@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId,...

	addressSellNftIncomeStore         *storage.PebbleStore // Store NftAddress, value: codeHash@genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@height,...
	addressSellNftSpendStore          *storage.PebbleStore // Store NftAddress, value: txid@index@codeHash@genesis@tokenIndex@value@height@usedTxId,...
	codeHashGenesisSellNftIncomeStore *storage.PebbleStore // Store codeHash@genesis, value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@height,...
	codeHashGenesisSellNftSpendStore  *storage.PebbleStore // Store codeHash@genesis, value: txid@index@NftAddress@tokenIndex@value@height@usedTxId,...

	contractNftInfoStore          *storage.PebbleStore // Store contract info key:codeHash@genesis@TokenIndex, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
	contractNftSummaryInfoStore   *storage.PebbleStore // Store contract info key:codeHash@genesis, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
	contractNftGenesisStore       *storage.PebbleStore // Store contract genesis info key:outpoint, value: sensibleId@tokenSupply@codeHash@genesis@MetaTxId@MetaOutputIndex
	contractNftGenesisOutputStore *storage.PebbleStore // Store used contract genesis output info key:usedOutpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
	contractNftGenesisUtxoStore   *storage.PebbleStore // Store contract genesis UTXO info key:outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}

	contractNftOwnersIncomeValidStore *storage.PebbleStore // Store contract owners income valid info key:codeHash@genesis, value: address@tokenIndex@txId@index,...
	contractNftOwnersIncomeStore      *storage.PebbleStore // Store contract owners info key:codeHash@genesis, value: address@tokenIndex@txId@index,...
	contractNftOwnersSpendStore       *storage.PebbleStore // Store contract owners info key:codeHash@genesis, value: address@tokenIndex@txId@index,...

	contractNftAddressHistoryStore *storage.PebbleStore // Store contract address history info key:address, value: txId@time@income/outcome@blockHeight,...
	contractNftGenesisHistoryStore *storage.PebbleStore // Store contract genesis history info key:codeHash@genesis, value: txId@time@income/outcome@blockHeight,...

	addressNftIncomeValidStore         *storage.PebbleStore // Store address-related NFT contract Utxo data key: NftAddress, value: CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
	codeHashGenesisNftIncomeValidStore *storage.PebbleStore // Store codeHash@genesis, value: NftAddress@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
	uncheckNftOutpointStore            *storage.PebbleStore // Store unchecked NFT contract Utxo data key: outpoint, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
	usedNftIncomeStore                 *storage.PebbleStore // Store used NFT contract Utxo data key: UsedtxID, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...

	invalidNftOutpointStore *storage.PebbleStore // Store invalid NFT contract Utxo data key: outpoint, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@reason,...

	metaStore   *storage.MetaStore // Store metadata
	mu          sync.RWMutex
	bar         *progressbar.ProgressBar
	params      config.IndexerParams
	mempoolMgr  NftMempoolManager
	mempoolInit bool // Whether mempool is initialized

	stopCh <-chan struct{}
}

var workers = 1
var batchSize = 1000

func NewContractNftIndexer(params config.IndexerParams,
	contractNftUtxoStore,
	addressNftIncomeStore,
	addressNftSpendStore,
	codeHashGenesisNftIncomeStore,
	codeHashGenesisNftSpendStore,
	addressSellNftIncomeStore,
	addressSellNftSpendStore,
	codeHashGenesisSellNftIncomeStore,
	codeHashGenesisSellNftSpendStore,
	contractNftInfoStore,
	contractNftSummaryInfoStore,
	contractNftGenesisStore,
	contractNftGenesisOutputStore,
	contractNftGenesisUtxoStore,
	contractNftOwnersIncomeValidStore,
	contractNftOwnersIncomeStore,
	contractNftOwnersSpendStore,
	contractNftAddressHistoryStore,
	contractNftGenesisHistoryStore,
	addressNftIncomeValidStore,
	codeHashGenesisNftIncomeValidStore,
	uncheckNftOutpointStore,
	usedNftIncomeStore,
	invalidNftOutpointStore *storage.PebbleStore,
	metaStore *storage.MetaStore) *ContractNftIndexer {
	return &ContractNftIndexer{
		params:                             params,
		contractNftUtxoStore:               contractNftUtxoStore,
		addressNftIncomeStore:              addressNftIncomeStore,
		addressNftSpendStore:               addressNftSpendStore,
		codeHashGenesisNftIncomeStore:      codeHashGenesisNftIncomeStore,
		codeHashGenesisNftSpendStore:       codeHashGenesisNftSpendStore,
		contractNftInfoStore:               contractNftInfoStore,
		contractNftSummaryInfoStore:        contractNftSummaryInfoStore,
		contractNftGenesisStore:            contractNftGenesisStore,
		contractNftGenesisOutputStore:      contractNftGenesisOutputStore,
		contractNftGenesisUtxoStore:        contractNftGenesisUtxoStore,
		contractNftOwnersIncomeValidStore:  contractNftOwnersIncomeValidStore,
		contractNftOwnersIncomeStore:       contractNftOwnersIncomeStore,
		contractNftOwnersSpendStore:        contractNftOwnersSpendStore,
		contractNftAddressHistoryStore:     contractNftAddressHistoryStore,
		contractNftGenesisHistoryStore:     contractNftGenesisHistoryStore,
		addressNftIncomeValidStore:         addressNftIncomeValidStore,
		codeHashGenesisNftIncomeValidStore: codeHashGenesisNftIncomeValidStore,
		uncheckNftOutpointStore:            uncheckNftOutpointStore,
		usedNftIncomeStore:                 usedNftIncomeStore,
		invalidNftOutpointStore:            invalidNftOutpointStore,
		addressSellNftIncomeStore:          addressSellNftIncomeStore,
		addressSellNftSpendStore:           addressSellNftSpendStore,
		codeHashGenesisSellNftIncomeStore:  codeHashGenesisSellNftIncomeStore,
		codeHashGenesisSellNftSpendStore:   codeHashGenesisSellNftSpendStore,
		metaStore:                          metaStore,
	}
}

func (i *ContractNftIndexer) InitProgressBar(totalBlocks, startHeight int) {
	remainingBlocks := totalBlocks - startHeight
	if remainingBlocks <= 0 {
		remainingBlocks = 1
	}
	i.bar = progressbar.NewOptions(remainingBlocks,
		progressbar.OptionSetWriter(colorable.NewColorableStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(50),
		progressbar.OptionSetDescription("Indexing NFT contract blocks..."),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionSetRenderBlankState(false),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(colorable.NewColorableStdout(), "\nDone!\n")
		}),
	)
}

func (i *ContractNftIndexer) IndexBlock(block *ContractNftBlock, updateHeight bool) error {
	if block == nil {
		return fmt.Errorf("cannot index nil block")
	}

	workers = i.params.WorkerCount
	batchSize = i.params.BatchSize

	if err := block.Validate(); err != nil {
		return fmt.Errorf("invalid block: %w", err)
	}

	// Add timer
	startTime := time.Now()
	txCount := len(block.Transactions)

	// Phase 1: Index all contract outputs
	if err := i.indexContractNftOutputs(block); err != nil {
		return fmt.Errorf("failed to index contract outputs: %w", err)
	}
	block.ContractNftOutputs = nil
	elapsed1 := time.Now().Sub(startTime)

	startTime2 := time.Now()
	// Phase 2: Process all contract inputs
	if err := i.processContractNftInputs(block); err != nil {
		return fmt.Errorf("failed to process contract inputs: %w", err)
	}
	block.Transactions = nil
	elapsed2 := time.Now().Sub(startTime2)

	// Check if log should be printed
	currentTime := time.Now()
	if lastLogTime.IsZero() || currentTime.Sub(lastLogTime) >= 5*time.Minute {
		log.Printf("[IndexBlock][%d] Completed indexContractNftOutputs, processed %d transactions, took: %v seconds", block.Height, txCount, elapsed1.Seconds())
		log.Printf("[IndexBlock][%d] Completed processContractNftInputs, processed %d transactions, total time: %v seconds", block.Height, txCount, elapsed2.Seconds())
		lastLogTime = currentTime
	}

	if !block.IsPartialBlock && updateHeight {
		heightStr := strconv.Itoa(block.Height)
		if err := i.metaStore.Set([]byte(common.MetaStoreKeyLastNftIndexedHeight), []byte(heightStr)); err != nil {
			return err
		}

		if err := i.metaStore.Sync(); err != nil {
			log.Printf("Failed to sync meta store: %v", err)
			return err
		}

		if i.bar != nil {
			i.bar.Add(1)
		}
	}

	block = nil
	return nil
}

func (i *ContractNftIndexer) indexContractNftOutputs(block *ContractNftBlock) error {
	txCount := len(block.Transactions)
	batchCount := (txCount + batchSize - 1) / batchSize

	for batchIndex := 0; batchIndex < batchCount; batchIndex++ {
		start := batchIndex * batchSize
		end := start + batchSize
		if end > txCount {
			end = txCount
		}

		batchSize := end - start
		contractNftUtxoMap := make(map[string][]string, batchSize*3)
		addressNftUtxoMap := make(map[string][]string, batchSize)
		codeHashGenesisNftIncomeMap := make(map[string][]string, batchSize)
		nftInfoMap := make(map[string]string, batchSize)
		contractSummaryInfoMap := make(map[string]string, batchSize)
		genesisMap := make(map[string]string, batchSize)
		genesisUtxoMap := make(map[string]string, batchSize)
		uncheckNftOutpointMap := make(map[string]string, batchSize)
		addressSellNftIncomeMap := make(map[string][]string, batchSize)
		codeHashGenesisSellNftIncomeMap := make(map[string][]string, batchSize)
		addressTxTimeMap := make(map[string][]string, batchSize)
		genesisTxTimeMap := make(map[string][]string, batchSize)
		contractNftOwnersIncomeMap := make(map[string][]string, batchSize)

		hasNft := false
		hasNftSell := false
		for i := start; i < end; i++ {
			tx := block.Transactions[i]
			for _, out := range tx.Outputs {
				// Process contract UTXO storage
				//key: txID, value:NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@contractType,...
				contractNftUtxoMap[tx.ID] = append(contractNftUtxoMap[tx.ID],
					common.ConcatBytesOptimized([]string{
						out.NftAddress,
						out.CodeHash,
						out.Genesis,
						out.SensibleId,
						strconv.FormatUint(out.TokenIndex, 10),
						strconv.Itoa(int(out.Index)),
						out.Value,
						strconv.FormatUint(out.TokenSupply, 10),
						out.MetaTxId,
						strconv.FormatUint(out.MetaOutputIndex, 10),
						strconv.FormatInt(out.Height, 10),
						out.ContractType,
					}, "@"))

				if out.ContractType == "nft" {
					hasNft = true

					if out.SensibleId != "000000000000000000000000000000000000000000000000000000000000000000000000" &&
						out.MetaTxId != "0000000000000000000000000000000000000000000000000000000000000000" {
						// Process NFT info storage
						// key:codeHash@genesis@TokenIndex, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
						nftInfoKey := common.ConcatBytesOptimized([]string{out.CodeHash, out.Genesis, fmt.Sprintf("%030d", out.TokenIndex)}, "@")
						if _, exists := nftInfoMap[nftInfoKey]; !exists {
							nftInfoMap[nftInfoKey] = common.ConcatBytesOptimized([]string{
								out.SensibleId,
								strconv.FormatUint(out.TokenSupply, 10),
								out.MetaTxId,
								strconv.FormatUint(out.MetaOutputIndex, 10),
							}, "@")
						}
						// Process contract summary info storage
						// key:codeHash@genesis, value: sensibleId@tokenSupply@MetaTxId@MetaOutputIndex
						contractSummaryInfoKey := common.ConcatBytesOptimized([]string{out.CodeHash, out.Genesis}, "@")
						if _, exists := contractSummaryInfoMap[contractSummaryInfoKey]; !exists {
							contractSummaryInfoMap[contractSummaryInfoKey] = common.ConcatBytesOptimized([]string{
								out.SensibleId,
								strconv.FormatUint(out.TokenSupply, 10),
								out.MetaTxId,
								strconv.FormatUint(out.MetaOutputIndex, 10),
							}, "@")
						}
					}
					// Process initial genesis UTXO storage
					// key: outpoint, value: sensibleId@tokenSupply@codeHash@genesis@MetaTxId@MetaOutputIndex
					if out.SensibleId == "000000000000000000000000000000000000000000000000000000000000000000000000" {

						genesisKey := common.ConcatBytesOptimized([]string{tx.ID, strconv.Itoa(int(out.Index))}, ":")
						if _, exists := genesisMap[genesisKey]; !exists {
							genesisMap[genesisKey] = common.ConcatBytesOptimized([]string{
								out.SensibleId,
								strconv.FormatUint(out.TokenSupply, 10),
								out.CodeHash,
								out.Genesis,
								out.MetaTxId,
								strconv.FormatUint(out.MetaOutputIndex, 10),
							}, "@")
						}
					}

					// Process new genesis UTXO records
					// key:outpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@index@value@MetaTxId@MetaOutputIndex{@IsSpent}
					if out.TokenIndex != 0 && out.MetaTxId == "0000000000000000000000000000000000000000000000000000000000000000" {
						genesisUtxoKey := common.ConcatBytesOptimized([]string{tx.ID, strconv.Itoa(int(out.Index))}, ":")
						if _, exists := genesisUtxoMap[genesisUtxoKey]; !exists {
							genesisUtxoMap[genesisUtxoKey] = common.ConcatBytesOptimized(
								[]string{
									out.SensibleId,
									strconv.FormatUint(out.TokenSupply, 10),
									out.CodeHash,
									out.Genesis,
									strconv.FormatUint(out.TokenIndex, 10),
									strconv.Itoa(int(out.Index)),
									out.Value,
									out.MetaTxId,
									strconv.FormatUint(out.MetaOutputIndex, 10),
								}, "@")
						}
					}

					// Process address NFT UTXO storage
					//  key: NftAddress, value: CodeHash@Genesis@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
					if _, exists := addressNftUtxoMap[out.NftAddress]; !exists {
						addressNftUtxoMap[out.NftAddress] = make([]string, 0, 4)
					}
					addressNftUtxoMap[out.NftAddress] = append(addressNftUtxoMap[out.NftAddress],
						common.ConcatBytesOptimized(
							[]string{
								out.CodeHash,
								out.Genesis,
								strconv.FormatUint(out.TokenIndex, 10),
								tx.ID,
								strconv.Itoa(int(out.Index)),
								out.Value,
								strconv.FormatUint(out.TokenSupply, 10),
								out.MetaTxId,
								strconv.FormatUint(out.MetaOutputIndex, 10),
								strconv.FormatInt(out.Height, 10),
							}, "@"))

					// Process codeHash@genesis NFT UTXO storage
					// key: codeHash@genesis, value: NftAddress@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
					codeHashGenesisNftIncomeKey := common.ConcatBytesOptimized([]string{out.CodeHash, out.Genesis}, "@")
					if _, exists := codeHashGenesisNftIncomeMap[codeHashGenesisNftIncomeKey]; !exists {
						codeHashGenesisNftIncomeMap[codeHashGenesisNftIncomeKey] = make([]string, 0, 4)
					}
					codeHashGenesisNftIncomeMap[codeHashGenesisNftIncomeKey] = append(codeHashGenesisNftIncomeMap[codeHashGenesisNftIncomeKey],
						common.ConcatBytesOptimized([]string{
							out.NftAddress,
							strconv.FormatUint(out.TokenIndex, 10),
							tx.ID,
							strconv.Itoa(int(out.Index)),
							out.Value,
							strconv.FormatUint(out.TokenSupply, 10),
							out.MetaTxId,
							strconv.FormatUint(out.MetaOutputIndex, 10),
							strconv.FormatInt(out.Height, 10),
						}, "@"))

					if out.SensibleId != "000000000000000000000000000000000000000000000000000000000000000000000000" && out.MetaTxId != "0000000000000000000000000000000000000000000000000000000000000000" {
						// Process address history storage
						// key: address, value: txId@time@income/outcome@blockHeight
						addressTxTimeKey := common.ConcatBytesOptimized([]string{out.NftAddress}, "@")
						if _, exists := addressTxTimeMap[addressTxTimeKey]; !exists {
							addressTxTimeMap[addressTxTimeKey] = make([]string, 0, 2)
						}
						addressTxTimeMap[addressTxTimeKey] = append(addressTxTimeMap[addressTxTimeKey], common.ConcatBytesOptimized([]string{tx.ID, strconv.FormatInt(tx.Timestamp, 10), "income", strconv.FormatInt(int64(block.Height), 10)}, "@"))

						// Process genesis history storage
						// key: codeHash@genesis, value: txId@time@income/outcome@blockHeight
						genesisTxTimeKey := common.ConcatBytesOptimized([]string{out.CodeHash, out.Genesis}, "@")
						if _, exists := genesisTxTimeMap[genesisTxTimeKey]; !exists {
							genesisTxTimeMap[genesisTxTimeKey] = make([]string, 0, 2)
						}
						genesisTxTimeMap[genesisTxTimeKey] = append(genesisTxTimeMap[genesisTxTimeKey], common.ConcatBytesOptimized([]string{tx.ID, strconv.FormatInt(tx.Timestamp, 10), "income", strconv.FormatInt(int64(block.Height), 10)}, "@"))
					}

					if out.SensibleId != "000000000000000000000000000000000000000000000000000000000000000000000000" && out.MetaTxId != "0000000000000000000000000000000000000000000000000000000000000000" {
						// Process contract owners income storage
						// key: codeHash@genesis, value: address@tokenIndex@txId@index,...
						contractNftOwnersIncomeKey := common.ConcatBytesOptimized([]string{out.CodeHash, out.Genesis}, "@")
						if _, exists := contractNftOwnersIncomeMap[contractNftOwnersIncomeKey]; !exists {
							contractNftOwnersIncomeMap[contractNftOwnersIncomeKey] = make([]string, 0, 2)
						}
						contractNftOwnersIncomeMap[contractNftOwnersIncomeKey] = append(contractNftOwnersIncomeMap[contractNftOwnersIncomeKey],
							common.ConcatBytesOptimized([]string{
								out.NftAddress,
								strconv.FormatUint(out.TokenIndex, 10),
								tx.ID,
								strconv.Itoa(int(out.Index)),
							}, "@"))
					}

					// Process unchecked NFT contract Utxo storage
					// key: outpoint, value: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height
					outpoint := common.ConcatBytesOptimized([]string{tx.ID, strconv.Itoa(int(out.Index))}, ":")
					if _, exists := uncheckNftOutpointMap[outpoint]; !exists {
						uncheckNftOutpointMap[outpoint] = common.ConcatBytesOptimized(
							[]string{
								out.NftAddress,
								out.CodeHash,
								out.Genesis,
								out.SensibleId,
								strconv.FormatUint(out.TokenIndex, 10),
								tx.ID,
								strconv.Itoa(int(out.Index)),
								out.Value,
								strconv.FormatUint(out.TokenSupply, 10),
								out.MetaTxId,
								strconv.FormatUint(out.MetaOutputIndex, 10),
								strconv.FormatInt(out.Height, 10),
							}, "@")
					}

				} else if out.ContractType == "nft_sell" {
					hasNftSell = true
					contractAddress := out.ContractAddress
					nftAddress := out.NftAddress
					// Process address Sell NFT UTXO storage
					// NftAddress, value: codeHash@genesis@TokenIndex@Price@ContractAddress@TxID@Index@Value@height,...
					addressSellNftIncomeMap[nftAddress] = append(addressSellNftIncomeMap[nftAddress],
						common.ConcatBytesOptimized(
							[]string{
								out.CodeHash,
								out.Genesis,
								strconv.FormatUint(out.TokenIndex, 10),
								strconv.FormatUint(out.Price, 10),
								contractAddress,
								tx.ID,
								strconv.Itoa(int(out.Index)),
								out.Value,
								strconv.FormatInt(out.Height, 10),
							}, "@"))

					// Process codeHash@genesis Sell NFT UTXO storage
					// codeHash@genesis, value: NftAddress@TokenIndex@Price@ContractAddress@TxID@Index@Value@height,...
					codeHashGenesisSellNftIncomeKey := common.ConcatBytesOptimized([]string{out.CodeHash, out.Genesis}, "@")
					if _, exists := codeHashGenesisSellNftIncomeMap[codeHashGenesisSellNftIncomeKey]; !exists {
						codeHashGenesisSellNftIncomeMap[codeHashGenesisSellNftIncomeKey] = make([]string, 0, 4)
					}
					codeHashGenesisSellNftIncomeMap[codeHashGenesisSellNftIncomeKey] = append(codeHashGenesisSellNftIncomeMap[codeHashGenesisSellNftIncomeKey],
						common.ConcatBytesOptimized([]string{
							nftAddress,
							strconv.FormatUint(out.TokenIndex, 10),
							strconv.FormatUint(out.Price, 10),
							contractAddress,
							tx.ID,
							strconv.Itoa(int(out.Index)),
							out.Value,
							strconv.FormatInt(out.Height, 10),
						}, "@"))
				} else {
					continue
				}

			}
		}

		if err := i.contractNftUtxoStore.BulkMergeMapConcurrent(&contractNftUtxoMap, workers); err != nil {
			return err
		}
		if hasNft {
			// Batch process various storages
			if err := i.addressNftIncomeStore.BulkMergeMapConcurrent(&addressNftUtxoMap, workers); err != nil {
				return err
			}

			if err := i.codeHashGenesisNftIncomeStore.BulkMergeMapConcurrent(&codeHashGenesisNftIncomeMap, workers); err != nil {
				return err
			}

			if err := i.contractNftOwnersIncomeStore.BulkMergeMapConcurrent(&contractNftOwnersIncomeMap, workers); err != nil {
				return err
			}

			if err := i.contractNftAddressHistoryStore.BulkMergeMapConcurrent(&addressTxTimeMap, workers); err != nil {
				return err
			}

			if err := i.contractNftGenesisHistoryStore.BulkMergeMapConcurrent(&genesisTxTimeMap, workers); err != nil {
				return err
			}

			if err := i.contractNftInfoStore.BulkWriteConcurrent(&nftInfoMap, workers); err != nil {
				return err
			}

			if err := i.contractNftSummaryInfoStore.BulkWriteConcurrent(&contractSummaryInfoMap, workers); err != nil {
				return err
			}

			if err := i.contractNftGenesisStore.BulkWriteConcurrent(&genesisMap, workers); err != nil {
				return err
			}

			if err := i.contractNftGenesisUtxoStore.BulkWriteConcurrent(&genesisUtxoMap, workers); err != nil {
				return err
			}

			if err := i.uncheckNftOutpointStore.BulkWriteConcurrent(&uncheckNftOutpointMap, workers); err != nil {
				return err
			}
		}

		if hasNftSell {
			if err := i.addressSellNftIncomeStore.BulkMergeMapConcurrent(&addressSellNftIncomeMap, workers); err != nil {
				return err
			}
			if err := i.codeHashGenesisSellNftIncomeStore.BulkMergeMapConcurrent(&codeHashGenesisSellNftIncomeMap, workers); err != nil {
				return err
			}
		}

		// Clean up memory
		for k := range contractNftUtxoMap {
			delete(contractNftUtxoMap, k)
		}
		for k := range addressNftUtxoMap {
			delete(addressNftUtxoMap, k)
		}
		for k := range codeHashGenesisNftIncomeMap {
			delete(codeHashGenesisNftIncomeMap, k)
		}
		for k := range nftInfoMap {
			delete(nftInfoMap, k)
		}
		for k := range contractSummaryInfoMap {
			delete(contractSummaryInfoMap, k)
		}
		for k := range addressTxTimeMap {
			delete(addressTxTimeMap, k)
		}
		for k := range genesisTxTimeMap {
			delete(genesisTxTimeMap, k)
		}
		for k := range genesisMap {
			delete(genesisMap, k)
		}
		for k := range genesisUtxoMap {
			delete(genesisUtxoMap, k)
		}
		for k := range addressSellNftIncomeMap {
			delete(addressSellNftIncomeMap, k)
		}
		for k := range codeHashGenesisSellNftIncomeMap {
			delete(codeHashGenesisSellNftIncomeMap, k)
		}
		for k := range uncheckNftOutpointMap {
			delete(uncheckNftOutpointMap, k)
		}
		for k := range contractNftOwnersIncomeMap {
			delete(contractNftOwnersIncomeMap, k)
		}
		contractNftUtxoMap = nil
		addressNftUtxoMap = nil
		codeHashGenesisNftIncomeMap = nil
		nftInfoMap = nil
		contractSummaryInfoMap = nil
		addressTxTimeMap = nil
		genesisTxTimeMap = nil
		genesisMap = nil
		genesisUtxoMap = nil
		addressSellNftIncomeMap = nil
		codeHashGenesisSellNftIncomeMap = nil
		uncheckNftOutpointMap = nil
		contractNftOwnersIncomeMap = nil

	}

	return nil
}

func (i *ContractNftIndexer) processContractNftInputs(block *ContractNftBlock) error {
	var allTxPoints []string
	var txPointUsedMap = make(map[string]string)
	// Query if all input points exist in contractNftGenesisUtxoStore
	var usedGenesisUtxoMap = make(map[string]string)

	// First collect all input points
	for _, tx := range block.Transactions {
		for _, in := range tx.Inputs {
			allTxPoints = append(allTxPoints, in.TxPoint)
			txPointUsedMap[in.TxPoint] = tx.ID
		}
	}

	for _, txPoint := range allTxPoints {
		value, err := i.contractNftGenesisUtxoStore.Get([]byte(txPoint))
		if err == nil {
			// Found genesis UTXO, record transaction outpoint
			usedGenesisUtxoMap[txPoint] = string(value)
		}
	}

	totalPoints := len(allTxPoints)
	batchCount := (totalPoints + batchSize - 1) / batchSize

	for batchIndex := 0; batchIndex < batchCount; batchIndex++ {
		start := batchIndex * batchSize
		end := start + batchSize
		if end > totalPoints {
			end = totalPoints
		}

		batchPoints := allTxPoints[start:end]
		addressNftResult, codeHashGenesisNftSpendResult, addressSellNftSpendResult, codeHashGenesisSellNftSpendResult, err := i.contractNftUtxoStore.QueryNftUTXOAddresses(&batchPoints, workers, txPointUsedMap)
		if err != nil {
			return err
		}

		contractNftOwnersSpendMap := make(map[string][]string)
		addressTxTimeMap := make(map[string][]string)
		genesisTxTimeMap := make(map[string][]string)
		for k, vList := range addressNftResult {
			for _, v := range vList {
				//k: NftAddress
				//v: txid@index@codeHash@genesis@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId
				vStrs := strings.Split(v, "@")
				if len(vStrs) != 12 {
					fmt.Println("Processing addressNftResult invalid vStrs: ", vStrs)
					continue
				}

				// Process contract owners spend storage
				// key: codeHash@genesis, value: address@tokenIndex@txId@index,...
				contractNftOwnersSpendKey := common.ConcatBytesOptimized([]string{vStrs[2], vStrs[3]}, "@")
				if _, exists := contractNftOwnersSpendMap[contractNftOwnersSpendKey]; !exists {
					contractNftOwnersSpendMap[contractNftOwnersSpendKey] = make([]string, 0)
				}
				contractNftOwnersSpendMap[contractNftOwnersSpendKey] = append(contractNftOwnersSpendMap[contractNftOwnersSpendKey],
					common.ConcatBytesOptimized([]string{
						k,
						vStrs[5],
						vStrs[0],
						vStrs[1],
					}, "@"))

				usedTxId := vStrs[11]
				if usedTxId != "" {
					// Process address history storage
					// key: address, value: txId@time@income/outcome@blockHeight
					addressTxTimeKey := common.ConcatBytesOptimized([]string{k}, "@")
					if _, exists := addressTxTimeMap[addressTxTimeKey]; !exists {
						addressTxTimeMap[addressTxTimeKey] = make([]string, 0, 2)
					}
					addressTxTimeMap[addressTxTimeKey] = append(addressTxTimeMap[addressTxTimeKey], common.ConcatBytesOptimized([]string{usedTxId, strconv.FormatInt(block.Timestamp, 10), "outcome", strconv.FormatInt(int64(block.Height), 10)}, "@"))

					// Process genesis history storage
					// key: codeHash@genesis, value: txId@time@income/outcome@blockHeight
					genesisTxTimeKey := common.ConcatBytesOptimized([]string{vStrs[2], vStrs[3]}, "@")
					if _, exists := genesisTxTimeMap[genesisTxTimeKey]; !exists {
						genesisTxTimeMap[genesisTxTimeKey] = make([]string, 0, 2)
					}
					genesisTxTimeMap[genesisTxTimeKey] = append(genesisTxTimeMap[genesisTxTimeKey], common.ConcatBytesOptimized([]string{usedTxId, strconv.FormatInt(block.Timestamp, 10), "outcome", strconv.FormatInt(int64(block.Height), 10)}, "@"))
				}

			}
		}

		//Process addressNftSpendStore
		if err := i.addressNftSpendStore.BulkMergeMapConcurrent(&addressNftResult, workers); err != nil {
			return err
		}

		if err := i.contractNftOwnersSpendStore.BulkMergeMapConcurrent(&contractNftOwnersSpendMap, workers); err != nil {
			return err
		}

		//Process codeHashGenesisNftSpendStore
		if err := i.codeHashGenesisNftSpendStore.BulkMergeMapConcurrent(&codeHashGenesisNftSpendResult, workers); err != nil {
			return err
		}

		if err := i.contractNftAddressHistoryStore.BulkMergeMapConcurrent(&addressTxTimeMap, workers); err != nil {
			return err
		}
		if err := i.contractNftGenesisHistoryStore.BulkMergeMapConcurrent(&genesisTxTimeMap, workers); err != nil {
			return err
		}

		//Process sellNftSpendStore
		if err := i.addressSellNftSpendStore.BulkMergeMapConcurrent(&addressSellNftSpendResult, workers); err != nil {
			return err
		}

		//Process codeHashGenesisSellNftSpendStore
		if err := i.codeHashGenesisSellNftSpendStore.BulkMergeMapConcurrent(&codeHashGenesisSellNftSpendResult, workers); err != nil {
			return err
		}

		//Process usedNftIncomeStore
		usedNftIncomeMap := make(map[string][]string)

		for k, vList := range addressNftResult {
			for _, v := range vList {
				//k: NftAddress
				//v: txid@index@codeHash@genesis@sensibleId@tokenIndex@value@TokenSupply@MetaTxId@MetaOutputIndex@height@usedTxId
				vStrs := strings.Split(v, "@")
				if len(vStrs) != 12 {
					fmt.Println("Processing addressNftResult invalid vStrs: ", vStrs)
					continue
				}
				//newKey: usedTxId
				//newValue: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height,...
				usedTxId := vStrs[11]
				if _, exists := usedNftIncomeMap[usedTxId]; !exists {
					usedNftIncomeMap[usedTxId] = make([]string, 0)
				}
				newValue := common.ConcatBytesOptimized([]string{
					k,
					vStrs[2],
					vStrs[3],
					vStrs[4],
					vStrs[5],
					vStrs[0],
					vStrs[1],
					vStrs[6],
					vStrs[7],
					vStrs[8],
					vStrs[9],
					vStrs[10],
				}, "@")
				usedNftIncomeMap[usedTxId] = append(usedNftIncomeMap[usedTxId], newValue)
			}
		}
		if err := i.usedNftIncomeStore.BulkMergeMapConcurrent(&usedNftIncomeMap, workers); err != nil {
			return err
		}

		// Process genesis UTXO consumption
		if len(usedGenesisUtxoMap) > 0 {
			genesisSpendMap := make(map[string]string)
			for txPoint, _ := range usedGenesisUtxoMap {
				// Get original UTXO info and add @IsSpent flag
				originalValue := usedGenesisUtxoMap[txPoint]
				genesisSpendMap[txPoint] = originalValue + "@1" // Add @IsSpent flag
			}
			if err := i.contractNftGenesisUtxoStore.BulkWriteConcurrent(&genesisSpendMap, workers); err != nil {
				return err
			}
		}

		if len(allTxPoints) > 0 {
			genesisOutputMap := make(map[string]string)
			for _, usedTxPoint := range allTxPoints {
				txID := txPointUsedMap[usedTxPoint]

				var tx *ContractNftTransaction
				for _, v := range block.Transactions {
					if v.ID == txID {
						tx = v
						break
					}
				}
				if tx == nil {
					continue
				}

				// Collect all output information for this transaction
				var outputs []string

				for _, out := range tx.Outputs {

					if out.ContractType != "nft" && out.ContractType != "nft_sell" {
						continue
					}

					//key: usedOutpoint, value: sensibleId@tokenSupply@codeHash@genesis@tokenIndex@txId@index@value@MetaTxId@MetaOutputIndex,...
					outputInfo := common.ConcatBytesOptimized([]string{
						out.SensibleId,
						strconv.FormatUint(out.TokenSupply, 10),
						out.CodeHash,
						out.Genesis,
						strconv.FormatUint(out.TokenIndex, 10),
						txID,
						strconv.Itoa(int(out.Index)),
						out.Value,
						out.MetaTxId,
						strconv.FormatUint(out.MetaOutputIndex, 10),
					}, "@")
					outputs = append(outputs, outputInfo)
				}
				if len(outputs) > 0 {
					genesisOutputMap[usedTxPoint] = strings.Join(outputs, ",")
				}
			}
			if len(genesisOutputMap) > 0 {
				if err := i.contractNftGenesisOutputStore.BulkWriteConcurrent(&genesisOutputMap, workers); err != nil {
					return err
				}
				for k := range genesisOutputMap {
					delete(genesisOutputMap, k)
				}
				genesisOutputMap = nil
			}
		}

		for k := range addressNftResult {
			delete(addressNftResult, k)
		}
		for k := range addressTxTimeMap {
			delete(addressTxTimeMap, k)
		}
		for k := range genesisTxTimeMap {
			delete(genesisTxTimeMap, k)
		}
		for k := range addressSellNftSpendResult {
			delete(addressSellNftSpendResult, k)
		}
		for k := range codeHashGenesisSellNftSpendResult {
			delete(codeHashGenesisSellNftSpendResult, k)
		}
		for k := range usedNftIncomeMap {
			delete(usedNftIncomeMap, k)
		}
		for k := range contractNftOwnersSpendMap {
			delete(contractNftOwnersSpendMap, k)
		}

		addressNftResult = nil
		addressTxTimeMap = nil
		genesisTxTimeMap = nil
		addressSellNftSpendResult = nil
		codeHashGenesisSellNftSpendResult = nil
		usedNftIncomeMap = nil
		batchPoints = nil
		contractNftOwnersSpendMap = nil
	}

	for k := range txPointUsedMap {
		delete(txPointUsedMap, k)
	}
	txPointUsedMap = nil
	allTxPoints = nil
	usedGenesisUtxoMap = nil
	return nil
}

func (i *ContractNftIndexer) GetLastIndexedHeight() (int, error) {
	heightBytes, err := i.metaStore.Get([]byte(common.MetaStoreKeyLastNftIndexedHeight))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Println("No previous height found, starting from genesis")
			return 0, nil
		}
		log.Printf("Error reading last height: %v", err)
		return 0, err
	}

	height, err := strconv.Atoi(string(heightBytes))
	if err != nil {
		log.Printf("Invalid height format: %s, error: %v", heightBytes, err)
		return 0, fmt.Errorf("invalid height format: %w", err)
	}

	return height, nil
}

type ContractNftBlock struct {
	Height             int                             `json:"height"`
	Timestamp          int64                           `json:"timestamp"`
	Transactions       []*ContractNftTransaction       `json:"transactions"`
	ContractNftOutputs map[string][]*ContractNftOutput `json:"contract_outputs"`
	IsPartialBlock     bool                            `json:"-"`
}

func (b *ContractNftBlock) Validate() error {
	if b.Height < 0 {
		return fmt.Errorf("invalid block height: %d", b.Height)
	}
	return nil
}

type ContractNftTransaction struct {
	ID        string
	Inputs    []*ContractNftInput
	Outputs   []*ContractNftOutput
	Timestamp int64 // timestamp in milliseconds
}

type ContractNftOutput struct {
	Address string
	Value   string
	Index   int64
	Height  int64

	ContractType string // nft, nft_sell
	//NftInfo
	CodeHash        string
	Genesis         string
	SensibleId      string
	TokenIndex      uint64
	TokenSupply     uint64
	NftAddress      string
	MetaTxId        string
	MetaOutputIndex uint64

	// NFT Sell Info
	ContractAddress string
	Price           uint64
}

type ContractNftInput struct {
	TxPoint string
}

// GetUtxoStore returns UTXO storage object
func (i *ContractNftIndexer) GetContractNftUtxoStore() *storage.PebbleStore {
	return i.contractNftUtxoStore
}

func (i *ContractNftIndexer) GetContractNftInfoStore() *storage.PebbleStore {
	return i.contractNftInfoStore
}

func (i *ContractNftIndexer) GetContractNftGenesisStore() *storage.PebbleStore {
	return i.contractNftGenesisStore
}

func (i *ContractNftIndexer) GetContractNftGenesisOutputStore() *storage.PebbleStore {
	return i.contractNftGenesisOutputStore
}

func (i *ContractNftIndexer) GetContractNftGenesisUtxoStore() *storage.PebbleStore {
	return i.contractNftGenesisUtxoStore
}

// SetMempoolManager sets mempool manager
func (i *ContractNftIndexer) SetMempoolManager(mempoolMgr NftMempoolManager) {
	i.mempoolMgr = mempoolMgr
}

// GetInvalidNftOutpointStore returns invalid NFT contract UTXO storage object
func (i *ContractNftIndexer) GetInvalidNftOutpointStore() *storage.PebbleStore {
	return i.invalidNftOutpointStore
}

// QueryInvalidNftOutpoint queries invalid NFT contract UTXO data
// outpoint: transaction output point, format is "txID:index"
// returns: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@height@reason
func (i *ContractNftIndexer) QueryInvalidNftOutpoint(outpoint string) (string, error) {
	value, err := i.invalidNftOutpointStore.Get([]byte(outpoint))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("failed to query invalid NFT contract UTXO: %w", err)
	}
	return string(value), nil
}
