package blockchain

import (
	"errors"
	"testing"

	"github.com/metaid/utxo_indexer/syslogs"
)

func TestDetectReorgFromLogsFindsCommonAncestor(t *testing.T) {
	logs := []syslogs.IndexerLog{
		{Height: 105, BlockHash: "local-105"},
		{Height: 104, BlockHash: "local-104"},
		{Height: 103, BlockHash: "shared-103"},
		{Height: 102, BlockHash: "shared-102"},
	}

	chainHashes := map[int]string{
		105: "chain-105",
		104: "chain-104",
		103: "shared-103",
		102: "shared-102",
	}

	detection, ok := detectReorgFromLogs(logs, func(height int) (string, error) {
		hash, exists := chainHashes[height]
		if !exists {
			return "", errors.New("missing hash")
		}
		return hash, nil
	})
	if !ok {
		t.Fatal("expected reorg to be detected")
	}

	if detection.lastSameHeight != 103 {
		t.Fatalf("expected common ancestor at height 103, got %d", detection.lastSameHeight)
	}
	if detection.startHeight != 104 {
		t.Fatalf("expected reorg start at height 104, got %d", detection.startHeight)
	}
	if detection.endHeight != 105 {
		t.Fatalf("expected reorg end at height 105, got %d", detection.endHeight)
	}
	if detection.reorgSize != 2 {
		t.Fatalf("expected reorg size 2, got %d", detection.reorgSize)
	}
	if detection.reorgHash != "local-104" {
		t.Fatalf("expected local divergent hash local-104, got %s", detection.reorgHash)
	}
	if detection.newHash != "chain-104" {
		t.Fatalf("expected chain divergent hash chain-104, got %s", detection.newHash)
	}
}

func TestDetectReorgFromLogsReturnsNoReorgWhenHashesMatch(t *testing.T) {
	logs := []syslogs.IndexerLog{
		{Height: 105, BlockHash: "shared-105"},
		{Height: 104, BlockHash: "shared-104"},
	}

	detection, ok := detectReorgFromLogs(logs, func(height int) (string, error) {
		switch height {
		case 105:
			return "shared-105", nil
		case 104:
			return "shared-104", nil
		default:
			return "", errors.New("missing hash")
		}
	})
	if ok {
		t.Fatalf("expected no reorg, got %+v", detection)
	}
}

func TestDetectReorgFromLogsRequiresKnownCommonAncestor(t *testing.T) {
	logs := []syslogs.IndexerLog{
		{Height: 105, BlockHash: "local-105"},
		{Height: 104, BlockHash: "local-104"},
	}

	detection, ok := detectReorgFromLogs(logs, func(height int) (string, error) {
		switch height {
		case 105:
			return "chain-105", nil
		case 104:
			return "chain-104", nil
		default:
			return "", errors.New("missing hash")
		}
	})
	if ok {
		t.Fatalf("expected no reorg without a common ancestor, got %+v", detection)
	}
}
