package timeseries

import (
	"errors"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// IsSorted
// ---------------------------------------------------------------------------

func TestIsSorted(t *testing.T) {
	cases := []struct {
		name   string
		points [][2]int // pairs of (second, value) for readability
		want   bool
	}{
		{"empty", nil, true},
		{"single", [][2]int{{0, 1}}, true},
		{"already sorted", [][2]int{{0, 1}, {10, 2}, {20, 3}}, true},
		{"equal timestamps are OK", [][2]int{{0, 1}, {0, 2}, {10, 3}}, true},
		{"descending", [][2]int{{20, 1}, {10, 2}, {0, 3}}, false},
		{"partial inversion", [][2]int{{0, 1}, {10, 2}, {5, 3}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ts TimeSeries
			for _, p := range c.points {
				ts.DataSeries = append(ts.DataSeries, NewDataUnit(tFromSec(p[0]), float64(p[1])))
			}
			if got := ts.IsSorted(); got != c.want {
				t.Errorf("IsSorted() = %v, want %v", got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Bounds
// ---------------------------------------------------------------------------

func TestBounds_skipsNaV(t *testing.T) {
	mn, mx := Bounds([]float64{3, NaV, 1, 5, NaV, 2})
	assertFloat(t, mn, 1, "Bounds min")
	assertFloat(t, mx, 5, "Bounds max")
}

func TestBounds_empty(t *testing.T) {
	mn, mx := Bounds(nil)
	if !IsNaV(mn) || !IsNaV(mx) {
		t.Errorf("Bounds of nil = (%s, %s), want (NaV, NaV)", Format(mn), Format(mx))
	}
}

func TestBounds_allNaV(t *testing.T) {
	mn, mx := Bounds([]float64{NaV, NaV})
	if !IsNaV(mn) || !IsNaV(mx) {
		t.Errorf("Bounds of all-NaV = (%s, %s), want (NaV, NaV)", Format(mn), Format(mx))
	}
}

// ---------------------------------------------------------------------------
// Regularize: grid alignment, empty-bucket NaV, no-leading/trailing-NaV
// ---------------------------------------------------------------------------

// regTestSeries builds a series from (second, value) pairs.
func regTestSeries(points ...[2]float64) *TimeSeries {
	ts := &TimeSeries{}
	for _, p := range points {
		ts.AddData(tFromSec(int(p[0])), p[1])
	}
	return ts
}

func TestRegularize_emptySeries(t *testing.T) {
	ts := &TimeSeries{}
	out := ts.Regularize(time.Second, AggAverage)
	if len(out.DataSeries) != 0 {
		t.Errorf("len = %d, want 0", len(out.DataSeries))
	}
}

func TestRegularize_zeroFreq(t *testing.T) {
	ts := regTestSeries([2]float64{0, 1}, [2]float64{10, 2})
	out := ts.Regularize(0, AggAverage)
	if len(out.DataSeries) != 0 {
		t.Errorf("zero freq should return empty, got len=%d", len(out.DataSeries))
	}
}

func TestRegularize_singlePoint(t *testing.T) {
	ts := regTestSeries([2]float64{0, 42})
	out := ts.Regularize(10*time.Second, AggAverage)
	if len(out.DataSeries) != 1 {
		t.Fatalf("len = %d, want 1", len(out.DataSeries))
	}
	assertFloat(t, out.DataSeries[0].Meas, 42, "single-point value preserved")
}

func TestRegularize_fillsEmptyBucketsWithNaV(t *testing.T) {
	// Points at t=0, t=30, t=50. With freq=10s, aligned grid is
	// {0, 10, 20, 30, 40, 50}. Buckets 10, 20, 40 are empty → NaV.
	ts := regTestSeries(
		[2]float64{0, 1},
		[2]float64{30, 2},
		[2]float64{50, 3},
	)
	out := ts.Regularize(10*time.Second, AggAverage)
	if len(out.DataSeries) != 6 {
		t.Fatalf("len = %d, want 6", len(out.DataSeries))
	}
	want := []float64{1, NaV, NaV, 2, NaV, 3}
	for i, du := range out.DataSeries {
		if !floatEqual(du.Meas, want[i]) {
			t.Errorf("bucket %d: got %s, want %s", i, Format(du.Meas), Format(want[i]))
		}
	}
}

func TestRegularize_aggregatesWithinBucket(t *testing.T) {
	// Three points in the same 10-second bucket at t=0 → averaged.
	ts := regTestSeries(
		[2]float64{0, 1},
		[2]float64{3, 2},
		[2]float64{7, 3},
		[2]float64{20, 10},
	)
	out := ts.Regularize(10*time.Second, AggAverage)
	if len(out.DataSeries) < 2 {
		t.Fatalf("len = %d, want >= 2", len(out.DataSeries))
	}
	// First bucket ends at t=0 (freq-aligned) and must include the point at 0.
	// Under the right-closed convention a point exactly on windowEnd is in the bucket.
	// So bucket 0 = {1}, bucket 10 = {2, 3}, bucket 20 = {10}.
	assertFloat(t, out.DataSeries[0].Meas, 1, "bucket 0")
	assertFloat(t, out.DataSeries[1].Meas, 2.5, "bucket 10")
}

func TestRegularize_outputDeltasValidFalse(t *testing.T) {
	ts := regTestSeries([2]float64{0, 1}, [2]float64{10, 2})
	ts.SortDeltasStats()
	out := ts.Regularize(10*time.Second, AggAverage)
	if out.DeltasValid() {
		t.Error("Regularize output should start with DeltasValid == false")
	}
	// Each emitted unit must carry the sentinel defaults.
	for i, du := range out.DataSeries {
		if du.Dchron != NaDuration {
			t.Errorf("bucket %d: Dchron = %v, want NaDuration", i, du.Dchron)
		}
		if !IsNaV(du.Dmeas) {
			t.Errorf("bucket %d: Dmeas = %s, want NaV", i, Format(du.Dmeas))
		}
	}
}

// ---------------------------------------------------------------------------
// RegularizeWithTolerance
// ---------------------------------------------------------------------------

func TestRegularizeWithTolerance_zeroToleranceEqualsBasic(t *testing.T) {
	ts := regTestSeries(
		[2]float64{0, 1},
		[2]float64{13, 2},
		[2]float64{30, 3},
	)
	baseline := ts.Regularize(10 * time.Second, AggAverage)
	withTol, err := ts.RegularizeWithTolerance(10*time.Second, AggAverage, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(baseline.DataSeries) != len(withTol.DataSeries) {
		t.Fatalf("length mismatch: baseline=%d withTol=%d", len(baseline.DataSeries), len(withTol.DataSeries))
	}
	for i := range baseline.DataSeries {
		if !floatEqual(baseline.DataSeries[i].Meas, withTol.DataSeries[i].Meas) {
			t.Errorf("bucket %d: baseline=%s withTol=%s", i,
				Format(baseline.DataSeries[i].Meas), Format(withTol.DataSeries[i].Meas))
		}
	}
}

func TestRegularizeWithTolerance_lateArrivalAttributedToCurrentBucket(t *testing.T) {
	// Point at t=13 would belong to bucket 20 under strict rules. With a
	// 5-second tolerance, the bucket ending at 10 accepts points up to 15,
	// so t=13 is attributed to bucket 10.
	ts := regTestSeries(
		[2]float64{0, 1},
		[2]float64{13, 2},
		[2]float64{30, 3},
	)
	out, err := ts.RegularizeWithTolerance(10*time.Second, AggLast, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// We expect the value 2 in the first occupied bucket after t=0 — which,
	// under tolerance 5s, is the one ending at 10 (not 20).
	// Exact output depends on grid alignment; what matters is that the
	// late-arriving 2 is folded in earlier than without tolerance.
	found := false
	for _, du := range out.DataSeries {
		if du.Meas == 2 {
			// The bucket that captured the value at t=13 must end at t=10,
			// not t=20.
			expected := tFromSec(10)
			if !du.Chron.Equal(expected) {
				t.Errorf("point t=13 attributed to bucket ending %v, want %v", du.Chron, expected)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find value 2 in the output")
	}
}

func TestRegularizeWithTolerance_errorIfToleranceGEFreq(t *testing.T) {
	ts := regTestSeries([2]float64{0, 1})
	_, err := ts.RegularizeWithTolerance(10*time.Second, AggAverage, 10*time.Second)
	if err == nil {
		t.Fatal("expected error when tolerance >= freq")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Kind != KindInvalidArg {
		t.Errorf("Kind = %v, want KindInvalidArg", e.Kind)
	}
}

func TestMustRegularizeWithTolerance_panicsOnBadArg(t *testing.T) {
	ts := regTestSeries([2]float64{0, 1})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when tolerance >= freq")
		}
	}()
	_ = ts.MustRegularizeWithTolerance(10*time.Second, AggAverage, 10*time.Second)
}

// ---------------------------------------------------------------------------
// Reduce / Expand round-trip
// ---------------------------------------------------------------------------

func TestReduce_keepsTransitionsOnly(t *testing.T) {
	// Values: 1,1,1,2,2,3,3,3,1 → after Reduce: 1,2,3,1 (4 points).
	ts := regTestSeries(
		[2]float64{0, 1},
		[2]float64{1, 1},
		[2]float64{2, 1},
		[2]float64{3, 2},
		[2]float64{4, 2},
		[2]float64{5, 3},
		[2]float64{6, 3},
		[2]float64{7, 3},
		[2]float64{8, 1},
	)
	ts.SortDeltasStats() // required: Reduce uses Dmeas to detect changes
	out := ts.Reduce()
	want := []float64{1, 2, 3, 1}
	if len(out.DataSeries) != len(want) {
		t.Fatalf("len = %d, want %d", len(out.DataSeries), len(want))
	}
	for i, du := range out.DataSeries {
		assertFloat(t, du.Meas, want[i], "Reduce[i]")
	}
}

func TestReduce_naVTransitions(t *testing.T) {
	// NaV -> NaV is no change; any transition involving NaV is a change.
	ts := regTestSeries()
	ts.AddDataUnit(NewDataUnit(tFromSec(0), 1))
	ts.AddDataUnit(NewDataUnit(tFromSec(1), NaV))
	ts.AddDataUnit(NewDataUnit(tFromSec(2), NaV))
	ts.AddDataUnit(NewDataUnit(tFromSec(3), 1))
	ts.SortDeltasStats()

	out := ts.Reduce()
	if len(out.DataSeries) != 3 {
		t.Fatalf("len = %d, want 3 (1 → NaV → 1)", len(out.DataSeries))
	}
	assertFloat(t, out.DataSeries[0].Meas, 1, "reduce[0]")
	if !IsNaV(out.DataSeries[1].Meas) {
		t.Errorf("reduce[1] = %s, want NaV", Format(out.DataSeries[1].Meas))
	}
	assertFloat(t, out.DataSeries[2].Meas, 1, "reduce[2]")
}
