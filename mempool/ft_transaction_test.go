package mempool

import (
	"testing"
)

// CleanAllMempool must stop the old ZMQ client before swapping it out.
func TestFtCleanAllMempoolStopsOldZMQClient(t *testing.T) {
	old := NewZMQClient([]string{"tcp://127.0.0.1:28332"}, nil)[0]
	m := &FtMempoolManager{basePath: t.TempDir(), zmqClient: old}
	defer m.Stop()

	if err := m.CleanAllMempool(); err != nil {
		t.Fatalf("CleanAllMempool: %v", err)
	}
	if old.ctx.Err() == nil {
		t.Fatal("old ZMQ client was not stopped")
	}
	if m.zmqClient == nil || m.zmqClient == old {
		t.Fatal("expected a fresh ZMQ client after rebuild")
	}
}

func TestFtInitializeMempoolSyncRejectsUnsupportedClient(t *testing.T) {
	m := &FtMempoolManager{}
	if err := m.InitializeMempoolSync("not a blockchain client"); err == nil {
		t.Fatal("expected error for unsupported blockchain client type")
	}
}
