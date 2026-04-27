package timeseries

import (
	"math"
	"sort"
)

// Min returns the smallest non-NaN, non-NaV value of input. NaV and NaN
// entries are skipped. If input is empty or contains only NaN/NaV, Min
// returns (NaV, *Error) with Kind == KindEmptyInput. Min does not
// allocate and does not modify input.
func Min(input []float64) (float64, error) {
	minV := NaV
	seen := false
	for _, v := range input {
		if math.IsNaN(v) {
			continue
		}
		if !seen || v < minV {
			minV = v
			seen = true
		}
	}
	if !seen {
		return NaV, &Error{Op: "Min", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}
	return minV, nil
}

// Max returns the greatest non-NaN, non-NaV value of input. NaV and NaN
// entries are skipped. If input is empty or contains only NaN/NaV, Max
// returns (NaV, *Error) with Kind == KindEmptyInput. Max does not
// allocate and does not modify input.
func Max(input []float64) (float64, error) {
	maxV := NaV
	seen := false
	for _, v := range input {
		if math.IsNaN(v) {
			continue
		}
		if !seen || v > maxV {
			maxV = v
			seen = true
		}
	}
	if !seen {
		return NaV, &Error{Op: "Max", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}
	return maxV, nil
}

// StdDev returns the sample (unbiased) standard deviation of input,
// with divisor n-1 where n is the count of non-NaN, non-NaV values.
// NaN and NaV are skipped, matching the conventions of Sum and Mean.
//
// If input has fewer than 2 valid values, StdDev returns (NaV, *Error)
// with Kind == KindBounds and Value == n: the sample standard deviation
// is undefined in that case. If input is empty, it returns (NaV,
// *Error) with Kind == KindEmptyInput.
//
// StdDev does not allocate and does not modify input.
func StdDev(data []float64) (float64, error) {
	if len(data) == 0 {
		return NaV, &Error{Op: "StdDev", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}

	var sum float64
	n := 0
	for _, v := range data {
		if math.IsNaN(v) {
			continue
		}
		sum += v
		n++
	}
	if n < 2 {
		return NaV, &Error{
			Op:    "StdDev",
			Kind:  KindBounds,
			Msg:   "sample standard deviation undefined for fewer than 2 finite values",
			Field: "n",
			Value: n,
		}
	}
	mean := sum / float64(n)

	var sq float64
	for _, v := range data {
		if math.IsNaN(v) {
			continue
		}
		d := v - mean
		sq += d * d
	}
	return math.Sqrt(sq / float64(n-1)), nil
}

// Percentile returns the p-th percentile of input using the nearest-rank
// definition on the ascendingly sorted data (1-indexed rank k =
// ceil(p/100*n)). Valid p is in (0, 100].
//
// NaN and NaV values are skipped before sorting. If input is empty or
// contains only NaN/NaV, Percentile returns (NaV, *Error) with Kind ==
// KindEmptyInput. If p is out of bounds it returns (NaV, *Error) with
// Kind == KindBounds and Field == "p".
//
// Percentile allocates a copy of the (filtered) input and sorts the
// copy; the caller's slice is not modified.
func Percentile(x []float64, p float64) (float64, error) {
	if len(x) == 0 {
		return NaV, &Error{Op: "Percentile", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}
	if p <= 0 || p > 100 {
		return NaV, &Error{
			Op:    "Percentile",
			Kind:  KindBounds,
			Msg:   "p must be in (0, 100]",
			Field: "p",
			Value: p,
		}
	}

	cp := make([]float64, 0, len(x))
	for _, v := range x {
		if math.IsNaN(v) {
			continue
		}
		cp = append(cp, v)
	}
	n := len(cp)
	if n == 0 {
		return NaV, &Error{Op: "Percentile", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}

	sort.Float64s(cp)

	k := int(math.Floor(p / 100 * float64(n)))
	if k < 1 {
		return cp[0], nil
	}
	if k >= n {
		return cp[n-1], nil
	}
	return cp[k-1], nil
}

// Median returns the median of input, using the standard definition on
// the sorted (non-NaN, non-NaV) values: for an odd count n the middle
// value; for an even count n the mean of the two middle values.
//
// NaN and NaV values are skipped. If input is empty or contains only
// NaN/NaV, Median returns (NaV, *Error) with Kind == KindEmptyInput.
//
// Median allocates a sorted copy of the filtered input; the caller's
// slice is not modified.
func Median(input []float64) (float64, error) {
	if len(input) == 0 {
		return NaV, &Error{Op: "Median", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}

	cp := make([]float64, 0, len(input))
	for _, v := range input {
		if math.IsNaN(v) {
			continue
		}
		cp = append(cp, v)
	}
	n := len(cp)
	if n == 0 {
		return NaV, &Error{Op: "Median", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}

	sort.Float64s(cp)

	if n%2 == 1 {
		return cp[n/2], nil
	}
	return (cp[n/2-1] + cp[n/2]) / 2, nil
}
