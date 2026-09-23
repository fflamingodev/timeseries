package timeseries

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

var origin = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// at returns the instant h hours after the origin of the tests.
func at(h int) time.Time {
	return origin.Add(time.Duration(h) * time.Hour)
}

// show renders a float64 so that a failing test says "NaV" or "NaN"
// instead of printing both as NaN.
func show(x float64) string {
	switch {
	case nav.IsNaV(x):
		return "NaV"
	case math.IsNaN(x):
		return "NaN"
	}
	return strconv.FormatFloat(x, 'g', -1, 64)
}

// sameFloat compares two measurements by category, since no NaN — NaV
// included — is ever equal to itself.
func sameFloat(a, b float64) bool {
	switch {
	case nav.IsNaV(a) || nav.IsNaV(b):
		return nav.IsNaV(a) && nav.IsNaV(b)
	case math.IsNaN(a) || math.IsNaN(b):
		return math.IsNaN(a) && math.IsNaN(b)
	}
	return a == b
}

// seriesOf builds a series by adding the given measurements at
// successive hours, in the order supplied.
func seriesOf(meas ...float64) *TimeSeries {
	ts := NewTimeSeries("test")
	for i, m := range meas {
		ts.Add(NewDatum(at(i), m))
	}
	return ts
}

// checkInvariant asserts the contract of TimeSeries: the points are in
// chronological order, the first one carries the "no predecessor"
// sentinels, and every other delta agrees with the neighbor it is
// computed from.
//
// Every test that mutates a series ends by calling this. The invariant
// is the whole reason the points are private, so it is what must be
// verified hardest.
func checkInvariant(t *testing.T, ts *TimeSeries) {
	t.Helper()
	n := ts.Len()
	if n == 0 {
		return
	}

	first := ts.At(0)
	if !IsNaDuration(first.Dchron) {
		t.Errorf("the first point has Dchron = %v, want NaDuration: it has no predecessor", first.Dchron)
	}
	if !nav.IsNaV(first.Dmeas) {
		t.Errorf("the first point has Dmeas = %s, want NaV", show(first.Dmeas))
	}
	if !first.IsFirst() {
		t.Error("IsFirst() is false on the first point")
	}

	for i := 1; i < n; i++ {
		prev, cur := ts.At(i-1), ts.At(i)

		if cur.Chron.Before(prev.Chron) {
			t.Fatalf("points %d and %d are out of order: %s then %s",
				i-1, i, prev.Chron.Format(time.RFC3339), cur.Chron.Format(time.RFC3339))
		}
		if want := cur.Chron.Sub(prev.Chron); cur.Dchron != want {
			t.Errorf("point %d: Dchron = %v, want %v", i, cur.Dchron, want)
		}
		if want := nav.Sub(cur.Meas, prev.Meas); !sameFloat(cur.Dmeas, want) {
			t.Errorf("point %d: Dmeas = %s, want %s", i, show(cur.Dmeas), show(want))
		}
		if cur.IsFirst() {
			t.Errorf("point %d reports IsFirst()", i)
		}
	}
}

// -----------------------------------------------------------------------
// Sentinels and points
// -----------------------------------------------------------------------

func TestNaDuration(t *testing.T) {
	if !IsNaDuration(NaDuration) {
		t.Error("IsNaDuration(NaDuration) is false")
	}
	for _, d := range []time.Duration{0, time.Second, -time.Hour, math.MaxInt64} {
		if IsNaDuration(d) {
			t.Errorf("IsNaDuration(%v) is true, want false", d)
		}
	}
}

