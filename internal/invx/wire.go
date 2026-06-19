package invx

import (
	"encoding/json"

	"snow-white/pkg/scale"
)

// decimalNumber renders a scaled int64 as an exact-decimal json.Number, so it
// marshals as a bare number (e.g. 70.00, 0.10000000) with no float64 rounding.
func decimalNumber(scaledInt int64, decimals int) json.Number {
	return json.Number(scale.Format(scaledInt, decimals))
}
