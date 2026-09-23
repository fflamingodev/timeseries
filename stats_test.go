package timeseries

import (
	"math"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// statsOf builds a series at successive hours and returns its summary.
func statsOf(meas ...float64) BasicStats {
	return seriesOf(meas...).Stats()
}

// eq fails unless got is the expected number.
func eq(t *testing.T, field string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %s, want %v", field, nav.Format(got), want)
	}
}

// isNaV fails unless got is the missing-value sentinel.
func isNaV(t *testing.T, field string, got float64) {
	t.Helper()
	if !nav.IsNaV(got) {
		t.Errorf("%s = %s, want NaV", field, nav.Format(got))
	}
}

// -----------------------------------------------------------------------
// A series with everything in it
// -----------------------------------------------------------------------

// The reference series: four points at 0, 1, 2 and 4 hours, measuring
// 10, nothing, 30 and 20. Every expected value below is computed by
// hand in the comments.
func referenceSeries() *TimeSeries {
	ts := NewTimeSeries("reference")
	ts.AddAll([]Datum{
		NewDatum(at(0), 10),
		NewDatum(at(1), nav.NaV),
		NewDatum(at(2), 30),
		NewDatum(at(4), 20),
	})
	return ts
}

func TestStatsOnTheMeasurements(t *testing.T) {
	bs := referenceSeries().Stats()

	if bs.Len != 4 {
		t.Errorf("Len = %d, want 4: the missing point is a point", bs.Len)
	}
	// The gap counts in both: NbreOfNaN is every non-number, NbreOfNaV
	// the missing ones alone. Their difference is the genuine errors.
	if bs.NbreOfNaN != 1 || bs.NbreOfNaV != 1 {
		t.Errorf("NbreOfNaN = %d, NbreOfNaV = %d, want 1 and 1", bs.NbreOfNaN, bs.NbreOfNaV)
	}

	eq(t, "Msmin", bs.Msmin, 10)
	eq(t, "Msmax", bs.Msmax, 30)
	eq(t, "Msmean", bs.Msmean, 20)  // (10+30+20)/3
	eq(t, "Msmed", bs.Msmed, 20)    // median of 10, 20, 30
	eq(t, "Msstd", bs.Msstd, 10)    // sample deviation: sqrt(200/2)
	if !bs.ChAtMsmin.Equal(at(0)) { // 10 was measured at hour 0
		t.Errorf("ChAtMsmin = %s, want hour 0", bs.ChAtMsmin.Format("15:04"))
	}
	if !bs.ChAtMsmax.Equal(at(2)) { // 30 at hour 2
		t.Errorf("ChAtMsmax = %s, want hour 2", bs.ChAtMsmax.Format("15:04"))
	}
}

func TestStatsOnTheExtent(t *testing.T) {
	bs := referenceSeries().Stats()

	if !bs.Chmin.Equal(at(0)) || !bs.Chmax.Equal(at(4)) {
		t.Errorf("extent = [%s, %s], want [hour 0, hour 4]",
			bs.Chmin.Format("15:04"), bs.Chmax.Format("15:04"))
	}
	eq(t, "ValAtChmin", bs.ValAtChmin, 10)
	eq(t, "ValAtChmax", bs.ValAtChmax, 20)

	// Mean of the offsets 0, 1, 2 and 4 hours: 1.75 h.
	if want := at(0).Add(105 * time.Minute); !bs.Chmean.Equal(want) {
		t.Errorf("Chmean = %s, want %s", bs.Chmean.Format("15:04:05"), want.Format("15:04:05"))
	}
	// Median of the same, nearest rank: the lower of the two middles.
	if !bs.Chmed.Equal(at(1)) {
		t.Errorf("Chmed = %s, want hour 1", bs.Chmed.Format("15:04:05"))
	}
}

func TestStatsOnTheIntervals(t *testing.T) {
	bs := referenceSeries().Stats()

	// Intervals: 1 h, 1 h, 2 h.
	if bs.DChmin != time.Hour {
		t.Errorf("DChmin = %v, want 1h", bs.DChmin)
	}
	if bs.DChmax != 2*time.Hour {
		t.Errorf("DChmax = %v, want 2h", bs.DChmax)
	}
	if !bs.ChAtDChmin.Equal(at(1)) {
		t.Errorf("ChAtDChmin = %s, want hour 1", bs.ChAtDChmin.Format("15:04"))
	}
	if !bs.ChAtDchmax.Equal(at(4)) {
		t.Errorf("ChAtDchmax = %s, want hour 4", bs.ChAtDchmax.Format("15:04"))
	}
	eq(t, "DChmean", bs.DChmean, float64(4*time.Hour)/3)
	eq(t, "DChmed", bs.DChmed, float64(time.Hour))

	// A regular feed has a near-zero DChstd; this one stutters.
	if bs.DChstd <= 0 {
		t.Errorf("DChstd = %v, want a positive dispersion on an irregular feed", bs.DChstd)
	}
}

