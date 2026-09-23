package timeseries

import (
	"errors"
	"math"
	"testing"
)

// The helpers want/check/describe live in navdefinition_test.go.

// closeTo asserts that got is within tol of expected. Used where the
// exact binary result depends on the order of the floating-point
// operations, as in StdDev.
func closeTo(t *testing.T, op string, got, expected, tol float64) {
	t.Helper()
	if math.Abs(got-expected) > tol {
		t.Errorf("%s = %s, want %v (± %v)", op, describe(got), expected, tol)
	}
}

// -----------------------------------------------------------------------
// Inventories
// -----------------------------------------------------------------------

func TestCounters(t *testing.T) {
	xs := []float64{1, 2, NaV, math.NaN(), 5, NaV}

	if got := CountUsable(xs); got != 3 {
		t.Errorf("CountUsable = %d, want 3", got)
	}
	if got := CountNaV(xs); got != 2 {
		t.Errorf("CountNaV = %d, want 2", got)
	}
	if got := CountNaN(xs); got != 1 {
		t.Errorf("CountNaN = %d, want 1", got)
	}
	// Everything but NaV: the usable numbers plus the broken one.
	if got := CountNonNaV(xs); got != 4 {
		t.Errorf("CountNonNaV = %d, want 4", got)
	}
}

// The three exclusive counters must account for every entry, always.
func TestCountersPartitionTheSlice(t *testing.T) {
	slices := [][]float64{
		{},
		{1, 2, 3},
		{NaV, NaV},
		{math.NaN()},
		{1, NaV, math.NaN(), math.Inf(1), math.Inf(-1), 0},
	}
	for _, xs := range slices {
		sum := CountUsable(xs) + CountNaV(xs) + CountNaN(xs)
		if sum != len(xs) {
			t.Errorf("on %v: CountUsable+CountNaV+CountNaN = %d, want len = %d",
				xs, sum, len(xs))
		}
		if got, want := CountNonNaV(xs), CountUsable(xs)+CountNaN(xs); got != want {
			t.Errorf("on %v: CountNonNaV = %d, want CountUsable+CountNaN = %d", xs, got, want)
		}
	}
}

// An infinity is an ordinary number for the counters.
func TestCountUsableCountsInfinities(t *testing.T) {
	if got := CountUsable([]float64{math.Inf(1), math.Inf(-1)}); got != 2 {
		t.Errorf("CountUsable([+Inf, -Inf]) = %d, want 2", got)
	}
}

// -----------------------------------------------------------------------
// Sum and Mean
// -----------------------------------------------------------------------

func TestSum(t *testing.T) {
	cases := []struct {
		name  string
		in    []float64
		want  want
		value float64
	}{
		{"ordinary", []float64{1, 2, 3}, wantNum, 6},
		{"a missing value is skipped", []float64{1, 2, NaV, 3}, wantNum, 6},
		{"nothing but missing values", []float64{NaV, NaV}, wantNaV, 0},
		{"empty", []float64{}, wantNaV, 0},
		{"nil", nil, wantNaV, 0},
		{"a single value", []float64{7}, wantNum, 7},
		// Rule 2: an error contaminates the result and stays visible.
		{"an error propagates", []float64{1, math.NaN(), 3}, wantNaN, 0},
		{"an error wins over a missing value", []float64{NaV, math.NaN()}, wantNaN, 0},
		// Infinities are ordinary numbers here.
		{"infinity", []float64{1, math.Inf(1)}, wantNum, math.Inf(1)},
	}
	for _, c := range cases {
		check(t, "Sum "+c.name, Sum(c.in), c.want, c.value)
	}
}

func TestMean(t *testing.T) {
	cases := []struct {
		name  string
		in    []float64
		want  want
		value float64
	}{
		{"ordinary", []float64{1, 2, 3}, wantNum, 2},
		// The divisor is 3, not 4: this is what makes a missing day harmless.
		{"a missing value is not counted in the divisor", []float64{1, 2, NaV, 3}, wantNum, 2},
		{"nothing but missing values", []float64{NaV, NaV, NaV}, wantNaV, 0},
		{"empty", []float64{}, wantNaV, 0},
		{"a single value", []float64{7}, wantNum, 7},
		{"an error propagates", []float64{1, math.NaN(), 3}, wantNaN, 0},
		{"an error wins over a missing value", []float64{1, NaV, math.NaN()}, wantNaN, 0},
	}
	for _, c := range cases {
		check(t, "Mean "+c.name, Mean(c.in), c.want, c.value)
	}
}

