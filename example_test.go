package timeseries_test

// The examples below are compiled and run by "go test", which compares
// what they print with their Output comment. An example that no longer
// matches the code fails the build instead of quietly misleading its
// reader, which is why they are written here rather than in prose.
//
// They live in the external test package, so they show the import and
// the timeseries. prefix a caller actually writes.

import (
	"fmt"
	"math"
	"time"

	nav "github.com/fflamingodev/notavalue"
	"usefulrisk.com/timeseries"
)

// at is a shorthand for a reading taken n minutes after 10:00 UTC.
func at(minutes int, meas float64) timeseries.Datum {
	base := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
	offset := time.Duration(minutes) * time.Minute
	return timeseries.NewDatum(base.Add(offset), meas)
}

// show prints a whole series on one line per point, in a form stable
// enough to be checked: the instant, then the measurement.
func show(ts *timeseries.TimeSeries) {
	ts.Range(func(i int, du timeseries.DataUnit) bool {
		fmt.Printf("  %s  %s\n",
			du.Chron.Format("15:04:05"), nav.Format(du.Meas))
		return true
	})
}

// The whole course of a series, from raw readings to a summary:
// implausible values out, a regular grid, short gaps bridged.
//
// The readings are handed over in no particular order, which costs
// nothing: AddBatchData sorts once and the series is chronological from
// then on.
func Example() {
	ts := timeseries.NewTimeSeries("outdoor temperature")
	ts.AddBatchData([]timeseries.Datum{
		at(40, 12.6),
		at(0, 11.8),
		at(20, 999), // the sensor went to its rail
		at(60, 13.1),
		at(120, 14.0), // one hour reported nothing at all
	})

	cleaned, rejected, err := ts.RemoveOutbounds(-40, 60)
	if err != nil {
		panic(err)
	}
	// The cleaned series still has five points: the rejected instant is
	// kept, its measurement now missing.
	fmt.Println("points:", cleaned.Len(), "rejected:", rejected.Len())

	hourly, err := cleaned.Regularize(time.Hour, timeseries.AggMean)
	if err != nil {
		panic(err)
	}
	fmt.Println("hourly grid:")
	show(hourly)

	filled, err := hourly.InterpolateWithin(
		timeseries.InterpLinear, 2*time.Hour)
	if err != nil {
		panic(err)
	}
	fmt.Println("after filling:")
	show(filled)

	fmt.Println("mean:", nav.Format(filled.Stats().Msmean))

	// Output:
	// points: 5 rejected: 1
	// hourly grid:
	//   10:00:00  11.8
	//   11:00:00  12.85
	//   12:00:00  14
	// after filling:
	//   10:00:00  11.8
	//   11:00:00  12.85
	//   12:00:00  14
	// mean: 12.883333333333333
}

// A gap and an error are not the same accident, and the summary says
// so. The first series has a reading missing; it still has a mean. The
// second carries the result of a broken computation; its mean is broken
// too, and refuses to look respectable.
func Example_gapVersusError() {
	withGap := timeseries.NewTimeSeries("a day short")
	withGap.AddBatchData([]timeseries.Datum{
		at(0, 10), at(20, nav.NaV), at(40, 20),
	})

	withError := timeseries.NewTimeSeries("a broken computation")
	withError.AddBatchData([]timeseries.Datum{
		at(0, 10), at(20, math.NaN()), at(40, 20),
	})

	gap, bad := withGap.Stats(), withError.Stats()
	fmt.Println("gap:   ", nav.Format(gap.Msmean), "NaV:", gap.NbreOfNaV)
	fmt.Println("broken:", nav.Format(bad.Msmean), "NaV:", bad.NbreOfNaV)

	// The same asymmetry on the variation from one point to the next: a
	// reading of 20 after a gap is not a rise of 20, so the delta is
	// missing rather than invented.
	after := withGap.At(2)
	fmt.Println("variation across the gap:", nav.Format(after.Dmeas))

	// Output:
	// gap:    15 NaV: 1
	// broken: NaN NaV: 0
	// variation across the gap: NaV
}

// Cleaning returns two series: what was kept and what was thrown out.
// The rejected readings are not lost, so the decision can be reviewed
// — and in the cleaned series the rejected instants are still there,
// now missing rather than absent.
func ExampleTimeSeries_RemoveOutbounds() {
	ts := timeseries.NewTimeSeries("pH")
	ts.AddBatchData([]timeseries.Datum{
		at(0, 7.1), at(20, -5), at(40, 7.3), at(60, 21), at(80, 7.0),
	})

	cleaned, rejected, err := ts.RemoveOutbounds(0, 14)
	if err != nil {
		panic(err)
	}
	fmt.Println("cleaned:")
	show(cleaned)
	fmt.Println("rejected:")
	show(rejected)

	// Output:
	// cleaned:
	//   10:00:00  7.1
	//   10:20:00  NaV
	//   10:40:00  7.3
	//   11:00:00  NaV
	//   11:20:00  7
	// rejected:
	//   10:20:00  -5
	//   11:00:00  21
}

