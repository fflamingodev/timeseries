// Package timeseries conditions series of measurements: readings that
// arrive when they please, with gaps where the instrument was silent
// and values nobody believes.
//
// It cleans them, puts them on a regular grid, fills what can honestly
// be filled, and summarizes them — without ever turning a missing
// reading into a zero, and without letting one bad reading destroy a
// month of statistics.
//
//	ts := timeseries.NewTimeSeries("outdoor temperature")
//	ts.AddBatchData(readings)                                // in any order
//
//	cleaned, rejected, _ := ts.RemoveOutbounds(-40, 60)       // implausible readings
//	hourly, _ := cleaned.RegularizeWithTolerance(time.Hour, 5*time.Minute, timeseries.AggMean)
//	filled, _ := hourly.InterpolateWithin(timeseries.InterpLinear, 2*time.Hour)
//
//	filled.PrintStats()
//
// # Missing is not wrong, and neither is zero
//
// The library is built on [notavalue], which distinguishes two kinds of
// non-number:
//
//   - NaV, a missing reading. The instrument was offline, the row was
//     absent from the join, the point was rejected as implausible.
//     Nothing was measured, and nothing pretends otherwise.
//   - NaN, a broken computation. A division by zero upstream, a
//     logarithm of a negative number. Something went wrong.
//
// They are treated in opposite ways, everywhere in this package: a
// missing reading is stepped over, a broken one propagates. A month
// with a missing day still has a mean; a month with a broken value does
// not, and says so. This is what makes a gap harmless and an error
// impossible to lose.
//
// The consequences show up in places one does not expect. A variation
// computed across a gap is missing rather than invented — a reading of
// 14 following a gap is not a rise of 14. A point rejected as an
// outlier becomes missing rather than broken, so the rest of the month
// still has statistics. An interpolation fills gaps and leaves broken
// values alone, because covering an error with a plausible number is
// how a bug stops being noticed.
//
// # What a series is
//
// A [Datum] is a reading: an instant and a value, never separated. A
// [DataUnit] is a Datum placed in a series, carrying the elapsed time
// and the variation since the point before it. A [TimeSeries] is a
// sequence of them, and a [TsContainer] holds several series that
// belong together — the raw readings, the cleaned version, each variant
// a recipe produced.
//
// A series is chronological at all times, with exact deltas. There is
// no method to sort it or to recompute anything, because there is never
// anything to repair: [TimeSeries.Add] and [TimeSeries.AddBatchData]
// maintain the order as they insert. Readings may therefore be handed
// over in any order at all.
//
// # What it can do
//
// Summarize: [TimeSeries.Stats] returns thirty statistics, from the
// span of the series to the regularity of its sampling.
//
// Clean: [TimeSeries.RemoveOutbounds] on fixed bounds,
// [TimeSeries.RemovePercentileOutliers] on percentile fences,
// [TimeSeries.RemoveZScoreOutliers] on a standard deviation, and
// [TimeSeries.RemovePeirceOutliers] on Peirce's criterion. Each returns
// the cleaned series and the readings it rejected, so the decision can
// be reviewed.
//
// Resample: [TimeSeries.Regularize] onto a grid of fixed steps, and
// [TimeSeries.RegularizeWithTolerance] when the logger drifts around
// its ticks. [TimeSeries.DownscaleDaily] and its siblings group by
// calendar period, for reporting to people who think in months.
//
// Fill: [TimeSeries.Interpolate] with any of seven methods, and
// [TimeSeries.InterpolateWithin] to bridge short silences without
// drawing a smooth line through a three-day outage.
//
// Compress: [TimeSeries.Reduce] keeps only the points where the value
// changed, [TimeSeries.Expand] puts the grid back, and
// [TimeSeries.MarkSilences] records a silence as a gap so that reducing
// a raw series does not lose it.
//
// Show: [TimeSeries.PrettyPrint] and [TimeSeries.PrintStats] for a
// terminal, [TimeSeries.ToJSON] for a front end.
//
// # Time
//
// Timestamps are time.Time values, kept as given — location included.
// The package never converts a series to another zone on its own, since
// the zone a reading carries is a statement about where it was taken.
//
// Two conventions differ, deliberately, and it is worth knowing which
// is which:
//
//   - Regularize closes its windows on the right. A reading taken at
//     exactly a tick belongs to the window ending there, and the
//     emitted point carries that tick.
//   - Downscale opens its periods on the left. A reading taken at
//     midnight sharp opens the new day. The emitted point carries the
//     last instant of the period, so it still reads as "everything up
//     to here".
//
// A regular grid is aligned on absolute time, which is to say on UTC.
// In a zone offset by whole hours — most of Europe — an hourly grid
// lands on the local hour. In a zone offset by half an hour — India,
// Nepal, parts of Australia — it lands on the local half hour: 10:30,
// 11:30, and so on. That is the price of grids that line up across
// zones, which is what makes two series comparable.
//
// Calendar periods, on the other hand, only mean something in a place,
// so Downscale computes its boundaries in the location of the series'
// first reading. It handles the days when the clocks change, including
// the zones where midnight itself does not exist — Santiago, Havana,
// the Azores — and where the day opens at 01:00.
//
// Durations are time.Duration. A duration that does not exist — the
// interval before the first point of a series — is [NaDuration], the
// counterpart of NaV for time.
//
// # What it costs
//
// A point occupies 32 bytes: a time.Time of 24 and a float64 of 8. A
// series of a million readings therefore weighs about 32 MB, and one of
// ten million about 320 MB.
//
// Statistics are computed on demand and never cached, so nothing can go
// stale; the expensive part is the medians, which sort. Reckon on the
// order of 60 ns per point for a full summary, against 1 ns for a mean.
//
// Loading is the operation to get right at scale: [TimeSeries.Add]
// inserts one reading and shifts the tail if it belongs earlier, which
// is quadratic on a batch arriving in random order — two and a half
// minutes for a million points. [TimeSeries.AddBatchData] sorts once
// instead, and takes about a second. Use it whenever the readings are
// already in hand.
//
// [notavalue]: https://pkg.go.dev/github.com/fflamingodev/notavalue
package timeseries
