package indicator

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestSMA(t *testing.T) {
	in := []int64{10, 20, 30, 40}
	got := SMA(in, 2)
	if len(got) != 4 {
		t.Fatalf("len = %d", len(got))
	}
	if IsWarm(got[0]) {
		t.Errorf("got[0] should be warm-up NaN")
	}
	if !approx(got[1], 15) || !approx(got[2], 25) || !approx(got[3], 35) {
		t.Errorf("SMA = %v", got)
	}
}

func TestSMAConstantSeries(t *testing.T) {
	in := []int64{7, 7, 7, 7, 7}
	got := SMA(in, 3)
	for i := 2; i < len(got); i++ {
		if !approx(got[i], 7) {
			t.Errorf("SMA of constant = %v at %d", got[i], i)
		}
	}
}

func TestEMAFirstValueIsSMA(t *testing.T) {
	in := []int64{10, 20, 30, 40, 50}
	got := EMA(in, 3)
	// First defined EMA (index period-1) equals SMA of first `period` values.
	if !approx(got[2], 20) {
		t.Errorf("EMA[2] = %v, want 20 (SMA seed)", got[2])
	}
	if IsWarm(got[1]) {
		t.Errorf("EMA[1] must be NaN warm-up")
	}
}

func TestRSIAllGainsIs100(t *testing.T) {
	in := []int64{1, 2, 3, 4, 5, 6}
	got := RSI(in, 3)
	last := got[len(got)-1]
	if !approx(last, 100) {
		t.Errorf("RSI of monotonic rise = %v, want 100", last)
	}
}
