package timeseries

import (
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// BasicStats is a summary of a series: where it starts and ends, what
// its measurements look like, and how regularly they arrive.
//
// # Reading the field names
//
//   - Ch* concern the timestamps (Chron).
//   - Ms* concern the measurements (Meas).
//   - DCh* and DMs* concern the deltas between consecutive points.
//   - ChAt* give the instant at which another statistic occurs — the
//     time of the maximum, say.
//
// # What is counted
//
// Every aggregate rests on the rules of the notavalue package:
// measurements that were never made are skipped, and a computation
// error propagates. A month with one missing day still has a mean; a
// month with a broken value does not, and says so.
//
// NbreOfNaN counts every non-numeric measurement, missing ones
// included, and NbreOfNaV counts the missing ones alone. The difference
// between the two is the number of genuine computation errors, which is
// what deserves investigation.
//
// A statistic that cannot be computed — an empty series, a series of
// nothing but gaps — is notavalue.NaV, never zero. Durations that
// cannot be computed are NaDuration.
type BasicStats struct {
	// Len is the number of points, missing measurements included.
	Len int

	// The extent of the series, and the measurements at its two ends.
	Chmin      time.Time
	ValAtChmin float64
	Chmax      time.Time
	ValAtChmax float64

	// Where the timestamps sit: their median and their mean.
	Chmed  time.Time
	Chmean time.Time

	// The first point actually carrying a measurement, and that
	// measurement. On a feed that starts before the sensor warms up, or
	// after an outage, Chmin is the start of the window while
	// ChFirstUsable is the start of the data.
	//
	// Unlike the aggregates, these two are unaffected by a broken value
	// elsewhere in the series: they state a fact about one point, not a
	// computation over all of them. Both are the zero time and NaV when
	// no measurement is usable.
	ChFirstUsable    time.Time
	ValAtFirstUsable float64

	// The measurements: extremes with the instants they occur at,
	// then the usual three.
	Msmin     float64
	ChAtMsmin time.Time
	Msmax     float64
	ChAtMsmax time.Time
	Msmean    float64
	Msmed     float64
	Msstd     float64

	// The intervals between consecutive points: the shortest and the
	// longest, with the instants they end at, then the usual three in
	// nanoseconds.
	//
	// DChstd is the one to watch: on a regular feed it is near zero, and
	// it grows as soon as the sampling stutters.
	DChmin     time.Duration
	ChAtDChmin time.Time
	DChmax     time.Duration
	ChAtDchmax time.Time
	DChmean    float64
	DChmed     float64
	DChstd     float64

	// The variations of the measurement from one point to the next.
	// Every variation computed across a gap is missing, so these rest on
	// fewer values than Len suggests.
	DMsmin  float64
	DMsmax  float64
	DMsmed  float64
	DMsmean float64
	DMsstd  float64

	// NbreOfNaN counts the measurements that are not numbers, missing
	// ones included. NbreOfNaV counts the missing ones alone.
	NbreOfNaN int
	NbreOfNaV int
}

// Stats computes the summary of the series, on the spot.
//
// Nothing is cached. Every call walks the points and allocates the
// working slices it needs, so a caller that wants the summary twice
// should keep the returned value rather than ask again. The expensive
// part is the medians, which sort: reckon on the order of 60 ns per
// point in total, against 1 ns for a mean alone.
//
// On an empty series, Stats returns the zero BasicStats with its
// measurements set to NaV and its durations to NaDuration: nothing can
// be said, and the result says exactly that rather than zero.
func (ts *TimeSeries) Stats() BasicStats {
	var bs BasicStats
	bs.nothingToSay()

	n := ts.Len()
	if n == 0 {
		return bs
	}
	bs.Len = n

	first, _ := ts.First()
	last, _ := ts.Last()
	bs.Chmin, bs.ValAtChmin = first.Chron, first.Meas
	bs.Chmax, bs.ValAtChmax = last.Chron, last.Meas

	ts.statsOnChron(&bs)
	ts.statsOnMeas(&bs)
	ts.statsOnDeltas(&bs)
	return bs
}

// nothingToSay sets every statistic to its "I don't know" sentinel, so
// that a series too poor to support a computation never reports a zero
// that would pass for a measurement.
func (bs *BasicStats) nothingToSay() {
	bs.ValAtChmin, bs.ValAtChmax = nav.NaV, nav.NaV
	bs.ValAtFirstUsable = nav.NaV
	bs.Msmin, bs.Msmax = nav.NaV, nav.NaV
	bs.Msmean, bs.Msmed, bs.Msstd = nav.NaV, nav.NaV, nav.NaV
	bs.DChmin, bs.DChmax = NaDuration, NaDuration
	bs.DChmean, bs.DChmed, bs.DChstd = nav.NaV, nav.NaV, nav.NaV
	bs.DMsmin, bs.DMsmax = nav.NaV, nav.NaV
	bs.DMsmean, bs.DMsmed, bs.DMsstd = nav.NaV, nav.NaV, nav.NaV
}

// statsOnChron computes the central tendency of the timestamps.
//
// The arithmetic is done on nanoseconds counted from the first point,
// not on nanoseconds since 1970. A float64 carries 53 bits of mantissa,
// while a modern Unix timestamp in nanoseconds needs 61: computing on
// absolute values would round to the nearest few hundred nanoseconds.
// Counting from the start of the series keeps the numbers small and the
// result exact. The previous version of the library did not do this.
func (ts *TimeSeries) statsOnChron(bs *BasicStats) {
	base := bs.Chmin
	offsets := make([]float64, 0, ts.Len())
	ts.Range(func(_ int, du DataUnit) bool {
		offsets = append(offsets, float64(du.Chron.Sub(base)))
		return true
	})

	bs.Chmean = offsetToTime(base, nav.Mean(offsets))
	bs.Chmed = offsetToTime(base, nav.Median(offsets))
}

// offsetToTime turns a number of nanoseconds counted from base back
// into an instant, and returns the zero time when there is nothing to
// convert.
func offsetToTime(base time.Time, offset float64) time.Time {
	if nav.IsNaV(offset) || offset != offset {
		return time.Time{}
	}
	return base.Add(time.Duration(offset))
}

// statsOnMeas computes the statistics of the measurements. The extremes
// are found in the same traversal as their instants, so that the time
// of the maximum is the time of *that* maximum, and not of the first
// value that happens to compare equal.
func (ts *TimeSeries) statsOnMeas(bs *BasicStats) {
	meas := ts.Meas()

	bs.Msmean = nav.Mean(meas)
	bs.Msmed = nav.Median(meas)
	bs.Msstd = nav.StdDev(meas)
	bs.Msmin, bs.Msmax = nav.Bounds(meas)

	bs.NbreOfNaN = bs.Len - nav.CountUsable(meas)
	bs.NbreOfNaV = nav.CountNaV(meas)

	bs.ChAtMsmin = ts.firstInstantOf(bs.Msmin)
	bs.ChAtMsmax = ts.firstInstantOf(bs.Msmax)

	// The first point that actually measured something. Looked up
	// directly rather than derived from the aggregates, so that a broken
	// value further along the series does not erase the fact.
	ts.Range(func(_ int, du DataUnit) bool {
		if du.Meas != du.Meas { // NaN-class: missing or broken
			return true
		}
		bs.ChFirstUsable, bs.ValAtFirstUsable = du.Chron, du.Meas
		return false
	})
}

// firstInstantOf returns the instant of the earliest point measuring
// exactly value, or the zero time when there is none — which is the
// case whenever value is a sentinel rather than a measurement, as it is
// on a series with nothing usable in it.
//
// Comparing floats for equality is sound here: value comes from the
// series itself, so the bit patterns match exactly. When several points
// share it, the earliest wins, since the series is chronological.
func (ts *TimeSeries) firstInstantOf(value float64) time.Time {
	if value != value { // NaN-class: no instant to point at
		return time.Time{}
	}
	var found time.Time
	ts.Range(func(_ int, du DataUnit) bool {
		if du.Meas == value {
			found = du.Chron
			return false
		}
		return true
	})
	return found
}

// statsOnDeltas computes the statistics of the intervals and of the
// variations. The first point is left out of both: it has no
// predecessor, so its deltas are sentinels rather than measurements.
func (ts *TimeSeries) statsOnDeltas(bs *BasicStats) {
	n := ts.Len()
	if n < 2 {
		return
	}

	intervals := make([]float64, 0, n-1)
	variations := make([]float64, 0, n-1)

	for i := 1; i < n; i++ {
		du := ts.At(i)

		intervals = append(intervals, float64(du.Dchron))
		variations = append(variations, du.Dmeas)

		if bs.DChmin == NaDuration || du.Dchron < bs.DChmin {
			bs.DChmin = du.Dchron
			bs.ChAtDChmin = du.Chron
		}
		if bs.DChmax == NaDuration || du.Dchron > bs.DChmax {
			bs.DChmax = du.Dchron
			bs.ChAtDchmax = du.Chron
		}
	}

	bs.DChmean = nav.Mean(intervals)
	bs.DChmed = nav.Median(intervals)
	bs.DChstd = nav.StdDev(intervals)

	bs.DMsmean = nav.Mean(variations)
	bs.DMsmed = nav.Median(variations)
	bs.DMsstd = nav.StdDev(variations)
	bs.DMsmin, bs.DMsmax = nav.Bounds(variations)
}
