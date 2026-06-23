package config

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMVCTxIDAliasBackfillDefaultsEnabled(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if !cfg.MVCTxIDAliasBackfillEnabled {
		t.Fatalf("expected MVC txid alias backfill to default enabled")
	}
	if cfg.MVCTxIDAliasBackfillBatchSize != 1000 {
		t.Fatalf("default MVC txid alias backfill batch size = %d, want 1000", cfg.MVCTxIDAliasBackfillBatchSize)
	}
	if cfg.MVCTxIDAliasBackfillWorkers != 4 {
		t.Fatalf("default MVC txid alias backfill workers = %d, want 4", cfg.MVCTxIDAliasBackfillWorkers)
	}
	if cfg.MVCTxIDAliasBackfillStartHeight != 0 {
		t.Fatalf("default MVC txid alias backfill start height = %d, want 0", cfg.MVCTxIDAliasBackfillStartHeight)
	}
	if cfg.MVCTxIDAliasBackfillRetryAttempts != 3 {
		t.Fatalf("default MVC txid alias backfill retry attempts = %d, want 3", cfg.MVCTxIDAliasBackfillRetryAttempts)
	}
	if cfg.MVCTxIDAliasBackfillRetryDelayMS != 1000 {
		t.Fatalf("default MVC txid alias backfill retry delay = %d, want 1000", cfg.MVCTxIDAliasBackfillRetryDelayMS)
	}
	if cfg.SyncBaseCountEnabled {
		t.Fatalf("expected sync base count to default disabled")
	}
	if cfg.SyncTouchedBalanceRows {
		t.Fatalf("expected touched balance row sync to default disabled")
	}
}

func TestMVCTxIDAliasBackfillCanBeDisabledByYAML(t *testing.T) {
	cfg := Config{
		MVCTxIDAliasBackfillEnabled: true,
	}

	data := []byte("mvc_txid_alias_backfill_enabled: false\n")
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if cfg.MVCTxIDAliasBackfillEnabled {
		t.Fatalf("expected YAML to disable MVC txid alias backfill")
	}
}

func TestMVCTxIDAliasBackfillStartHeightFromYAML(t *testing.T) {
	cfg := Config{}

	data := []byte("mvc_txid_alias_backfill_start_height: 150000\n")
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if cfg.MVCTxIDAliasBackfillStartHeight != 150000 {
		t.Fatalf("MVC txid alias backfill start height = %d, want 150000", cfg.MVCTxIDAliasBackfillStartHeight)
	}
}

func TestSyncBaseCountCanBeEnabledByYAML(t *testing.T) {
	cfg := Config{}

	data := []byte("sync_base_count_enabled: true\n")
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if !cfg.SyncBaseCountEnabled {
		t.Fatalf("expected YAML to enable sync base count")
	}
}

func TestSyncTouchedBalanceRowsCanBeEnabledByYAML(t *testing.T) {
	cfg := Config{}

	data := []byte("sync_touched_balance_rows: true\n")
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if !cfg.SyncTouchedBalanceRows {
		t.Fatalf("expected YAML to enable touched balance row sync")
	}
}
