package wallet

import (
	"net/http"
	"testing"
)

func TestSatoshiToDecimalString(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		want string
	}{
		{name: "zero", in: 0, want: "0"},
		{name: "small", in: 1000, want: "0.00001000"},
		{name: "btc", in: 100000000, want: "1.00000000"},
		{name: "fraction", in: 135758, want: "0.00135758"},
		{name: "large-doge-safe", in: uint64(1<<63) + 99, want: "92233720368.54775907"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SatoshiToDecimalString(tt.in); got != tt.want {
				t.Fatalf("SatoshiToDecimalString(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSignedSatoshiToDecimalString(t *testing.T) {
	if got, want := SignedSatoshiToDecimalString(-1), "-0.00000001"; got != want {
		t.Fatalf("SignedSatoshiToDecimalString(-1) = %q, want %q", got, want)
	}
	if got, want := SignedSatoshiToDecimalString(1000), "0.00001000"; got != want {
		t.Fatalf("SignedSatoshiToDecimalString(1000) = %q, want %q", got, want)
	}
	if got, want := SignedSatoshiToDecimalString(-1<<63), "-92233720368.54775808"; got != want {
		t.Fatalf("SignedSatoshiToDecimalString(min int64) = %q, want %q", got, want)
	}
}

func TestWalletBalanceSafeSatoshi(t *testing.T) {
	balance := WalletBalance{
		ConfirmedSatoshi:     135758,
		MempoolIncomeSatoshi: 100,
		MempoolSpendSatoshi:  50,
		UnsafeSatoshi:        134862,
	}
	if got, want := balance.SafeSatoshi(), uint64(946); got != want {
		t.Fatalf("SafeSatoshi() = %d, want %d", got, want)
	}

	negative := WalletBalance{
		ConfirmedSatoshi: 100,
		UnsafeSatoshi:    200,
	}
	if got := negative.SafeSatoshi(); got != 0 {
		t.Fatalf("negative SafeSatoshi() = %d, want 0", got)
	}
}

func TestWalletBalanceUnconfirmedCanBeNegative(t *testing.T) {
	balance := WalletBalance{
		MempoolIncomeSatoshi: 10,
		MempoolSpendSatoshi:  25,
	}
	if got, want := balance.UnconfirmedSatoshi(), int64(-15); got != want {
		t.Fatalf("UnconfirmedSatoshi() = %d, want %d", got, want)
	}
}

func TestWalletBalanceUnconfirmedSatoshiClampsAtInt64Boundaries(t *testing.T) {
	positive := WalletBalance{
		MempoolIncomeSatoshi: uint64(1 << 63),
	}
	if got, want := positive.UnconfirmedSatoshi(), int64(1<<63-1); got != want {
		t.Fatalf("positive clamp = %d, want %d", got, want)
	}

	negative := WalletBalance{
		MempoolSpendSatoshi: uint64(1 << 63),
	}
	if got, want := negative.UnconfirmedSatoshi(), int64(-1<<63); got != want {
		t.Fatalf("negative clamp = %d, want %d", got, want)
	}
}

func TestWalletBalanceUTXOCountSaturatesOnOverflow(t *testing.T) {
	balance := WalletBalance{
		ConfirmedUTXOCount: ^uint64(0),
		MempoolUTXOCount:   1,
	}
	if got, want := balance.UTXOCount(), ^uint64(0); got != want {
		t.Fatalf("UTXOCount() = %d, want %d", got, want)
	}
}

func TestWalletBalanceSupportsLargeDogeAmounts(t *testing.T) {
	const large = uint64(1<<63) + 99
	balance := WalletBalance{
		ConfirmedSatoshi: large,
		UnsafeSatoshi:    1,
	}
	if got, want := balance.SafeSatoshi(), large-1; got != want {
		t.Fatalf("SafeSatoshi() = %d, want %d", got, want)
	}
	if got, want := SatoshiToDecimalString(large), "92233720368.54775907"; got != want {
		t.Fatalf("large decimal = %q, want %q", got, want)
	}
}

func TestNormalizeUTXOsIncludesMempoolByDefaultAndSortsDesc(t *testing.T) {
	in := []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "b", Vout: 0, Satoshi: 200, Confirmed: true, Mempool: false, Height: int64Ptr(10)},
		{Chain: ChainBTC, Address: "addr", TxID: "a", Vout: 1, Satoshi: 300, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
		{Chain: ChainBTC, Address: "addr", TxID: "c", Vout: 0, Satoshi: 100, Confirmed: true, Mempool: false, Height: int64Ptr(11)},
	}
	got, err := NormalizeUTXOs(in, UTXOOptions{Sort: "desc"})
	if err != nil {
		t.Fatalf("NormalizeUTXOs: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Outpoint() != "a:1" || got[1].Outpoint() != "b:0" || got[2].Outpoint() != "c:0" {
		t.Fatalf("unexpected order: %s, %s, %s", got[0].Outpoint(), got[1].Outpoint(), got[2].Outpoint())
	}
	if !got[0].Mempool || got[0].Confirmed {
		t.Fatalf("first UTXO should be mempool and unconfirmed: %+v", got[0])
	}
}

func TestNormalizeUTXOsConfirmedOnlySortsAscAndDeduplicates(t *testing.T) {
	in := []WalletUTXO{
		{Chain: ChainBTC, Address: "addr", TxID: "dup", Vout: 0, Satoshi: 50, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
		{Chain: ChainBTC, Address: "addr", TxID: "dup", Vout: 0, Satoshi: 50, Confirmed: true, Mempool: false, Height: int64Ptr(12)},
		{Chain: ChainBTC, Address: "addr", TxID: "small", Vout: 0, Satoshi: 10, Confirmed: true, Mempool: false, Height: int64Ptr(12)},
		{Chain: ChainBTC, Address: "addr", TxID: "mem", Vout: 0, Satoshi: 5, Confirmed: false, Mempool: true, Height: int64Ptr(-1)},
	}
	got, err := NormalizeUTXOs(in, UTXOOptions{ConfirmedOnly: true, Sort: "asc"})
	if err != nil {
		t.Fatalf("NormalizeUTXOs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Outpoint() != "small:0" || got[1].Outpoint() != "dup:0" {
		t.Fatalf("unexpected order: %s, %s", got[0].Outpoint(), got[1].Outpoint())
	}
	if !got[1].Confirmed || got[1].Mempool {
		t.Fatalf("dedupe should prefer confirmed UTXO over mempool duplicate: %+v", got[1])
	}
}

func TestNormalizeUTXOsRejectsInvalidSort(t *testing.T) {
	_, err := NormalizeUTXOs(nil, UTXOOptions{Sort: "largest"})
	if err == nil {
		t.Fatal("expected invalid sort error")
	}
}

func TestParseVout(t *testing.T) {
	got, err := parseVout("12")
	if err != nil {
		t.Fatalf("parseVout valid: %v", err)
	}
	if got != 12 {
		t.Fatalf("parseVout valid = %d, want 12", got)
	}

	tests := []struct {
		name string
		in   string
	}{
		{name: "non-integer", in: "abc"},
		{name: "empty", in: ""},
		{name: "negative", in: "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseVout(tt.in)
			assertInvalidUpstreamWalletError(t, err)
		})
	}
}

func assertInvalidUpstreamWalletError(t *testing.T, err error) {
	t.Helper()
	walletErr, ok := err.(*WalletError)
	if !ok {
		t.Fatalf("err = %T %v, want *WalletError", err, err)
	}
	if walletErr.HTTPStatus != http.StatusBadGateway {
		t.Fatalf("HTTPStatus = %d, want %d", walletErr.HTTPStatus, http.StatusBadGateway)
	}
	if walletErr.Code != CodeInvalidUpstream {
		t.Fatalf("Code = %d, want %d", walletErr.Code, CodeInvalidUpstream)
	}
}
