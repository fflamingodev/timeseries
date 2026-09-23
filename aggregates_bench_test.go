package timeseries

import (
	"math"
	"math/rand"
	"testing"
)

// These benchmarks are kept in the repository for two reasons: they
// guard against a performance regression, and they hold the evidence
// behind two decisions documented in aggregates.go — Mean reading the
// slice once rather than composing Sum and CountUsable, and StdDev
// reading it twice rather than using Welford's method.
//
// Run them with:
//
//	go test -run XXX -bench . -benchmem
//
// Figures measured on an Apple M-series laptop, 100 000 points, 5% of
// them missing:
//
//	Mean, single pass          117 µs      0 allocs
//	Mean, composed             145 µs      0 allocs   (+24%)
//	StdDev, two passes         201 µs      0 allocs
//	StdDev, Welford            337 µs      0 allocs   (+68%)
//	StdDev, copy + two passes  296 µs      1 alloc, 800 kB
//	Median (copies and sorts)  5.0 ms      1 alloc, 800 kB

// benchSeries builds a series that looks like real instrument data:
// values far from zero, where a naive sum-of-squares variance would lose
// precision, and one missing value in twenty.
func benchSeries(n int) []float64 {
	r := rand.New(rand.NewSource(1))
	xs := make([]float64, n)
	for i := range xs {
		if i%20 == 0 {
			xs[i] = NaV
			continue
		}
		xs[i] = 300 + r.NormFloat64() // a temperature in kelvin
	}
	return xs
}

// sink keeps the compiler from removing the computation under test.
var sink float64

const benchSize = 100000

// -----------------------------------------------------------------------
// Mean: one pass against composition
// -----------------------------------------------------------------------

func BenchmarkMean(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = Mean(xs)
	}
}

// meanComposed is the rejected alternative: correct, shorter, and
// reading the slice twice for the same answer.
func meanComposed(xs []float64) float64 {
	return Div(Sum(xs), float64(CountUsable(xs)))
}

func BenchmarkMeanComposed(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = meanComposed(xs)
	}
}

// -----------------------------------------------------------------------
// StdDev: two passes against the one-pass alternatives
// -----------------------------------------------------------------------

func BenchmarkStdDev(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = StdDev(xs)
	}
}

// stdDevWelford is the rejected one-pass alternative: it updates the
// running mean and the squared deviations together, at the cost of one
// division per value.
func stdDevWelford(xs []float64) float64 {
	n := 0
	var mean, squares float64
	for _, x := range xs {
		if math.IsNaN(x) {
			if !IsNaV(x) {
				return math.NaN()
			}
			continue
		}
		n++
		delta := x - mean
		mean += delta / float64(n)
		squares += delta * (x - mean)
	}
	if n < 2 {
		return NaV
	}
	return math.Sqrt(squares / float64(n-1))
}

func BenchmarkStdDevWelford(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = stdDevWelford(xs)
	}
}

// stdDevCopy is what StdDev did before: it materializes the usable
// values, then walks that copy twice.
func stdDevCopy(xs []float64) float64 {
	usable, s := usableValues(xs)
	if v, forced := s.verdict(); forced {
		return v
	}
	n := len(usable)
	if n < 2 {
		return NaV
	}
	mean := Mean(usable)
	var squares float64
	for _, x := range usable {
		d := x - mean
		squares += d * d
	}
	return math.Sqrt(squares / float64(n-1))
}

func BenchmarkStdDevCopy(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = stdDevCopy(xs)
	}
}

// The three algorithms must agree. A benchmark that measures a wrong
// answer measures nothing, and this also guards the rejected variants
// from drifting if someone revisits the decision.
func TestStdDevAlternativesAgree(t *testing.T) {
	xs := benchSeries(10000)
	ref := StdDev(xs)
	for _, alt := range []struct {
		name string
		got  float64
	}{
		{"Welford", stdDevWelford(xs)},
		{"copy then two passes", stdDevCopy(xs)},
	} {
		if math.Abs(alt.got-ref) > 1e-12 {
			t.Errorf("%s gives %v, StdDev gives %v", alt.name, alt.got, ref)
		}
	}
}

// Same requirement for the two ways of computing a mean.
func TestMeanAlternativeAgrees(t *testing.T) {
	xs := benchSeries(10000)
	if got, ref := meanComposed(xs), Mean(xs); math.Abs(got-ref) > 1e-12 {
		t.Errorf("composed mean gives %v, Mean gives %v", got, ref)
	}
}

// -----------------------------------------------------------------------
// The other aggregates, for regression watching
// -----------------------------------------------------------------------

func BenchmarkSum(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = Sum(xs)
	}
}

func BenchmarkBounds(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = Bounds(xs)
	}
}

// Median is the costly one: it must copy the usable values and sort
// them. The copy is what the benchmark is really watching.
func BenchmarkMedian(b *testing.B) {
	xs := benchSeries(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = Median(xs)
	}
}
