package storage

import (
	"strings"
	"testing"
)

func TestDedupMerger_BasicDedup(t *testing.T) {
	m := &dedupMerger{
		buf: []byte(",a@100@t1,b@200@t2"),
	}
	m.MergeNewer([]byte(",b@200@t2,c@300@t3"))
	result, closer, err := m.Finish(true)
	if err != nil {
		t.Fatalf("Finish error: %v", err)
	}
	if closer != nil {
		closer.Close()
	}

	s := string(result)
	if strings.Count(s, "a@100@t1") != 1 {
		t.Errorf("expected one a@100@t1, got: %s", s)
	}
	if strings.Count(s, "b@200@t2") != 1 {
		t.Errorf("expected one b@200@t2 (deduped), got: %s", s)
	}
	if strings.Count(s, "c@300@t3") != 1 {
		t.Errorf("expected one c@300@t3, got: %s", s)
	}
	if s[0] != ',' {
		t.Errorf("expected leading comma, got: %s", s)
	}
}

func TestDedupMerger_Idempotent(t *testing.T) {
	// Same value merged twice should produce the same value
	val := []byte(",x@1@t1,y@2@t2")

	m1 := &dedupMerger{buf: append([]byte(nil), val...)}
	m1.MergeNewer(val)
	r1, _, _ := m1.Finish(true)

	m2 := &dedupMerger{buf: append([]byte(nil), val...)}
	r2, _, _ := m2.Finish(true)

	if string(r1) != string(r2) {
		t.Errorf("idempotent mismatch: %s vs %s", r1, r2)
	}
	// Should have exactly 2 segments (only x@1@t1 and y@2@t2)
	segments := strings.Split(string(r1), ",")
	nonEmpty := 0
	for _, s := range segments {
		if s != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 2 {
		t.Errorf("expected 2 segments, got %d: %s", nonEmpty, r1)
	}
}

func TestDedupMerger_PreservesFirstSeenSegmentOrder(t *testing.T) {
	m := &dedupMerger{
		buf: []byte(",zaddr@200@t2,aaddr@100@t1,zaddr@200@t2"),
	}
	result, closer, err := m.Finish(true)
	if err != nil {
		t.Fatalf("Finish error: %v", err)
	}
	if closer != nil {
		closer.Close()
	}

	expected := ",zaddr@200@t2,aaddr@100@t1"
	if string(result) != expected {
		t.Fatalf("expected order-preserving dedup %q, got %q", expected, string(result))
	}
}

func TestDedupMerger_EmptyInput(t *testing.T) {
	m := &dedupMerger{buf: nil}
	result, _, err := m.Finish(true)
	if err != nil {
		t.Fatalf("Finish error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty input, got: %s", result)
	}
}

func TestDedupMerger_SingleSegment(t *testing.T) {
	m := &dedupMerger{buf: []byte(",addr@100@time")}
	result, _, err := m.Finish(true)
	if err != nil {
		t.Fatalf("Finish error: %v", err)
	}
	if string(result) != ",addr@100@time" {
		t.Errorf("expected single segment unchanged, got: %s", result)
	}
}

func TestDedupMerger_NoLeadingComma(t *testing.T) {
	// Handle values without leading comma (should be treated the same)
	m := &dedupMerger{buf: []byte("a@1@t1,b@2@t2")}
	m.MergeNewer([]byte(",b@2@t2,c@3@t3"))
	result, _, err := m.Finish(true)
	if err != nil {
		t.Fatalf("Finish error: %v", err)
	}
	s := string(result)
	if !strings.Contains(s, "a@1@t1") {
		t.Errorf("missing a segment, got: %s", s)
	}
	if !strings.Contains(s, "c@3@t3") {
		t.Errorf("missing c segment, got: %s", s)
	}
}

func TestDedupMerger_MergeOlder(t *testing.T) {
	m := &dedupMerger{buf: []byte(",b@2@t2")}
	m.MergeOlder([]byte(",a@1@t1"))
	result, _, err := m.Finish(true)
	if err != nil {
		t.Fatalf("Finish error: %v", err)
	}
	s := string(result)
	if !strings.Contains(s, "a@1@t1") {
		t.Errorf("missing older segment, got: %s", s)
	}
	if !strings.Contains(s, "b@2@t2") {
		t.Errorf("missing base segment, got: %s", s)
	}
}

func TestDedupMerger_MergerFactory(t *testing.T) {
	vm, err := dedupMergerFactory.Merge([]byte("key"), []byte(",old@1@t1"))
	if err != nil {
		t.Fatalf("Merge factory error: %v", err)
	}
	vm.MergeNewer([]byte(",new@2@t2"))
	result, closer, err := vm.Finish(true)
	if err != nil {
		t.Fatalf("Finish error: %v", err)
	}
	if closer != nil {
		closer.Close()
	}
	if string(result) != ",old@1@t1,new@2@t2" {
		t.Errorf("unexpected result: %s", result)
	}
}
