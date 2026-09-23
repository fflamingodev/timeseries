package timeseries

import (
	"errors"
	"math"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// gridOf returns what a regularized series holds, as pairs of minute
// offset and measurement, so a test can state a whole grid in one line.
type slot struct {
	minute int
	value  float64
}

func gridOf(ts *TimeSeries) []slot {
	var out []slot
	ts.Range(func(_ int, du DataUnit) bool {
		out = append(out, slot{int(du.Chron.Sub(origin).Minutes()), du.Meas})
		return true
	})
	return out
}

func sameGrid(t *testing.T, name string, got, want []slot) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d windows, want %d\n got: %v\nwant: %v", name, len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i].minute != want[i].minute || !sameFloat(got[i].value, want[i].value) {
			t.Errorf("%s: window %d is %dmin=%s, want %dmin=%s",
				name, i, got[i].minute, nav.Format(got[i].value),
				want[i].minute, nav.Format(want[i].value))
		}
	}
}

// seriesAtMinutes builds a series from minute offsets and values.
func seriesAtMinutes(pairs ...slot) *TimeSeries {
	ts := NewTimeSeries("source")
	data := make([]Datum, 0, len(pairs))
	for _, p := range pairs {
		data = append(data, NewDatum(atMinute(p.minute), p.value))
	}
	ts.AddBatchData(data)
	return ts
}

// -----------------------------------------------------------------------
// The grid
// -----------------------------------------------------------------------

// Windows are closed on the right and aligned on the clock: a reading
// at exactly a tick belongs to the window ending there.
func TestRegularizeWindowsCloseOnTheRight(t *testing.T) {
	ts := seriesAtMinutes(
		slot{30, 1},  // 00:30, in the window ending at 01:00
		slot{60, 2},  // 01:00 exactly, same window
		slot{61, 3},  // 01:01, the next one
		slot{120, 4}, // 02:00 exactly, same as the previous
	)

	out, err := ts.Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, out)

	sameGrid(t, "hourly", gridOf(out), []slot{
		{60, 1.5}, // mean of 1 and 2
		{120, 3.5},
	})
}

// The grid follows the clock, not the first reading: two series that
// started at different moments land on the same instants, which is what
// makes them comparable point to point.
func TestRegularizeAlignsOnTheClockNotOnTheFirstReading(t *testing.T) {
	a, err := seriesAtMinutes(slot{7, 1}, slot{67, 2}).Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := seriesAtMinutes(slot{52, 10}, slot{112, 20}).Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if a.Len() != b.Len() {
		t.Fatalf("the two grids have %d and %d windows", a.Len(), b.Len())
	}
	for i := 0; i < a.Len(); i++ {
		if !a.At(i).Chron.Equal(b.At(i).Chron) {
			t.Errorf("window %d: %s against %s — the grids do not line up",
				i, a.At(i).Chron.Format("15:04"), b.At(i).Chron.Format("15:04"))
		}
	}
}

// A window that caught nothing is a gap, and the grid keeps its step.
// Nothing is emitted before the first reading or after the last.
func TestRegularizeEmptyWindowsBecomeGaps(t *testing.T) {
	ts := seriesAtMinutes(slot{30, 1}, slot{210, 2}) // 00:30 then 03:30

	out, err := ts.Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sameGrid(t, "with a hole", gridOf(out), []slot{
		{60, 1},
		{120, nav.NaV},
		{180, nav.NaV},
		{240, 2},
	})
}

// -----------------------------------------------------------------------
// The tolerance
// -----------------------------------------------------------------------

// A logger that drifts a few minutes around the hour: without a
// tolerance its readings pile up two to a window and leave others
// empty; with one, each lands where it was meant to.
func TestRegularizeWithTolerance(t *testing.T) {
	ts := seriesAtMinutes(
		slot{57, 57},   // meant for 01:00, three minutes early
		slot{123, 123}, // meant for 02:00, three minutes late
		slot{180, 180}, // on time
		slot{241, 241}, // one minute late
	)

	strict, err := ts.Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 02:03 falls with 03:00 in the window ending at 03:00, and the
	// window ending at 02:00 catches nothing.
	sameGrid(t, "without tolerance", gridOf(strict), []slot{
		{60, 57},
		{120, nav.NaV},
		{180, 151.5}, // mean of 123 and 180
		{240, nav.NaV},
		{300, 241},
	})

	lenient, err := ts.RegularizeWithTolerance(time.Hour, 5*time.Minute, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "with five minutes of tolerance", gridOf(lenient), []slot{
		{60, 57},
		{120, 123},
		{180, 180},
		{240, 241},
	})
}

// A tolerance of zero is Regularize itself.
func TestZeroToleranceIsRegularize(t *testing.T) {
	ts := seriesAtMinutes(slot{57, 1}, slot{123, 2}, slot{180, 3})

	strict, _ := ts.Regularize(time.Hour, AggMean)
	zero, _ := ts.RegularizeWithTolerance(time.Hour, 0, AggMean)

	sameGrid(t, "zero tolerance", gridOf(zero), gridOf(strict))
}

