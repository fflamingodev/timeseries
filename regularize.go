package timeseries

import (
	"math"
	"time"
)

// IsSorted reports whether the DataSeries is sorted in non-decreasing
// chronological order. It's used as a fast-path by resampling routines to
// avoid a redundant O(N log N) sort when the caller already guarantees the
// order.
func (ts *TimeSeries) IsSorted() bool {
	for i := 1; i < len(ts.DataSeries); i++ {
		if ts.DataSeries[i].Chron.Before(ts.DataSeries[i-1].Chron) {
			return false
		}
	}
	return true
}

// Regularize resamples ts on a regular grid of step freq and returns a new
// TimeSeries. It runs in a single O(N+M) pass (N inputs, M output buckets)
// and reuses a working buffer; no allocation per bucket.
//
// Grid alignment. The first emitted timestamp is the smallest multiple of
// freq that is >= the first input timestamp (where "multiple of freq" is
// defined by time.Time.Truncate). A point whose Chron falls in the half-open
// window (windowEnd-freq, windowEnd] is attributed to the bucket at
// windowEnd. The point that is exactly on a grid tick belongs to that tick's
// bucket (right-closed convention).
//
// Missing buckets. Any bucket lying between two populated buckets but
// containing no input point is emitted with Meas = NaV. Leading empty
// buckets (before the first input) and trailing empty buckets (after the
// last input) are NOT emitted.
//
// Aggregation. The caller supplies an AggFunc that receives the slice of
// input Meas values for each non-empty bucket. Empty buckets bypass agg and
// are emitted as NaV directly. Note: agg may receive NaV or NaN values; it
// is expected to handle them (most aggregators in this package skip NaV).
//
// If freq <= 0 or ts is empty, Regularize returns an empty series.
// Regularize may sort ts in place if it is not already sorted.
func (ts *TimeSeries) Regularize(freq time.Duration, agg AggFunc) TimeSeries {
	return ts.regularize(freq, agg, 0)
}

// RegularizeWithTolerance is Regularize with a tolerance on late-arriving
// samples: a point whose Chron falls in (windowEnd, windowEnd+tolerance]
// is attributed to the bucket at windowEnd instead of being pushed into
// the next one. This is the common case for jittery sensors whose k-th
// tick can arrive a few seconds or minutes past k*freq.
//
// tolerance must satisfy 0 <= tolerance < freq. Negative values are
// treated as 0. An *Error with Kind == KindInvalidArg is returned when
// tolerance >= freq (window membership would be ambiguous) or when freq
// <= 0. When tolerance == 0 the function is exactly equivalent to
// Regularize.
func (ts *TimeSeries) RegularizeWithTolerance(freq time.Duration, agg AggFunc, tolerance time.Duration) (TimeSeries, error) {
	if freq <= 0 {
		return TimeSeries{}, &Error{
			Op:    "RegularizeWithTolerance",
			Kind:  KindInvalidArg,
			Msg:   "freq must be > 0",
			Field: "freq",
			Value: freq,
		}
	}
	if tolerance < 0 {
		tolerance = 0
	}
	if tolerance >= freq {
		return TimeSeries{}, &Error{
			Op:    "RegularizeWithTolerance",
			Kind:  KindInvalidArg,
			Msg:   "tolerance must be strictly less than freq",
			Field: "tolerance",
			Value: tolerance,
		}
	}
	return ts.regularize(freq, agg, tolerance), nil
}

// MustRegularizeWithTolerance is the panicking variant of
// RegularizeWithTolerance, for callers who treat the constraints on
// freq and tolerance as coding invariants rather than runtime data.
func (ts *TimeSeries) MustRegularizeWithTolerance(freq time.Duration, agg AggFunc, tolerance time.Duration) TimeSeries {
	out, err := ts.RegularizeWithTolerance(freq, agg, tolerance)
	if err != nil {
		panic(err)
	}
	return out
}

// regularize is the shared implementation. Both public entry points delegate
// to it. Keeping a single loop here avoids code duplication and a double
// code path for the hot loop.
func (ts *TimeSeries) regularize(freq time.Duration, agg AggFunc, tolerance time.Duration) TimeSeries {
	var out TimeSeries
	if freq <= 0 {
		return out
	}
	n := len(ts.DataSeries)
	if n == 0 {
		return out
	}
	if !ts.IsSorted() {
		ts.SortChronAsc()
	}

	first := ts.DataSeries[0].Chron
	last := ts.DataSeries[n-1].Chron

	// Align windowEnd to the first grid tick >= first.
	windowEnd := first.Truncate(freq)
	if windowEnd.Before(first) {
		windowEnd = windowEnd.Add(freq)
	}

	// Pre-size the output slice: (last - windowEnd)/freq + 1 buckets is a
	// tight upper bound when no tolerance is in play; +1 for rounding slack.
	est := int(last.Sub(windowEnd)/freq) + 2
	if est < 1 {
		est = 1
	}
	out.DataSeries = make([]DataUnit, 0, est)

	// Reusable scratch buffer for the current bucket's Meas values.
	local := make([]float64, 0, 16)

	i := 0
	for i < n {
		upper := windowEnd.Add(tolerance)

		// Advance past empty buckets, emitting NaV for each. Using After
		// keeps the right edge inclusive (matches the right-closed window
		// convention documented above).
		for ts.DataSeries[i].Chron.After(upper) {
			out.DataSeries = append(out.DataSeries, NewDataUnit(windowEnd, NaV))
			windowEnd = windowEnd.Add(freq)
			upper = windowEnd.Add(tolerance)
		}

		// Gather points into current bucket.
		local = local[:0]
		for i < n && !ts.DataSeries[i].Chron.After(upper) {
			local = append(local, ts.DataSeries[i].Meas)
			i++
		}

		out.DataSeries = append(out.DataSeries, NewDataUnit(windowEnd, agg(local)))
		if i < n {
			windowEnd = windowEnd.Add(freq)
		}
	}

	return out
}

