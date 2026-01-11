package decoder

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"golang.org/x/crypto/ripemd160"

	scriptDecoder "github.com/mvc-labs/metacontract-script-decoder"
)

type NFTUtxoInfo struct {
	CodeType   uint32
	CodeHash   string
	GenesisId  string
	Genesis    string
	SensibleId string // GenesisTx outpoint

	MetaTxId        string // nft metatxid
	MetaOutputIndex uint64
	TokenIndex      uint64 // nft tokenIndex
	TokenSupply     uint64 // nft tokenSupply

	Address string
}

type NFTSellUtxoInfo struct {
	CodeHash        string
	Genesis         string
	GenesisId       string
	ContractAddress string
	TokenIndex      uint64 // nft tokenIndex
	Price           uint64 // nft price
	Address         string
}

// IsNFTContract checks if it's an NFT contract
func IsNFTContract(script []byte) bool {
	txoData := &scriptDecoder.TxoData{}
	isValid := scriptDecoder.DecodeMvcTxo(script, txoData)
	if !isValid {
		return false
	}
	return txoData.CodeType == scriptDecoder.CodeType_NFT
}

// IsNftSellContract checks if it's an NFT sell contract
func IsNftSellContract(script []byte) bool {
	txoData := &scriptDecoder.TxoData{}
	isValid := scriptDecoder.DecodeMvcTxo(script, txoData)
	if !isValid {
		return false
	}
	return txoData.CodeType == scriptDecoder.CodeType_NFT_SELL
}

// IsNFTSellContract checks if it's an NFTSell contract
func IsNFTSellContract(script []byte) bool {
	txoData := &scriptDecoder.TxoData{}
	isValid := scriptDecoder.DecodeMvcTxo(script, txoData)
	if !isValid {
		return false
	}
	return txoData.CodeType == scriptDecoder.CodeType_NFT_SELL
}

func ExtractNFTUtxoInfo(script []byte, param *chaincfg.Params) (*NFTUtxoInfo, error) {
	txoData, err := ExtractNFTInfo(script)
	if err != nil {
		return nil, err
	}
	if txoData == nil {
		return nil, nil
	}
	// Verify if FT data exists
	if txoData.NFT == nil {
		return nil, nil
	}

	nftUtxoInfo := &NFTUtxoInfo{
		CodeType:        txoData.CodeType,
		CodeHash:        hex.EncodeToString(txoData.CodeHash[:]),
		GenesisId:       hex.EncodeToString(txoData.GenesisId[:]),
		SensibleId:      hex.EncodeToString(txoData.NFT.SensibleId),
		MetaTxId:        hex.EncodeToString(txoData.NFT.MetaTxId[:32]),
		MetaOutputIndex: uint64(txoData.NFT.MetaOutputIndex),
		TokenIndex:      txoData.NFT.TokenIndex,
		TokenSupply:     txoData.NFT.TokenSupply,
	}
	if txoData.GenesisIdLen == 40 {
		// If GenesisIdLen is 40, the last 20 bytes are genesis
		nftUtxoInfo.Genesis = hex.EncodeToString(txoData.GenesisId[20:])
	} else if txoData.GenesisIdLen == 20 {
		// If GenesisIdLen is 20, the entire GenesisId is genesis
		nftUtxoInfo.Genesis = hex.EncodeToString(txoData.GenesisId[:20])
	}

	// If address information exists, add to result
	if txoData.HasAddress {
		address, err := PkhToAddress(hex.EncodeToString(txoData.AddressPkh[:]), param)
		if err != nil {
			return nil, err
		}
		nftUtxoInfo.Address = address
	}

	return nftUtxoInfo, nil
}

func ExtractNFTInfo(script []byte) (*scriptDecoder.TxoData, error) {
	txoData := &scriptDecoder.TxoData{}
	isValid := scriptDecoder.DecodeMvcTxo(script, txoData)
	if !isValid || txoData.CodeType != scriptDecoder.CodeType_NFT {
		return nil, nil
	}
	return txoData, nil
}

// ExtractFTInfo extracts FT contract information
func ExtractNFTSellInfo(script []byte) (*scriptDecoder.TxoData, error) {
	txoData := &scriptDecoder.TxoData{}
	isValid := scriptDecoder.DecodeMvcTxo(script, txoData)
	if !isValid || txoData.CodeType != scriptDecoder.CodeType_NFT_SELL {
		return nil, nil
	}
	return txoData, nil
}

func ExtractNFTSellUtxoInfo(script []byte, param *chaincfg.Params) (*NFTSellUtxoInfo, error) {
	txoData, err := ExtractNFTSellInfo(script)
	if err != nil {
		return nil, err
	}
	if txoData == nil {
		return nil, nil
	}
	// Verify if FT data exists
	if txoData.NFTSell == nil {
		return nil, nil
	}

	nftsellUtxoInfo := &NFTSellUtxoInfo{
		CodeHash:   hex.EncodeToString(txoData.CodeHash[:]),
		GenesisId:  hex.EncodeToString(txoData.GenesisId[:]),
		TokenIndex: txoData.NFTSell.TokenIndex,
		Price:      txoData.NFTSell.Price,
	}

	if txoData.GenesisIdLen == 40 {
		// If GenesisIdLen is 40, the last 20 bytes are genesis
		nftsellUtxoInfo.Genesis = hex.EncodeToString(txoData.GenesisId[20:])
	} else if txoData.GenesisIdLen == 20 {
		// If GenesisIdLen is 20, the entire GenesisId is genesis
		nftsellUtxoInfo.Genesis = hex.EncodeToString(txoData.GenesisId[:20])
	}

	contractAddress, _ := Hash160ToAddress(script, param)
	nftsellUtxoInfo.ContractAddress = contractAddress

	// If address information exists, add to result
	if txoData.HasAddress {
		address, err := PkhToAddress(hex.EncodeToString(txoData.AddressPkh[:]), param)
		if err != nil {
			return nil, err
		}
		nftsellUtxoInfo.Address = address
	}

	return nftsellUtxoInfo, nil
}

// Hash160ToAddress performs hash160 on script and converts to address
// hash160 = RIPEMD160(SHA256(script))
func Hash160ToAddress(script []byte, params *chaincfg.Params) (string, error) {
	// Step 1: SHA256 hash
	sha := sha256.Sum256(script)

	// Step 2: RIPEMD160 hash
	ripemd := ripemd160.New()
	_, err := ripemd.Write(sha[:])
	if err != nil {
		return "", err
	}
	hash160 := ripemd.Sum(nil)

	// Step 3: Convert to address
	addr, err := btcutil.NewAddressPubKeyHash(hash160, params)
	if err != nil {
		return "", err
	}

	return addr.String(), nil
}
