package wallet

import "testing"

func TestTruncateAddressForLog(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "short", in: "short", want: "short"},
		{name: "twelve", in: "123456789012", want: "123456789012"},
		{name: "btc", in: "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ", want: "12ghVW...nMUikZ"},
		{name: "doge", in: "DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L", want: "DH5yai...R3mr7L"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateAddressForLog(tt.in); got != tt.want {
				t.Fatalf("truncateAddressForLog(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
