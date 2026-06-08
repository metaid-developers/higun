package wallet

import (
	"fmt"
	"strings"
)

const (
	maxInt64AsUint64 = uint64(1<<63) - 1
	minInt64Abs      = uint64(1 << 63)
	maxUint64Value   = ^uint64(0)
)

type Chain string

const (
	ChainBTC  Chain = "btc"
	ChainMVC  Chain = "mvc"
	ChainDOGE Chain = "doge"
)

type ResponseFormat string

const (
	FormatStandard ResponseFormat = "standard"
	FormatMetalet  ResponseFormat = "metalet"
)

type WalletBalance struct {
	Chain                Chain
	Address              string
	ConfirmedSatoshi     uint64
	MempoolIncomeSatoshi uint64
	MempoolSpendSatoshi  uint64
	UnsafeSatoshi        uint64
	ConfirmedUTXOCount   uint64
	MempoolUTXOCount     uint64
}

func (b WalletBalance) UnconfirmedSatoshi() int64 {
	return uint64DeltaToInt64(b.MempoolIncomeSatoshi, b.MempoolSpendSatoshi)
}

func (b WalletBalance) SafeSatoshi() uint64 {
	total := saturatingAddUint64(b.ConfirmedSatoshi, b.MempoolIncomeSatoshi)
	if total <= b.MempoolSpendSatoshi {
		return 0
	}
	total -= b.MempoolSpendSatoshi
	if total <= b.UnsafeSatoshi {
		return 0
	}
	return total - b.UnsafeSatoshi
}

func (b WalletBalance) UTXOCount() uint64 {
	return b.ConfirmedUTXOCount + b.MempoolUTXOCount
}

func uint64DeltaToInt64(income, spend uint64) int64 {
	if income >= spend {
		diff := income - spend
		if diff > maxInt64AsUint64 {
			return int64(maxInt64AsUint64)
		}
		return int64(diff)
	}
	diff := spend - income
	if diff >= minInt64Abs {
		return -1 << 63
	}
	return -int64(diff)
}

func saturatingAddUint64(a, b uint64) uint64 {
	if maxUint64Value-a < b {
		return maxUint64Value
	}
	return a + b
}

type WalletUTXO struct {
	Chain     Chain
	Address   string
	TxID      string
	Vout      int
	Satoshi   uint64
	Confirmed bool
	Mempool   bool
	Height    *int64
}

func (u WalletUTXO) Outpoint() string {
	return fmt.Sprintf("%s:%d", u.TxID, u.Vout)
}

type UTXOOptions struct {
	ConfirmedOnly bool
	Sort          string
}

func int64Ptr(value int64) *int64 {
	return &value
}

func NormalizeChain(raw string) (Chain, bool) {
	switch Chain(strings.ToLower(strings.TrimSpace(raw))) {
	case ChainBTC:
		return ChainBTC, true
	case ChainMVC:
		return ChainMVC, true
	case ChainDOGE:
		return ChainDOGE, true
	default:
		return "", false
	}
}

func NormalizeFormat(raw string) (ResponseFormat, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return FormatStandard, true
	}
	switch ResponseFormat(value) {
	case FormatStandard:
		return FormatStandard, true
	case FormatMetalet:
		return FormatMetalet, true
	default:
		return "", false
	}
}
