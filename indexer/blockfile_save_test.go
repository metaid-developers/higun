package indexer

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

func TestSaveBlockFileReturnsForNilInputs(t *testing.T) {
	oldGlobalConfig := config.GlobalConfig
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	config.GlobalConfig = nil
	SaveBlockFile("utxo", nil, false)

	config.GlobalConfig = &config.Config{BlockFilesEnabled: false}
	SaveBlockFile("utxo", &Block{}, false)

	config.GlobalConfig = &config.Config{BlockFilesEnabled: true}
	SaveBlockFile("utxo", &Block{}, false)
	SaveBlockFile("spend", &Block{}, false)
}