// The point of the whole library, stated as a test: one missing month
// out of twelve must not turn a yearly mean into an error.
func TestMeanSurvivesAMissingMonth(t *testing.T) {
	year := []float64{10, 12, 14, 16, 18, 20, NaV, 20, 18, 16, 14, 12}
	got := Mean(year)
	if IsNaV(got) || math.IsNaN(got) {
		t.Fatalf("Mean over a year with one missing month = %s, want a number", describe(got))
	}
	closeTo(t, "Mean over 11 observed months", got, 170.0/11.0, 1e-12)
}

// -----------------------------------------------------------------------
// Min, Max, Bounds
// -----------------------------------------------------------------------

func TestBounds(t *testing.T) {
	cases := []struct {
		name         string
		in           []float64
		wantKind     want
		wantMinValue float64
		wantMaxValue float64
	}{
		{"ordinary", []float64{3, 1, 4, 1, 5}, wantNum, 1, 5},
		{"missing values are skipped", []float64{3, NaV, 4, NaV, 5}, wantNum, 3, 5},
		{"negative values", []float64{-3, -1, -4}, wantNum, -4, -1},
		{"a single value", []float64{7}, wantNum, 7, 7},
		{"nothing but missing values", []float64{NaV, NaV}, wantNaV, 0, 0},
		{"empty", []float64{}, wantNaV, 0, 0},
		{"an error propagates to both bounds", []float64{1, math.NaN(), 5}, wantNaN, 0, 0},
	}
	for _, c := range cases {
		minV, maxV := Bounds(c.in)
		check(t, "Bounds "+c.name+" (min)", minV, c.wantKind, c.wantMinValue)
		check(t, "Bounds "+c.name+" (max)", maxV, c.wantKind, c.wantMaxValue)

		// Min and Max must agree with Bounds in every case.
		check(t, "Min "+c.name, Min(c.in), c.wantKind, c.wantMinValue)
		check(t, "Max "+c.name, Max(c.in), c.wantKind, c.wantMaxValue)
	}
}

// A NaV in first position used to poison the comparison chain. It must
// not.
func TestBoundsWithLeadingNaV(t *testing.T) {
	minV, maxV := Bounds([]float64{NaV, 10, 20})
	check(t, "Bounds([NaV, 10, 20]) (min)", minV, wantNum, 10)
	check(t, "Bounds([NaV, 10, 20]) (max)", maxV, wantNum, 20)
}

// -----------------------------------------------------------------------
// Median and Percentile
// -----------------------------------------------------------------------

func TestMedian(t *testing.T) {
	cases := []struct {
		name  string
		in    []float64
		want  want
		value float64
	}{
		{"odd count", []float64{10, 20, 30, 40, 50}, wantNum, 30},
		// Deliberate: the lower of the two middle values, not their mean.
		{"even count returns an observed value", []float64{10, 20, 30, 40}, wantNum, 20},
		// An ON/OFF signal must never be reported as half on.
		{"ON/OFF signal", []float64{0, 0, 1, 1}, wantNum, 0},
		{"unsorted input", []float64{40, 10, 50, 20, 30}, wantNum, 30},
		{"missing values are skipped", []float64{10, NaV, 30, 40, 50}, wantNum, 30},
		{"nothing but missing values", []float64{NaV, NaV}, wantNaV, 0},
		{"empty", []float64{}, wantNaV, 0},
		{"an error propagates", []float64{10, math.NaN(), 30}, wantNaN, 0},
	}
	for _, c := range cases {
		check(t, "Median "+c.name, Median(c.in), c.want, c.value)
	}
}

// Median is defined as Percentile(50); the two must never diverge.
func TestMedianEqualsPercentile50(t *testing.T) {
	slices := [][]float64{
		{10, 20, 30, 40},
		{10, 20, 30, 40, 50},
		{0, 0, 1, 1},
		{7},
		{5, NaV, 9, NaV, 1},
	}
	for _, xs := range slices {
		p, err := Percentile(xs, 50)
		if err != nil {
			t.Fatalf("Percentile(%v, 50) returned an error: %v", xs, err)
		}
		if med := Median(xs); med != p {
			t.Errorf("on %v: Median = %s but Percentile(50) = %s", xs, describe(med), describe(p))
		}
	}
}

