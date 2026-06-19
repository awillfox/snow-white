// Package scale converts fixed-point decimal strings to scaled int64 and back,
// without ever using float64 (no precision drift on money or asset amounts).
package scale

import (
	"fmt"
	"strings"
)

// Parse converts a decimal string to a scaled int64 with `decimals` places.
// Digits beyond `decimals` are truncated. Example: Parse("896789.99", 2) = 89678999.
func Parse(s string, decimals int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("scale: empty string")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	intPart, fracPart, _ := strings.Cut(s, ".")

	// Pad/truncate the fractional part to exactly `decimals` digits.
	if len(fracPart) < decimals {
		fracPart += strings.Repeat("0", decimals-len(fracPart))
	} else {
		fracPart = fracPart[:decimals]
	}

	digits := intPart + fracPart
	if digits == "" {
		return 0, fmt.Errorf("scale: no digits in %q", s)
	}
	var v int64
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("scale: invalid digit in %q", s)
		}
		v = v*10 + int64(r-'0')
	}
	if neg {
		v = -v
	}
	return v, nil
}

// Format renders a scaled int64 as a decimal string with `decimals` places.
func Format(v int64, decimals int) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d", v)
	if decimals == 0 {
		if neg {
			return "-" + s
		}
		return s
	}
	for len(s) <= decimals {
		s = "0" + s
	}
	split := len(s) - decimals
	out := s[:split] + "." + s[split:]
	if neg {
		out = "-" + out
	}
	return out
}
