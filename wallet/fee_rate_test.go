package wallet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStandardFeeRateResponse(t *testing.T) {
	body := marshalResponse(t, NewStandardFeeRateResponse(ChainBTC, FeeRate{
		Source:  FeeRateSourceConfig,
		Unit:    FeeRateUnitSatPerByte,
		Slow:    1,
		Normal:  3,
		Fast:    5,
		Default: "normal",
	}))

	for _, want := range []string{
		`"chain":"btc"`,
		`"source":"config"`,
		`"unit":"sat_per_byte"`,
		`"slow":1`,
		`"normal":3`,
		`"fast":5`,
		`"default":"normal"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fee-rate response missing %s: %s", want, body)
		}
	}

	var payload struct {
		Data struct {
			Slow int64 `json:"slow"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Data.Slow != 1 {
		t.Fatalf("slow = %d, want 1", payload.Data.Slow)
	}
}
