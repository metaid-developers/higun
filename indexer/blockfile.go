// 区块预处理数据归档和使用
package indexer

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/klauspost/compress/zstd"
	"github.com/metaid/utxo_indexer/config"
	"google.golang.org/protobuf/proto"
)

// GetBlockFilePath 根据区块高度计算存储路径
// 采用 /百万位/千位/高度.dat.zst 的结构
func GetBlockFilePath(height int64, partType string, partIndex int) string {
	million := height / 1000000
	thousand := (height % 1000000) / 1000
	lastName := strconv.FormatInt(height, 10) + ".dat.zst"
	if partType != "" {
		lastName = strconv.FormatInt(height, 10) + "_" + partType + ".dat.zst"
	}
	if partIndex >= 0 {
		lastName = strconv.FormatInt(height, 10) + "_" + partType + "_" + strconv.Itoa(partIndex) + ".dat.zst"
	}
	return filepath.Join(
		config.GlobalConfig.DataDir+"/blockFiles",
		strconv.FormatInt(million, 10),
		strconv.FormatInt(thousand, 10),
		lastName,
	)
}

// SaveBlock 将一个区块序列化、压缩并保存到文件
func SaveFBlockPart(block *FBlock, partType string, partIndex int) error {
	// 1. 使用 Protobuf 序列化
	data, err := proto.Marshal(block)
	if err != nil {
		return fmt.Errorf("序列化区块 %d 失败: %w", block.Height, err)
	}

	// 2. 使用 Zstandard 压缩
	var compressedData []byte
	encoder, _ := zstd.NewWriter(nil)
	compressedData = encoder.EncodeAll(data, make([]byte, 0, len(data)))
	encoder.Close()
	// 3. 计算并创建存储路径
	filePath := GetBlockFilePath(int64(block.Height), partType, partIndex)
	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("创建目录 %s 失败: %w", dirPath, err)
	}

	// 4. 写入文件
	if err := os.WriteFile(filePath, compressedData, 0644); err != nil {
		return fmt.Errorf("写入文件 %s 失败: %w", filePath, err)
	}

	//log.Printf("成功保存区块 %d 到 %s (原始大小: %d, 压缩后: %d)", block.Height, filePath, len(data), len(compressedData))
	return nil
}

// LoadFBlock 从文件加载、解压并反序列化一个区块
func LoadFBlockPart(height int64, partType string, partIndex int) (*Block, error) {
	filePath := GetBlockFilePath(height, partType, partIndex)
	if _, err := os.Stat(filePath); err != nil {
		return nil, errors.New("noFile")
	}
	// 1. 读取压缩文件
	compressedData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件 %s 失败: %w", filePath, err)
	}

	// 2. 使用 Zstandard 解压
	decoder, _ := zstd.NewReader(nil)
	decompressedData, err := decoder.DecodeAll(compressedData, nil)
	if err != nil {
		return nil, fmt.Errorf("解压区块 %d 失败: %w", height, err)
	}
	decoder.Close()

	// 3. 使用 Protobuf 反序列化
	var fblock FBlock
	if err := proto.Unmarshal(decompressedData, &fblock); err != nil {
		return nil, fmt.Errorf("反序列化区块 %d 失败: %w", height, err)
	}
	log.Printf("成功从 %s 加载区块 %d", filePath, height)
	block := FBlockToBlock(&fblock)
	return block, nil
}

// Block -> FBlock
func BlockToFBlock(b *Block, fileType string) *FBlock {
	if b == nil {
		return nil
	}
	//fmt.Println(">>", len(b.UtxoData), len(b.IncomeData), len(b.SpendData))
	fb := &FBlock{
		Height:     int64(b.Height),
		BlockHash:  b.BlockHash,
		UtxoData:   make(map[string]*StringList),
		IncomeData: make(map[string]*StringList),
		SpendData:  make(map[string]*StringList),
	}
	if fileType == "utxo" {
		// UtxoData
		for k, v := range b.UtxoData {
			fb.UtxoData[k] = &StringList{Items: v}
		}
		// IncomeData
		for k, v := range b.IncomeData {
			fb.IncomeData[k] = &StringList{Items: v}
		}
	}
	if fileType == "spend" {
		// SpendData
		for k, v := range b.SpendData {
			fb.SpendData[k] = &StringList{Items: v}
		}
	}
	return fb
}

// FBlock -> Block
func FBlockToBlock(fb *FBlock) *Block {
	if fb == nil {
		return nil
	}
	b := &Block{
		Height:     int(fb.Height),
		BlockHash:  fb.BlockHash,
		UtxoData:   make(map[string][]string),
		IncomeData: make(map[string][]string),
		SpendData:  make(map[string][]string),
	}
	// UtxoData
	for k, v := range fb.UtxoData {
		if v != nil {
			b.UtxoData[k] = v.Items
		}
	}
	// IncomeData
	for k, v := range fb.IncomeData {
		if v != nil {
			b.IncomeData[k] = v.Items
		}
	}
	// SpendData
	for k, v := range fb.SpendData {
		if v != nil {
			b.SpendData[k] = v.Items
		}
	}
	return b
}