// The variations rest on fewer values than the series has points: every
// one computed across a gap is missing.
func TestStatsOnTheVariations(t *testing.T) {
	bs := referenceSeries().Stats()

	// Variations: NaV (no predecessor), NaV (into the gap), NaV (out of
	// it), then -10. One usable value out of four.
	eq(t, "DMsmin", bs.DMsmin, -10)
	eq(t, "DMsmax", bs.DMsmax, -10)
	eq(t, "DMsmean", bs.DMsmean, -10)
	eq(t, "DMsmed", bs.DMsmed, -10)
	// A single value says nothing about dispersion.
	isNaV(t, "DMsstd", bs.DMsstd)
}

// -----------------------------------------------------------------------
// Series too poor to say anything
// -----------------------------------------------------------------------

// Nothing may come out as zero: a zero mean is a statement about the
// data, and here there is no statement to make.
func TestStatsOnAnEmptySeries(t *testing.T) {
	bs := NewTimeSeries("empty").Stats()

	if bs.Len != 0 {
		t.Errorf("Len = %d, want 0", bs.Len)
	}
	for _, c := range []struct {
		name string
		got  float64
	}{
		{"Msmin", bs.Msmin}, {"Msmax", bs.Msmax}, {"Msmean", bs.Msmean},
		{"Msmed", bs.Msmed}, {"Msstd", bs.Msstd},
		{"ValAtChmin", bs.ValAtChmin}, {"ValAtChmax", bs.ValAtChmax},
		{"DChmean", bs.DChmean}, {"DChmed", bs.DChmed}, {"DChstd", bs.DChstd},
		{"DMsmin", bs.DMsmin}, {"DMsmax", bs.DMsmax}, {"DMsmean", bs.DMsmean},
		{"DMsmed", bs.DMsmed}, {"DMsstd", bs.DMsstd},
	} {
		isNaV(t, c.name, c.got)
	}
	if !IsNaDuration(bs.DChmin) || !IsNaDuration(bs.DChmax) {
		t.Errorf("DChmin = %v, DChmax = %v, want NaDuration", bs.DChmin, bs.DChmax)
	}
	if !bs.Chmin.IsZero() || !bs.Chmax.IsZero() {
		t.Error("the extent of an empty series is not the zero time")
	}
}

func TestStatsOnNothingButGaps(t *testing.T) {
	bs := statsOf(nav.NaV, nav.NaV, nav.NaV)

	if bs.Len != 3 {
		t.Errorf("Len = %d, want 3: the points exist, their measurements do not", bs.Len)
	}
	if bs.NbreOfNaV != 3 || bs.NbreOfNaN != 3 {
		t.Errorf("NbreOfNaV = %d, NbreOfNaN = %d, want 3 and 3", bs.NbreOfNaV, bs.NbreOfNaN)
	}
	isNaV(t, "Msmean", bs.Msmean)
	isNaV(t, "Msmin", bs.Msmin)
	if !bs.ChAtMsmin.IsZero() {
		t.Error("ChAtMsmin points at an instant although there is no minimum")
	}
	// The clock kept running, so the intervals are real.
	if bs.DChmin != time.Hour {
		t.Errorf("DChmin = %v, want 1h: timestamps are never missing", bs.DChmin)
	}
}

func TestStatsOnASinglePoint(t *testing.T) {
	bs := statsOf(42)

	eq(t, "Msmean", bs.Msmean, 42)
	eq(t, "Msmed", bs.Msmed, 42)
	eq(t, "Msmin", bs.Msmin, 42)
	// One point says nothing about dispersion, nor about intervals.
	isNaV(t, "Msstd", bs.Msstd)
	if !IsNaDuration(bs.DChmin) {
		t.Errorf("DChmin = %v, want NaDuration on a single point", bs.DChmin)
	}
	isNaV(t, "DChmean", bs.DChmean)
}

// -----------------------------------------------------------------------
// Errors are not gaps
// -----------------------------------------------------------------------

