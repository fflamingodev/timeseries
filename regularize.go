package timeseries

import (
	"errors"
	"fmt"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// Regularizing means putting readings that arrived when they pleased
// onto a grid of fixed steps, so that two series can be compared point
// to point, a chart drawn without lying about the spacing, and a
// difference taken between neighbours that are really a step apart.
//
// # The grid
//
// Windows are closed on the right: a reading taken at exactly a tick
// belongs to the window that ends there, and the emitted point carries
// the tick as its instant. The grid is aligned on the clock — hourly
// windows end on the hour, not on the first reading — so that two
// series regularized at the same step land on the very same instants,
// whatever time they each started.
//
// # Empty windows
//
// A window that caught no reading is emitted as NaV. This is the whole
// point: a regular grid must have a point per step, and a step where
// nothing arrived is a gap, not an absence of step. Leading and
// trailing empty windows are not emitted, since a series says nothing
// about what happened before it started or after it ended.

// ErrRegularizeArg reports a step or a tolerance that cannot define a
// grid. Test for it with errors.Is.
var ErrRegularizeArg = errors.New("timeseries: impossible regularization argument")

// Regularize resamples the series on a grid of step freq, condensing
// the readings of each window with agg.
//
//	hourly, err := ts.Regularize(time.Hour, timeseries.AggMean)
//
// The step must be above zero. The returned series is chronological
// with exact deltas, like any other.
func (ts *TimeSeries) Regularize(freq time.Duration, agg AggFunc) (*TimeSeries, error) {
	return ts.RegularizeWithTolerance(freq, 0, agg)
}

// RegularizeWithTolerance is Regularize with a grace period for late
// readings: a reading arriving up to tolerance after a tick is counted
// in the window that just closed, instead of opening the next one.
//
// This is what a drifting logger calls for. A sensor meant to report on
// the hour that reports at 10:00:04 has not skipped its window, and
// without a tolerance every such reading would land one window late,
// leaving the hourly window empty and the next one holding two
// readings.
//
// The tolerance must be below the step — beyond that, a reading could
// belong to two windows — and at least zero, where the method is
// exactly Regularize.
func (ts *TimeSeries) RegularizeWithTolerance(freq, tolerance time.Duration, agg AggFunc) (*TimeSeries, error) {
	if freq <= 0 {
		return nil, fmt.Errorf("%w: the step must be above zero, got %v", ErrRegularizeArg, freq)
	}
	if tolerance < 0 {
		return nil, fmt.Errorf("%w: the tolerance cannot be negative, got %v", ErrRegularizeArg, tolerance)
	}
	if tolerance >= freq {
		return nil, fmt.Errorf("%w: the tolerance %v must stay below the step %v, "+
			"otherwise a reading belongs to two windows", ErrRegularizeArg, tolerance, freq)
	}
	if agg == nil {
		return nil, fmt.Errorf("%w: no aggregator given", ErrRegularizeArg)
	}

	out := NewTimeSeries(ts.Name + " regularized")
	out.ID = ts.ID
	out.Comment = fmt.Sprintf("every %v", freq)
	if tolerance > 0 {
		out.Comment += fmt.Sprintf(", late readings tolerated up to %v", tolerance)
	}

	n := ts.Len()
	if n == 0 {
		return out, nil
	}

	// The first tick at or after the first reading. Truncate aligns on
	// the clock, so hourly windows end on the hour.
	windowEnd := ts.At(0).Chron.Truncate(freq)
	if windowEnd.Before(ts.At(0).Chron) {
		windowEnd = windowEnd.Add(freq)
	}

	grid := make([]Datum, 0, estimateWindows(ts, freq))
	window := make([]float64, 0, 16)

	i := 0
	for i < n {
		limit := windowEnd.Add(tolerance)

		// Step over the windows that caught nothing. They are emitted as
		// gaps, except before the first reading, where there is no series
		// yet to have a gap in.
		for ts.At(i).Chron.After(limit) {
			if len(grid) > 0 {
				grid = append(grid, NewDatum(windowEnd, nav.NaV))
			}
			windowEnd = windowEnd.Add(freq)
			limit = windowEnd.Add(tolerance)
		}

		window = window[:0]
		for i < n && !ts.At(i).Chron.After(limit) {
			window = append(window, ts.At(i).Meas)
			i++
		}

		grid = append(grid, NewDatum(windowEnd, agg(window)))
		windowEnd = windowEnd.Add(freq)
	}

	out.AddBatchData(grid)
	return out, nil
}

// estimateWindows guesses how many points the grid will hold, to size
// the slice once instead of growing it. Being wrong only costs a
// reallocation.
func estimateWindows(ts *TimeSeries, freq time.Duration) int {
	first, _ := ts.First()
	last, _ := ts.Last()
	span := last.Chron.Sub(first.Chron)
	if span <= 0 {
		return 1
	}
	return int(span/freq) + 2
}
