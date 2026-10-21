# timeseries

**Conditioning for series of measurements in Go: readings that arrive
when they please, gaps where the instrument was silent, and values
nobody believes.**

[![Go Reference](https://pkg.go.dev/badge/usefulrisk.com/timeseries.svg)](https://pkg.go.dev/usefulrisk.com/timeseries)

```go
import "usefulrisk.com/timeseries"

ts := timeseries.NewTimeSeries("outdoor temperature")
ts.AddBatchData(readings)                            // in any order

cleaned, rejected, _ := ts.RemoveOutbounds(-40, 60)  // implausible readings
hourly, _ := cleaned.RegularizeWithTolerance(time.Hour, 5*time.Minute, timeseries.AggMean)
filled, _ := hourly.InterpolateWithin(timeseries.InterpLinear, 2*time.Hour)

filled.PrintStats()
```

## The problem

A measured series is never the clean grid the textbooks assume. The
logger drifts a few minutes around the hour. The sensor goes offline for
a night. A join leaves a row missing. Some readings are plainly wrong,
and a division by zero upstream turns one into garbage.

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
[notavalue](https://pkg.go.dev/github.com/fflamingodev/notavalue), which
keeps two kinds of non-number apart:

- **NaV** — nothing was measured. Stepped over by every aggregate, neutral in addition.
- **NaN** — a computation broke. Propagates everywhere, so it cannot be lost.

A month with one missing day still has a mean; a month with one broken
value does not, and says so. The distinction runs through the whole
library, and it shows up where you would not expect it: a variation
across a gap is missing rather than invented, a point rejected as an
outlier becomes missing rather than broken, and interpolation fills gaps
while leaving broken values alone — covering an error with a plausible
number is how a bug stops being noticed.

The idea is not local to this library. Prometheus draws the same line the
same way, in the payload of a NaN, between a series that stopped and a
computation that failed.

## What is in the box

- **Series** — `Datum` is an instant and a value, never separated. A `TimeSeries` is chronological with exact deltas at all times, whatever order the readings arrive in. A `TsContainer` holds the variants of one signal.
- **Summary** — `Stats()` returns thirty statistics, computed on demand and never cached.
- **Cleaning** — fixed bounds, percentile fences, a z-score envelope, or Peirce's criterion. Each returns the cleaned series *and* the readings it rejected.
- **Resampling** — `Regularize` onto a grid of fixed steps, with a tolerance for loggers that drift; `DownscaleDaily` and its siblings for calendar periods, daylight saving included.
- **Filling** — seven interpolation methods, on time rather than on rank, with a limit on how long a silence may be bridged.
- **Compressing** — `Reduce` keeps only the changes, `Expand` puts the grid back, `MarkSilences` records an outage as a gap.
- **Showing** — readable tables for a terminal, `ToJSON` for a front end.

## Install

```
go get usefulrisk.com/timeseries
```

Its only dependency is
[notavalue](https://pkg.go.dev/github.com/fflamingodev/notavalue), which
itself depends on nothing but the standard library.

**Loading is what to get right at scale.** `Add` shifts the tail of the
slice for every reading that belongs earlier, which is quadratic;
`AddBatchData` sorts once. On a million readings arriving in random
order, that is two and a half minutes against one second. Use
`AddBatchData` whenever the readings are already in hand.

## Documentation

[**pkg.go.dev**](https://pkg.go.dev/usefulrisk.com/timeseries) has the API
reference and thirteen runnable examples — seven here, six in
`notavalue`. `go test` checks each of them against its printed output, so
an example that no longer matches the code fails the build.

[**docs/GUIDE.fr.md**](https://github.com/fflamingodev/timeseries/blob/main/docs/GUIDE.fr.md) is the long form, in French: the
reasoning behind the NaV policy, how databases answer the same question,
the handling of time and time zones, and a chapter per operation with
what it does to gaps and to errors. A typeset PDF is attached to each
release.

The guide is written in LaTeX, and `make` builds the PDF and regenerates
the Markdown. Its appendices — the API signatures and the examples — are
extracted from the Go sources, so neither can describe a function that no
longer exists. That toolchain concerns nobody who only wants to use the
library: `docs/` is a separate module, and none of it is downloaded by
`go get`.

## Status

Stable. From `v1.0.0` on, the library follows semantic versioning: no
exported name, signature or documented behavior changes before a `v2`.
Minor versions only add.

**v0.1.0 is retracted.** It had the NaV semantics inverted — NaV
propagated through `Add` and `Sub`, and aggregates silently skipped plain
`NaN`. Use v0.2.0 or later.

## License

The library is MIT. See [LICENSE](LICENSE).

The guide is not: it is a separate work, all rights reserved, and the
code it prints stays MIT. See [NOTICE](NOTICE) for what falls on which
side.