// stateChanged reports whether the Meas value transitioned between prev and
// cur. It is used by Reduce to collapse runs of identical measurements.
// NaV↔NaV transitions are "no change"; any other transition involving NaV
// counts as a change.
func stateChanged(prevMeas, curMeas, dmeas float64) bool {
	if IsNaV(prevMeas) && IsNaV(curMeas) {
		return false
	}
	if IsNaV(prevMeas) || IsNaV(curMeas) {
		return true
	}
	return dmeas != 0
}

// Reduce returns a new series keeping only the points where the measured
// value changed compared to the previous one. The first point is always
// kept. This is the inverse of Expand and is useful to compress signals
// that stay constant between updates (event-driven series, state changes).
func (ts *TimeSeries) Reduce() TimeSeries {
	var out TimeSeries
	n := len(ts.DataSeries)
	if n == 0 {
		return out
	}

	if !ts.IsSorted() {
		ts.SortChronAsc()
	}

	out.AddDataUnit(ts.DataSeries[0])

	for i := 1; i < n; i++ {
		prev := ts.DataSeries[i-1]
		cur := ts.DataSeries[i]
		if stateChanged(prev.Meas, cur.Meas, cur.Dmeas) {
			out.AddDataUnit(cur)
		}
	}
	return out
}

// RoundedStartTime returns timetoround truncated down to a multiple of per.
// It is kept as a small helper so callers can pre-compute a grid origin
// outside of Regularize (e.g., to align multiple series on the same phase).
func RoundedStartTime(timetoround time.Time, per time.Duration) time.Time {
	return timetoround.Truncate(per)
}

// Bounds returns (min, max) of xs, skipping NaN and NaV values. If xs is
// empty or contains only NaN/NaV, it returns (NaV, NaV).
//
// Bounds does not allocate and does not modify its input.
func Bounds(xs []float64) (minV, maxV float64) {
	minV, maxV = NaV, NaV
	started := false
	for _, x := range xs {
		if math.IsNaN(x) {
			continue
		}
		if !started {
			minV, maxV = x, x
			started = true
			continue
		}
		if x < minV {
			minV = x
		}
		if x > maxV {
			maxV = x
		}
	}
	return minV, maxV
}

// HourlyAvg returns a fixed-size array of 24 floats where index h holds the
// mean of all measurements whose Chron.Hour() == h. NaV and NaN values are
// ignored in the average. If no valid samples exist for a given hour, that
// cell is NaV.
//
// This is a convenience for day-of-day profiles (typical demand curves,
// etc.) and is not a substitute for a full downscaling.
func (ts *TimeSeries) HourlyAvg() [24]float64 {
	var sum [24]float64
	var count [24]float64
	for _, val := range ts.DataSeries {
		if math.IsNaN(val.Meas) {
			continue
		}
		h := val.Chron.Hour()
		sum[h] += val.Meas
		count[h]++
	}

	var hr [24]float64
	for h := 0; h < 24; h++ {
		if count[h] == 0 {
			hr[h] = NaV
		} else {
			hr[h] = sum[h] / count[h]
		}
	}
	return hr
}

// DownscaleMonthly groups the series by calendar months.
//   - For each month, agg() is applied to the month's measurements.
//   - The output timestamp is the last nanosecond of that month (end-of-month).
//   - Months with no input produce a point with Meas = NaV.
func (ts *TimeSeries) DownscaleMonthly(agg AggFunc) TimeSeries {
	var out TimeSeries
	if len(ts.DataSeries) == 0 {
		return out
	}
	if !ts.IsSorted() {
		ts.SortChronAsc()
	}

	first := ts.DataSeries[0].Chron
	last := ts.DataSeries[len(ts.DataSeries)-1].Chron
	loc := first.Location()
	monthStart := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, loc)

	i := 0
	for !monthStart.After(last) {
		nextMonthStart := monthStart.AddDate(0, 1, 0)

		var local []float64
		for i < len(ts.DataSeries) &&
			!ts.DataSeries[i].Chron.Before(monthStart) &&
			ts.DataSeries[i].Chron.Before(nextMonthStart) {
			local = append(local, ts.DataSeries[i].Meas)
			i++
		}

		monthEnd := nextMonthStart.Add(-time.Nanosecond)
		du := NewDataUnit(monthEnd, NaV)
		if len(local) > 0 {
			du.Meas = agg(local)
		}
		out.AddDataUnit(du)

		monthStart = nextMonthStart
	}
	return out
}

