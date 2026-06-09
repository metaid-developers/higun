package storage

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

func TestPebbleBudgetDividesAcrossStoresAndShards(t *testing.T) {
	params := config.IndexerParams{
		MemoryBudget: config.MemoryBudget{
			PebbleCacheBytes:    120 << 20,
			PebbleMemTableBytes: 240 << 20,
		},
		PebbleMainStoreCount: 5,
	}

	got := ComputePebbleOpenOptions(params, 6)

	if got.CacheSizeBytes != 24<<20 {
		t.Fatalf("CacheSizeBytes = %d, want %d", got.CacheSizeBytes, int64(24<<20))
	}
	if got.MemTableSizeBytes != 8<<20 {
		t.Fatalf("MemTableSizeBytes = %d, want %d", got.MemTableSizeBytes, uint64(8<<20))
	}
}

func TestPebbleBudgetHasSafeMinimums(t *testing.T) {
	params := config.IndexerParams{
		MemoryBudget: config.MemoryBudget{
			PebbleCacheBytes:    8 << 20,
			PebbleMemTableBytes: 8 << 20,
		},
		PebbleMainStoreCount: 5,
	}

	got := ComputePebbleOpenOptions(params, 6)

	if got.CacheSizeBytes < 4<<20 {
		t.Fatalf("CacheSizeBytes = %d, want at least %d", got.CacheSizeBytes, int64(4<<20))
	}
	if got.MemTableSizeBytes < 4<<20 {
		t.Fatalf("MemTableSizeBytes = %d, want at least %d", got.MemTableSizeBytes, uint64(4<<20))
	}
}

func TestPebbleBudgetUsesLegacyDefaultsWhenUnconfigured(t *testing.T) {
	got := ComputePebbleOpenOptions(config.IndexerParams{}, 6)

	if got.CacheSizeBytes != defaultPebbleCacheSizeBytes {
		t.Fatalf("CacheSizeBytes = %d, want %d", got.CacheSizeBytes, defaultPebbleCacheSizeBytes)
	}
	if got.MemTableSizeBytes != defaultPebbleMemTableSizeBytes {
		t.Fatalf("MemTableSizeBytes = %d, want %d", got.MemTableSizeBytes, uint64(defaultPebbleMemTableSizeBytes))
	}
}

func TestPebbleBudgetDefaultsInvalidStoreAndShardCounts(t *testing.T) {
	params := config.IndexerParams{
		MemoryBudget: config.MemoryBudget{
			PebbleCacheBytes:    12 << 20,
			PebbleMemTableBytes: 16 << 20,
		},
		PebbleMainStoreCount: 0,
	}

	got := ComputePebbleOpenOptions(params, 0)

	if got.CacheSizeBytes != 12<<20 {
		t.Fatalf("CacheSizeBytes = %d, want %d", got.CacheSizeBytes, int64(12<<20))
	}
	if got.MemTableSizeBytes != 16<<20 {
		t.Fatalf("MemTableSizeBytes = %d, want %d", got.MemTableSizeBytes, uint64(16<<20))
	}
}

func TestNewPebbleDBOptionsPreservesExplicitOpenOptions(t *testing.T) {
	params := config.IndexerParams{
		MemoryBudget: config.MemoryBudget{
			PebbleCacheBytes:    120 << 20,
			PebbleMemTableBytes: 240 << 20,
		},
		PebbleMainStoreCount: 5,
	}

	options, cache := newPebbleDBOptions(params, PebbleOpenOptions{
		CacheSizeBytes:              7 << 20,
		MemTableSizeBytes:           9 << 20,
		MemTableStopWritesThreshold: 3,
		MaxConcurrentCompactions:    2,
		MaxOpenFiles:                123,
		ReadOnly:                    true,
	}, 1)
	defer cache.Unref()

	if options.Cache.MaxSize() != 7<<20 {
		t.Fatalf("cache max size = %d, want %d", options.Cache.MaxSize(), int64(7<<20))
	}
	if options.MemTableSize != 9<<20 {
		t.Fatalf("MemTableSize = %d, want %d", options.MemTableSize, uint64(9<<20))
	}
	if options.MemTableStopWritesThreshold != 3 {
		t.Fatalf("MemTableStopWritesThreshold = %d, want 3", options.MemTableStopWritesThreshold)
	}
	if got := options.MaxConcurrentCompactions(); got != 2 {
		t.Fatalf("MaxConcurrentCompactions = %d, want 2", got)
	}
	if options.MaxOpenFiles != 123 {
		t.Fatalf("MaxOpenFiles = %d, want 123", options.MaxOpenFiles)
	}
	if !options.ReadOnly {
		t.Fatalf("ReadOnly = false, want true")
	}
}

func TestNewPebbleDBOptionsFillsMissingMemTableWithShardAwareBudget(t *testing.T) {
	params := config.IndexerParams{
		MemoryBudget: config.MemoryBudget{
			PebbleCacheBytes:    120 << 20,
			PebbleMemTableBytes: 240 << 20,
		},
		PebbleMainStoreCount: 5,
	}

	options, cache := newPebbleDBOptions(params, PebbleOpenOptions{
		CacheSizeBytes: 7 << 20,
	}, 6)
	defer cache.Unref()

	if options.Cache.MaxSize() != 7<<20 {
		t.Fatalf("cache max size = %d, want %d", options.Cache.MaxSize(), int64(7<<20))
	}
	if options.MemTableSize != 8<<20 {
		t.Fatalf("MemTableSize = %d, want %d", options.MemTableSize, uint64(8<<20))
	}
}

func TestStoreTypeStringMainRuntimeNames(t *testing.T) {
	tests := []struct {
		storeType StoreType
		want      string
	}{
		{StoreTypeUTXO, "utxo"},
		{StoreTypeIncome, "income"},
		{StoreTypeSpend, "spend"},
		{StoreTypeAddressBalance, "address_balance"},
		{StoreTypeBalanceRank, "balance_rank"},
		{StoreType(999), "store_type_999"},
	}

	for _, tt := range tests {
		if got := tt.storeType.String(); got != tt.want {
			t.Fatalf("%v.String() = %q, want %q", int(tt.storeType), got, tt.want)
		}
	}
}
