package timeseries

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// ErrPercentileRange is returned by Percentile when its rank argument
// lies outside (0, 100]. Test for it with errors.Is.
var ErrPercentileRange = errors.New("percentile rank must be in (0, 100]")

// -----------------------------------------------------------------------
// Aggregates
// -----------------------------------------------------------------------
//
// Every aggregate in this file obeys the same three rules, which follow
// from the package doc:
//
//  1. NaV values are skipped. A missing day does not turn a monthly mean
//     into an error; the result covers the values actually observed.
//  2. A plain NaN propagates: if any input is the result of a broken
//     computation, so is the aggregate. The error stays visible.
//  3. With no value left to work on — an empty input, or one made of
//     nothing but NaV — the result is NaV.
//
// Rule 2 is checked first, so a NaN wins over a NaV here as it does in
// arithmetic.
//
// Infinities are ordinary numbers for these functions: they are neither
// missing nor erroneous, and they propagate through sums and means the
// way IEEE-754 prescribes.
//
// None of these functions modifies its input, and none returns an error
// for lack of data: "nothing to say" is what NaV means. The single
// exception is Percentile, whose rank argument can be plainly wrong —
// that is a programming mistake, not a property of the data.

// -----------------------------------------------------------------------
// Inventories
// -----------------------------------------------------------------------
//
// The four functions below describe the contents of a slice rather than
// compute on its values. Nothing can break in a tally, so they never
// propagate a NaN and always return an int.
//
// CountUsable, CountNaV and CountNaN partition the slice: their sum is
// always len(xs). CountNonNaV is the combination of the first and the
// third.

// CountUsable returns how many entries of xs are real numbers, that is,
// neither NaV nor NaN. Use it to judge how much a result is worth: a
// mean over 3 usable points out of 30 is not the same statement as a
// mean over 30.
func CountUsable(xs []float64) int {
	n := 0
	for _, x := range xs {
		if !math.IsNaN(x) {
			n++
		}
	}
	return n
}

// CountNaV returns how many entries of xs are NaV, that is, observations
// that were never made. Plain NaN values are NOT counted here: they are
// computation errors, not missing data.
func CountNaV(xs []float64) int {
	n := 0
	for _, x := range xs {
		if IsNaV(x) {
			n++
		}
	}
	return n
}

// CountNaN returns how many entries of xs are plain NaN, that is,
// results of a broken computation. A non-zero count is worth
// investigating: it points at an error upstream, not at a gap in the
// data.
func CountNaN(xs []float64) int {
	n := 0
	for _, x := range xs {
		if IsStdNaN(x) {
			n++
		}
	}
	return n
}

// CountNonNaV returns how many entries of xs are anything but NaV, that
// is, usable numbers and plain NaN together. It answers "how much was
// transmitted, broken or not", whereas CountUsable answers "how much can
// be computed with".
func CountNonNaV(xs []float64) int {
	n := 0
	for _, x := range xs {
		if !IsNaV(x) {
			n++
		}
	}
	return n
}

// -----------------------------------------------------------------------
// One pass, one place where the rules live
// -----------------------------------------------------------------------
//
// Sum, Mean, Min, Max and Bounds all need the same facts about a slice:
// is there an error in it, how many values are usable, what is their
// total, and what are the extremes. scanUsable gathers all of it in a
// single traversal, and verdict turns rules 2 and 3 into the one place
// where they are implemented. The public functions then read the fields
// they need.

// scan holds everything a single traversal can tell about a slice.
type scan struct {
	hasNaN   bool    // a plain NaN was met: the aggregate must propagate it
	n        int     // count of usable values
	total    float64 // their sum
	min, max float64 // their extremes, meaningless when n == 0
}

// scanUsable traverses xs once. It stops early on a plain NaN, since
// every aggregate propagates it and nothing else needs to be known.
func scanUsable(xs []float64) scan {
	var s scan
	for _, x := range xs {
		if math.IsNaN(x) {
			if !IsNaV(x) {
				return scan{hasNaN: true}
			}
			continue // a NaV is skipped, silently, by design
		}
		s.total += x
		if s.n == 0 {
			s.min, s.max = x, x
		} else if x < s.min {
			s.min = x
		} else if x > s.max {
			s.max = x
		}
		s.n++
	}
	return s
}

// verdict reports the result imposed by rule 2 (an error propagates) or
// rule 3 (nothing usable gives NaV). When forced is false, the caller
// computes its own answer from the scan.
func (s scan) verdict() (value float64, forced bool) {
	switch {
	case s.hasNaN:
		return math.NaN(), true
	case s.n == 0:
		return NaV, true
	default:
		return 0, false
	}
}

// usableValues returns a new slice holding the usable values of xs, in
// their original order, along with the verdict of the same traversal.
// Used by the aggregates that must sort their input. The caller's slice
// is never modified.
func usableValues(xs []float64) ([]float64, scan) {
	var s scan
	out := make([]float64, 0, len(xs))
	for _, x := range xs {
		if math.IsNaN(x) {
			if !IsNaV(x) {
				return nil, scan{hasNaN: true}
			}
			continue
		}
		out = append(out, x)
	}
	s.n = len(out)
	return out, s
}