// -----------------------------------------------------------------------
// Gaps and errors in the input
// -----------------------------------------------------------------------

func TestRegularizeCarriesTheRulesThrough(t *testing.T) {
	ts := seriesAtMinutes(
		slot{10, 10}, slot{20, nav.NaV}, slot{30, 20}, // window 1: a gap among readings
		slot{70, nav.NaV}, slot{80, nav.NaV}, // window 2: nothing but gaps
		slot{130, 5}, slot{140, math.NaN()}, // window 3: a broken value
	)

	out, err := ts.Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := gridOf(out)
	if len(got) != 3 {
		t.Fatalf("%d windows, want 3: %v", len(got), got)
	}
	// The gap is skipped, the mean covers what was measured.
	if got[0].value != 15 {
		t.Errorf("window 1 = %s, want 15, the mean of 10 and 20", nav.Format(got[0].value))
	}
	// A window of nothing but gaps has nothing to say.
	if !nav.IsNaV(got[1].value) {
		t.Errorf("window 2 = %s, want NaV", nav.Format(got[1].value))
	}
	// The broken value propagates, and stays distinguishable from a gap.
	if !nav.IsStdNaN(got[2].value) {
		t.Errorf("window 3 = %s, want a plain NaN", nav.Format(got[2].value))
	}
}

// -----------------------------------------------------------------------
// Arguments
// -----------------------------------------------------------------------

func TestRegularizeRejectsImpossibleArguments(t *testing.T) {
	ts := seriesOf(1, 2, 3)

	cases := []struct {
		name      string
		freq, tol time.Duration
		agg       AggFunc
	}{
		{"a step of zero", 0, 0, AggMean},
		{"a negative step", -time.Hour, 0, AggMean},
		{"a negative tolerance", time.Hour, -time.Minute, AggMean},
		{"a tolerance as wide as the step", time.Hour, time.Hour, AggMean},
		{"a tolerance wider than the step", time.Hour, 2 * time.Hour, AggMean},
		{"no aggregator", time.Hour, 0, nil},
	}
	for _, c := range cases {
		_, err := ts.RegularizeWithTolerance(c.freq, c.tol, c.agg)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if !errors.Is(err, ErrRegularizeArg) {
			t.Errorf("%s: error is %v, want an ErrRegularizeArg", c.name, err)
		}
	}
}

func TestRegularizeAnEmptySeries(t *testing.T) {
	out, err := NewTimeSeries("empty").Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("%d windows from an empty series", out.Len())
	}
}

func TestRegularizeASinglePoint(t *testing.T) {
	out, err := seriesAtMinutes(slot{30, 7}).Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "one reading", gridOf(out), []slot{{60, 7}})
}

// The regularized series is a series like any other: chronological,
// with exact deltas, and it carries the identity of its source.
func TestRegularizeProducesAProperSeries(t *testing.T) {
	ts := seriesAtMinutes(slot{10, 1}, slot{70, 2}, slot{130, 3})
	ts.ID = "device-42"

	out, err := ts.Regularize(time.Hour, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checkInvariant(t, out)
	if out.ID != "device-42" {
		t.Error("the regularized series lost the identity of its source")
	}
	if out.Comment == "" {
		t.Error("nothing says how the series was regularized")
	}
	// Every interval is exactly the step: that is the whole purpose.
	bs := out.Stats()
	if bs.DChmin != time.Hour || bs.DChmax != time.Hour {
		t.Errorf("intervals span [%v, %v], want an hour throughout", bs.DChmin, bs.DChmax)
	}
	if bs.DChstd != 0 {
		t.Errorf("interval dispersion = %v, want zero on a regular grid", bs.DChstd)
	}
}

// Down to the minute or up to the day, the step is the caller's.
func TestRegularizeAtSeveralSteps(t *testing.T) {
	ts := seriesAtMinutes(
		slot{0, 1}, slot{15, 2}, slot{30, 3}, slot{45, 4},
		slot{60, 5}, slot{75, 6},
	)

	halfHourly, err := ts.Regularize(30*time.Minute, AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "every half hour", gridOf(halfHourly), []slot{
		{0, 1},    // 00:00 exactly
		{30, 2.5}, // 00:15 and 00:30
		{60, 4.5}, // 00:45 and 01:00
		{90, 6},   // 01:15
	})

	// A daily grid shows the convention at its starkest: windows close
	// on the right, so the reading taken at midnight sharp belongs to
	// the day that just ended — a window of its own here — and the rest
	// of the morning belongs to the window closing at the next midnight.
	daily, err := ts.Regularize(24*time.Hour, AggSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "every day", gridOf(daily), []slot{
		{0, 1},     // the reading at midnight, closing the previous day
		{1440, 20}, // 2+3+4+5+6, closing the day that follows
	})
}