// A logger meant to report on the hour that reports a few seconds
// late. Without a tolerance every such reading lands in the next
// window, which leaves one window empty and the next one holding two
// readings.
func ExampleTimeSeries_RegularizeWithTolerance() {
	base := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
	ts := timeseries.NewTimeSeries("drifting logger")
	ts.AddBatchData([]timeseries.Datum{
		timeseries.NewDatum(base, 10),
		timeseries.NewDatum(base.Add(time.Hour+4*time.Second), 11),
		timeseries.NewDatum(base.Add(2*time.Hour+9*time.Second), 12),
	})

	strict, err := ts.Regularize(time.Hour, timeseries.AggMean)
	if err != nil {
		panic(err)
	}
	fmt.Println("without tolerance:")
	show(strict)

	tolerant, err := ts.RegularizeWithTolerance(
		time.Hour, time.Minute, timeseries.AggMean)
	if err != nil {
		panic(err)
	}
	fmt.Println("with a minute of grace:")
	show(tolerant)

	// Output:
	// without tolerance:
	//   10:00:00  10
	//   11:00:00  NaV
	//   12:00:00  11
	//   13:00:00  12
	// with a minute of grace:
	//   10:00:00  10
	//   11:00:00  11
	//   12:00:00  12
}

// Bridging a short silence is reasonable; drawing a smooth line
// through an outage is not. The limit is measured between the readings
// surrounding the gap, so it means the same thing on a regular series
// and on a raw one.
func ExampleTimeSeries_InterpolateWithin() {
	ts := timeseries.NewTimeSeries("flow rate")
	ts.AddBatchData([]timeseries.Datum{
		at(0, 100),
		at(20, nav.NaV), // one point missing: 40 minutes to bridge
		at(40, 140),
		at(60, nav.NaV), // an outage: 80 minutes to bridge
		at(80, nav.NaV),
		at(100, nav.NaV),
		at(120, 60),
	})

	// Note what the limit is measured on: not the 20 minutes of the
	// grid, but the 40 minutes between the readings on either side of
	// the gap. A limit of 30 minutes here would fill nothing at all.
	filled, err := ts.InterpolateWithin(
		timeseries.InterpLinear, 45*time.Minute)
	if err != nil {
		panic(err)
	}
	show(filled)

	// Output:
	//   10:00:00  100
	//   10:20:00  120
	//   10:40:00  140
	//   11:00:00  NaV
	//   11:20:00  NaV
	//   11:40:00  NaV
	//   12:00:00  60
}

// A signal that spends its time not moving. Reduce keeps the instants
// where something happened, Expand puts the grid back. The pair is
// lossless on a stepped signal, which is what makes it worth storing
// the reduced form.
func ExampleTimeSeries_Reduce() {
	ts := timeseries.NewTimeSeries("valve")
	ts.AddBatchData([]timeseries.Datum{
		at(0, 0), at(20, 0), at(40, 1), at(60, 1),
		at(80, 1), at(100, 0), at(120, 0),
	})

	reduced := ts.Reduce()
	fmt.Println("reduced to", reduced.Len(), "points out of", ts.Len())
	show(reduced)

	first, _ := ts.First()
	last, _ := ts.Last()
	back, err := reduced.Expand(first.Chron, last.Chron, 20*time.Minute)
	if err != nil {
		panic(err)
	}
	fmt.Println("expanded again:", back.Len(), "points")
	show(back)

	// Output:
	// reduced to 4 points out of 7
	//   10:00:00  0
	//   10:40:00  1
	//   11:40:00  0
	//   12:00:00  0
	// expanded again: 7 points
	//   10:00:00  0
	//   10:20:00  0
	//   10:40:00  1
	//   11:00:00  1
	//   11:20:00  1
	//   11:40:00  0
	//   12:00:00  0
}

// A container holds the series that belong together — the raw
// readings, the cleaned version, each variant produced along the way —
// and Range walks them without copying anything.
//
// MeasTo is the companion for the inner loop: handed a slice with its
// length set to zero, it refills that slice instead of allocating a
// new one, so a pass over a hundred series allocates once.
func ExampleTsContainer() {
	raw := timeseries.NewTimeSeries("raw")
	raw.AddBatchData([]timeseries.Datum{
		at(0, 11.8), at(20, 999), at(40, 12.6),
	})
	cleaned, _, err := raw.RemoveOutbounds(-40, 60)
	if err != nil {
		panic(err)
	}

	tsc := timeseries.NewTsContainer("outdoor temperature")
	tsc.Put("raw", raw)
	tsc.Put("cleaned", cleaned)

	buf := make([]float64, 0, 1024)
	for _, name := range tsc.Names() {
		ts := tsc.Series(name)
		buf = ts.MeasTo(buf[:0])
		fmt.Printf("%-8s n=%d usable=%d mean=%s\n", name, ts.Len(),
			nav.CountUsable(buf), nav.Format(nav.Mean(buf)))
	}

	// Output:
	// raw      n=3 usable=3 mean=341.1333333333333
	// cleaned  n=3 usable=2 mean=12.2
}
