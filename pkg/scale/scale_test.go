package scale

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in       string
		decimals int
		want     int64
		wantErr  bool
	}{
		{"896789.99000000", 2, 89678999, false},
		{"896789.99", 2, 89678999, false},
		{"0.00000000", 8, 0, false},
		{"1.23456789", 8, 123456789, false},
		{"100", 2, 10000, false},
		{"0.005", 2, 0, false},   // truncates beyond scale
		{"abc", 2, 0, true},
		{"", 2, 0, true},
	}
	for _, tc := range tests {
		got, err := Parse(tc.in, tc.decimals)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Parse(%q,%d) expected error", tc.in, tc.decimals)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q,%d): %v", tc.in, tc.decimals, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Parse(%q,%d) = %d, want %d", tc.in, tc.decimals, got, tc.want)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format(89678999, 2); got != "896789.99" {
		t.Errorf("Format = %q, want 896789.99", got)
	}
	if got := Format(123456789, 8); got != "1.23456789" {
		t.Errorf("Format = %q, want 1.23456789", got)
	}
	if got := Format(0, 2); got != "0.00" {
		t.Errorf("Format = %q, want 0.00", got)
	}
}

func TestParseRoundTripNoFloatDrift(t *testing.T) {
	// Large value that would lose precision via float64.
	const s = "999999999.99"
	v, err := Parse(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if v != 99999999999 {
		t.Fatalf("got %d", v)
	}
}
