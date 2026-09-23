package timeseries

import (
	"math/rand"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// batch builds the Datum slice for the given hours, with the
// measurement equal to ten times the hour. A hour divisible by five is
// recorded as missing.
func batch(hours ...int) []Datum {
	out := make([]Datum, 0, len(hours))
	for _, h := range hours {
		m := float64(h * 10)
		if h%5 == 0 {
			m = nav.NaV
		}
		out = append(out, NewDatum(at(h), m))
	}
	return out
}

func TestAddAllInOrder(t *testing.T) {
	ts := NewTimeSeries("batch")
	ts.AddAll(batch(0, 1, 2, 3))

	if ts.Len() != 4 {
		t.Fatalf("Len = %d, want 4", ts.Len())
	}
	checkInvariant(t, ts)
	if got := ts.At(2).Dmeas; got != 10 {
		t.Errorf("point 2: Dmeas = %s, want 10", show(got))
	}
}

func TestAddAllOutOfOrder(t *testing.T) {
	ts := NewTimeSeries("shuffled batch")
	ts.AddAll(batch(3, 0, 2, 1))

	checkInvariant(t, ts)
	for i := 0; i < ts.Len(); i++ {
		if !ts.At(i).Chron.Equal(at(i)) {
			t.Fatalf("point %d is at %s, want %s", i, ts.At(i).Chron, at(i))
		}
	}
}

func TestAddAllOnEmptyInput(t *testing.T) {
	ts := seriesOf(1, 2)
	before := ts.Len()

	ts.AddAll(nil)
	ts.AddAll([]Datum{})

	if ts.Len() != before {
		t.Errorf("Len = %d, want %d", ts.Len(), before)
	}
	checkInvariant(t, ts)
}

// A batch that extends the series, and one that reaches back before its
// first point: both must leave the series correct.
func TestAddAllOnAPopulatedSeries(t *testing.T) {
	ts := NewTimeSeries("mixed")
	ts.AddAll(batch(4, 6))

	ts.AddAll(batch(8, 7)) // after the end, in the wrong order
	checkInvariant(t, ts)

	ts.AddAll(batch(1, 3)) // before the beginning
	checkInvariant(t, ts)

	wantHours := []int{1, 3, 4, 6, 7, 8}
	if ts.Len() != len(wantHours) {
		t.Fatalf("Len = %d, want %d", ts.Len(), len(wantHours))
	}
	for i, h := range wantHours {
		if !ts.At(i).Chron.Equal(at(h)) {
			t.Errorf("point %d is at %s, want hour %d", i, ts.At(i).Chron.Format("15:04"), h)
		}
	}
	// The former first point has a predecessor now.
	if ts.At(2).IsFirst() {
		t.Error("the point at hour 4 still reports IsFirst()")
	}
}

// AddAll and Add must produce the very same series, points and deltas
// alike. Anything else would make the two methods two different
// semantics wearing similar names.
func TestAddAllAgreesWithAdd(t *testing.T) {
	const n = 60
	r := rand.New(rand.NewSource(11))

	for round := 0; round < 200; round++ {
		hours := r.Perm(n)
		data := batch(hours...)

		oneByOne := NewTimeSeries("one by one")
		for _, d := range data {
			oneByOne.Add(d)
		}

		batched := NewTimeSeries("batched")
		batched.AddAll(data)

		if oneByOne.Len() != batched.Len() {
			t.Fatalf("round %d: Len %d vs %d", round, oneByOne.Len(), batched.Len())
		}
		for i := 0; i < oneByOne.Len(); i++ {
			a, b := oneByOne.At(i), batched.At(i)
			if !a.Chron.Equal(b.Chron) || !sameFloat(a.Meas, b.Meas) ||
				a.Dchron != b.Dchron || !sameFloat(a.Dmeas, b.Dmeas) {
				t.Fatalf("round %d, point %d: Add gives {%s %s %v %s}, AddAll gives {%s %s %v %s}",
					round, i,
					a.Chron.Format("15:04"), show(a.Meas), a.Dchron, show(a.Dmeas),
					b.Chron.Format("15:04"), show(b.Meas), b.Dchron, show(b.Dmeas))
			}
		}
		checkInvariant(t, batched)
	}
}

// Points sharing an instant keep the order they were given, and land
// after those already in the series.
func TestAddAllKeepsTheOrderOfDuplicates(t *testing.T) {
	ts := NewTimeSeries("duplicates")
	ts.Add(NewDatum(at(0), 1))
	ts.AddAll([]Datum{
		NewDatum(at(0), 2),
		NewDatum(at(0), 3),
	})

	checkInvariant(t, ts)
	for i, want := range []float64{1, 2, 3} {
		if got := ts.At(i).Meas; got != want {
			t.Errorf("point %d: Meas = %s, want %v", i, show(got), want)
		}
	}
}

func TestAddAllMissingValues(t *testing.T) {
	ts := NewTimeSeries("gaps")
	ts.AddAll([]Datum{
		NewDatum(at(0), 10),
		NewDatum(at(1), nav.NaV),
		NewDatum(at(2), 14),
	})
	checkInvariant(t, ts)

	if got := ts.At(2).Dmeas; !nav.IsNaV(got) {
		t.Errorf("the variation out of a gap = %s, want NaV", show(got))
	}
	if got := nav.Mean(ts.Meas()); got != 12 {
		t.Errorf("Mean = %s, want 12", show(got))
	}
}

// -----------------------------------------------------------------------
// The cost, measured
// -----------------------------------------------------------------------
//
// Loading the same shuffled points, on an Apple M-series laptop:
//
//	points   Add one by one   AddAll
//	20 000        61 ms        9.1 ms
//	40 000       255 ms       20.6 ms
//	80 000     1 014 ms       45.2 ms
//
// Doubling the input quadruples the first and merely doubles the second:
// shifting the tail on every insertion is quadratic, sorting once is
// n·log n. Extrapolated to a million points, that is about two and a
// half minutes against roughly one second — the whole reason AddAll
// exists.
//
// On an input already in order, both take the fast path and cost the
// same, around 0.6 ms for 20 000 points: no sort, no shifting.

const benchPoints = 20000

func shuffledBatch(n int) []Datum {
	r := rand.New(rand.NewSource(3))
	data := make([]Datum, 0, n)
	for _, h := range r.Perm(n) {
		data = append(data, NewDatum(origin.Add(time.Duration(h)*time.Minute), float64(h)))
	}
	return data
}

func orderedBatch(n int) []Datum {
	data := make([]Datum, 0, n)
	for h := 0; h < n; h++ {
		data = append(data, NewDatum(origin.Add(time.Duration(h)*time.Minute), float64(h)))
	}
	return data
}

func BenchmarkAddAllShuffled(b *testing.B) {
	data := shuffledBatch(benchPoints)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := NewTimeSeries("bench")
		ts.AddAll(data)
	}
}

// The ordered case takes the fast path: no sort at all.
func BenchmarkAddAllOrdered(b *testing.B) {
	data := orderedBatch(benchPoints)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := NewTimeSeries("bench")
		ts.AddAll(data)
	}
}

func BenchmarkAddOneByOneShuffled(b *testing.B) {
	data := shuffledBatch(benchPoints)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := NewTimeSeries("bench")
		for _, d := range data {
			ts.Add(d)
		}
	}
}

func BenchmarkAddOneByOneOrdered(b *testing.B) {
	data := orderedBatch(benchPoints)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := NewTimeSeries("bench")
		for _, d := range data {
			ts.Add(d)
		}
	}
}
