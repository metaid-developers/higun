package blockchain

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

// 测试 BTC 适配器创建
func TestNewBTCAdapter(t *testing.T) {
	cfg := &config.Config{
		Chain:   config.ChainBTC,
		Network: "regtest",
		RPC: config.RPCConfig{
			Chain:    "btc",
			Host:     "127.0.0.1",
			Port:     "18443",
			User:     "test",
			Password: "test",
		},
	}

	adapter, err := NewBTCAdapter(cfg)
	if err != nil {
		t.Skipf("Skip BTC adapter test (node not available): %v", err)
		return
	}

	if adapter == nil {
		t.Fatal("BTC adapter should not be nil")
	}

	if adapter.GetChainName() != "btc" {
		t.Errorf("Expected chain name 'btc', got '%s'", adapter.GetChainName())
	}

	adapter.Shutdown()
}

// 测试 MVC 适配器创建
func TestNewMVCAdapter(t *testing.T) {
	cfg := &config.Config{
		Chain:   config.ChainMVC,
		Network: "mainnet",
		RPC: config.RPCConfig{
			Chain:    "mvc",
			Host:     "127.0.0.1",
			Port:     "9882",
			User:     "mvc_user",
			Password: "mvc_pass",
		},
	}

	adapter, err := NewMVCAdapter(cfg)
	if err != nil {
		t.Skipf("Skip MVC adapter test (node not available): %v", err)
		return
	}

	if adapter == nil {
		t.Fatal("MVC adapter should not be nil")
	}

	if adapter.GetChainName() != "mvc" {
		t.Errorf("Expected chain name 'mvc', got '%s'", adapter.GetChainName())
	}

	adapter.Shutdown()
}

// 测试工厂方法 - BTC
func TestNewChainAdapter_BTC(t *testing.T) {
	cfg := &config.Config{
		Chain:   config.ChainBTC,
		Network: "regtest",
		RPC: config.RPCConfig{
			Chain:    "btc",
			Host:     "127.0.0.1",
			Port:     "18443",
			User:     "test",
			Password: "test",
		},
	}

	adapter, err := NewChainAdapter(cfg)
	if err != nil {
		t.Skipf("Skip chain adapter test (node not available): %v", err)
		return
	}

	if adapter == nil {
		t.Fatal("Chain adapter should not be nil")
	}

	if adapter.GetChainName() != "btc" {
		t.Errorf("Expected chain 'btc', got '%s'", adapter.GetChainName())
	}

	// 测试接口方法
	_, err = adapter.GetBlockCount()
	if err != nil {
		t.Logf("GetBlockCount failed (node may not be running): %v", err)
	}

	adapter.Shutdown()
}

// 测试工厂方法 - MVC
func TestNewChainAdapter_MVC(t *testing.T) {
	cfg := &config.Config{
		Chain:   config.ChainMVC,
		Network: "mainnet",
		RPC: config.RPCConfig{
			Chain:    "mvc",
			Host:     "127.0.0.1",
			Port:     "9882",
			User:     "mvc_user",
			Password: "mvc_pass",
		},
	}

	adapter, err := NewChainAdapter(cfg)
	if err != nil {
		t.Skipf("Skip MVC chain adapter test (node not available): %v", err)
		return
	}

	if adapter == nil {
		t.Fatal("Chain adapter should not be nil")
	}

	if adapter.GetChainName() != "mvc" {
		t.Errorf("Expected chain 'mvc', got '%s'", adapter.GetChainName())
	}

	adapter.Shutdown()
}

// 测试工厂方法 - 不支持的链
func TestNewChainAdapter_Unsupported(t *testing.T) {
	cfg := &config.Config{
		Chain: "unsupported_chain",
		RPC: config.RPCConfig{
			Chain: "unsupported",
		},
	}

	_, err := NewChainAdapter(cfg)
	if err == nil {
		t.Error("Expected error for unsupported chain, got nil")
	}
}

// 测试 DOGE 适配器创建
func TestNewDOGEAdapter(t *testing.T) {
	cfg := &config.Config{
		Chain:   config.ChainDOGE,
		Network: "mainnet",
		RPC: config.RPCConfig{
			Chain:    "doge",
			Host:     "127.0.0.1",
			Port:     "22555",
			User:     "doge_user",
			Password: "doge_pass",
		},
	}

	adapter, err := NewDOGEAdapter(cfg)
	if err != nil {
		t.Skipf("Skip DOGE adapter test (node not available): %v", err)
		return
	}

	if adapter == nil {
		t.Fatal("DOGE adapter should not be nil")
	}

	if adapter.GetChainName() != "doge" {
		t.Errorf("Expected chain name 'doge', got '%s'", adapter.GetChainName())
	}

	// 验证网络参数
	params := adapter.GetChainParams()
	if params.PubKeyHashAddrID != 0x1e {
		t.Errorf("Expected DOGE mainnet PubKeyHashAddrID 0x1e, got 0x%x", params.PubKeyHashAddrID)
	}

	adapter.Shutdown()
}

// 测试工厂方法 - DOGE
func TestNewChainAdapter_DOGE(t *testing.T) {
	cfg := &config.Config{
		Chain:   config.ChainDOGE,
		Network: "mainnet",
		RPC: config.RPCConfig{
			Chain:    "doge",
			Host:     "127.0.0.1",
			Port:     "22555",
			User:     "doge_user",
			Password: "doge_pass",
		},
	}

	adapter, err := NewChainAdapter(cfg)
	if err != nil {
		t.Skipf("Skip DOGE chain adapter test (node not available): %v", err)
		return
	}

	if adapter == nil {
		t.Fatal("Chain adapter should not be nil")
	}

	if adapter.GetChainName() != "doge" {
		t.Errorf("Expected chain 'doge', got '%s'", adapter.GetChainName())
	}

	adapter.Shutdown()
}

// 测试适配器接口完整性
func TestAdapterInterface(t *testing.T) {
	// 编译时验证适配器实现了接口
	var _ ChainAdapter = (*BTCAdapter)(nil)
	var _ ChainAdapter = (*MVCAdapter)(nil)
	var _ ChainAdapter = (*DOGEAdapter)(nil)
}
