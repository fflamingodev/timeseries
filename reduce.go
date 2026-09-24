package timeseries

import (
	"fmt"
	"math"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// Reduce and Expand are the two directions of the same idea: a signal
// that holds its value between changes can be stored as its changes
// alone, and rebuilt on demand.
//
// A door that opens twice a day, a setpoint an operator moves now and
// then, a state machine: recording such a signal every minute stores
// the same number over and over. Reduce keeps only the points where it
// moved; Expand puts the minutes back.
//
// # What a reduced series says
//
// A reduced series keeps its first and its last reading, whether they
// moved or not, and every change in between. The first reading says
// where the knowledge starts, the last where it stops: between them,
// each point holds until the next; before the first and after the last,
// nothing is known.
//
// # Gaps
//
// Reduce sees a gap only where the series says NaV. It cannot tell a
// sensor that fell silent for three days from a value that held for
// three days: both leave no reading behind, and the held value would be
// expanded over the silence.
//
// A regularized series already says it — a window that caught nothing
// is NaV, so a silence longer than the step reaches Reduce as a gap. A
// raw series must have its silences marked as NaV before it is reduced.
//
// # Lossless or not
//
// Reduce is lossless: the reduced series holds every change, at its
// instant. Expanding it on the grid of a regular source gives the
// source back, reading for reading. For an irregular source, the
// readings that held are gone, and only their instants are lost.
//
// ReduceWithDeadband trades exactness for size: it keeps a reading only
// when it strays from the value held by more than a band. Every dropped
// reading was within the band of the value that replaces it, so the
// expanded series never errs by more than the band.

// Reduce returns the series stripped of its repetitions: the first and
// the last reading, and the ones where the measurement changed.
//
// What counts as a change follows the rules of the library rather than
// a plain comparison, since no NaN is equal to itself:
//
//   - two consecutive gaps are not a change — the signal is still
//     unknown, and nothing happened to say otherwise;
//   - going into a gap or coming out of one is a change: it is the most
//     interesting thing a series can record;
//   - two broken values in a row are not a change either, and turning
//     broken is one.
func (ts *TimeSeries) Reduce() *TimeSeries {
	return ts.reduce(0, "transitions only")
}

// ReduceWithDeadband returns the series stripped of the readings that
// stay within band of the value held: a reading is kept only when it
// differs from the last kept one by more than band. The first and the
// last reading are always kept, and going into or out of a gap or a
// broken value is always a change, whatever the band.
//
// The comparison is with the last kept value, not with the previous
// reading: a slow drift, a little at a time, is caught as soon as it
// has moved by more than band in total.
//
// A band of zero is Reduce. An infinite band keeps only the changes
// between measured, gap and broken.
func (ts *TimeSeries) ReduceWithDeadband(band float64) (*TimeSeries, error) {
	if math.IsNaN(band) || band < 0 {
		return nil, fmt.Errorf("%w: the band must be zero or above, got %v", ErrRegularizeArg, band)
	}
	return ts.reduce(band, fmt.Sprintf("transitions beyond ±%g", band)), nil
}

// reduce keeps the readings that moved by more than band from the value
// held, plus the first and the last.
func (ts *TimeSeries) reduce(band float64, comment string) *TimeSeries {
	out := NewTimeSeries(ts.Name + " reduced")
	out.ID = ts.ID
	out.Comment = comment

	n := ts.Len()
	if n == 0 {
		return out
	}

	kept := make([]Datum, 0, 16)
	kept = append(kept, ts.At(0).Datum)
	held := ts.At(0).Meas
	lastKept := 0

	for i := 1; i < n; i++ {
		current := ts.At(i).Meas
		if moved(held, current, band) {
			kept = append(kept, ts.At(i).Datum)
			held = current
			lastKept = i
		}
	}

	// The last reading closes the observation, even when it only
	// repeats the value held: without it, the reduced series would not
	// say how long that value was known to last.
	if lastKept != n-1 {
		kept = append(kept, ts.At(n-1).Datum)
	}

	out.AddBatchData(kept)
	return out
}

// moved reports whether current strays from the value held by more than
// band. Equality is not enough: a NaN — gap or error — is never equal to
// itself, so a plain comparison would call every gap a change and
// Reduce would keep everything.
func moved(held, current, band float64) bool {
	switch {
	case nav.IsNaV(held) && nav.IsNaV(current):
		return false
	case nav.IsStdNaN(held) && nav.IsStdNaN(current):
		return false
	case math.IsNaN(held) || math.IsNaN(current):
		// One of the two is a non-number and the other is not, or they
		// are a gap and an error: in every case, something happened.
		return true
	}
	// Two equal infinities differ by NaN, which is above no band.
	return math.Abs(current-held) > band
}

// Expand rebuilds a regular series from start to end, one point every
// step, by holding each value until the next change — the inverse of
// Reduce.
//
// A point of the reduced series states the value from its own instant
// onwards, inclusive, up to the next point. Outside the span of the
// reduced series, from its first point to its last, the grid is NaV:
// the series says nothing about what the signal was doing there, and a
// made-up value would be worse than an admitted gap.
//
// The grid runs from start, and includes end when the span is a whole
// number of steps.
func (ts *TimeSeries) Expand(start, end time.Time, step time.Duration) (*TimeSeries, error) {
	if step <= 0 {
		return nil, fmt.Errorf("%w: the step must be above zero, got %v", ErrRegularizeArg, step)
	}
	if end.Before(start) {
		return nil, fmt.Errorf("%w: the end %v comes before the start %v",
			ErrRegularizeArg, end.Format(time.RFC3339), start.Format(time.RFC3339))
	}

	out := NewTimeSeries(ts.Name + " expanded")
	out.ID = ts.ID
	out.Comment = fmt.Sprintf("held every %v", step)

	n := ts.Len()
	grid := make([]Datum, 0, int(end.Sub(start)/step)+1)

	// The instant after which nothing is known any more.
	var known time.Time
	if n > 0 {
		last, _ := ts.Last()
		known = last.Chron
	}

	// The value in force: unknown until the first change is reached.
	held := nav.NaV
	i := 0

	for t := start; !t.After(end); t = t.Add(step) {
		for i < n && !ts.At(i).Chron.After(t) {
			held = ts.At(i).Meas
			i++
		}
		value := held
		if n == 0 || t.After(known) {
			value = nav.NaV
		}
		grid = append(grid, NewDatum(t, value))
	}

	out.AddBatchData(grid)
	return out, nil
}
