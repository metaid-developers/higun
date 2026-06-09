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
	if cfg.MVCTxIDAliasBackfillRetryAttempts != 3 {
		t.Fatalf("default MVC txid alias backfill retry attempts = %d, want 3", cfg.MVCTxIDAliasBackfillRetryAttempts)
	}
	if cfg.MVCTxIDAliasBackfillRetryDelayMS != 1000 {
		t.Fatalf("default MVC txid alias backfill retry delay = %d, want 1000", cfg.MVCTxIDAliasBackfillRetryDelayMS)
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
