# timeseries

[![CI](https://github.com/fflamingodev/timeseries/actions/workflows/ci.yml/badge.svg)](https://github.com/fflamingodev/timeseries/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/usefulrisk.com/timeseries.svg)](https://pkg.go.dev/usefulrisk.com/timeseries)
[![Go Report Card](https://goreportcard.com/badge/usefulrisk.com/timeseries)](https://goreportcard.com/report/usefulrisk.com/timeseries)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A Go library for working with time series that arrive out of order, with gaps,
and from sources that occasionally go silent. Its distinguishing feature is a
`float64` sentinel called **`NaV`** ("Not a Value") that encodes *missing by
nature* as distinct from a plain `math.NaN()` produced by a broken
computation. Both are NaN-class, so existing code that guards against `NaN`
keeps working; but the library can tell them apart, and its aggregates,
statistics, and exports treat them differently.

If you have ever written `if v != -999 { ... }`, filtered out NaN before
computing a mean, or wondered whether the hole in your hourly sensor feed
meant "sensor offline" or "sensor lied", this library is for you.

```go
ts := timeseries.TimeSeries{Name: "sensor_42"}
ts.AddData(t0, 23.4)
ts.AddData(t1, 24.1)
ts.AddData(t2, timeseries.NaV)   // the sensor didn't report
ts.AddData(t3, 22.9)
ts.SortDeltasStats()

fmt.Println(ts.Msmean)    // 23.47 — averaged over the three real readings
fmt.Println(ts.NbreOfNaV) // 1
```

## Why NaV

```
math.NaN()  =  "the computation broke"    — a diagnostic about arithmetic
NaV         =  "the observation is absent" — a diagnostic about data provenance
```

Python has `np.nan` and pandas `NA`; R has `NA` and `NaN`; SQL has `NULL`.
Go's standard library has one `math.NaN()` and no dedicated missing-value
marker, so every Go data pipeline invents its own convention — sentinel
numbers (`-999`), a parallel `isValid []bool`, an option type, a struct
wrapper. NaV is a lightweight alternative that costs zero extra memory and
survives any arithmetic that would already survive NaN.

**And no, `0` is not a good shortcut.** A missing measurement is not a
measurement worth zero. A temperature sensor legitimately reports 0 °C
when water freezes; a meter in an empty home legitimately reports 0 kWh.
Worse, Go zero-initializes every unassigned `float64` to `0.0`, so a
field that was never set is byte-identical to one set to zero on
purpose — after a `json.Unmarshal` with a missing key, or a SQL `NULL`
read without `.Valid`, you have already silently fabricated data.
`NaV` removes the shortcut altogether: real zeroes stay zero, absences
stay NaV, `math.IsNaN(NaV)` is `true` so any accidental arithmetic
short-circuits. See [NAV_SEMANTICS.md](NAV_SEMANTICS.md) for the full
pedagogical deep dive.

### How it works — a data-domain application of NaN boxing

The trick used here is called **NaN boxing** and has a long lineage in
language runtimes. It started in Lisp/Scheme implementations, became
famous through LuaJIT (Mike Pall, 2005) and V8's early "SMI / HeapObject"
encoding, and is today how JavaScriptCore (Safari), SpiderMonkey
(Firefox), LuaJIT, MoarVM (Raku) and many others represent pointers and
small integers inside a single `double` register. The insight is simple
and exploits a feature of IEEE-754: a `float64` whose exponent bits are
all ones and whose mantissa is non-zero is a NaN *regardless* of the
rest of the payload — which leaves 51 free bits of mantissa to stash any
tag or pointer you want, while staying transparently NaN to any piece of
code that doesn't look at the bits.

Those 51 free bits usually carry pointers in a language runtime. Here,
we use exactly one of them — a single tag bit — to carry a semantic
distinction: "the measurement is absent" vs "the computation broke". The
same foundational trick, applied not to types but to *data provenance*.

Concretely, `NaV` is a "quiet NaN" (no floating-point exception when
used) with a dedicated tag bit in the mantissa. The tag is preserved
across `math.Float64bits` / `math.Float64frombits` and survives the
wire, so you can serialize it to a file or a network and get it back
unchanged.

```go
timeseries.NaV                     // the sentinel
timeseries.IsNaV(x)                // distinguishes NaV from math.NaN()
timeseries.IsStdNaN(x)             // the inverse: a plain NaN, not a NaV
math.IsNaN(timeseries.NaV)         // true — everything that checks NaN keeps working
```

> **Going deeper.** Why element-wise propagates but aggregates skip
> — and why that distinction is not arbitrary — is the single most
> common source of confusion for newcomers. A dedicated bilingual deep
> dive with truth tables, worked examples, a decision tree, a
> comparison with pandas/NumPy/R/SQL and an FAQ lives in
> [NAV_SEMANTICS.md](NAV_SEMANTICS.md). Read it if the rules below
> feel counterintuitive.

### Semantic contract

The library has two rules you need to remember.

**Element-wise arithmetic propagates strictly.** `Add`, `Sub`, `Mul`, `Div`
return `NaV` the moment any operand is `NaV`, and plain `NaN` when any
operand is plain `NaN` (NaV wins on conflict).

| call | returns |
| --- | --- |
| `Add(2, 3)` | `5` |
| `Add(NaV, 3)` | `NaV` |
| `Sub(5, NaV)` | `NaV` |
| `Mul(NaV, 0)` | `NaV` |
| `Add(math.NaN(), 3)` | `math.NaN()` |
| `Add(NaV, math.NaN())` | `NaV` |

**Aggregates skip NaV (and plain NaN) by default.** `Sum`, `Mean`, `Median`,
`StdDev`, `Min`, `Max` ignore NaN-class values, returning `NaV` only when
the input is empty or entirely missing. This matches `pandas.mean()` with
`skipna=True` and `numpy.nanmean`. If you want strict propagation at the
aggregate level, use `SumStrict` / `MeanStrict`.

```go
timeseries.Mean([]float64{1, 2, timeseries.NaV, 3})         // 2
timeseries.Mean([]float64{timeseries.NaV, timeseries.NaV})  // NaV
timeseries.MeanStrict([]float64{1, 2, timeseries.NaV, 3})   // NaV
```

Missing-by-nature companions of `NaV` are available for durations and
timestamps too: `NaDuration` and `NaDate`, with a NaD-aware subtraction
`SafeSub(a, b)` that returns `NaDuration` when either operand is the zero
`time.Time`.

## Install

```
go get usefulrisk.com/timeseries@v0.1.0
```

The library targets Go 1.22+.

The import path is the vanity domain `usefulrisk.com/timeseries`; the
backing repository is at `github.com/fflamingodev/timeseries`. Both work
for `go get`.

## Core types

```go
type Datum struct {
    Chron time.Time
    Meas  float64
}

type DataUnit struct {
    Datum                    // embedded: du.Chron, du.Meas work transparently
    Dchron time.Duration     // interval to previous point (NaDuration if none)
    Dmeas  float64           // delta of Meas to previous point (NaV if none)
}

type TimeSeries struct {
    MemId       uint64
    Name        string
    Comment     string
    DataSeries  []DataUnit
    BasicStats                // embedded: Msmin, Msmax, Msmean, …, NbreOfNaV, …
    // deltasValid — see DeltasValid()
}
```

`Datum` is the "pure observation" type: what just arrived. It has no notion
of delta. It is the right type for the signatures of ingestion code —
parsers, sensor drivers, CSV row unmarshalling — because what just arrived
doesn't know its predecessor yet.

`DataUnit` enriches a `Datum` with its first-order deltas against the
previous point in a sorted series. Thanks to field promotion, `du.Chron`
and `du.Meas` keep working as before; `du.Datum` exposes the embedded
observation explicitly when you need to pass it around on its own.

A fresh `DataUnit` built via `NewDataUnit` has `Dchron == NaDuration` and
`Dmeas == NaV` — it honestly says "I do not know my deltas yet". The
values become real only once the unit is placed in a sorted series and
`SortDeltasStats` runs.

### The `DeltasValid` invariant

Deltas are *derived* state. Any mutation of the series (append, sort by a
different key, truncate, reset, interpolate) flips an internal flag that
`DeltasValid() bool` exposes. `SortDeltasStats` is the single operation
that sets it back to true.

```go
var ts timeseries.TimeSeries
ts.AddData(t0, 1)
ts.AddData(t1, 2)
ts.DeltasValid()         // false — fresh points, no deltas computed

ts.SortDeltasStats()
ts.DeltasValid()         // true — deltas and stats are consistent

ts.SortMeasAsc()         // reordering invalidates deltas
ts.DeltasValid()         // false
```

This makes the contract with callers explicit: if you read `Dchron` or
`Dmeas` when `DeltasValid()` is false, the values may be stale. Call
`SortDeltasStats` to refresh.

## Resampling: `Regularize`

Most real-world time series arrive at irregular intervals. Before feeding
them into downstream analytics, you usually want them on a regular grid.
`Regularize` turns a series with arbitrary spacing into one sampled every
`freq`, aggregating each bucket with the function of your choice.

```go
hourly := ts.Regularize(time.Hour, timeseries.AggAverage)
daily  := ts.Regularize(24*time.Hour, timeseries.AggMaximum)
```

- Empty buckets between two populated buckets produce a point with
  `Meas = NaV`. Leading and trailing empty buckets are not emitted.
- The output grid is aligned on multiples of `freq` (via
  `time.Time.Truncate`) relative to the first point.
- Points whose Chron equals a bucket boundary are attributed to that
  bucket (right-closed convention).

Built-in aggregators: `AggAverage`, `AggMaximum`, `AggMinimum`, `AggLast`,
`AggOpen`, `AggMedian`, `AggCountValid`, `AggSlope`, `AggIntegral`,
`AggIncrementalCounter`. Any `func(local []float64) float64` works;
`AggFunc` is just an alias.

### Tolerance for jittery sensors

Physical sensors do not emit their k-th tick at exactly `k*period`. A
1-hour reading can arrive at 00:00:03 or 01:00:47 depending on the
vagaries of the network. `RegularizeWithTolerance` accepts a `tolerance`
that extends each bucket's right edge: a point with Chron in
`(windowEnd, windowEnd+tolerance]` is attributed to the bucket at
`windowEnd` instead of being pushed into the next one.

```go
hourly := ts.RegularizeWithTolerance(
    time.Hour, timeseries.AggAverage,
    3*time.Minute, // a tick arriving up to 3 min late still counts as on-time
)
```

`tolerance` must satisfy `0 <= tolerance < freq`; `tolerance >= freq`
panics (window membership would be ambiguous). `tolerance == 0` is
equivalent to `Regularize`.

Both entry points run in a single O(N+M) pass, reuse a shared scratch
buffer, and skip re-sorting when `IsSorted()` already returns true.

### Calendar-bucket variants

For non-fixed cadences (months vary, years vary, DST shifts weeks), use
the `Downscale*` family:

```go
ts.DownscaleDaily(timeseries.AggAverage)
ts.DownscaleWeekly(timeseries.AggSum)      // ISO week, Monday start
ts.DownscaleMonthly(timeseries.AggMaximum)
ts.DownscaleYearly(timeseries.AggMean)
```

## Interpolation

Seven methods, chosen via the `InterpolationMethod` enum and applied in
place on NaN-class values:

| constant | description |
| --- | --- |
| `InterpLinear` | piecewise linear between surrounding valid points |
| `InterpNearest` | nearest neighbor |
| `InterpForwardFill` | propagate last known value forward |
| `InterpBackwardFill` | propagate first known value backward |
| `InterpLogLinear` | linear interpolation in log space (positive values only) |
| `InterpCubicSpline` | natural cubic spline |
| `InterpMonotoneSpline` | Fritsch-Carlson PCHIP — preserves local monotonicity |

```go
ts.Interpolate(timeseries.InterpMonotoneSpline)
```

Interpolation invalidates `DeltasValid`; call `SortDeltasStats` again if
you need fresh deltas and stats.

## Outlier removal

```go
// Keep only values within [min, max], move outliers to the rejected series.
cleaned, rejected := ts.RemoveOutbounds(&min, &max, "")

// Symmetric percentile fences: lower = P(p), upper = P(100-p).
cleaned, rejected = ts.PercCleaning(5) // drops the extreme 5% on each side

// z-score fences around the mean.
cleaned, rejected = ts.ZscoreCleaning(3.0)

// Peirce's criterion: rigorous small-sample outlier test.
cleaned, rejected = ts.PeirceOutlierRemoval()

// Flush all NaN-class / infinite Meas into the rejected bin.
cleaned, rejected = ts.RemovedNonValid()
```

Cleaned series preserve the original length: rejected slots are replaced
by `Meas = NaV` so index alignment with the source is kept.

## JSON

`TimeSeriesJSON` is a wire-friendly DTO: `NaV` and `NaN` serialize as
`null`, `NaDuration` serializes as `null`, and `BasicStats` is exported
with the `NbreOfNaN` / `NbreOfNaV` counters so the receiver can audit
data quality.

```go
dto := ts.ToJSON()
b, _ := json.Marshal(dto)
```

## Errors

All errors returned by the library are of type `*timeseries.Error` and
carry structured context:

```go
type Error struct {
    Op    string  // operation name, e.g. "Regularize"
    Kind  Kind    // stable category (Bounds, EmptyInput, InvalidArg, …)
    Msg   string  // short, lowercase, no trailing punctuation
    Field string  // optional: offending argument name
    Value any     // optional: offending value
    Err   error   // wrapped cause, if any
    Stack string  // optional: runtime stack trace for panics
}
```

Comparison is by `Kind` via the package-level sentinels, so a consumer
can stay agnostic to the exact call site:

```go
if errors.Is(err, timeseries.ErrEmptyInput) { ... }
if errors.Is(err, timeseries.ErrBounds)     { ... }
if errors.Is(err, timeseries.ErrInvalidArg) { ... }
```

Extraction of the fields goes through `errors.As`:

```go
var e *timeseries.Error
if errors.As(err, &e) {
    log.Printf("%s failed on %s=%v", e.Op, e.Field, e.Value)
}
```

**The library does not panic under normal use.** Bad inputs return
errors with `Kind == KindInvalidArg`; empty or all-NaV series return
`KindEmptyInput`; downstream panics inside `ApplyPolishing` are
recovered into an `*Error` with `Kind == KindPanic` that carries the
runtime stack (`e.Stack`) and the list of completed pipeline steps.
Explicit panicking variants are offered via `Must*` wrappers (e.g.
`MustRegularizeWithTolerance`) for callers who treat their arguments
as coding invariants.

Partial-progress semantics: when a step fails mid-pipeline, earlier
successful variants remain in the `TsContainer` (e.g. if `Reduce`
succeeded but `Regularize` failed, the `"Reduced"` key is still
populated) — the error describes which step broke, the container keeps
what was produced so far.

## Where it fits

| feature                        | `timeseries`  | `gonum/stat`  | `gota/series` | `influxdata/flux` | `pandas`        |
|--------------------------------|:-------------:|:-------------:|:-------------:|:-----------------:|:---------------:|
| missing-by-nature vs NaN       | ✓             |               | partial       |                   | ✓ (via `NA`)    |
| irregular input, auto-sort     | ✓             |               | manual        | ✓                 | ✓               |
| regularize with tolerance      | ✓             |               |               | some              |                 |
| PCHIP monotone interpolation   | ✓             |               |               |                   | ✓ (SciPy)       |
| Peirce outlier criterion       | ✓             |               |               |                   |                 |
| zero-allocation hot loop       | ✓             | ✓             |               |                   |                 |
| Go-native, no CGO              | ✓             | ✓             | ✓             | ✓                 |                 |

`timeseries` is not a replacement for `gonum` (which is the right
dependency when you need serious numerical linear algebra and optimization)
nor for `flux` (which targets the InfluxDB query engine). It aims at the
middle ground: a pure-Go library that ingests noisy, irregular float
time series and gets them into a clean, stats-rich, NaV-aware shape for
downstream use.

## Status

**v0.1.0 — initial public release.** The core API (NaV, `Datum` /
`DataUnit`, `Regularize` / `RegularizeWithTolerance`, the seven
interpolation methods, the cleaners, structured errors) is stable and
covered by 80+ unit tests. The `*Strict` aggregate variants and
`MustRegularizeWithTolerance` are part of the contract.

The pre-v1.0 caveat applies: the public surface may evolve in minor
releases as feedback comes in. Breaking changes will be called out in
[`CHANGELOG.md`](CHANGELOG.md) and bumped accordingly.

Feedback, issues and PRs are welcome — see
[`CONTRIBUTING.md`](CONTRIBUTING.md) for the short version.

License: MIT.
