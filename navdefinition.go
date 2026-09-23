package timeseries

import "math"

// -----------------------------------------------------------------------
// The NaV sentinel
// -----------------------------------------------------------------------
//
// NaV is a quiet NaN carrying a tag bit in its mantissa. IEEE-754 leaves
// 51 mantissa bits free in a quiet NaN, so a tag costs nothing: a NaV is
// an ordinary float64, math.IsNaN reports it as NaN, and any code written
// for NaN keeps working. Only IsNaV tells the two apart.
const (
	nanBits = 0x7FF8000000000000 // Quiet NaN, as produced by math.NaN().
	navTag  = 0x0001000000000000 // Our tag: mantissa bit 48.
	navBits = nanBits | navTag
)

// NaV ("Not a Value") marks a missing observation. See the package doc
// for the semantics; in short, NaV never stops a computation.
var NaV = math.Float64frombits(navBits)

// IsNaV reports whether x is the NaV sentinel. It is false for a plain
// NaN, for the infinities and for every ordinary number.
func IsNaV(x float64) bool {
	return math.IsNaN(x) && math.Float64bits(x)&navTag != 0
}

// IsStdNaN reports whether x is a NaN that is NOT a NaV, that is, the
// result of a broken computation rather than a missing observation.
func IsStdNaN(x float64) bool {
	return math.IsNaN(x) && !IsNaV(x)
}

// -----------------------------------------------------------------------
// The four operations
// -----------------------------------------------------------------------
//
// Every operation below applies the same two rules, in this order:
//
//  1. A plain NaN in either operand yields NaN. An error stays visible,
//     and it wins over a missing value.
//  2. A NaV is then handled per operation: neutral in Add, missing in
//     Sub, Mul and Div.
//
// The asymmetry between Add and Sub is deliberate. A sum tolerates an
// absent term, the way a monthly mean tolerates a missing day. A
// difference compares two values: if one of them is unknown, so is the
// difference. Returning the other operand would amount to reading the
// missing one as zero, and inventing a variation that was never
// measured.

// Add returns a+b, with NaV as the neutral element:
//
//	Add(NaV, 5)   → 5
//	Add(5, NaV)   → 5
//	Add(NaV, NaV) → NaV
//	Add(NaN, NaV) → NaN
func Add(a, b float64) float64 {
	if IsStdNaN(a) || IsStdNaN(b) {
		return math.NaN()
	}
	switch {
	case IsNaV(a) && IsNaV(b):
		return NaV
	case IsNaV(a):
		return b
	case IsNaV(b):
		return a
	}
	return a + b
}

// Sub returns a-b. A missing operand makes the difference missing:
//
//	Sub(NaV, 5)   → NaV
//	Sub(5, NaV)   → NaV
//	Sub(NaN, NaV) → NaN
func Sub(a, b float64) float64 {
	if IsStdNaN(a) || IsStdNaN(b) {
		return math.NaN()
	}
	if IsNaV(a) || IsNaV(b) {
		return NaV
	}
	return a - b
}

// Mul returns a*b. A missing factor makes the product missing:
//
//	Mul(NaV, 5)   → NaV
//	Mul(NaV, 0)   → NaV
//	Mul(NaN, NaV) → NaN
//
// Note that Mul(NaV, 0) is NaV and not 0: an unknown quantity of
// something may itself be unknown, and treating NaV as a neutral factor
// would amount to reading it as 1.
func Mul(a, b float64) float64 {
	if IsStdNaN(a) || IsStdNaN(b) {
		return math.NaN()
	}
	if IsNaV(a) || IsNaV(b) {
		return NaV
	}
	return a * b
}

// Div returns a/b, with the same rule as Mul:
//
//	Div(NaV, 5)   → NaV
//	Div(5, NaV)   → NaV
//	Div(NaV, 0)   → NaV
//	Div(NaN, NaV) → NaN
//
// When neither operand is NaN-class, IEEE-754 applies unchanged: a
// non-zero number divided by zero is ±Inf, and 0/0 is a plain NaN. The
// latter is a computation error, not a missing value, so it is NOT a
// NaV.
//
// # Why Div(NaV, 0) is NaV and not NaN
//
// The precedence of NaN over NaV applies to a NaN among the operands.
// Here there is none: there is a missing value and an ordinary zero.
// What the result would have been depends on what is missing — 5/0
// would be +Inf, 0/0 would be NaN — so nothing can be asserted, and
// that is precisely what NaV means. Calling it an error would claim a
// computation went wrong, when in fact a measurement was never made.
//
// This is not a corner case: it is what makes Mean(nil) and
// Mean([NaV, NaV]) return NaV rather than NaN, since Mean divides a sum
// by a count that is then zero. An empty series is not a bug.
func Div(a, b float64) float64 {
	if IsStdNaN(a) || IsStdNaN(b) {
		return math.NaN()
	}
	if IsNaV(a) || IsNaV(b) {
		return NaV
	}
	return a / b
}