// DownscaleDaily groups the series by calendar days, using the local
// timezone of the first point.
//   - Each output point corresponds to one calendar day.
//   - The output timestamp is the last nanosecond of that day.
//   - Days with no input produce a point with Meas = NaV.
func (ts *TimeSeries) DownscaleDaily(agg AggFunc) TimeSeries {
	var out TimeSeries
	if len(ts.DataSeries) == 0 {
		return out
	}
	if !ts.IsSorted() {
		ts.SortChronAsc()
	}

	first := ts.DataSeries[0].Chron
	last := ts.DataSeries[len(ts.DataSeries)-1].Chron
	loc := first.Location()
	dayStart := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, loc)

	i := 0
	for !dayStart.After(last) {
		nextDayStart := dayStart.AddDate(0, 0, 1)

		var local []float64
		for i < len(ts.DataSeries) &&
			!ts.DataSeries[i].Chron.Before(dayStart) &&
			ts.DataSeries[i].Chron.Before(nextDayStart) {
			local = append(local, ts.DataSeries[i].Meas)
			i++
		}

		dayEnd := nextDayStart.Add(-time.Nanosecond)
		du := NewDataUnit(dayEnd, NaV)
		if len(local) > 0 {
			du.Meas = agg(local)
		}
		out.AddDataUnit(du)

		dayStart = nextDayStart
	}
	return out
}

// DownscaleYearly groups the series by calendar years.
//   - One output point per year, covering [Jan 1 00:00, Jan 1 next year).
//   - The output timestamp is the last nanosecond of the year.
//   - Years with no input produce a point with Meas = NaV.
func (ts *TimeSeries) DownscaleYearly(agg AggFunc) TimeSeries {
	var out TimeSeries
	if len(ts.DataSeries) == 0 {
		return out
	}
	if !ts.IsSorted() {
		ts.SortChronAsc()
	}

	first := ts.DataSeries[0].Chron
	last := ts.DataSeries[len(ts.DataSeries)-1].Chron
	loc := first.Location()
	yearStart := time.Date(first.Year(), 1, 1, 0, 0, 0, 0, loc)

	i := 0
	for !yearStart.After(last) {
		nextYearStart := yearStart.AddDate(1, 0, 0)

		var local []float64
		for i < len(ts.DataSeries) &&
			!ts.DataSeries[i].Chron.Before(yearStart) &&
			ts.DataSeries[i].Chron.Before(nextYearStart) {
			local = append(local, ts.DataSeries[i].Meas)
			i++
		}

		yearEnd := nextYearStart.Add(-time.Nanosecond)
		du := NewDataUnit(yearEnd, NaV)
		if len(local) > 0 {
			du.Meas = agg(local)
		}
		out.AddDataUnit(du)

		yearStart = nextYearStart
	}
	return out
}

// DownscaleWeekly groups the series by ISO-style calendar weeks starting on
// Monday.
//   - Each output point spans [Monday 00:00, next Monday 00:00).
//   - The output timestamp is the last nanosecond of the week.
//   - Weeks with no input produce a point with Meas = NaV.
func (ts *TimeSeries) DownscaleWeekly(agg AggFunc) TimeSeries {
	var out TimeSeries
	if len(ts.DataSeries) == 0 {
		return out
	}
	if !ts.IsSorted() {
		ts.SortChronAsc()
	}

	first := ts.DataSeries[0].Chron
	last := ts.DataSeries[len(ts.DataSeries)-1].Chron
	loc := first.Location()

	firstDayStart := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, loc)
	// Go weekday: Sunday=0, Monday=1, ..., Saturday=6.
	// Offset to Monday of the same ISO week.
	offsetDays := (int(firstDayStart.Weekday()) + 6) % 7
	weekStart := firstDayStart.AddDate(0, 0, -offsetDays)

	i := 0
	for !weekStart.After(last) {
		nextWeekStart := weekStart.AddDate(0, 0, 7)

		var local []float64
		for i < len(ts.DataSeries) &&
			!ts.DataSeries[i].Chron.Before(weekStart) &&
			ts.DataSeries[i].Chron.Before(nextWeekStart) {
			local = append(local, ts.DataSeries[i].Meas)
			i++
		}

		weekEnd := nextWeekStart.Add(-time.Nanosecond)
		du := NewDataUnit(weekEnd, NaV)
		if len(local) > 0 {
			du.Meas = agg(local)
		}
		out.AddDataUnit(du)

		weekStart = nextWeekStart
	}
	return out
}