// Sum returns the sum of the usable values of xs.
//
//	Sum([1, 2, NaV, 3]) → 6
//	Sum([NaV, NaV])     → NaV
//	Sum([1, NaN, 3])    → NaN
func Sum(xs []float64) float64 {
	s := scanUsable(xs)
	if v, forced := s.verdict(); forced {
		return v
	}
	return s.total
}

// Mean returns the arithmetic mean of the usable values of xs. The
// divisor is the number of usable values, not the length of xs: this is
// what makes a missing day harmless.
//
//	Mean([1, 2, NaV, 3]) → 2
//	Mean([NaV, NaV])     → NaV
//	Mean([1, NaN, 3])    → NaN
func Mean(xs []float64) float64 {
	// The sum and the count come from the same traversal, so the slice
	// is walked once. Going through Sum and CountUsable would read it
	// twice for the same answer.
	s := scanUsable(xs)
	if v, forced := s.verdict(); forced {
		return v
	}
	return s.total / float64(s.n)
}

// Min returns the smallest usable value of xs, or NaV if there is none.
func Min(xs []float64) float64 {
	minV, _ := Bounds(xs)
	return minV
}

// Max returns the greatest usable value of xs, or NaV if there is none.
func Max(xs []float64) float64 {
	_, maxV := Bounds(xs)
	return maxV
}

// Bounds returns the smallest and the greatest usable values of xs in a
// single pass. Both are NaV when xs holds no usable value, and both are
// NaN when xs holds a plain NaN.
func Bounds(xs []float64) (minV, maxV float64) {
	s := scanUsable(xs)
	if v, forced := s.verdict(); forced {
		return v, v
	}
	return s.min, s.max
}

// Median returns the median of the usable values of xs. It is defined as
// Percentile(xs, 50), so it always returns a value that was actually
// observed. It returns NaV when no value is usable, and NaN when xs
// holds a plain NaN.
//
// # A deliberate departure from the textbook definition
//
// On an even count, the classical median is the mean of the two middle
// values: 25 for [10, 20, 30, 40]. This function returns 20, the lower
// of the two.
//
// The reason is that a measured series is not always a continuum. On an
// ON/OFF signal coded 0 and 1, the classical median can return 0.5, a
// state the equipment never occupied. On monthly counts it can invent a
// fraction of a month. Averaging two observations manufactures a value
// that was never measured, and a library about honest data must not do
// that.
//
// The practical consequences: on an even count this median is slightly
// biased low, and it differs from the default of numpy, R and pandas.
// Anyone cross-checking a result against those tools will see the gap on
// even counts, and it is intended.
//
// Median sorts a copy; the caller's slice keeps its order.
func Median(xs []float64) float64 {
	// p = 50 is always in range, so the error cannot occur here.
	med, _ := Percentile(xs, 50)
	return med
}

// Percentile returns the p-th percentile of the usable values of xs,
// using the nearest-rank definition: the value at rank ceil(p/100*n) of
// the ascending order, where n is the count of usable values. Percentile
// returns NaV when no value is usable, and NaN when xs holds a plain NaN.
//
// The returned value is therefore always one that was actually
// observed, never an interpolation between two of them. Median is
// defined as Percentile(xs, 50) for that very reason; see its
// documentation for what that implies on an even count.
//
// p must lie in (0, 100]. Any other value is a programming mistake
// rather than a property of the data, so it is the one case in this file
// reported as an error; the returned float64 is then NaV.
//
// Percentile sorts a copy; the caller's slice keeps its order.
func Percentile(xs []float64, p float64) (float64, error) {
	if p <= 0 || p > 100 || math.IsNaN(p) {
		return NaV, fmt.Errorf("%w, got %g", ErrPercentileRange, p)
	}
	usable, s := usableValues(xs)
	if v, forced := s.verdict(); forced {
		return v, nil
	}
	n := len(usable)
	sort.Float64s(usable)

	rank := int(math.Ceil(p / 100 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return usable[rank-1], nil
}

// StdDev returns the sample standard deviation of the usable values of
// xs, with divisor n-1. The sample form is the one that applies to a
// series of measurements: the values at hand are a sample of the signal,
// not the whole of it.
//
// StdDev returns NaV when fewer than two values are usable — a single
// point says nothing about dispersion — and NaN when xs holds a plain
// NaN.
//
// Dispersion cannot be had in a single traversal without paying for it
// elsewhere: the mean must be known before the deviations can be
// squared. StdDev therefore reads the slice twice — once for the mean,
// once for the deviations — but copies nothing and allocates nothing.
//
// The alternatives were measured on 100 000 points and rejected:
// Welford's streaming method needs one division per value and came out
// 40% slower for an identical result, its advantage lying in cases
// where the data arrives point by point; and the sum-of-squares
// identity, which does fit in one pass, subtracts two large and nearly
// equal numbers when the values are far from zero — a daily temperature
// in kelvin, say — and loses precision doing so.
func StdDev(xs []float64) float64 {
	s := scanUsable(xs)
	if v, forced := s.verdict(); forced {
		return v
	}
	if s.n < 2 {
		return NaV
	}
	mean := s.total / float64(s.n)

	var squares float64
	for _, x := range xs {
		if math.IsNaN(x) {
			continue // NaV: already excluded from the mean and the count
		}
		d := x - mean
		squares += d * d
	}
	return math.Sqrt(squares / float64(s.n-1))
}
