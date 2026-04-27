package timeseries

import (
	"fmt"
	"math"
)

// -----------------------------------------------------------------------
// NaV: Not-a-Value — a tagged NaN for "missing by nature"
// -----------------------------------------------------------------------
//
// The library distinguishes two kinds of invalid float64:
//
//   NaV            — the value has never been observed (sensor offline,
//                    row absent in a join, bucket empty, field not
//                    transmitted, point rejected as an outlier, ...).
//                    It is a business-level statement about data
//                    provenance: "we have nothing to say here".
//
//   math.NaN()     — the result of an arithmetic error (0/0, log(-1),
//                    Inf-Inf, ...). It is a numerical accident; the
//                    computation broke, so any downstream use is
//                    poisoned until explicitly cleaned.
//
// Both are NaN-class (math.IsNaN returns true for either) so they are
// transparently handled by IEEE-754 arithmetic and by existing Go code
// that guards against NaN. The distinction is carried in the bits of the
// mantissa: NaV sets a dedicated tag bit that plain NaN does not. Tests:
//
//   IsNaV(NaV)           → true
//   IsNaV(math.NaN())    → false
//   math.IsNaN(NaV)      → true
//   math.IsNaN(NaN)      → true
//
// -----------------------------------------------------------------------
// Arithmetic semantics: STRICT PROPAGATION
// -----------------------------------------------------------------------
//
// Element-wise operations (Add, Sub, Mul, Div) propagate NaV and NaN
// unconditionally:
//
//   Add(NaV, 5)       → NaV
//   Add(NaV, NaV)     → NaV
//   Sub(NaV, 5)       → NaV    (not -5)
//   Sub(5, NaV)       → NaV    (not 5)
//   Mul(NaV, 5)       → NaV
//   Div(NaV, 5)       → NaV
//
// Rationale: "unknown plus five" is unknown. This matches the semantics
// of pandas element-wise operators, NumPy ufuncs (without skipna), R
// (with default na.rm=FALSE), and SQL NULL arithmetic. Anything else
// silently conjures values out of thin air, which is exactly what a
// library about honest statistics must not do.
//
// If both operands are non-NaN, the operation is a plain IEEE-754 +, -,
// *, /. No branches are taken on the hot path past the NaN guard.
//
// -----------------------------------------------------------------------
// Aggregate semantics: SKIP-NaV by default, STRICT via *Strict
// -----------------------------------------------------------------------
//
// Reductions (Sum, Mean, and the equivalents in computewithnav.go:
// Median, StdDev, Min, Max) skip NaN and NaV by default. They return NaV
// only when the input is empty or contains no valid value:
//
//   Mean([]float64{1, 2, NaV, 3})   → 2   (mean of {1, 2, 3})
//   Mean([]float64{NaV, NaV})       → NaV
//   Mean([]float64{})               → NaV
//
// Rationale: the typical user wants "the mean of what I have", not a
// NaV-poisoned scalar. This matches pandas .mean() (skipna=True is the
// default) and numpy.nanmean.
//
// Callers who want strict propagation at the aggregate level (i.e. "tell
// me NaV if ANY value is NaV") should use the *Strict variants:
//
//   SumStrict([]float64{1, 2, NaV, 3})   → NaV
//   MeanStrict([]float64{1, 2, NaV, 3})  → NaV
//
// -----------------------------------------------------------------------
// Why NaN-boxing, concretely
// -----------------------------------------------------------------------
//
// IEEE-754 defines two flavors of NaN: "quiet" NaN (qNaN) and "signaling"
// NaN (sNaN). Both have the NaN pattern in the exponent (all ones) and a
// non-zero mantissa; they differ by the high bit of the mantissa:
//
//   - quiet NaN: high mantissa bit = 1. Propagates silently through
//     arithmetic; no exception is raised. This is the normal NaN a Go
//     programmer ever encounters — math.NaN() returns a qNaN, and any
//     invalid operation (0/0, log(-1), …) produces a qNaN.
//   - signaling NaN: high mantissa bit = 0 (but the rest of the mantissa
//     non-zero). When used in arithmetic it is supposed to raise the
//     floating-point invalid-operation signal. In most modern runtimes —
//     Go included — signaling NaNs are either never produced or get
//     silently promoted to quiet ones on the first arithmetic use, so
//     you can treat them as a niche curiosity. We never deal with sNaN
//     in this package.
//
// A quiet NaN therefore leaves 51 free bits in its mantissa to stash
// whatever payload we want. NaV sets bit 48 (the 'navTag' constant below)
// to distinguish itself from every other quiet NaN.
//
// This means:
//   - zero memory overhead per point (no extra byte, no extra field);
//   - full binary compatibility with any Go code that reads a float64;
//   - math.IsNaN(NaV) is true, so generic NaN-aware code already "sees"
//     missingness without being recompiled.
//
// The navTag bit position is an implementation detail but is kept stable
// so serialized NaV survive a round-trip via math.Float64bits /
// Float64frombits. Do not rely on the exact constant; use IsNaV.
//
// Future extension: the remaining free bits of the mantissa are open for
// sub-tagging — for example, to distinguish "missing" from "interpolated"
// from "rejected" — without breaking backward compatibility.

