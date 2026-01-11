package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"gopkg.in/yaml.v3"
)

// 支持的链类型常量
const (
	ChainBTC  = "btc"
	ChainMVC  = "mvc"
	ChainDOGE = "doge"
)

type RPCConfig struct {
	Chain    string `yaml:"chain"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

var GlobalConfig *Config
var GlobalNetwork *chaincfg.Params

type Config struct {
	Chain                   string    `yaml:"chain"` // 新增: 链类型标识
	Network                 string    `yaml:"network"`
	DataDir                 string    `yaml:"data_dir"`
	BlockInfoIndexer        bool      `yaml:"block_info_indexer"`
	BlockFilesEnabled       bool      `yaml:"block_files_enabled"` // 是否启用区块归档文件，关闭可提升索引速度
	BlockFilesDir           string    `yaml:"block_files_dir"`
	BackupDir               string    `yaml:"backup_dir"`
	ShardCount              int       `yaml:"shard_count"`
	BatchSize               int       `yaml:"batch_size"`
	OnceTxCount             int       `yaml:"once_tx_count"`
	TxConcurrency           int       `yaml:"tx_concurrency"`
	Workers                 int       `yaml:"workers"`
	MemUTXOMaxCount         int       `yaml:"mem_utxo_max_count"` // Memory UTXO cache size
	CPUCores                int       `yaml:"cpu_cores"`
	MemoryGB                int       `yaml:"memory_gb"`
	HighPerf                bool      `yaml:"high_perf"`
	APIPort                 string    `yaml:"api_port"`
	ZMQAddress              []string  `yaml:"zmq_address"`
	ZmqReconnectInterval    int       `yaml:"zmq_reconnect_interval"`
	MemPoolCleanStartHeight int       `yaml:"mempool_clean_start_height"` // 已废弃: 现在自动判断，仅保留向后兼容
	MaxTxPerBatch           int       `yaml:"max_tx_per_batch"`
	RPC                     RPCConfig `yaml:"rpc"`
}

func (c *Config) GetChainParams() (*chaincfg.Params, error) {
	// 对于 DOGE 链，返回 nil，让调用者使用 adapter 的 GetChainParams()
	// 或者使用专门的 DOGE 参数
	if c.Chain == ChainDOGE {
		// DOGE 使用自定义参数，在 blockchain/adapter_doge.go 中定义
		// 这里返回一个占位符，实际使用时应该用 adapter.GetChainParams()
		return &chaincfg.Params{
			Name:             "dogecoin-mainnet",
			PubKeyHashAddrID: 0x1e, // 'D' addresses
			ScriptHashAddrID: 0x16, // '9' or 'A' addresses
		}, nil
	}

	switch c.Network {
	case "mainnet":
		return &chaincfg.MainNetParams, nil
	case "testnet":
		return &chaincfg.TestNet3Params, nil
	case "regtest":
		return &chaincfg.RegressionNetParams, nil
	default:
		return nil, fmt.Errorf("unknown network: %s", c.Network)
	}
}

// ValidateChain 验证链配置
func (c *Config) ValidateChain() error {
	if c.Chain == "" {
		return fmt.Errorf("chain field is required")
	}

	supportedChains := map[string]bool{
		ChainBTC:  true,
		ChainMVC:  true,
		ChainDOGE: true,
	}

	if !supportedChains[c.Chain] {
		return fmt.Errorf("unsupported chain: %s, supported chains: btc, mvc, doge", c.Chain)
	}

	if c.Chain != c.RPC.Chain {
		return fmt.Errorf("chain mismatch: config.chain=%s but rpc.chain=%s", c.Chain, c.RPC.Chain)
	}

	return nil
}

// GetChainName 获取链名称
func (c *Config) GetChainName() string {
	if c.Chain != "" {
		return c.Chain
	}
	if c.RPC.Chain != "" {
		return c.RPC.Chain
	}
	return ChainBTC
}

func LoadConfig(path string) (*Config, error) {
	configFlag := flag.String("config", "", "path to config file")
	flag.Parse()
	// Default config
	cfg := &Config{
		Chain:                   ChainBTC, // 默认 BTC
		Network:                 "testnet",
		DataDir:                 "data",
		BackupDir:               "data/backups",
		ShardCount:              16,
		APIPort:                 "8080",
		ZMQAddress:              []string{"tcp://localhost:28332"},
		MemPoolCleanStartHeight: 0,    // 已废弃: 自动判断最新区块时才清理
		MaxTxPerBatch:           3000, // Default: process up to 3000 transactions per batch
		RPC: RPCConfig{
			Chain: ChainBTC, // 默认 BTC
			Host:  "localhost",
			Port:  "8332",
		},
		ZmqReconnectInterval: 5,
	}

	// Try to load from config file
	configPath := *configFlag
	if configPath == "" {
		configPath = path
	}
	fmt.Println("configPath", configPath)

	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables if set
	if chain := os.Getenv("CHAIN"); chain != "" {
		cfg.Chain = chain
	}
	if network := os.Getenv("NETWORK"); network != "" {
		cfg.Network = network
	}
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		cfg.DataDir = dir
	}
	if backupDir := os.Getenv("BACKUP_DIR"); backupDir != "" {
		cfg.BackupDir = backupDir
	}
	if user := os.Getenv("RPC_USER"); user != "" {
		cfg.RPC.User = user
	}
	if pass := os.Getenv("RPC_PASS"); pass != "" {
		cfg.RPC.Password = pass
	}
	if host := os.Getenv("RPC_HOST"); host != "" {
		cfg.RPC.Host = host
	}
	if port := os.Getenv("RPC_PORT"); port != "" {
		cfg.RPC.Port = port
	}
	if zmq := os.Getenv("ZMQ_ADDRESS"); zmq != "" {
		cfg.ZMQAddress = strings.Split(zmq, ",")
	}
	if startHeight := os.Getenv("MEMPOOL_CLEAN_START_HEIGHT"); startHeight != "" {
		height, err := strconv.Atoi(startHeight)
		if err == nil && height >= 0 {
			cfg.MemPoolCleanStartHeight = height
		}
	}
	// if maxTxPerBatch := os.Getenv("MAX_TX_PER_BATCH"); maxTxPerBatch != "" {
	// 	val, err := strconv.Atoi(maxTxPerBatch)
	// 	if err == nil && val > 0 {
	// 		cfg.MaxTxPerBatch = val
	// 	}
	// }

	// 验证链配置
	if err := cfg.ValidateChain(); err != nil {
		return nil, fmt.Errorf("chain configuration validation failed: %w", err)
	}

	// 输出链信息
	fmt.Printf("Initialized for chain: %s, network: %s\n", cfg.GetChainName(), cfg.Network)
	fmt.Printf("Data directory: %s\n", cfg.DataDir)

	// Ensure data dir exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	GlobalConfig = cfg
	return cfg, nil
}
