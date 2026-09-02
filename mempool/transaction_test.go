package mempool

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

// RebuildMempool must stop the old ZMQ clients before swapping them out,
// wipe the mempool databases and create fresh clients.
func TestRebuildMempoolStopsOldClientsAndWipesDBs(t *testing.T) {
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{ZMQAddress: []string{"tcp://127.0.0.1:28333"}}
	defer func() { config.GlobalConfig = oldConfig }()

	m := NewMempoolManager(t.TempDir(), nil, nil, []string{"tcp://127.0.0.1:28332"})
	if m == nil {
		t.Fatal("failed to create mempool manager")
	}
	defer m.Stop()

	oldClients := m.zmqClient
	if len(oldClients) == 0 {
		t.Fatal("expected initial ZMQ clients")
	}

	if err := m.MempoolIncomeDB.AddMempolRecord("addr_txid:0_123", []byte("100")); err != nil {
		t.Fatalf("seed income record: %v", err)
	}

	if err := m.RebuildMempool(); err != nil {
		t.Fatalf("RebuildMempool: %v", err)
	}

	for i, client := range oldClients {
		if client.ctx.Err() == nil {
			t.Fatalf("old ZMQ client %d was not stopped", i)
		}
	}
	if len(m.zmqClient) == 0 {
		t.Fatal("expected fresh ZMQ clients after rebuild")
	}
	for _, client := range m.zmqClient {
		for _, old := range oldClients {
			if client == old {
				t.Fatal("rebuilt mempool still references an old ZMQ client")
			}
		}
	}
	records, err := m.MempoolIncomeDB.GetByPrefix("addr")
	if err != nil {
		t.Fatalf("scan rebuilt income DB: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("income DB not wiped, got %d records", len(records))
	}
}

func TestInitializeMempoolSyncRejectsUnsupportedClient(t *testing.T) {
	m := &MempoolManager{}
	if err := m.InitializeMempoolSync("not a blockchain client"); err == nil {
		t.Fatal("expected error for unsupported blockchain client type")
	}
}

// RestartMempool must fully rebuild the mempool data even though the node
// client is unavailable (the re-ingest only logs in the background).
func TestRestartMempoolRebuildsMempoolData(t *testing.T) {
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{ZMQAddress: []string{"tcp://127.0.0.1:28333"}}
	defer func() { config.GlobalConfig = oldConfig }()

	m := NewMempoolManager(t.TempDir(), nil, nil, []string{"tcp://127.0.0.1:28332"})
	if m == nil {
		t.Fatal("failed to create mempool manager")
	}
	defer m.Stop()

	if err := m.MempoolIncomeDB.AddMempolRecord("addr_txid:0_123", []byte("100")); err != nil {
		t.Fatalf("seed income record: %v", err)
	}
	oldClients := m.zmqClient

	if err := m.RestartMempool("not a blockchain client"); err != nil {
		t.Fatalf("RestartMempool: %v", err)
	}

	for i, client := range oldClients {
		if client.ctx.Err() == nil {
			t.Fatalf("old ZMQ client %d was not stopped", i)
		}
	}
	if len(m.zmqClient) == 0 {
		t.Fatal("expected fresh ZMQ clients after restart")
	}
	records, err := m.MempoolIncomeDB.GetByPrefix("addr")
	if err != nil {
		t.Fatalf("scan rebuilt income DB: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("income DB not wiped, got %d records", len(records))
	}
}