// Bit layout constants. We use the 52 bits of mantissa in a quiet NaN to
// encode the NaV tag.
const (
	nanBits = 0x7FF8000000000000 // Standard quiet NaN (bit 51 = 1).
	navTag  = 0x0001000000000000 // Our NaV tag (bit 48 = 1).
	navBits = nanBits | navTag   // The bit pattern of the NaV sentinel.
)

// NaV ("Not-a-Value") is the sentinel returned for "missing by nature"
// float64 values. See the package-level doc for the full semantics.
var NaV = math.Float64frombits(navBits)

// IsNaV reports whether x is exactly the NaV sentinel (a quiet NaN with
// the NaV tag bit set). IsNaV returns false for plain NaN, infinities,
// and all regular numbers.
func IsNaV(x float64) bool {
	return math.IsNaN(x) && math.Float64bits(x)&navTag != 0
}

// IsStdNaN reports whether x is a NaN-class float64 that is NOT a NaV,
// i.e. a "plain" NaN produced by an arithmetic error. Use it to audit
// computations for numerical accidents without conflating them with
// missing data.
func IsStdNaN(x float64) bool {
	return math.IsNaN(x) && !IsNaV(x)
}

// -----------------------------------------------------------------------
// Element-wise arithmetic: strict propagation
// -----------------------------------------------------------------------

// Add returns a+b, propagating NaV and NaN. If either operand is NaN-class,
// the result is NaV if that operand is NaV, else math.NaN().
//
// The preference for NaV over NaN on propagation is intentional: a missing
// input explains the absence of the output more accurately than a
// generic "not a number" would.
func Add(a, b float64) float64 {
	if math.IsNaN(a) || math.IsNaN(b) {
		if IsNaV(a) || IsNaV(b) {
			return NaV
		}
		return math.NaN()
	}
	return a + b
}

// Sub returns a-b with the same propagation rules as Add.
func Sub(a, b float64) float64 {
	if math.IsNaN(a) || math.IsNaN(b) {
		if IsNaV(a) || IsNaV(b) {
			return NaV
		}
		return math.NaN()
	}
	return a - b
}

// Mul returns a*b with the same propagation rules as Add.
func Mul(a, b float64) float64 {
	if math.IsNaN(a) || math.IsNaN(b) {
		if IsNaV(a) || IsNaV(b) {
			return NaV
		}
		return math.NaN()
	}
	return a * b
}

// Div returns a/b with the same propagation rules as Add. Division by a
// non-NaN zero follows the usual IEEE-754 rules: ±Inf for non-zero
// numerator, and math.NaN() for 0/0 (not NaV — a computation error, not
// missingness).
func Div(a, b float64) float64 {
	if math.IsNaN(a) || math.IsNaN(b) {
		if IsNaV(a) || IsNaV(b) {
			return NaV
		}
		return math.NaN()
	}
	return a / b
}

