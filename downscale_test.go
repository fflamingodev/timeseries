package timeseries

import (
	"errors"
	"math"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// period is what a downscaled series holds, stated by the instant the
// NEXT period opens — the emitted point sits one nanosecond before it —
// so that a test reads "the day ending at midnight on the 2nd".
type period struct {
	closes time.Time
	value  float64
}

func samePeriods(t *testing.T, name string, got *TimeSeries, want []period) {
	t.Helper()
	if got.Len() != len(want) {
		var seen []string
		got.Range(func(_ int, du DataUnit) bool {
			seen = append(seen, du.Chron.Format(time.RFC3339Nano)+"="+nav.Format(du.Meas))
			return true
		})
		t.Errorf("%s: %d periods, want %d\n got: %v", name, got.Len(), len(want), seen)
		return
	}
	for i, w := range want {
		du := got.At(i)
		wantChron := w.closes.Add(-time.Nanosecond)
		if !du.Chron.Equal(wantChron) || !sameFloat(du.Meas, w.value) {
			t.Errorf("%s: period %d is %s=%s, want %s=%s", name, i,
				du.Chron.Format(time.RFC3339Nano), nav.Format(du.Meas),
				wantChron.Format(time.RFC3339Nano), nav.Format(w.value))
		}
	}
}

// seriesAt builds a series from instants and values.
func seriesAt(pairs ...struct {
	t time.Time
	v float64
}) *TimeSeries {
	ts := NewTimeSeries("source")
	data := make([]Datum, 0, len(pairs))
	for _, p := range pairs {
		data = append(data, NewDatum(p.t, p.v))
	}
	ts.AddBatchData(data)
	return ts
}

type tv = struct {
	t time.Time
	v float64
}

func day(y int, m time.Month, d, h int, loc *time.Location) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, loc)
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone %s not available: %v", name, err)
	}
	return loc
}

// -----------------------------------------------------------------------
// The helpers opening a period
// -----------------------------------------------------------------------

func TestStartOfPeriods(t *testing.T) {
	utc := time.UTC
	// Thursday 1 January 2026, mid-afternoon.
	x := time.Date(2026, 1, 1, 15, 42, 7, 123, utc)

	cases := []struct {
		name string
		got  time.Time
		want time.Time
	}{
		{"day", startOfDay(x), day(2026, 1, 1, 0, utc)},
		{"week of a Thursday", startOfWeek(x), day(2025, 12, 29, 0, utc)},
		{"week of a Monday", startOfWeek(day(2026, 1, 5, 9, utc)), day(2026, 1, 5, 0, utc)},
		{"week of a Sunday", startOfWeek(day(2026, 1, 11, 23, utc)), day(2026, 1, 5, 0, utc)},
		{"month", startOfMonth(x), day(2026, 1, 1, 0, utc)},
		{"month, last day", startOfMonth(day(2026, 3, 31, 23, utc)), day(2026, 3, 1, 0, utc)},
		{"year", startOfYear(day(2026, 12, 31, 23, utc)), day(2026, 1, 1, 0, utc)},
	}
	for _, c := range cases {
		if !c.got.Equal(c.want) {
			t.Errorf("%s: %s, want %s", c.name, c.got.Format(time.RFC3339Nano), c.want.Format(time.RFC3339))
		}
	}
}

// -----------------------------------------------------------------------
// Daily
// -----------------------------------------------------------------------

func TestDownscaleDaily(t *testing.T) {
	utc := time.UTC
	ts := seriesAt(
		tv{day(2026, 1, 1, 6, utc), 1},
		tv{day(2026, 1, 1, 18, utc), 2},
		tv{day(2026, 1, 2, 12, utc), 10},
	)

	out, err := ts.DownscaleDaily(AggSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, out)
	samePeriods(t, "daily", out, []period{
		{day(2026, 1, 2, 0, utc), 3},
		{day(2026, 1, 3, 0, utc), 10},
	})
}