func TestDatumIsMissing(t *testing.T) {
	cases := []struct {
		name string
		meas float64
		want bool
	}{
		{"a measurement", 21.5, false},
		{"zero is a measurement", 0, false},
		// The distinction the whole library rests on.
		{"NaV means never measured", nav.NaV, true},
		{"a plain NaN is an error, not a gap", math.NaN(), false},
	}
	for _, c := range cases {
		d := NewDatum(at(0), c.meas)
		if got := d.IsMissing(); got != c.want {
			t.Errorf("%s: IsMissing() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNewDataUnitHasNoPredecessor(t *testing.T) {
	du := NewDataUnit(at(0), 12)
	if !IsNaDuration(du.Dchron) {
		t.Errorf("Dchron = %v, want NaDuration", du.Dchron)
	}
	if !nav.IsNaV(du.Dmeas) {
		t.Errorf("Dmeas = %s, want NaV", show(du.Dmeas))
	}
	if !du.IsFirst() {
		t.Error("IsFirst() is false")
	}
	// The embedded Datum is promoted.
	if du.Meas != 12 || !du.Chron.Equal(at(0)) {
		t.Errorf("the embedded Datum did not survive: %v", du)
	}
}

// -----------------------------------------------------------------------
// An empty series
// -----------------------------------------------------------------------

func TestEmptySeries(t *testing.T) {
	ts := NewTimeSeries("empty")

	if ts.Len() != 0 {
		t.Errorf("Len = %d, want 0", ts.Len())
	}
	if _, ok := ts.First(); ok {
		t.Error("First() reports a point on an empty series")
	}
	if _, ok := ts.Last(); ok {
		t.Error("Last() reports a point on an empty series")
	}
	if got := ts.Meas(); len(got) != 0 {
		t.Errorf("Meas() = %v, want an empty slice", got)
	}
	ts.Range(func(int, DataUnit) bool {
		t.Error("Range called f on an empty series")
		return true
	})
	checkInvariant(t, ts)
}

// A nil *TimeSeries must not panic on the read-only operations: a map
// lookup that finds nothing is a common way to get one.
func TestNilSeriesIsSafeToRead(t *testing.T) {
	var ts *TimeSeries

	if ts.Len() != 0 {
		t.Error("Len on a nil series is not 0")
	}
	if got := ts.MeasTo(nil); got != nil {
		t.Errorf("MeasTo on a nil series returned %v", got)
	}
	ts.Range(func(int, DataUnit) bool {
		t.Error("Range called f on a nil series")
		return true
	})
}

// -----------------------------------------------------------------------
// Insertion
// -----------------------------------------------------------------------

// The ordinary case: a sensor feed, each point later than the last.
func TestAddInOrder(t *testing.T) {
	ts := seriesOf(10, 12, 15)

	if ts.Len() != 3 {
		t.Fatalf("Len = %d, want 3", ts.Len())
	}
	checkInvariant(t, ts)

	if got := ts.At(1).Dmeas; got != 2 {
		t.Errorf("point 1: Dmeas = %s, want 2", show(got))
	}
	if got := ts.At(1).Dchron; got != time.Hour {
		t.Errorf("point 1: Dchron = %v, want 1h", got)
	}

	first, _ := ts.First()
	last, _ := ts.Last()
	if first.Meas != 10 || last.Meas != 15 {
		t.Errorf("First = %v, Last = %v", first.Meas, last.Meas)
	}
}

// A point that belongs in the middle must be placed there, and must
// leave both neighbors describing the truth.
func TestAddInTheMiddle(t *testing.T) {
	ts := NewTimeSeries("middle")
	ts.Add(NewDatum(at(0), 10))
	ts.Add(NewDatum(at(2), 14))
	ts.Add(NewDatum(at(1), 12)) // late arrival

	checkInvariant(t, ts)

	if ts.Len() != 3 {
		t.Fatalf("Len = %d, want 3", ts.Len())
	}
	for i, wantMeas := range []float64{10, 12, 14} {
		if got := ts.At(i).Meas; got != wantMeas {
			t.Errorf("point %d: Meas = %s, want %v", i, show(got), wantMeas)
		}
	}
	// The point that now follows the newcomer must have been recomputed:
	// its Dmeas was 4 against the first point, it is 2 against the new one.
	if got := ts.At(2).Dmeas; got != 2 {
		t.Errorf("the successor was not recomputed: Dmeas = %s, want 2", show(got))
	}
}

// A point older than every other becomes the first, and the former first
// point loses its sentinels: it now has a predecessor.
func TestAddBeforeTheFirst(t *testing.T) {
	ts := NewTimeSeries("front")
	ts.Add(NewDatum(at(1), 12))
	ts.Add(NewDatum(at(0), 10))

	checkInvariant(t, ts)

	if got := ts.At(0).Meas; got != 10 {
		t.Errorf("the new point is not first: Meas = %s", show(got))
	}
	if ts.At(1).IsFirst() {
		t.Error("the former first point still reports IsFirst()")
	}
	if got := ts.At(1).Dmeas; got != 2 {
		t.Errorf("the former first point kept its sentinel: Dmeas = %s, want 2", show(got))
	}
}

func TestAddDuplicateTimestamps(t *testing.T) {
	ts := NewTimeSeries("duplicates")
	ts.Add(NewDatum(at(0), 10))
	ts.Add(NewDatum(at(1), 30))
	ts.Add(NewDatum(at(0), 20)) // same instant as the first

	checkInvariant(t, ts)

	// Inserted after the point it shares its instant with.
	for i, wantMeas := range []float64{10, 20, 30} {
		if got := ts.At(i).Meas; got != wantMeas {
			t.Errorf("point %d: Meas = %s, want %v", i, show(got), wantMeas)
		}
	}
	// Two points at the same instant: no time elapsed between them.
	if got := ts.At(1).Dchron; got != 0 {
		t.Errorf("Dchron between duplicates = %v, want 0", got)
	}
}

// Inserting in a random order must yield the same series as inserting in
// chronological order. This is the test that hunts for an index slip in
// Add: it tries a thousand shuffles.
func TestAddIsOrderIndependent(t *testing.T) {
	const n = 40
	r := rand.New(rand.NewSource(7))

	for round := 0; round < 1000; round++ {
		order := r.Perm(n)
		ts := NewTimeSeries("shuffled")
		for _, i := range order {
			// One point in five was never measured.
			m := float64(i * 10)
			if i%5 == 0 {
				m = nav.NaV
			}
			ts.Add(NewDatum(at(i), m))
		}

		if ts.Len() != n {
			t.Fatalf("round %d: Len = %d, want %d", round, ts.Len(), n)
		}
		for i := 0; i < n; i++ {
			if !ts.At(i).Chron.Equal(at(i)) {
				t.Fatalf("round %d: point %d is at %s, want %s",
					round, i, ts.At(i).Chron, at(i))
			}
		}
		checkInvariant(t, ts)
		if t.Failed() {
			t.Fatalf("round %d broke the invariant, insertion order: %v", round, order)
		}
	}
}

// -----------------------------------------------------------------------
// Deltas and missing values — the point of the library
// -----------------------------------------------------------------------

// A variation computed from a measurement that was never made is
// missing, not zero and not a number made up from the other side.
func TestDeltaAroundAMissingMeasurement(t *testing.T) {
	ts := seriesOf(10, nav.NaV, 14)
	checkInvariant(t, ts)

	if got := ts.At(1).Dmeas; !nav.IsNaV(got) {
		t.Errorf("the variation into a gap = %s, want NaV", show(got))
	}
	if got := ts.At(2).Dmeas; !nav.IsNaV(got) {
		t.Errorf("the variation out of a gap = %s, want NaV: 14 is not a rise of 14", show(got))
	}
	// Time, however, is never missing: the clock kept running.
	if got := ts.At(2).Dchron; got != time.Hour {
		t.Errorf("Dchron across a gap = %v, want 1h", got)
	}
}

// An error in a measurement propagates into the variation, and stays
// distinguishable from a gap.
func TestDeltaAroundAnError(t *testing.T) {
	ts := seriesOf(10, math.NaN(), 14)
	checkInvariant(t, ts)

	for _, i := range []int{1, 2} {
		got := ts.At(i).Dmeas
		if nav.IsNaV(got) {
			t.Errorf("point %d: Dmeas = NaV, want a plain NaN: an error is not a gap", i)
		}
		if !math.IsNaN(got) {
			t.Errorf("point %d: Dmeas = %s, want a plain NaN", i, show(got))
		}
	}
}

// When an error precedes a gap, the variation must be the error, since
// a NaN wins over a NaV.
//
// This case is what proves the deltas go through notavalue.Sub and not
// through a plain subtraction. The processor propagates the payload of
// the first operand, so NaV - NaN would come out tagged as a NaV on this
// machine — the opposite of the rule, and a behavior neither Go nor
// IEEE-754 guarantees across architectures. Only this ordering tells the
// two implementations apart.
func TestDeltaWhenAnErrorPrecedesAGap(t *testing.T) {
	ts := seriesOf(math.NaN(), nav.NaV)
	checkInvariant(t, ts)

	got := ts.At(1).Dmeas
	if nav.IsNaV(got) {
		t.Error("Dmeas = NaV, want a plain NaN: an error wins over a gap, " +
			"and a raw subtraction would have kept the NaV tag here")
	}
	if !nav.IsStdNaN(got) {
		t.Errorf("Dmeas = %s, want a plain NaN", show(got))
	}
}

// -----------------------------------------------------------------------
// Reading the measurements out
// -----------------------------------------------------------------------

func TestMeas(t *testing.T) {
	ts := seriesOf(10, nav.NaV, 14)

	got := ts.Meas()
	if len(got) != 3 {
		t.Fatalf("Meas() returned %d values, want 3", len(got))
	}
	if got[0] != 10 || got[2] != 14 {
		t.Errorf("Meas() = %v", got)
	}
	if !nav.IsNaV(got[1]) {
		t.Errorf("the gap did not survive the extraction: %s", show(got[1]))
	}

	// The aggregates of notavalue apply directly to it.
	if mean := nav.Mean(got); mean != 12 {
		t.Errorf("Mean over the extracted values = %s, want 12", show(mean))
	}
	if n := nav.CountNaV(got); n != 1 {
		t.Errorf("CountNaV = %d, want 1", n)
	}
}

func TestMeasToAppendsAndPreservesTheBuffer(t *testing.T) {
	ts := seriesOf(1, 2, 3)

	got := ts.MeasTo([]float64{99})
	want := []float64{99, 1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("MeasTo = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MeasTo = %v, want %v", got, want)
		}
	}
}

// The reason MeasTo exists: a loop over many series must not allocate.
func TestMeasToDoesNotAllocateWhenReused(t *testing.T) {
	ts := seriesOf(1, 2, 3, 4, 5, 6, 7, 8)
	buf := make([]float64, 0, ts.Len())

	allocs := testing.AllocsPerRun(100, func() {
		buf = ts.MeasTo(buf[:0])
	})
	if allocs != 0 {
		t.Errorf("MeasTo allocated %.0f time(s) per call on a reused buffer, want 0", allocs)
	}
	if len(buf) != ts.Len() {
		t.Errorf("the buffer holds %d values, want %d", len(buf), ts.Len())
	}
}

func TestRangeStopsEarly(t *testing.T) {
	ts := seriesOf(1, 2, 3, 4)

	seen := 0
	ts.Range(func(i int, du DataUnit) bool {
		seen++
		return i < 1 // stop after the second point
	})
	if seen != 2 {
		t.Errorf("Range visited %d points, want 2", seen)
	}
}

func TestRangeWalksInOrder(t *testing.T) {
	ts := NewTimeSeries("order")
	ts.Add(NewDatum(at(2), 30))
	ts.Add(NewDatum(at(0), 10))
	ts.Add(NewDatum(at(1), 20))

	var got []float64
	ts.Range(func(i int, du DataUnit) bool {
		if i != len(got) {
			t.Errorf("Range passed index %d at position %d", i, len(got))
		}
		got = append(got, du.Meas)
		return true
	})
	for i, want := range []float64{10, 20, 30} {
		if got[i] != want {
			t.Fatalf("Range order = %v", got)
		}
	}
}

// -----------------------------------------------------------------------
// Identifiers
// -----------------------------------------------------------------------

func TestNewID(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := NewID()

		if len(id) != 36 {
			t.Fatalf("NewID = %q, want 36 characters", id)
		}
		for _, pos := range []int{8, 13, 18, 23} {
			if id[pos] != '-' {
				t.Fatalf("NewID = %q, want a dash at position %d", id, pos)
			}
		}
		// Version 4 and RFC 4122 variant.
		if id[14] != '4' {
			t.Fatalf("NewID = %q, want version 4", id)
		}
		if c := id[19]; c != '8' && c != '9' && c != 'a' && c != 'b' {
			t.Fatalf("NewID = %q, wrong variant marker %q", id, c)
		}
		if seen[id] {
			t.Fatalf("NewID returned %q twice", id)
		}
		seen[id] = true
	}
}
