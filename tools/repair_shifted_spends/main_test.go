package main

import "testing"

func TestRowHasVoutEntries(t *testing.T) {
	cases := []struct {
		row  string
		want bool
	}{
		{"", false},
		{",", false},
		{"a@100@1700", false},                                // single mempool entry
		{"a@100@1700,b@200@1700", false},                     // two mempool entries
		{"a@100@1700@0", true},                               // single confirmed entry
		{"a@100@1700@0,b@200@1700@1", true},                  // two confirmed entries
		{"m1@100@1700,m2@200@1700,c1@100@1700@0,c2@200@1700@1", true}, // mixed
	}
	for _, c := range cases {
		if got := rowHasVoutEntries([]byte(c.row)); got != c.want {
			t.Fatalf("rowHasVoutEntries(%q) = %v, want %v", c.row, got, c.want)
		}
	}
}

func TestDropVoutlessEntries(t *testing.T) {
	// Mixed row: mempool-era prefix dropped, confirmed entries kept.
	row := "m1@100@1700,m2@200@1700,c1@100@1700@0,c2@200@1700@1"
	want := "c1@100@1700@0,c2@200@1700@1"
	if got := string(dropVoutlessEntries([]byte(row))); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Pure rows and empty results must be left untouched.
	for _, row := range []string{"", ",", "m1@100@1700,m2@200@1700"} {
		if got := string(dropVoutlessEntries([]byte(row))); got != row {
			t.Fatalf("pure row %q changed to %q", row, got)
		}
	}
}
