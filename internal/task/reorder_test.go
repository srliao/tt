package task

import (
	"math"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestMidpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		before *float64
		after  *float64
		want   float64
	}{
		{"both_nil_returns_zero", nil, nil, 0.0},
		{"before_nil_top_edge", nil, ptr(1.0), 0.0},
		{"after_nil_bottom_edge", ptr(5.0), nil, 6.0},
		{"between_one_and_three", ptr(1.0), ptr(3.0), 2.0},
		{"between_close_neighbors", ptr(1.0), ptr(1.5), 1.25},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Midpoint(tc.before, tc.after)
			if math.Abs(got-tc.want) > 1e-12 {
				t.Fatalf("Midpoint(%v, %v) = %v, want %v", tc.before, tc.after, got, tc.want)
			}
		})
	}
}

func TestNeedsRebalance(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b float64
		want bool
	}{
		// Spec §3: when adjacent keys are within rebalanceEpsilon (1e-9) of
		// each other, rebalance is required. A gap of 1e-10 is < 1e-9 so
		// rebalance is needed; a gap of 1e-12 is also < 1e-9 so rebalance is
		// needed; a gap of 1.0 is well above the threshold.
		{"tight_gap_1e-10_rebalance", 1.0, 1.0 + 1e-10, true},
		{"tight_gap_1e-12_rebalance", 1.0, 1.0 + 1e-12, true},
		{"gap_of_one_is_healthy", 1.0, 2.0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NeedsRebalance(tc.a, tc.b)
			if got != tc.want {
				t.Fatalf("NeedsRebalance(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
