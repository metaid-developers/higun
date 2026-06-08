package wallet

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

func SatoshiToDecimalString(value uint64) string {
	if value == 0 {
		return "0"
	}
	whole := value / 100000000
	frac := value % 100000000
	return fmt.Sprintf("%d.%08d", whole, frac)
}

func SignedSatoshiToDecimalString(value int64) string {
	if value == -1<<63 {
		return "-92233720368.54775808"
	}
	if value < 0 {
		return "-" + SatoshiToDecimalString(uint64(-value))
	}
	return SatoshiToDecimalString(uint64(value))
}

func NormalizeUTXOs(input []WalletUTXO, opts UTXOOptions) ([]WalletUTXO, error) {
	sortOrder := opts.Sort
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "sort must be asc or desc")
	}

	byOutpoint := make(map[string]WalletUTXO, len(input))
	order := make([]string, 0, len(input))
	for _, utxo := range input {
		if opts.ConfirmedOnly && !utxo.Confirmed {
			continue
		}
		key := utxo.Outpoint()
		existing, exists := byOutpoint[key]
		if !exists {
			order = append(order, key)
			byOutpoint[key] = utxo
			continue
		}
		if !existing.Confirmed && utxo.Confirmed {
			byOutpoint[key] = utxo
		}
	}

	result := make([]WalletUTXO, 0, len(byOutpoint))
	for _, key := range order {
		if utxo, ok := byOutpoint[key]; ok {
			result = append(result, utxo)
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Satoshi == result[j].Satoshi {
			return result[i].Outpoint() < result[j].Outpoint()
		}
		if sortOrder == "asc" {
			return result[i].Satoshi < result[j].Satoshi
		}
		return result[i].Satoshi > result[j].Satoshi
	})
	return result, nil
}

func parseVout(value string) (int, error) {
	vout, err := strconv.Atoi(value)
	if err != nil || vout < 0 {
		return 0, NewHTTPWalletError(http.StatusBadGateway, CodeInvalidUpstream, "upstream utxo index is not an integer")
	}
	return vout, nil
}
