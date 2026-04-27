package timeseries_test

import (
	"errors"
	"fmt"
	"time"

	"usefulrisk.com/timeseries"
)

// Example demonstrates the canonical use of the package: ingest a few
// observations including a missing one, sort + compute deltas + stats,
// then read the count of NaV.
func Example() {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var ts timeseries.TimeSeries
	ts.Name = "sensor_42"
	ts.AddData(t0, 23.4)
	ts.AddData(t0.Add(1*time.Minute), 24.1)
	ts.AddData(t0.Add(2*time.Minute), timeseries.NaV) // sensor was offline
	ts.AddData(t0.Add(3*time.Minute), 22.9)
	ts.SortDeltasStats()

	fmt.Printf("len = %d\n", ts.Len)
	fmt.Printf("missing = %d\n", ts.NbreOfNaV)
	fmt.Printf("mean = %.2f\n", ts.Msmean) // averaged over the three real readings
	// Output:
	// len = 4
	// missing = 1
	// mean = 23.47
}

// ExampleSub_propagation shows that element-wise operators propagate
// NaV strictly — they do not silently treat absence as zero.
func ExampleSub_propagation() {
	x := timeseries.Sub(timeseries.NaV, 5)
	fmt.Println(timeseries.Format(x))
	// Output: NaV
}

// ExampleMean_skipNaV shows that aggregates skip NaV by default and
// return the average of the remaining valid values.
func ExampleMean_skipNaV() {
	xs := []float64{1, 2, timeseries.NaV, 3}
	fmt.Println(timeseries.Mean(xs))
	// Output: 2
}

// ExampleMeanStrict shows the opt-in propagation flavor: a single NaV
// in the input poisons the entire aggregate.
func ExampleMeanStrict() {
	xs := []float64{1, 2, timeseries.NaV, 3}
	out := timeseries.MeanStrict(xs)
	fmt.Println(timeseries.Format(out))
	// Output: NaV
}

// ExampleTimeSeries_Regularize resamples an irregular series onto a
// fixed-frequency grid. Empty buckets between populated ones become
// NaV; leading and trailing empty buckets are not emitted.
func ExampleTimeSeries_Regularize() {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var ts timeseries.TimeSeries
	ts.AddData(t0, 1)
	ts.AddData(t0.Add(30*time.Second), 2)
	ts.AddData(t0.Add(50*time.Second), 3)
	ts.SortDeltasStats()

	out := ts.Regularize(10*time.Second, timeseries.AggAverage)
	for _, du := range out.DataSeries {
		fmt.Printf("%s -> %s\n", du.Chron.Format("15:04:05"), timeseries.Format(du.Meas))
	}
	// Output:
	// 00:00:00 -> 1
	// 00:00:10 -> NaV
	// 00:00:20 -> NaV
	// 00:00:30 -> 2
	// 00:00:40 -> NaV
	// 00:00:50 -> 3
}

// ExampleTimeSeries_RegularizeWithTolerance shows the late-arrival
// tolerance: a point that arrives slightly past a bucket boundary is
// still attributed to the previous bucket instead of being pushed to
// the next one.
func ExampleTimeSeries_RegularizeWithTolerance() {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var ts timeseries.TimeSeries
	ts.AddData(t0, 1)
	// Tick due at +10s but arriving 3s late.
	ts.AddData(t0.Add(13*time.Second), 2)
	ts.SortDeltasStats()

	out, err := ts.RegularizeWithTolerance(10*time.Second, timeseries.AggLast, 5*time.Second)
	if err != nil {
		panic(err)
	}
	for _, du := range out.DataSeries {
		fmt.Printf("%s -> %s\n", du.Chron.Format("15:04:05"), timeseries.Format(du.Meas))
	}
	// Output:
	// 00:00:00 -> 1
	// 00:00:10 -> 2
}

// Example_errorsIs shows the structured-error contract: comparison by
// Kind via the package-level sentinels.
func Example_errorsIs() {
	_, err := timeseries.Percentile([]float64{1, 2, 3}, 200)
	switch {
	case errors.Is(err, timeseries.ErrBounds):
		fmt.Println("argument out of range")
	case errors.Is(err, timeseries.ErrEmptyInput):
		fmt.Println("empty input")
	default:
		fmt.Println("other error")
	}
	// Output: argument out of range
}

// Example_errorsAs shows how to recover the structured fields of a
// returned error for diagnostics.
func Example_errorsAs() {
	_, err := timeseries.Percentile([]float64{1, 2, 3}, 200)
	var e *timeseries.Error
	if errors.As(err, &e) {
		fmt.Printf("%s failed on %s=%v\n", e.Op, e.Field, e.Value)
	}
	// Output: Percentile failed on p=200
}