// -----------------------------------------------------------------------
// Aggregates: skip-NaV by default
// -----------------------------------------------------------------------

// Sum returns the sum of xs, skipping NaN-class values. It returns NaV if
// xs is empty or contains only NaN/NaV values.
func Sum(xs []float64) float64 {
	var acc float64
	seen := false
	for _, x := range xs {
		if math.IsNaN(x) {
			continue
		}
		acc += x
		seen = true
	}
	if !seen {
		return NaV
	}
	return acc
}

// Mean returns the arithmetic mean of xs, skipping NaN-class values. It
// returns NaV if xs is empty or contains only NaN/NaV values.
func Mean(xs []float64) float64 {
	var sum float64
	n := 0
	for _, x := range xs {
		if math.IsNaN(x) {
			continue
		}
		sum += x
		n++
	}
	if n == 0 {
		return NaV
	}
	return sum / float64(n)
}

// -----------------------------------------------------------------------
// Aggregates: strict propagation (NaV if any input is NaN-class)
// -----------------------------------------------------------------------

// SumStrict returns the sum of xs, propagating any NaN-class value: if any
// xi is NaV or NaN, SumStrict returns NaV (for NaV) or math.NaN() (for a
// plain NaN, with NaV winning on ties). Use it when "missing-in, missing-
// out" is required.
func SumStrict(xs []float64) float64 {
	if len(xs) == 0 {
		return NaV
	}
	hadNaV := false
	hadNaN := false
	var acc float64
	for _, x := range xs {
		if IsNaV(x) {
			hadNaV = true
			continue
		}
		if math.IsNaN(x) {
			hadNaN = true
			continue
		}
		acc += x
	}
	switch {
	case hadNaV:
		return NaV
	case hadNaN:
		return math.NaN()
	default:
		return acc
	}
}

// MeanStrict is to Mean what SumStrict is to Sum.
func MeanStrict(xs []float64) float64 {
	if len(xs) == 0 {
		return NaV
	}
	for _, x := range xs {
		if IsNaV(x) {
			return NaV
		}
		if math.IsNaN(x) {
			return math.NaN()
		}
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// -----------------------------------------------------------------------
// Miscellaneous helpers
// -----------------------------------------------------------------------

// Coalesce returns the first non-NaN, non-NaV value from xs, or NaV if
// none exists. Matches the behavior of SQL COALESCE on NULLs.
func Coalesce(xs ...float64) float64 {
	for _, x := range xs {
		if !math.IsNaN(x) {
			return x
		}
	}
	return NaV
}

// Format renders a float64 as a human-readable string, with NaV and plain
// NaN clearly distinguished.
func Format(x float64) string {
	if IsNaV(x) {
		return "NaV"
	}
	if math.IsNaN(x) {
		return "NaN"
	}
	return fmt.Sprintf("%g", x)
}

// DebugBits returns the binary layout of x for debugging (sign, exponent,
// mantissa). Useful to verify the NaV tag is preserved through a
// roundtrip.
func DebugBits(x float64) string {
	b := math.Float64bits(x)
	return fmt.Sprintf("sign=%d exp=%011b mantissa=%052b",
		b>>63,
		(b>>52)&0x7FF,
		b&0x000FFFFFFFFFFFFF,
	)
}

// Lift converts a plain float64 into the NaV-safe world by calling the
// provided isAbsent predicate. If the predicate returns true, NaV is
// returned; otherwise the input is returned unchanged. Useful to ingest
// external sources that encode missingness with ad-hoc sentinels.
func Lift(x float64, isAbsent func(float64) bool) float64 {
	if isAbsent(x) {
		return NaV
	}
	return x
}

// LiftSentinel is a shortcut for Lift when the external source uses a
// single sentinel value (e.g. -999, -1) to mark missing data.
func LiftSentinel(x, sentinel float64) float64 {
	if x == sentinel {
		return NaV
	}
	return x
}
