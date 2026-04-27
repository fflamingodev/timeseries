package timeseries

import (
	"time"
)

// Expand rebuilds a regular grid of points from start to end with the
// given step, using forward-fill (step-function) semantics on top of
// tsReduced.
//
// Convention:
//   - A reduced point at Chron = tc means "value becomes active at tc
//     (inclusive)".
//   - If start is before the first reduced Chron, the grid begins with NaV.
//   - end is included if (end-start) is a multiple of step; otherwise the
//     last emitted tick is the greatest tick <= end.
//
// Expand is the inverse of Reduce and is typically used to restore a
// dense, evenly-spaced view over a state-machine-like signal.
func (tsReduced *TimeSeries) Expand(start, end time.Time, step time.Duration) (TimeSeries, error) {
	var out TimeSeries

	if step <= 0 {
		return out, &Error{
			Op:    "Expand",
			Kind:  KindInvalidArg,
			Msg:   "step must be > 0",
			Field: "step",
			Value: step,
		}
	}
	if end.Before(start) {
		return out, &Error{
			Op:   "Expand",
			Kind: KindInvalidArg,
			Msg:  "end must not be before start",
		}
	}
	if len(tsReduced.DataSeries) == 0 {
		// Empty grid filled with NaV.
		for t := start; !t.After(end); t = t.Add(step) {
			out.AddDataUnit(NewDataUnit(t, NaV))
		}
		return out, nil
	}

	tsReduced.SortChronAsc()

	// Index into the reduced series.
	k := 0

	// Current state: NaV by default (if start is before the first point).
	cur := NaV

	// If reduced points are <= start, initialize cur with the last of those.
	for k < len(tsReduced.DataSeries) && !tsReduced.DataSeries[k].Chron.After(start) {
		cur = tsReduced.DataSeries[k].Meas
		k++
	}

	for t := start; !t.After(end); t = t.Add(step) {
		// Advance state while we have changes <= t.
		for k < len(tsReduced.DataSeries) && !tsReduced.DataSeries[k].Chron.After(t) {
			cur = tsReduced.DataSeries[k].Meas
			k++
		}
		out.AddDataUnit(NewDataUnit(t, cur))
	}

	return out, nil
}
