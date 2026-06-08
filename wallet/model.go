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

const (
	FeeRateSourceConfig   = "config"
	FeeRateUnitSatPerByte = "sat_per_byte"
)

type FeeRate struct {
	Source  string
	Unit    string
	Slow    int64
	Normal  int64
	Fast    int64
	Default string
}

func (r FeeRate) Validate() error {
	if strings.TrimSpace(r.Source) == "" {
		return fmt.Errorf("fee source is required")
	}
	if strings.TrimSpace(r.Unit) != FeeRateUnitSatPerByte {
		return fmt.Errorf("fee unit must be %s", FeeRateUnitSatPerByte)
	}
	if r.Slow <= 0 || r.Normal <= 0 || r.Fast <= 0 {
		return fmt.Errorf("fee tiers must be positive")
	}
	switch r.Default {
	case "slow", "normal", "fast":
		return nil
	default:
		return fmt.Errorf("fee default must be slow, normal, or fast")
	}
}

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
	return saturatingAddUint64(b.ConfirmedUTXOCount, b.MempoolUTXOCount)
}

// Unconfirmed satoshi deltas are capped to int64 because response models expose signed amounts.
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

type BroadcastResult struct {
	Chain    Chain
	TxID     string
	Accepted bool
}

type WalletTxInput struct {
	TxID     string
	Vout     uint32
	Address  string
	Satoshi  *uint64
	Coinbase string
}

type WalletTxOutput struct {
	Vout    uint32
	Address string
	Satoshi uint64
}

type WalletTxDetail struct {
	Chain         Chain
	TxID          string
	Confirmed     bool
	Mempool       bool
	Confirmations uint64
	Height        *int64
	BlockHash     string
	BlockTime     *int64
	Inputs        []WalletTxInput
	Outputs       []WalletTxOutput
	FeeSatoshi    *uint64
	Size          int32
	Vsize         int32
}

type HistoryOptions struct {
	Page          int
	Limit         int
	ConfirmedOnly bool
	Sort          string
}

type WalletHistoryItem struct {
	TxID          string
	Direction     string
	IncomeSatoshi uint64
	SpendSatoshi  uint64
	NetSatoshi    int64
	Confirmed     bool
	Mempool       bool
	Confirmations *uint64
	Height        *int64
	Timestamp     int64
	Time          string
}

type WalletHistoryPage struct {
	Chain         Chain
	Address       string
	Page          int
	Limit         int
	ConfirmedOnly bool
	Sort          string
	Total         int64
	Items         []WalletHistoryItem
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

func NormalizeTxID(raw string) (string, bool) {
	txid := strings.ToLower(strings.TrimSpace(raw))
	if len(txid) != 64 {
		return "", false
	}
	for _, ch := range txid {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return "", false
		}
	}
	return txid, true
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
