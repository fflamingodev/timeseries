package timeseries

import (
	"fmt"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// Downscaling groups the readings by social period — day, week, month,
// year — for presentation. It is not a finer form of Regularize: it is a
// coarser one, and the computation loses by it.
//
// Regularize is the tool for computing. Its windows all last the same
// duration, so two of its points always weigh the same, and a sum, a
// count or an integral can be compared from one window to the next.
//
// The periods of the calendar hide a variability in duration behind a
// familiar name:
//
//   - a month lasts 28, 29, 30 or 31 days: February holds about 10 %
//     less time than March;
//   - a year lasts 365 or 366 days;
//   - a day lasts 24 hours, except when the clocks change: 23 hours in
//     spring, 25 in autumn;
//   - only the week holds a constant number of days, and even it may
//     hold an hour more or less.
//
// Two monthly points therefore do not weigh the same. An aggregator that
// grows with the length of the window — AggSum, AggCountUsable,
// AggIntegral — gives numbers that differ partly because the periods
// do; February comes out lower for the sole reason that it is shorter.
// A mean, a minimum or a maximum suffers less, but its points still rest
// on unequal spans of time.
//
// Downscaling is for reporting to people, who think in months: regularize
// first to compute, then downscale, last, to show.
//
// # The convention
//
//   - A period covers [start, next start): the first instant of the
//     period, up to but not including the first instant of the next. A
//     reading at midnight sharp opens the new day — where Regularize,
//     closed on the right, would count it in the day that just ended.
//   - The emitted point carries the last instant of the period, one
//     nanosecond before the next one starts, so that it reads as
//     "everything up to here".
//   - A period without a single reading is emitted as NaV, so that no
//     month is silently skipped.
//
// # Time zones
//
// Calendar periods only mean something in a place: a day in Luxembourg
// is not a day in Tokyo. The period boundaries are computed in the
// location of the series' first reading, which is the location its
// timestamps carry. Convert the series to the location you mean to
// report in before downscaling.

// DownscaleDaily groups the readings by calendar day, for presentation.
// Days last 24 hours, except the two of the year when the clocks change;
// for a fixed duration, use Regularize(24*time.Hour, …).
func (ts *TimeSeries) DownscaleDaily(agg AggFunc) (*TimeSeries, error) {
	return ts.downscale(agg, "daily", startOfDay, func(t time.Time) time.Time {
		y, m, d := t.Date()
		return firstInstant(y, m, d+1, t.Location())
	})
}

// DownscaleWeekly groups the readings by calendar week, running from
// Monday to Sunday as ISO 8601 has it, for presentation. The week that
// holds a change of clock is an hour shorter or longer than the others.
func (ts *TimeSeries) DownscaleWeekly(agg AggFunc) (*TimeSeries, error) {
	return ts.downscale(agg, "weekly", startOfWeek, func(t time.Time) time.Time {
		y, m, d := t.Date()
		return firstInstant(y, m, d+7, t.Location())
	})
}

// DownscaleMonthly groups the readings by calendar month, for
// presentation. Months last from 28 to 31 days: their points do not
// weigh the same, and a sum or a count mostly reflects their length.
func (ts *TimeSeries) DownscaleMonthly(agg AggFunc) (*TimeSeries, error) {
	return ts.downscale(agg, "monthly", startOfMonth, func(t time.Time) time.Time {
		return firstInstant(t.Year(), t.Month()+1, 1, t.Location())
	})
}

// DownscaleYearly groups the readings by calendar year, for
// presentation. A leap year holds one day more than the others.
func (ts *TimeSeries) DownscaleYearly(agg AggFunc) (*TimeSeries, error) {
	return ts.downscale(agg, "yearly", startOfYear, func(t time.Time) time.Time {
		return firstInstant(t.Year()+1, 1, 1, t.Location())
	})
}

// firstInstant returns the first instant of the calendar day y-m-d in
// loc. Out-of-range values are normalized, as time.Date does: the 32nd
// of January is the 1st of February.
//
// That instant is midnight almost everywhere and almost always, but not
// where the clocks spring forward at midnight — Santiago, Havana, the
// Azores — and there, midnight does not exist: the day opens at 01:00.
// time.Date then hands back an instant of the day before, and a period
// built on it would close an hour early, and every period after it too.
// In that case the day is searched for its true first instant.
func firstInstant(y int, m time.Month, d int, loc *time.Location) time.Time {
	// Noon exists on every day of every zone, and fixes the normalized date.
	noon := time.Date(y, m, d, 12, 0, 0, 0, loc)
	y, m, d = noon.Date()

	midnight := time.Date(y, m, d, 0, 0, 0, 0, loc)
	if my, mm, md := midnight.Date(); my == y && mm == m && md == d &&
		midnight.Hour() == 0 && midnight.Minute() == 0 && midnight.Second() == 0 {
		return midnight
	}

	// Bisect between an instant surely on the day before and noon: the
	// first instant whose local date is y-m-d is the day's opening.
	before, after := noon.Add(-36*time.Hour), noon
	for after.Sub(before) > 1 {
		mid := before.Add(after.Sub(before) / 2)
		if my, mm, md := mid.Date(); my == y && mm == m && md == d {
			after = mid
		} else {
			before = mid
		}
	}
	return after
}

// startOfDay returns the first instant of the day of t.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return firstInstant(y, m, d, t.Location())
}

// startOfWeek returns the first instant of the Monday of the week of t.
func startOfWeek(t time.Time) time.Time {
	y, m, d := t.Date()
	// Go counts Sunday as 0; ISO weeks start on Monday.
	back := (int(t.Weekday()) + 6) % 7
	return firstInstant(y, m, d-back, t.Location())
}

// startOfMonth returns the first instant of the first day of the month
// of t.
func startOfMonth(t time.Time) time.Time {
	return firstInstant(t.Year(), t.Month(), 1, t.Location())
}

// startOfYear returns the first instant of the first of January of the
// year of t.
func startOfYear(t time.Time) time.Time {
	return firstInstant(t.Year(), 1, 1, t.Location())
}

// downscale walks the calendar periods covering the series, from the
// one holding the first reading to the one holding the last, and
// condenses each with agg.
//
// startOf opens the period containing an instant, and next moves to the
// following period. Keeping those two as arguments is what lets the
// same loop serve days, weeks, months and years without a single
// special case for the lengths of February or for a change of clocks.
func (ts *TimeSeries) downscale(agg AggFunc, how string,
	startOf func(time.Time) time.Time, next func(time.Time) time.Time) (*TimeSeries, error) {

	if agg == nil {
		return nil, fmt.Errorf("%w: no aggregator given", ErrRegularizeArg)
	}

	out := NewTimeSeries(ts.Name + " " + how)
	out.ID = ts.ID
	out.Comment = "grouped by calendar " + how + ", periods of unequal length"

	n := ts.Len()
	if n == 0 {
		return out, nil
	}

	last, _ := ts.Last()
	periodStart := startOf(ts.At(0).Chron)

	var grid []Datum
	window := make([]float64, 0, 16)

	i := 0
	for !periodStart.After(last.Chron) {
		periodEnd := next(periodStart)

		window = window[:0]
		for i < n && ts.At(i).Chron.Before(periodEnd) {
			window = append(window, ts.At(i).Meas)
			i++
		}

		// The point is dated at the last instant of the period, so that
		// it reads as "everything up to here".
		value := nav.NaV
		if len(window) > 0 {
			value = agg(window)
		}
		grid = append(grid, NewDatum(periodEnd.Add(-time.Nanosecond), value))

		periodStart = periodEnd
	}

	out.AddBatchData(grid)
	return out, nil
}
