# timeseries

**Conditioning for series of measurements in Go: readings that arrive
when they please, gaps where the instrument was silent, and values
nobody believes.**

[![Go Reference](https://pkg.go.dev/badge/usefulrisk.com/timeseries.svg)](https://pkg.go.dev/usefulrisk.com/timeseries)

```go
import "usefulrisk.com/timeseries"

ts := timeseries.NewTimeSeries("outdoor temperature")
ts.AddBatchData(readings)                                  // in any order

cleaned, rejected, _ := ts.RemoveOutbounds(-40, 60)        // implausible readings
hourly, _  := cleaned.RegularizeWithTolerance(time.Hour, 5*time.Minute, timeseries.AggMean)
filled, _  := hourly.InterpolateWithin(timeseries.InterpLinear, 2*time.Hour)

filled.PrintStats()
```

## The problem

A measured series is never the clean grid the textbooks assume. The
logger drifts a few minutes around the hour. The sensor goes offline
for a night. A join leaves a row missing. Some readings are plainly
wrong, and a division by zero upstream turns one into garbage.

Handled carelessly, each of these silently corrupts the result:

| What happens | The careless answer | What it costs |
|---|---|---|
| A reading is missing | Store `0` | A missing day becomes a day at freezing point; every mean is wrong |
| A reading is missing | Store `math.NaN()` | Honest, but one gap in January makes the yearly mean `NaN` |
| A reading is implausible | Delete the point | The series now claims nothing happened then; the grid is broken |
| A computation failed upstream | Treat it like a gap | The bug disappears and nobody investigates |
| The logger drifts by 3 minutes | Bucket by rank | Hourly windows hold two readings, then none |

## The answer

This package rests on
[notavalue](https://github.com/fflamingodev/notavalue), which keeps two
kinds of non-number apart:

- **NaV** — nothing was measured. Stepped over by every computation.
- **NaN** — a computation broke. Propagates everywhere, so it cannot be lost.

That single distinction runs through the whole library, and it shows up
where you would not expect it:

```go
// A month with one missing day still has a mean.
// A month with one broken value does not, and says so.

// A variation across a gap is missing, not invented:
// a reading of 14 after a gap is not a rise of 14.

// A point rejected as an outlier becomes NaV, not NaN:
// the rest of the month keeps its statistics.

// Interpolation fills gaps and leaves broken values alone:
// covering an error with a plausible number is how a bug stops being noticed.
```

## What is in the box

**Series** — `Datum` is a reading: an instant and a value, never
separated. `TimeSeries` keeps them chronological with exact deltas at
all times; there is no method to sort or to repair, because there is
never anything to repair. `TsContainer` holds the variants of one
signal, in the order they were produced.

**Summary** — `Stats()` returns thirty statistics: the span, the
extremes with the instants they occurred at, the first usable reading,
the regularity of the sampling, and the counts of gaps and errors.

**Cleaning** — fixed bounds, percentile fences, a z-score envelope, or
Peirce's criterion. Each returns the cleaned series *and* the readings
it rejected, so the decision can be reviewed.

**Resampling** — `Regularize` onto a grid of fixed steps, with a
tolerance for loggers that drift; `DownscaleDaily` and its siblings for
calendar periods, when reporting to people who think in months.

**Filling** — seven interpolation methods, and a limit on how long a
silence may be bridged.

**Compressing** — `Reduce` keeps only the changes, `Expand` puts the
grid back, `MarkSilences` records an outage as a gap so that reducing a
raw series does not lose it.

**Showing** — readable tables for a terminal, `ToJSON` for a front end.

## Three decisions worth knowing

**Windows close on the right, calendar periods open on the left.** A
reading at exactly a tick belongs to the window ending there; a reading
at midnight sharp opens the new day. The first suits a computation, the
second suits a calendar.

**A regular grid is aligned on UTC.** In a zone offset by whole hours,
an hourly grid lands on the local hour; in India or Nepal it lands on
the local half hour. That is the price of grids that line up across
zones.

**Statistics are never cached.** `Stats()` computes on demand, so a
summary can never describe a series that has since changed.

## Cost

A point is 32 bytes — a `time.Time` and a `float64` — so a million
readings weigh about 32 MB.

Loading is what to get right at scale:

| A million readings, arriving in random order | Time |
|---|---|
| `Add`, one at a time | ~2.5 minutes |
| `AddBatchData` | ~1 second |

`Add` shifts the tail of the slice for every reading that belongs
earlier, which is quadratic; `AddBatchData` sorts once. Use the second
whenever the readings are already in hand.

## Install

```
go get usefulrisk.com/timeseries
```

Requires Go 1.21 or later. Its only dependency is
[notavalue](https://github.com/fflamingodev/notavalue), which itself
depends on nothing but the standard library.

## Going further

[GUIDE.md](GUIDE.md) is the long form: the reasoning behind the NaV
policy, the handling of time and time zones in detail, and a chapter
per operation with what it does to gaps and to errors.

## Status

Pre-1.0. The semantics described here are settled and covered by tests,
but names and signatures may still move before `v1.0.0`.

## License

MIT. See [LICENSE](LICENSE).
