package indexer

import "testing"

func TestFilterSortPaginateHistoryTxs(t *testing.T) {
	in := []HistoryTx{
		{TxID: "confirmed-old", TimestampUnix: 100, IsMempool: false},
		{TxID: "mempool-new", TimestampUnix: 300, IsMempool: true},
		{TxID: "confirmed-mid", TimestampUnix: 200, IsMempool: false},
	}

	got, total := filterSortPaginateHistoryTxs(in, HistoryQueryOptions{Page: 1, Limit: 2, Sort: "desc"})
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(got))
	}
	if got[0].TxID != "mempool-new" || got[1].TxID != "confirmed-mid" {
		t.Fatalf("unexpected desc page: %+v", got)
	}

	got, total = filterSortPaginateHistoryTxs(in, HistoryQueryOptions{Page: 1, Limit: 10, Sort: "asc", ConfirmedOnly: true})
	if total != 2 {
		t.Fatalf("expected confirmed-only total 2, got %d", total)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 confirmed history items, got %d", len(got))
	}
	if got[0].TxID != "confirmed-old" || got[1].TxID != "confirmed-mid" {
		t.Fatalf("unexpected asc confirmed-only page: %+v", got)
	}
}

func TestNormalizeHistoryQueryOptions(t *testing.T) {
	got := normalizeHistoryQueryOptions("0", "500", "true", "ASC")
	if got.Page != 1 {
		t.Fatalf("expected page 1, got %d", got.Page)
	}
	if got.Limit != 100 {
		t.Fatalf("expected limit 100, got %d", got.Limit)
	}
	if !got.ConfirmedOnly {
		t.Fatalf("expected confirmedOnly true")
	}
	if got.Sort != "asc" {
		t.Fatalf("expected sort asc, got %q", got.Sort)
	}

	got = normalizeHistoryQueryOptions("2", "20", "false", "bad")
	if got.Page != 2 {
		t.Fatalf("expected page 2, got %d", got.Page)
	}
	if got.Limit != 20 {
		t.Fatalf("expected limit 20, got %d", got.Limit)
	}
	if got.ConfirmedOnly {
		t.Fatalf("expected confirmedOnly false")
	}
	if got.Sort != "desc" {
		t.Fatalf("expected sort desc, got %q", got.Sort)
	}
}
