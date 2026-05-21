package task

import "math"

// rebalanceEpsilon is the minimum gap between two adjacent fractional keys
// before a rebalance pass is required. When two neighbors are closer than
// this, the midpoint between them rounds to one of the endpoints in float64
// and the ordering would become indeterminate.
const rebalanceEpsilon = 1e-9

// Midpoint computes the new fractional key for an item being placed between
// the two visible neighbors before and after. Either pointer may be nil to
// represent a top (before == nil) or bottom (after == nil) edge.
//
// Behavior:
//
//	before == nil && after == nil → 0.0  (empty list)
//	before == nil                  → *after - 1.0
//	after  == nil                  → *before + 1.0
//	both non-nil                   → (*before + *after) / 2
func Midpoint(before, after *float64) float64 {
	switch {
	case before == nil && after == nil:
		return 0.0
	case before == nil:
		return *after - 1.0
	case after == nil:
		return *before + 1.0
	default:
		return (*before + *after) / 2.0
	}
}

// NeedsRebalance reports whether two adjacent keys are too close together to
// admit a meaningful midpoint. Callers should rebalance the affected list
// before computing a new Midpoint when this returns true.
func NeedsRebalance(a, b float64) bool {
	return math.Abs(b-a) < rebalanceEpsilon
}

// EvenSpread returns the canonical evenly spaced key sequence [0, 1, …, n-1]
// used by the rebalance pass to reassign keys across a list.
func EvenSpread(n int) []float64 {
	if n <= 0 {
		return nil
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i)
	}
	return out
}
