package timeseries

import (
	"testing"
	"time"
)

func TestAgg_emptyBucketReturnsNaV(t *testing.T) {
	empty := []float64{}
	for _, tc := range []struct {
		name string
		fn   AggFunc
	}{
		{"AggAverage", AggAverage},
		{"AggMaximum", AggMaximum},
		{"AggMinimum", AggMinimum},
		{"AggLast", AggLast},
		{"AggOpen", AggOpen},
		{"AggMedian", AggMedian},
		{"AggIntegral", AggIntegral(time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(empty)
			if !IsNaV(got) {
				t.Errorf("%s([]) = %s, want NaV", tc.name, Format(got))
			}
		})
	}
}

func TestAggAverage_skipsNaV(t *testing.T) {
	assertFloat(t, AggAverage([]float64{1, NaV, 3}), 2, "AggAverage skips NaV")
}

func TestAggCountValid(t *testing.T) {
	assertFloat(t, AggCountValid([]float64{1, NaV, 2, NaNumber, 3}), 3, "counts only non-NaN")
	assertFloat(t, AggCountValid([]float64{NaV, NaV}), 0, "all-NaV is 0")
}

func TestAggLast(t *testing.T) {
	assertFloat(t, AggLast([]float64{1, 2, 3}), 3, "last is the chronologically last")
	// NaV at the end is returned as-is.
	if !IsNaV(AggLast([]float64{1, 2, NaV})) {
		t.Error("AggLast should return NaV when bucket ends with NaV")
	}
}

func TestAggOpen(t *testing.T) {
	assertFloat(t, AggOpen([]float64{1, 2, 3}), 1, "open is first")
}

func TestAggIntegral_skipsNaV(t *testing.T) {
	// freq=1s → dt=1. Sum of {1, NaV, 2, 3} skipping NaV = 6. Integral = 6.
	agg := AggIntegral(time.Second)
	assertFloat(t, agg([]float64{1, NaV, 2, 3}), 6, "integral ignores NaV")
}

func TestAggSlope_basic(t *testing.T) {
	// y = 0, 1, 2, 3 → slope = 1 (x=0..3)
	assertFloat(t, AggSlope([]float64{0, 1, 2, 3}), 1, "perfect linear")
	// Constant → slope = 0
	assertFloat(t, AggSlope([]float64{5, 5, 5, 5}), 0, "constant line")
	// Not enough valid points → NaV
	if !IsNaV(AggSlope([]float64{5})) {
		t.Error("AggSlope with 1 point should be NaV")
	}
	if !IsNaV(AggSlope([]float64{NaV, NaV, 5})) {
		t.Error("AggSlope with <2 valid points should be NaV")
	}
}

func TestAggIncrementalCounter(t *testing.T) {
	counter := AggIncrementalCounter()

	first := counter([]float64{100})
	if !IsNaV(first) {
		t.Errorf("first call should be NaV (no previous bucket), got %s", Format(first))
	}

	second := counter([]float64{105, 108, 110})
	assertFloat(t, second, 10, "110 - 100")

	third := counter([]float64{112})
	assertFloat(t, third, 2, "112 - 110")

	// Empty bucket → NaV (no data to compare), state is preserved.
	empty := counter([]float64{})
	if !IsNaV(empty) {
		t.Errorf("empty bucket → %s, want NaV", Format(empty))
	}
	// Next non-empty call should compute against the preserved last=112.
	next := counter([]float64{114})
	assertFloat(t, next, 2, "state preserved across empty bucket")
}