func TestPercentile(t *testing.T) {
	xs := []float64{10, 20, 30, 40, 50}
	cases := []struct {
		p    float64
		want float64
	}{
		// Nearest rank: the smallest value with at least p% of the data
		// at or below it.
		{50, 30},
		{100, 50},
		{80, 40},
		{20, 10},
		{1, 10},
		{99, 50},
	}
	for _, c := range cases {
		got, err := Percentile(xs, c.p)
		if err != nil {
			t.Errorf("Percentile(%g) returned an error: %v", c.p, err)
			continue
		}
		if got != c.want {
			t.Errorf("Percentile(%g) = %s, want %v", c.p, describe(got), c.want)
		}
	}
}

func TestPercentileMissingAndBroken(t *testing.T) {
	// The rank is computed on the usable values only: four of them here,
	// so the 50th percentile is the second in ascending order.
	got, err := Percentile([]float64{10, NaV, 30, 40, 50}, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	check(t, "Percentile with a missing value", got, wantNum, 30)

	got, _ = Percentile([]float64{NaV, NaV}, 50)
	check(t, "Percentile on nothing but missing values", got, wantNaV, 0)

	got, _ = Percentile([]float64{}, 50)
	check(t, "Percentile on an empty slice", got, wantNaV, 0)

	got, _ = Percentile([]float64{10, math.NaN(), 30}, 50)
	check(t, "Percentile with an error in the input", got, wantNaN, 0)
}

// A rank outside (0, 100] is a programming mistake, and the only error
// this file reports.
func TestPercentileRejectsAnImpossibleRank(t *testing.T) {
	for _, p := range []float64{0, -1, 100.5, 1000, math.NaN()} {
		got, err := Percentile([]float64{1, 2, 3}, p)
		if err == nil {
			t.Errorf("Percentile(p = %v) returned no error", p)
			continue
		}
		if !errors.Is(err, ErrPercentileRange) {
			t.Errorf("Percentile(p = %v) returned %v, want an ErrPercentileRange", p, err)
		}
		check(t, "Percentile with an impossible rank", got, wantNaV, 0)
	}
}

// Sorting must happen on a copy: a caller's series is a series, and its
// order carries meaning.
func TestPercentileAndMedianLeaveTheInputAlone(t *testing.T) {
	xs := []float64{40, 10, 50, 20, 30}
	before := append([]float64(nil), xs...)

	_, _ = Percentile(xs, 50)
	_ = Median(xs)

	for i := range before {
		if xs[i] != before[i] {
			t.Fatalf("the input was reordered: %v, was %v", xs, before)
		}
	}
}

// -----------------------------------------------------------------------
// StdDev
// -----------------------------------------------------------------------

func TestStdDev(t *testing.T) {
	// [1 2 3 4]: mean 2.5, squared deviations 2.25+0.25+0.25+2.25 = 5,
	// divided by n-1 = 3, square root ≈ 1.29099.
	closeTo(t, "StdDev([1 2 3 4])", StdDev([]float64{1, 2, 3, 4}), 1.2909944487358056, 1e-12)

	// The same values with a missing one added: the result must not move.
	closeTo(t, "StdDev with a missing value",
		StdDev([]float64{1, 2, NaV, 3, 4}), 1.2909944487358056, 1e-12)

	// Constant series: no dispersion at all.
	closeTo(t, "StdDev of a constant series", StdDev([]float64{5, 5, 5}), 0, 1e-12)
}

func TestStdDevEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		in   []float64
		want want
	}{
		// A single point says nothing about dispersion.
		{"a single value", []float64{7}, wantNaV},
		{"a single usable value among missing ones", []float64{NaV, 7, NaV}, wantNaV},
		{"empty", []float64{}, wantNaV},
		{"nothing but missing values", []float64{NaV, NaV}, wantNaV},
		{"an error propagates", []float64{1, math.NaN(), 3}, wantNaN},
	}
	for _, c := range cases {
		check(t, "StdDev "+c.name, StdDev(c.in), c.want, 0)
	}
}

// The sample form divides by n-1, not by n. On [1 2 3 4] the population
// form would give ≈ 1.118, which must not be what we return.
func TestStdDevIsTheSampleForm(t *testing.T) {
	got := StdDev([]float64{1, 2, 3, 4})
	population := math.Sqrt(5.0 / 4.0)
	if math.Abs(got-population) < 1e-9 {
		t.Errorf("StdDev = %v, which is the population form; want the sample form (divisor n-1)", got)
	}
}
