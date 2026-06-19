package trader

import (
	"testing"

	"snow-white/internal/order"
)

func TestGuard(t *testing.T) {
	caps := Caps{MaxOrder: 5000_00, MaxDaily: 50000_00, MaxLoss: 10000_00}
	tests := []struct {
		name      string
		state     order.RiskState
		value     int64
		wantOK    bool
		wantHalt  bool
	}{
		{"ok", order.RiskState{}, 1000_00, true, false},
		{"halted", order.RiskState{Halted: true}, 100, false, false},
		{"over per-order", order.RiskState{}, 6000_00, false, false},
		{"over daily", order.RiskState{SpentToday: 48000_00}, 3000_00, false, false},
		{"loss stop trips halt", order.RiskState{LossToday: 10000_00}, 100, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Check(tc.state, caps, tc.value)
			if d.Allowed != tc.wantOK {
				t.Fatalf("Allowed = %v, want %v (%s)", d.Allowed, tc.wantOK, d.Reason)
			}
			if d.TripHalt != tc.wantHalt {
				t.Fatalf("TripHalt = %v, want %v", d.TripHalt, tc.wantHalt)
			}
		})
	}
}
