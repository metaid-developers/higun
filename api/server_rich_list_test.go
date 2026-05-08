package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/metaid/utxo_indexer/config"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/storage"
)

type richListResponse struct {
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
	List     []indexer.AddressBalance `json:"list"`
}

func newRichListTestServer(t *testing.T, cache []indexer.AddressBalance, totalAddressCount int) *Server {
	t.Helper()

	oldGlobalConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{MemUTXOMaxCount: 1}
	t.Cleanup(func() {
		config.GlobalConfig = oldGlobalConfig
	})

	metaStore, err := storage.NewMetaStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewMetaStore: %v", err)
	}
	t.Cleanup(func() {
		if err := metaStore.Close(); err != nil {
			t.Fatalf("MetaStore.Close: %v", err)
		}
	})

	if totalAddressCount > 0 {
		if err := metaStore.Set([]byte("total_address_count"), []byte(fmt.Sprintf("%d", totalAddressCount))); err != nil {
			t.Fatalf("seed total_address_count: %v", err)
		}
	}
	if cache != nil {
		data, err := json.Marshal(cache)
		if err != nil {
			t.Fatalf("marshal cache: %v", err)
		}
		if err := metaStore.Set([]byte("rich_list_cache"), data); err != nil {
			t.Fatalf("seed rich_list_cache: %v", err)
		}
	}

	idx := indexer.NewUTXOIndexer(config.IndexerParams{}, nil, nil, metaStore, nil)
	return NewServer(idx, metaStore, make(chan struct{}))
}

func TestGetRichListSupportsLimitAndCapsAt100(t *testing.T) {
	cache := make([]indexer.AddressBalance, 120)
	for i := range cache {
		cache[i] = indexer.AddressBalance{
			Address:        fmt.Sprintf("addr-%03d", i+1),
			BalanceSatoshi: int64(1000 - i),
			Balance:        float64(1000-i) / 1e8,
			UTXOCount:      int64(i + 1),
		}
	}

	server := newRichListTestServer(t, cache, len(cache))
	req := httptest.NewRequest(http.MethodGet, "/rich-list?limit=200", nil)
	rec := httptest.NewRecorder()

	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp richListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Page != 1 {
		t.Fatalf("expected page 1, got %d", resp.Page)
	}
	if resp.PageSize != 100 {
		t.Fatalf("expected page_size 100, got %d", resp.PageSize)
	}
	if resp.Total != 120 {
		t.Fatalf("expected total 120, got %d", resp.Total)
	}
	if len(resp.List) != 100 {
		t.Fatalf("expected 100 entries, got %d", len(resp.List))
	}
	if resp.List[0].Address != "addr-001" {
		t.Fatalf("expected first address addr-001, got %s", resp.List[0].Address)
	}
}

func TestGetRichListReturnsServiceUnavailableWhenCacheNotReady(t *testing.T) {
	server := newRichListTestServer(t, nil, 5)
	req := httptest.NewRequest(http.MethodGet, "/rich-list?limit=10", nil)
	rec := httptest.NewRecorder()

	server.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d: %s", rec.Code, rec.Body.String())
	}
}