// A period covers [start, next start): a reading at midnight sharp opens
// the new day. This is the opposite of Regularize, where it closes the
// day that just ended.
func TestDownscaleMidnightOpensTheDay(t *testing.T) {
	utc := time.UTC
	ts := seriesAt(
		tv{day(2026, 1, 1, 12, utc), 1},
		tv{day(2026, 1, 2, 0, utc), 100}, // midnight sharp
	)

	out, err := ts.DownscaleDaily(AggSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	samePeriods(t, "midnight", out, []period{
		{day(2026, 1, 2, 0, utc), 1},
		{day(2026, 1, 3, 0, utc), 100},
	})

	// The same two readings, regularized: one window, closing at midnight.
	reg, _ := ts.Regularize(24*time.Hour, AggSum)
	if reg.Len() != 1 || reg.At(0).Meas != 101 {
		t.Errorf("Regularize: %d windows, want a single one worth 101", reg.Len())
	}
}

// A day without a reading is a gap, never skipped.
func TestDownscaleEmptyDaysBecomeGaps(t *testing.T) {
	utc := time.UTC
	ts := seriesAt(
		tv{day(2026, 1, 1, 12, utc), 1},
		tv{day(2026, 1, 4, 12, utc), 4},
	)

	out, err := ts.DownscaleDaily(AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	samePeriods(t, "with a hole", out, []period{
		{day(2026, 1, 2, 0, utc), 1},
		{day(2026, 1, 3, 0, utc), nav.NaV},
		{day(2026, 1, 4, 0, utc), nav.NaV},
		{day(2026, 1, 5, 0, utc), 4},
	})
}

// -----------------------------------------------------------------------
// Weekly, monthly, yearly
// -----------------------------------------------------------------------

func TestDownscaleWeekly(t *testing.T) {
	utc := time.UTC
	ts := seriesAt(
		tv{day(2026, 1, 1, 12, utc), 1},  // Thursday, week of Mon 29 Dec
		tv{day(2026, 1, 4, 23, utc), 2},  // Sunday, same week
		tv{day(2026, 1, 5, 0, utc), 10},  // Monday midnight, next week
		tv{day(2026, 1, 20, 8, utc), 50}, // Tuesday two weeks later
	)

	out, err := ts.DownscaleWeekly(AggSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, out)
	samePeriods(t, "weekly", out, []period{
		{day(2026, 1, 5, 0, utc), 3},
		{day(2026, 1, 12, 0, utc), 10},
		{day(2026, 1, 19, 0, utc), nav.NaV},
		{day(2026, 1, 26, 0, utc), 50},
	})
}

// February is shorter, a leap February one day longer, and each month
// closes on its own last day.
func TestDownscaleMonthly(t *testing.T) {
	utc := time.UTC
	ts := seriesAt(
		tv{day(2028, 1, 31, 23, utc), 1},
		tv{day(2028, 2, 29, 23, utc), 2}, // leap day
		tv{day(2028, 3, 1, 0, utc), 3},
		tv{day(2028, 5, 15, 0, utc), 5},
	)

	out, err := ts.DownscaleMonthly(AggSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, out)
	samePeriods(t, "monthly", out, []period{
		{day(2028, 2, 1, 0, utc), 1},
		{day(2028, 3, 1, 0, utc), 2},
		{day(2028, 4, 1, 0, utc), 3},
		{day(2028, 5, 1, 0, utc), nav.NaV},
		{day(2028, 6, 1, 0, utc), 5},
	})
}

func TestDownscaleYearly(t *testing.T) {
	utc := time.UTC
	ts := seriesAt(
		tv{day(2024, 12, 31, 23, utc), 1},
		tv{day(2025, 1, 1, 0, utc), 2},
		tv{day(2027, 6, 1, 0, utc), 7},
	)

	out, err := ts.DownscaleYearly(AggSum)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	samePeriods(t, "yearly", out, []period{
		{day(2025, 1, 1, 0, utc), 1},
		{day(2026, 1, 1, 0, utc), 2},
		{day(2027, 1, 1, 0, utc), nav.NaV},
		{day(2028, 1, 1, 0, utc), 7},
	})
}

// -----------------------------------------------------------------------
// Time zones and changes of clock
// -----------------------------------------------------------------------

// A reading every hour across both changes of clock in Paris: the day of
// spring holds 23 readings, the day of autumn 25, and every period opens
// at local midnight.
func TestDownscaleDailyAcrossDST(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")

	check := func(name string, from time.Time, hours int, wantCounts []float64, wantCloses []time.Time) {
		t.Helper()
		ts := NewTimeSeries(name)
		data := make([]Datum, 0, hours)
		for h := 0; h < hours; h++ {
			data = append(data, NewDatum(from.Add(time.Duration(h)*time.Hour), 1))
		}
		ts.AddBatchData(data)

		out, err := ts.DownscaleDaily(AggCountUsable)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		want := make([]period, len(wantCounts))
		for i := range wantCounts {
			want[i] = period{wantCloses[i], wantCounts[i]}
		}
		samePeriods(t, name, out, want)
	}

	// 28 March 00:00 to 30 March 23:00 local: 24 + 23 + 24 real hours.
	check("spring", day(2026, 3, 28, 0, paris), 71,
		[]float64{24, 23, 24},
		[]time.Time{day(2026, 3, 29, 0, paris), day(2026, 3, 30, 0, paris), day(2026, 3, 31, 0, paris)})

	// 24 October 00:00 to 26 October 23:00 local: 24 + 25 + 24 real hours.
	check("autumn", day(2026, 10, 24, 0, paris), 73,
		[]float64{24, 25, 24},
		[]time.Time{day(2026, 10, 25, 0, paris), day(2026, 10, 26, 0, paris), day(2026, 10, 27, 0, paris)})
}

// The same instants, carried in two locations, fall in different days.
func TestDownscaleFollowsTheLocationOfTheSeries(t *testing.T) {
	tokyo := mustLoad(t, "Asia/Tokyo")
	// 20:00 UTC on 1 January is 05:00 on 2 January in Tokyo.
	a := day(2026, 1, 1, 10, time.UTC)
	b := day(2026, 1, 1, 20, time.UTC)

	inUTC, _ := seriesAt(tv{a, 1}, tv{b, 2}).DownscaleDaily(AggSum)
	samePeriods(t, "UTC", inUTC, []period{{day(2026, 1, 2, 0, time.UTC), 3}})

	inTokyo, _ := seriesAt(tv{a.In(tokyo), 1}, tv{b.In(tokyo), 2}).DownscaleDaily(AggSum)
	samePeriods(t, "Tokyo", inTokyo, []period{
		{day(2026, 1, 2, 0, tokyo), 1},
		{day(2026, 1, 3, 0, tokyo), 2},
	})
}

// Where midnight does not exist — Santiago moves its clocks from 00:00
// to 01:00 on 6 September 2026 — the day still opens at the first
// instant of that day, and the days after it open at midnight again.
func TestDownscaleDailyWhereMidnightDoesNotExist(t *testing.T) {
	santiago := mustLoad(t, "America/Santiago")

	start := day(2026, 9, 5, 12, santiago)
	ts := NewTimeSeries("santiago")
	var data []Datum
	for h := 0; h < 72; h++ {
		data = append(data, NewDatum(start.Add(time.Duration(h)*time.Hour), 1))
	}
	ts.AddBatchData(data)

	out, err := ts.DownscaleDaily(AggCountUsable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var total float64
	for i := 0; i < out.Len(); i++ {
		du := out.At(i)
		total += du.Meas
		opens := du.Chron.Add(time.Nanosecond)
		if h, m, s := opens.Clock(); (h != 0 && h != 1) || m != 0 || s != 0 {
			t.Errorf("period %d closes at %s, not at a local midnight", i, du.Chron.Format(time.RFC3339Nano))
		}
	}
	if total != 72 {
		t.Errorf("%v readings counted, want 72", total)
	}
}

// -----------------------------------------------------------------------
// Gaps, errors, arguments, edges
// -----------------------------------------------------------------------

func TestDownscaleCarriesTheRulesThrough(t *testing.T) {
	utc := time.UTC
	ts := seriesAt(
		tv{day(2026, 1, 1, 1, utc), 10}, tv{day(2026, 1, 1, 2, utc), nav.NaV}, tv{day(2026, 1, 1, 3, utc), 20},
		tv{day(2026, 1, 2, 1, utc), nav.NaV}, tv{day(2026, 1, 2, 2, utc), nav.NaV},
		tv{day(2026, 1, 3, 1, utc), 5}, tv{day(2026, 1, 3, 2, utc), math.NaN()},
	)

	out, err := ts.DownscaleDaily(AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 3 {
		t.Fatalf("%d periods, want 3", out.Len())
	}
	if out.At(0).Meas != 15 {
		t.Errorf("day 1 = %s, want 15", nav.Format(out.At(0).Meas))
	}
	if !nav.IsNaV(out.At(1).Meas) {
		t.Errorf("day 2 = %s, want NaV", nav.Format(out.At(1).Meas))
	}
	if !nav.IsStdNaN(out.At(2).Meas) {
		t.Errorf("day 3 = %s, want a plain NaN", nav.Format(out.At(2).Meas))
	}
}

func TestDownscaleRejectsANilAggregator(t *testing.T) {
	ts := seriesOf(1, 2, 3)
	for name, f := range map[string]func(AggFunc) (*TimeSeries, error){
		"daily": ts.DownscaleDaily, "weekly": ts.DownscaleWeekly,
		"monthly": ts.DownscaleMonthly, "yearly": ts.DownscaleYearly,
	} {
		_, err := f(nil)
		if !errors.Is(err, ErrRegularizeArg) {
			t.Errorf("%s: error is %v, want an ErrRegularizeArg", name, err)
		}
	}
}

func TestDownscaleAnEmptySeries(t *testing.T) {
	out, err := NewTimeSeries("empty").DownscaleMonthly(AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("%d periods from an empty series", out.Len())
	}
}

func TestDownscaleASinglePoint(t *testing.T) {
	utc := time.UTC
	out, err := seriesAt(tv{day(2026, 7, 14, 9, utc), 7}).DownscaleMonthly(AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	samePeriods(t, "one reading", out, []period{{day(2026, 8, 1, 0, utc), 7}})
}

func TestDownscaleProducesAProperSeries(t *testing.T) {
	ts := seriesOf(1, 2, 3)
	ts.ID = "device-42"

	out, err := ts.DownscaleDaily(AggMean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, out)
	if out.ID != "device-42" {
		t.Error("the downscaled series lost the identity of its source")
	}
	if out.Comment == "" {
		t.Error("nothing says how the series was downscaled")
	}
	// The source is untouched.
	if ts.Len() != 3 {
		t.Errorf("the source now holds %d points", ts.Len())
	}
}