// A broken measurement contaminates the statistics that depend on it,
// and the counters tell the two kinds of non-number apart.
func TestStatsPropagateAnError(t *testing.T) {
	bs := statsOf(10, math.NaN(), 30)

	if bs.NbreOfNaN != 1 || bs.NbreOfNaV != 0 {
		t.Errorf("NbreOfNaN = %d, NbreOfNaV = %d, want 1 and 0: an error is not a gap",
			bs.NbreOfNaN, bs.NbreOfNaV)
	}
	for _, c := range []struct {
		name string
		got  float64
	}{{"Msmean", bs.Msmean}, {"Msmed", bs.Msmed}, {"Msstd", bs.Msstd}, {"Msmin", bs.Msmin}} {
		if nav.IsNaV(c.got) {
			t.Errorf("%s = NaV, want a plain NaN: the error must stay visible", c.name)
		}
		if !math.IsNaN(c.got) {
			t.Errorf("%s = %s, want a plain NaN", c.name, nav.Format(c.got))
		}
	}
	// The timestamps are untouched by a broken measurement.
	if !bs.Chmin.Equal(at(0)) || bs.DChmin != time.Hour {
		t.Error("an error in a measurement disturbed the statistics of the timestamps")
	}
}

// -----------------------------------------------------------------------
// Details that were wrong before
// -----------------------------------------------------------------------

// When several points share the extreme value, the instant reported is
// the first one.
func TestStatsDateTheFirstOccurrenceOfAnExtreme(t *testing.T) {
	bs := statsOf(5, 12, 5, 12)

	if !bs.ChAtMsmin.Equal(at(0)) {
		t.Errorf("ChAtMsmin = %s, want hour 0", bs.ChAtMsmin.Format("15:04"))
	}
	if !bs.ChAtMsmax.Equal(at(1)) {
		t.Errorf("ChAtMsmax = %s, want hour 1", bs.ChAtMsmax.Format("15:04"))
	}
}

// The mean of the timestamps is computed on offsets from the first
// point, not on nanoseconds since 1970. A float64 has 53 bits of
// mantissa where a current Unix timestamp in nanoseconds needs 61, so
// the absolute arithmetic — what the previous version of the library
// did — rounds to a few hundred nanoseconds.
func TestStatsChronMeanIsExactToTheNanosecond(t *testing.T) {
	base := time.Date(2026, 9, 23, 14, 30, 0, 0, time.UTC)
	ts := NewTimeSeries("nanoseconds")
	ts.AddAll([]Datum{
		NewDatum(base, 1),
		NewDatum(base.Add(2*time.Nanosecond), 2),
		NewDatum(base.Add(4*time.Nanosecond), 3),
	})

	bs := ts.Stats()
	want := base.Add(2 * time.Nanosecond)
	if !bs.Chmean.Equal(want) {
		t.Errorf("Chmean = %s, want %s (off by %v)",
			bs.Chmean.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano),
			bs.Chmean.Sub(want))
	}
}

// Stats must not disturb the series it summarizes: it sorts copies.
func TestStatsLeavesTheSeriesAlone(t *testing.T) {
	ts := referenceSeries()
	before := make([]DataUnit, 0, ts.Len())
	ts.Range(func(_ int, du DataUnit) bool {
		before = append(before, du)
		return true
	})

	_ = ts.Stats()

	checkInvariant(t, ts)
	for i, was := range before {
		now := ts.At(i)
		if !now.Chron.Equal(was.Chron) || !sameFloat(now.Meas, was.Meas) ||
			now.Dchron != was.Dchron || !sameFloat(now.Dmeas, was.Dmeas) {
			t.Fatalf("point %d changed during Stats()", i)
		}
	}
}

// Asking twice must answer the same thing: nothing is cached, so
// nothing can go stale — and nothing can drift either.
func TestStatsIsRepeatable(t *testing.T) {
	ts := referenceSeries()
	a, b := ts.Stats(), ts.Stats()

	if a.Len != b.Len || !sameFloat(a.Msmean, b.Msmean) || a.DChmin != b.DChmin {
		t.Error("two consecutive calls to Stats disagree")
	}

	// And a summary taken before a change must not be affected by it:
	// BasicStats is a value, not a window onto the series.
	ts.Add(NewDatum(at(10), 1000))
	if !sameFloat(a.Msmean, 20) {
		t.Errorf("the earlier summary moved to %s when the series grew", nav.Format(a.Msmean))
	}
	if c := ts.Stats(); sameFloat(c.Msmean, a.Msmean) {
		t.Error("the new summary did not take the added point into account")
	}
}
