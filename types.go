package timeseries

import (
	"math"
	"sort"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// -----------------------------------------------------------------------
// Missing durations
// -----------------------------------------------------------------------

// NaDuration is the "not a duration" sentinel, the counterpart of
// notavalue.NaV for a time.Duration. It marks an interval that does not
// exist: the one before the first point of a series, which has no
// predecessor.
//
// Its value is math.MinInt64 nanoseconds, roughly -292 years, which no
// honest measurement will ever produce.
const NaDuration = time.Duration(math.MinInt64)

// IsNaDuration reports whether d is the NaDuration sentinel.
func IsNaDuration(d time.Duration) bool {
	return d == NaDuration
}

// -----------------------------------------------------------------------
// A point
// -----------------------------------------------------------------------

// Datum is the smallest unit of data in this package: an instant and
// what was measured then. Nothing else — a Datum knows nothing of its
// neighbors.
//
// It is the type to use when ingesting raw samples: a row from a sensor,
// a line of CSV, a column from a database. A measurement that was not
// made is recorded as notavalue.NaV, never as 0 and never as a missing
// row, so that the gap stays visible downstream.
//
// The timestamp is a time.Time and not a count of nanoseconds. It costs
// 24 bytes instead of 8, and that is a deliberate price: a measurement
// separated from its instant is no longer a measurement, and time.Time
// is the type every Go developer already knows how to handle — time
// zones, formatting, comparison.
type Datum struct {
	Chron time.Time
	Meas  float64
}

// NewDatum returns the Datum measured at chron.
func NewDatum(chron time.Time, meas float64) Datum {
	return Datum{Chron: chron, Meas: meas}
}

// IsMissing reports whether the measurement is absent, that is, whether
// Meas is NaV. A Datum whose Meas is a plain NaN is not missing: it
// carries the result of a broken computation, which is a different
// statement.
func (d Datum) IsMissing() bool {
	return nav.IsNaV(d.Meas)
}

// DataUnit is a Datum placed in a series, enriched with its two deltas
// against the point that precedes it:
//
//   - Dchron, the elapsed time since the previous point;
//   - Dmeas, the variation of the measurement.
//
// The embedded Datum is promoted, so du.Chron and du.Meas read as if
// they were fields of DataUnit itself.
//
// The deltas are always exact. A TimeSeries keeps its points in
// chronological order and updates the deltas on every insertion, so
// there is no such thing as a series whose deltas are stale — and thus
// no flag to consult before trusting them.
//
// Two sentinels appear in the deltas:
//
//   - the first point of a series has no predecessor: its Dchron is
//     NaDuration and its Dmeas is notavalue.NaV;
//   - a variation computed from a missing measurement is itself missing:
//     Dmeas is NaV, because notavalue.Sub refuses to invent a variation
//     out of a value that was never observed.
type DataUnit struct {
	Datum
	Dchron time.Duration
	Dmeas  float64
}

// NewDataUnit returns a DataUnit with no predecessor: its deltas are the
// "I don't know" sentinels. Inserting it into a TimeSeries is what gives
// it neighbors, and therefore real deltas.
func NewDataUnit(chron time.Time, meas float64) DataUnit {
	return DataUnit{
		Datum:  NewDatum(chron, meas),
		Dchron: NaDuration,
		Dmeas:  nav.NaV,
	}
}

// IsFirst reports whether the unit has no predecessor, that is, whether
// its Dchron is NaDuration.
func (du DataUnit) IsFirst() bool {
	return IsNaDuration(du.Dchron)
}

// -----------------------------------------------------------------------
// A series
// -----------------------------------------------------------------------

// TimeSeries is a chronologically ordered sequence of DataUnit.
//
// # The invariant
//
// At any moment, the points are sorted by Chron and every delta agrees
// with the current order. The type has no "recompute the deltas" method
// because there is never anything to recompute: Add maintains the
// invariant as it inserts.
//
// This is why the points are not an exported field. Handing out the
// slice would let a caller append out of order or sort by measurement,
// leaving the deltas describing an order that no longer exists — a
// silent falsehood that an earlier version of this library tracked with
// a boolean flag. Making the state impossible is better than reporting
// it. Read access goes through Len, At, Range, Meas and MeasTo.
type TimeSeries struct {
	// ID identifies this series among others; it must be unique within
	// whatever collection holds it. Any scheme works — a database
	// primary key, a device identifier, a path. NewID returns a fresh
	// UUID for callers who have nothing to use.
	ID string

	// Name is meant for humans: a legend on a chart, a column header.
	Name string

	// Comment records how the series came to be — the recipe applied,
	// the source it was read from, a warning about its quality.
	Comment string

	points []DataUnit
}

// NewTimeSeries returns an empty series with the given name.
func NewTimeSeries(name string) *TimeSeries {
	return &TimeSeries{Name: name}
}

// Len returns the number of points.
func (ts *TimeSeries) Len() int {
	if ts == nil {
		return 0
	}
	return len(ts.points)
}

// At returns the i-th point in chronological order. It panics if i is
// out of range, like any slice index.
func (ts *TimeSeries) At(i int) DataUnit {
	return ts.points[i]
}

// First returns the earliest point, and false if the series is empty.
func (ts *TimeSeries) First() (DataUnit, bool) {
	if ts.Len() == 0 {
		return DataUnit{}, false
	}
	return ts.points[0], true
}

// Last returns the latest point, and false if the series is empty.
func (ts *TimeSeries) Last() (DataUnit, bool) {
	n := ts.Len()
	if n == 0 {
		return DataUnit{}, false
	}
	return ts.points[n-1], true
}

// Range calls f for every point in chronological order, stopping early
// if f returns false. It is the way to walk a series without copying it.
func (ts *TimeSeries) Range(f func(i int, du DataUnit) bool) {
	if ts == nil {
		return
	}
	for i, du := range ts.points {
		if !f(i, du) {
			return
		}
	}
}

// Add inserts a measurement, keeping the series chronological and its
// deltas exact.
//
// Appending a point later than the last one — the ordinary case of a
// sensor feed — costs one insertion at the end. Inserting an older point
// shifts the tail and recomputes exactly two deltas: the one of the new
// point and the one of the point that now follows it.
//
// A point bearing the same instant as an existing one is inserted after
// it. Duplicate timestamps are allowed; deciding what they mean is the
// caller's business.
func (ts *TimeSeries) Add(d Datum) {
	du := DataUnit{Datum: d, Dchron: NaDuration, Dmeas: nav.NaV}

	n := len(ts.points)
	if n == 0 {
		ts.points = append(ts.points, du)
		return
	}

	// The common case: the new point closes the series.
	if !d.Chron.Before(ts.points[n-1].Chron) {
		fillDeltas(&du, ts.points[n-1])
		ts.points = append(ts.points, du)
		return
	}

	// Otherwise, find where it belongs: after any point sharing its
	// instant, before the first later one.
	i := sort.Search(n, func(k int) bool {
		return ts.points[k].Chron.After(d.Chron)
	})

	ts.points = append(ts.points, DataUnit{})
	copy(ts.points[i+1:], ts.points[i:])

	if i > 0 {
		fillDeltas(&du, ts.points[i-1])
	}
	ts.points[i] = du
	fillDeltas(&ts.points[i+1], du)
}

// fillDeltas fills the deltas of du against its predecessor prev.
//
// Dmeas goes through notavalue.Sub rather than a plain subtraction: when
// either measurement is missing, the variation is missing too, and when
// either is a plain NaN, the error propagates.
func fillDeltas(du *DataUnit, prev DataUnit) {
	du.Dchron = du.Chron.Sub(prev.Chron)
	du.Dmeas = nav.Sub(du.Meas, prev.Meas)
}

// Meas returns the measurements, in chronological order, as a freshly
// allocated slice ready for the notavalue aggregates:
//
//	nav.Mean(ts.Meas())
//
// The allocation is proportional to the series. In a loop over many
// series, or over one very long series, prefer MeasTo with a buffer you
// reuse.
func (ts *TimeSeries) Meas() []float64 {
	return ts.MeasTo(nil)
}

// MeasTo appends the measurements to dst and returns the result,
// reusing dst's capacity. Passing back the slice returned by the
// previous call makes a loop over many series allocate nothing:
//
//	var buf []float64
//	for _, ts := range all {
//	    buf = ts.MeasTo(buf[:0])
//	    fmt.Println(ts.Name, nav.Mean(buf))
//	}
func (ts *TimeSeries) MeasTo(dst []float64) []float64 {
	if ts == nil {
		return dst
	}
	if cap(dst)-len(dst) < len(ts.points) {
		grown := make([]float64, len(dst), len(dst)+len(ts.points))
		copy(grown, dst)
		dst = grown
	}
	for _, du := range ts.points {
		dst = append(dst, du.Meas)
	}
	return dst
}
