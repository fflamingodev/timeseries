package timeseries

import (
	"errors"
	"math"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// everyMinute builds a regular series, one reading a minute from the
// origin, so that a test can state a signal as a list of values.
func everyMinute(values ...float64) *TimeSeries {
	pairs := make([]slot, len(values))
	for i, v := range values {
		pairs[i] = slot{i, v}
	}
	return seriesAtMinutes(pairs...)
}

// expandBack rebuilds the reduced series on the minute grid of a source
// of n readings.
func expandBack(t *testing.T, reduced *TimeSeries, n int) *TimeSeries {
	t.Helper()
	out, err := reduced.Expand(atMinute(0), atMinute(n-1), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

// -----------------------------------------------------------------------
// Reduce
// -----------------------------------------------------------------------

// Only the changes survive, with the first and the last reading.
func TestReduceKeepsTheChangesAndBothEnds(t *testing.T) {
	ts := everyMinute(1, 1, 1, 2, 2, 3, 3, 3)

	out := ts.Reduce()
	checkInvariant(t, out)
	sameGrid(t, "reduced", gridOf(out), []slot{
		{0, 1}, // first reading
		{3, 2},
		{5, 3},
		{7, 3}, // last reading, kept although it repeats
	})
}

// When the last reading is itself a change, it is kept once.
func TestReduceDoesNotRepeatALastChange(t *testing.T) {
	out := everyMinute(1, 1, 2).Reduce()
	sameGrid(t, "last is a change", gridOf(out), []slot{{0, 1}, {2, 2}})
}

// A constant signal reduces to its two ends.
func TestReduceAConstantSignal(t *testing.T) {
	out := everyMinute(5, 5, 5, 5, 5).Reduce()
	sameGrid(t, "constant", gridOf(out), []slot{{0, 5}, {4, 5}})
}

func TestReduceTheRulesOfChange(t *testing.T) {
	broken := math.NaN()
	ts := everyMinute(
		1,       // 0 first
		nav.NaV, // 1 into a gap: a change
		nav.NaV, // 2 still a gap: nothing
		1,       // 3 out of the gap: a change, even back to the same value
		broken,  // 4 turning broken: a change
		broken,  // 5 still broken: nothing
		nav.NaV, // 6 broken to gap: a change
		4,       // 7 a change
		4,       // 8 last
	)

	sameGrid(t, "rules", gridOf(ts.Reduce()), []slot{
		{0, 1}, {1, nav.NaV}, {3, 1}, {4, broken}, {6, nav.NaV}, {7, 4}, {8, 4},
	})
}

// Two equal infinities are no change; opposite ones are.
func TestReduceInfinities(t *testing.T) {
	inf := math.Inf(1)
	out := everyMinute(inf, inf, -inf, -inf, 0).Reduce()
	sameGrid(t, "infinities", gridOf(out), []slot{{0, inf}, {2, -inf}, {4, 0}})
}

func TestReduceAnEmptySeries(t *testing.T) {
	if out := NewTimeSeries("empty").Reduce(); out.Len() != 0 {
		t.Errorf("%d points from an empty series", out.Len())
	}
}

// A single reading is both ends at once, and is kept once.
func TestReduceASinglePoint(t *testing.T) {
	sameGrid(t, "one reading", gridOf(everyMinute(7).Reduce()), []slot{{0, 7}})
}

// Reducing twice changes nothing.
func TestReduceIsIdempotent(t *testing.T) {
	once := everyMinute(1, 1, 2, nav.NaV, nav.NaV, 2, 2).Reduce()
	sameGrid(t, "twice", gridOf(once.Reduce()), gridOf(once))
}

func TestReduceProducesAProperSeries(t *testing.T) {
	ts := everyMinute(1, 1, 2, 2)
	ts.ID = "device-42"

	out := ts.Reduce()
	checkInvariant(t, out)
	if out.ID != "device-42" {
		t.Error("the reduced series lost the identity of its source")
	}
	if out.Comment == "" {
		t.Error("nothing says how the series was reduced")
	}
	if ts.Len() != 4 {
		t.Errorf("the source now holds %d points", ts.Len())
	}
}

// -----------------------------------------------------------------------
// Expand, and the round trip
// -----------------------------------------------------------------------

// On the grid of a regular source, Expand gives the source back, gaps
// and broken values included.
func TestReduceThenExpandGivesTheSourceBack(t *testing.T) {
	ts := everyMinute(1, 1, 1, nav.NaV, nav.NaV, 2, math.NaN(), math.NaN(), 3, 3)

	back := expandBack(t, ts.Reduce(), ts.Len())
	checkInvariant(t, back)
	sameGrid(t, "round trip", gridOf(back), gridOf(ts))
}

// Before the first point and after the last, nothing is known.
func TestExpandIsNaVOutsideTheReducedSpan(t *testing.T) {
	reduced := seriesAtMinutes(slot{2, 1}, slot{4, 2}, slot{5, 2})

	out, err := reduced.Expand(atMinute(0), atMinute(7), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "outside the span", gridOf(out), []slot{
		{0, nav.NaV}, {1, nav.NaV}, // before the first point
		{2, 1}, {3, 1}, {4, 2}, {5, 2}, // the span, ends included
		{6, nav.NaV}, {7, nav.NaV}, // after the last point
	})
}

// A change before the grid starts is already in force at its start.
func TestExpandHoldsAChangeFromBeforeTheStart(t *testing.T) {
	reduced := seriesAtMinutes(slot{0, 1}, slot{3, 2}, slot{10, 2})

	out, err := reduced.Expand(atMinute(2), atMinute(4), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "from before", gridOf(out), []slot{{2, 1}, {3, 2}, {4, 2}})
}

// The grid includes end only when the span is a whole number of steps.
func TestExpandGridBounds(t *testing.T) {
	reduced := seriesAtMinutes(slot{0, 1}, slot{60, 1})

	out, err := reduced.Expand(atMinute(0), atMinute(25), 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "partial last step", gridOf(out), []slot{{0, 1}, {10, 1}, {20, 1}})

	one, err := reduced.Expand(atMinute(30), atMinute(30), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "start equals end", gridOf(one), []slot{{30, 1}})
}

func TestExpandAnEmptySeries(t *testing.T) {
	out, err := NewTimeSeries("empty").Expand(atMinute(0), atMinute(2), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "empty", gridOf(out), []slot{{0, nav.NaV}, {1, nav.NaV}, {2, nav.NaV}})
}

func TestExpandRejectsImpossibleArguments(t *testing.T) {
	ts := everyMinute(1, 2)
	cases := []struct {
		name       string
		start, end time.Time
		step       time.Duration
	}{
		{"a step of zero", atMinute(0), atMinute(1), 0},
		{"a negative step", atMinute(0), atMinute(1), -time.Minute},
		{"an end before the start", atMinute(1), atMinute(0), time.Minute},
	}
	for _, c := range cases {
		_, err := ts.Expand(c.start, c.end, c.step)
		if !errors.Is(err, ErrRegularizeArg) {
			t.Errorf("%s: error is %v, want an ErrRegularizeArg", c.name, err)
		}
	}
}

// -----------------------------------------------------------------------
// ReduceWithDeadband
// -----------------------------------------------------------------------

// Readings within the band of the value held are dropped.
func TestDeadbandDropsTheNoise(t *testing.T) {
	ts := everyMinute(10, 10.2, 9.9, 10.1, 12, 12.3, 11.8, 12)

	out, err := ts.ReduceWithDeadband(0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, out)
	sameGrid(t, "noise", gridOf(out), []slot{{0, 10}, {4, 12}, {7, 12}})
}

// The comparison is with the last kept value: a drift that never moves
// more than the band from one reading to the next is caught all the
// same.
func TestDeadbandCatchesASlowDrift(t *testing.T) {
	ts := everyMinute(0, 0.3, 0.6, 0.9, 1.2, 1.5)

	out, err := ts.ReduceWithDeadband(0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "drift", gridOf(out), []slot{{0, 0}, {2, 0.6}, {4, 1.2}, {5, 1.5}})
}

// Expanded again, the deadband series never errs by more than the band.
func TestDeadbandBoundsTheError(t *testing.T) {
	const band = 0.5
	values := make([]float64, 200)
	for i := range values {
		values[i] = 3*math.Sin(float64(i)/10) + 0.2*math.Cos(float64(i)*7)
	}
	ts := everyMinute(values...)

	reduced, err := ts.ReduceWithDeadband(band)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reduced.Len() >= ts.Len() {
		t.Errorf("nothing was dropped: %d points out of %d", reduced.Len(), ts.Len())
	}

	back := expandBack(t, reduced, ts.Len())
	for i := 0; i < ts.Len(); i++ {
		if d := math.Abs(back.At(i).Meas - ts.At(i).Meas); d > band {
			t.Errorf("minute %d: off by %v, more than the band", i, d)
		}
	}
}

// Gaps and broken values are changes whatever the band.
func TestDeadbandKeepsTheGaps(t *testing.T) {
	ts := everyMinute(1, nav.NaV, 1, math.NaN(), 1, 1)

	out, err := ts.ReduceWithDeadband(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "gaps", gridOf(out), gridOf(ts.Reduce()))
}

// A band of zero is Reduce.
func TestZeroDeadbandIsReduce(t *testing.T) {
	ts := everyMinute(1, 1, 2, nav.NaV, 2, 2, math.NaN(), 3, 3)

	zero, err := ts.ReduceWithDeadband(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "zero band", gridOf(zero), gridOf(ts.Reduce()))
}

// An infinite band keeps only the passages between measured, gap and
// broken.
func TestInfiniteDeadband(t *testing.T) {
	ts := everyMinute(1, 50, -3, nav.NaV, 8, 9, 10)

	out, err := ts.ReduceWithDeadband(math.Inf(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "infinite band", gridOf(out), []slot{{0, 1}, {3, nav.NaV}, {4, 8}, {6, 10}})
}

func TestDeadbandRejectsImpossibleBands(t *testing.T) {
	ts := everyMinute(1, 2)
	for name, band := range map[string]float64{
		"a negative band": -0.1,
		"a NaN band":      math.NaN(),
		"a NaV band":      nav.NaV,
		"minus infinity":  math.Inf(-1),
	} {
		if _, err := ts.ReduceWithDeadband(band); !errors.Is(err, ErrRegularizeArg) {
			t.Errorf("%s: error is %v, want an ErrRegularizeArg", name, err)
		}
	}
}
