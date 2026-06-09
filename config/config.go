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

type WalletGatewayConfig struct {
	Enabled        bool                                `yaml:"enabled"`
	TimeoutSeconds int                                 `yaml:"timeout_seconds"`
	Chains         map[string]WalletGatewayChainConfig `yaml:"chains"`
}

type WalletGatewayChainConfig struct {
	Enabled       bool                       `yaml:"enabled"`
	CoreURL       string                     `yaml:"core_url"`
	BroadcastPath string                     `yaml:"broadcast_path"`
	FeeRate       WalletGatewayFeeRateConfig `yaml:"fee_rate"`
}

type WalletGatewayFeeRateConfig struct {
	Unit    string `yaml:"unit"`
	Slow    int64  `yaml:"slow"`
	Normal  int64  `yaml:"normal"`
	Fast    int64  `yaml:"fast"`
	Default string `yaml:"default"`
}

var GlobalConfig *Config
var GlobalNetwork *chaincfg.Params

type Config struct {
	Chain                   string   `yaml:"chain"` // 新增: 链类型标识
	Network                 string   `yaml:"network"`
	DataDir                 string   `yaml:"data_dir"`
	BlockInfoIndexer        bool     `yaml:"block_info_indexer"`
	BlockFilesEnabled       bool     `yaml:"block_files_enabled"` // 是否启用区块归档文件，关闭可提升索引速度
	BlockFilesDir           string   `yaml:"block_files_dir"`
	BackupDir               string   `yaml:"backup_dir"`
	ShardCount              int      `yaml:"shard_count"`
	BatchSize               int      `yaml:"batch_size"`
	OnceTxCount             int      `yaml:"once_tx_count"`
	TxConcurrency           int      `yaml:"tx_concurrency"`
	Workers                 int      `yaml:"workers"`
	MemUTXOMaxCount         int      `yaml:"mem_utxo_max_count"` // Memory UTXO cache size
	CPUCores                int      `yaml:"cpu_cores"`
	MemoryGB                int      `yaml:"memory_gb"`
	HighPerf                bool     `yaml:"high_perf"`
	APIPort                 string   `yaml:"api_port"`
	ZMQAddress              []string `yaml:"zmq_address"`
	ZmqReconnectInterval    int      `yaml:"zmq_reconnect_interval"`
	MemPoolCleanStartHeight int      `yaml:"mempool_clean_start_height"` // 已废弃: 现在自动判断，仅保留向后兼容
	MaxTxPerBatch           int      `yaml:"max_tx_per_batch"`
	// 大区块处理策略:区块体积(字节)超过此阈值时，改用逐笔TX拉取模式，避免整块加载导致OOM
	// 默认 209715200 = 200MB；BTC/DOGE 永远不会触发（它们的块上限远小于此值）
	// 启用此功能要求节点已开启 txindex=1
	LargeBlockThresholdBytes int64 `yaml:"large_block_threshold_bytes"`
	// 大区块逐TX拉取时的并发 goroutine 数，默认 20
	LargeBlockFetchWorkers            int                 `yaml:"large_block_fetch_workers"`
	MVCTxIDAliasBackfillEnabled       bool                `yaml:"mvc_txid_alias_backfill_enabled"`
	MVCTxIDAliasBackfillBatchSize     int                 `yaml:"mvc_txid_alias_backfill_batch_size"`
	MVCTxIDAliasBackfillWorkers       int                 `yaml:"mvc_txid_alias_backfill_workers"`
	MVCTxIDAliasBackfillRetryAttempts int                 `yaml:"mvc_txid_alias_backfill_retry_attempts"`
	MVCTxIDAliasBackfillRetryDelayMS  int                 `yaml:"mvc_txid_alias_backfill_retry_delay_ms"`
	UTXOValidationEnabled             bool                `yaml:"utxo_validation_enabled"`
	UTXOValidationConcurrency         int                 `yaml:"utxo_validation_concurrency"`
	UTXOValidationRPCTimeoutSeconds   int                 `yaml:"utxo_validation_rpc_timeout_seconds"`
	RPC                               RPCConfig           `yaml:"rpc"`
	Wallet                            WalletGatewayConfig `yaml:"wallet"`
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
		Chain:                             ChainBTC, // 默认 BTC
		Network:                           "testnet",
		DataDir:                           "data",
		BackupDir:                         "data/backups",
		ShardCount:                        16,
		APIPort:                           "8080",
		ZMQAddress:                        []string{"tcp://localhost:28332"},
		MemPoolCleanStartHeight:           0,                 // 已废弃: 自动判断最新区块时才清理
		MaxTxPerBatch:                     3000,              // Default: process up to 3000 transactions per batch
		LargeBlockThresholdBytes:          200 * 1024 * 1024, // 200MB，超过此值改用逐TX拉取
		LargeBlockFetchWorkers:            20,                // 大块逐TX拉取并发数
		MVCTxIDAliasBackfillEnabled:       true,
		MVCTxIDAliasBackfillBatchSize:     1000,
		MVCTxIDAliasBackfillWorkers:       4,
		MVCTxIDAliasBackfillRetryAttempts: 3,
		MVCTxIDAliasBackfillRetryDelayMS:  1000,
		UTXOValidationEnabled:             true,
		UTXOValidationConcurrency:         8,
		UTXOValidationRPCTimeoutSeconds:   3,
		RPC: RPCConfig{
			Chain: ChainBTC, // 默认 BTC
			Host:  "localhost",
			Port:  "8332",
		},
		Wallet: WalletGatewayConfig{
			Enabled:        false,
			TimeoutSeconds: 10,
			Chains: map[string]WalletGatewayChainConfig{
				ChainBTC: {
					Enabled:       false,
					CoreURL:       "",
					BroadcastPath: "/btc/broadcast",
					FeeRate: WalletGatewayFeeRateConfig{
						Unit:    "sat_per_byte",
						Slow:    1,
						Normal:  3,
						Fast:    5,
						Default: "normal",
					},
				},
				ChainMVC: {
					Enabled:       false,
					CoreURL:       "",
					BroadcastPath: "/btc/broadcast",
					FeeRate: WalletGatewayFeeRateConfig{
						Unit:    "sat_per_byte",
						Slow:    1,
						Normal:  2,
						Fast:    3,
						Default: "normal",
					},
				},
				ChainDOGE: {
					Enabled:       false,
					CoreURL:       "",
					BroadcastPath: "/btc/broadcast",
					FeeRate: WalletGatewayFeeRateConfig{
						Unit:    "sat_per_byte",
						Slow:    1,
						Normal:  2,
						Fast:    5,
						Default: "normal",
					},
				},
			},
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
	if enabled := os.Getenv("UTXO_VALIDATION_ENABLED"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			cfg.UTXOValidationEnabled = val
		}
	}
	if concurrency := os.Getenv("UTXO_VALIDATION_CONCURRENCY"); concurrency != "" {
		val, err := strconv.Atoi(concurrency)
		if err == nil && val > 0 {
			cfg.UTXOValidationConcurrency = val
		}
	}
	if timeout := os.Getenv("UTXO_VALIDATION_RPC_TIMEOUT_SECONDS"); timeout != "" {
		val, err := strconv.Atoi(timeout)
		if err == nil && val > 0 {
			cfg.UTXOValidationRPCTimeoutSeconds = val
		}
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
	if enabled := os.Getenv("WALLET_GATEWAY_ENABLED"); enabled != "" {
		if val, err := strconv.ParseBool(enabled); err == nil {
			cfg.Wallet.Enabled = val
		}
	}
	if timeout := os.Getenv("WALLET_GATEWAY_TIMEOUT_SECONDS"); timeout != "" {
		val, err := strconv.Atoi(timeout)
		if err == nil && val > 0 {
			cfg.Wallet.TimeoutSeconds = val
		}
	}
	ensureWalletChainConfig(cfg)
	applyWalletChainEnv(cfg, ChainBTC, "WALLET_BTC_CORE_URL")
	applyWalletChainEnv(cfg, ChainMVC, "WALLET_MVC_CORE_URL")
	applyWalletChainEnv(cfg, ChainDOGE, "WALLET_DOGE_CORE_URL")
	applyWalletV11Env(cfg, ChainBTC, "WALLET_BTC")
	applyWalletV11Env(cfg, ChainMVC, "WALLET_MVC")
	applyWalletV11Env(cfg, ChainDOGE, "WALLET_DOGE")
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

func defaultWalletChainConfig(chain string) WalletGatewayChainConfig {
	switch chain {
	case ChainMVC:
		return WalletGatewayChainConfig{
			BroadcastPath: "/btc/broadcast",
			FeeRate:       WalletGatewayFeeRateConfig{Unit: "sat_per_byte", Slow: 1, Normal: 2, Fast: 3, Default: "normal"},
		}
	case ChainDOGE:
		return WalletGatewayChainConfig{
			BroadcastPath: "/btc/broadcast",
			FeeRate:       WalletGatewayFeeRateConfig{Unit: "sat_per_byte", Slow: 1, Normal: 2, Fast: 5, Default: "normal"},
		}
	default:
		return WalletGatewayChainConfig{
			BroadcastPath: "/btc/broadcast",
			FeeRate:       WalletGatewayFeeRateConfig{Unit: "sat_per_byte", Slow: 1, Normal: 3, Fast: 5, Default: "normal"},
		}
	}
}

func ensureWalletChainConfig(cfg *Config) {
	if cfg.Wallet.TimeoutSeconds <= 0 {
		cfg.Wallet.TimeoutSeconds = 10
	}
	if cfg.Wallet.Chains == nil {
		cfg.Wallet.Chains = make(map[string]WalletGatewayChainConfig)
	}
	for _, chain := range []string{ChainBTC, ChainMVC, ChainDOGE} {
		defaults := defaultWalletChainConfig(chain)
		chainCfg, exists := cfg.Wallet.Chains[chain]
		if !exists {
			cfg.Wallet.Chains[chain] = defaults
			continue
		}
		if strings.TrimSpace(chainCfg.BroadcastPath) == "" {
			chainCfg.BroadcastPath = defaults.BroadcastPath
		}
		if strings.TrimSpace(chainCfg.FeeRate.Unit) == "" {
			chainCfg.FeeRate.Unit = defaults.FeeRate.Unit
		}
		if chainCfg.FeeRate.Slow == 0 {
			chainCfg.FeeRate.Slow = defaults.FeeRate.Slow
		}
		if chainCfg.FeeRate.Normal == 0 {
			chainCfg.FeeRate.Normal = defaults.FeeRate.Normal
		}
		if chainCfg.FeeRate.Fast == 0 {
			chainCfg.FeeRate.Fast = defaults.FeeRate.Fast
		}
		if strings.TrimSpace(chainCfg.FeeRate.Default) == "" {
			chainCfg.FeeRate.Default = defaults.FeeRate.Default
		}
		cfg.Wallet.Chains[chain] = chainCfg
	}
}

func applyWalletChainEnv(cfg *Config, chain string, envName string) {
	value := strings.TrimSpace(os.Getenv(envName))
	if value == "" {
		return
	}
	chainCfg := cfg.Wallet.Chains[chain]
	chainCfg.Enabled = true
	chainCfg.CoreURL = value
	cfg.Wallet.Chains[chain] = chainCfg
}

func applyWalletV11Env(cfg *Config, chain string, prefix string) {
	chainCfg := cfg.Wallet.Chains[chain]
	if value := strings.TrimSpace(os.Getenv(prefix + "_BROADCAST_PATH")); value != "" {
		chainCfg.BroadcastPath = value
	}
	if value := strings.TrimSpace(os.Getenv(prefix + "_FEE_RATE_UNIT")); value != "" {
		chainCfg.FeeRate.Unit = value
	}
	if value := strings.TrimSpace(os.Getenv(prefix + "_FEE_RATE_DEFAULT")); value != "" {
		chainCfg.FeeRate.Default = value
	}
	if value, ok := parsePositiveInt64Env(prefix + "_FEE_RATE_SLOW"); ok {
		chainCfg.FeeRate.Slow = value
	}
	if value, ok := parsePositiveInt64Env(prefix + "_FEE_RATE_NORMAL"); ok {
		chainCfg.FeeRate.Normal = value
	}
	if value, ok := parsePositiveInt64Env(prefix + "_FEE_RATE_FAST"); ok {
		chainCfg.FeeRate.Fast = value
	}
	cfg.Wallet.Chains[chain] = chainCfg
}

func parsePositiveInt64Env(name string) (int64, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}
