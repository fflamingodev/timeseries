# The timeseries guide

Everything the package does, and why it does it that way.

The [README](README.md) is the tour; this is the manual. It can be read
in order, or opened at the chapter that matches the question at hand.

*Une version française est disponible : [GUIDE.fr.md](GUIDE.fr.md).*

---

## Contents

1. [What a measured series really looks like](#1-what-a-measured-series-really-looks-like)
2. [The NaV policy](#2-the-nav-policy)
   — [Two kinds of non-number](#21-two-kinds-of-non-number)
   · [The three rules](#22-the-three-rules)
   · [What the rules produce](#23-what-the-rules-produce)
   · [NaN boxing in detail](#24-nan-boxing-in-detail)
   · [Why not a struct with a bool](#25-why-not-a-struct-with-a-bool)
   · [One warning](#26-one-warning)
3. [Time](#3-time)
   — [How an instant is stored](#31-how-an-instant-is-stored)
   · [Unix time, underneath](#32-unix-time-underneath)
   · [RFC 3339, the written form](#33-rfc-3339-the-written-form)
   · [A duration that does not exist](#34-a-duration-that-does-not-exist)
   · [Two conventions](#35-two-conventions-deliberately-different)
   · [Alignment on UTC](#36-a-regular-grid-is-aligned-on-utc)
   · [Calendar periods](#37-calendar-periods-and-the-days-that-are-not-24-hours-long)
   · [Precision](#38-precision)
4. [A series](#4-a-series)
5. [The summary](#5-the-summary)
6. [Cleaning](#6-cleaning)
7. [Regularization](#7-regularization)
8. [Downscaling by calendar](#8-downscaling-by-calendar)
9. [Compressing and restoring](#9-compressing-and-restoring)
10. [Interpolation](#10-interpolation)
11. [Containers](#11-containers)
12. [Output](#12-output)
13. [Performance, measured](#13-performance-measured)
14. [Coming from the previous version](#14-coming-from-the-previous-version)
15. [Where the reasoning lives](#15-where-the-reasoning-lives)

---

## 1. What a measured series really looks like

A time series, in a textbook, is a value per instant on a regular grid.
A measured one is never that.

The logger reports "every hour" and lands at 00:57, 02:03, 03:00,
04:01. The sensor goes offline for a night. A join leaves a row
missing. A battery dies mid-afternoon and the last readings before it
are nonsense. A division by zero upstream turns one value into garbage.
A technician resets a counter and the series jumps by ten thousand.

Every one of these is ordinary, and every one of them quietly corrupts
a result if it is handled carelessly. This package exists to handle
them explicitly, and to make the handling visible afterwards.

Its guiding principle is stated once and applied everywhere: **a
computation may say "I don't know", and must never say something it
cannot support.**

---

## 2. The NaV policy

### 2.1 Two kinds of non-number

Go gives a `float64` two ways of not being a number, and they are the
same way: `NaN`. The package draws a line through it, using the
[notavalue](https://github.com/fflamingodev/notavalue) module:

| | Meaning | Example | Treatment |
|---|---|---|---|
| **NaV** | Nothing was measured | Sensor offline, row absent, point rejected | Skipped |
| **NaN** | A computation broke | `0/0` upstream, `log(-1)` | Propagates |

Technically, NaV is a quiet NaN carrying a tag bit in its mantissa.
This costs nothing: a NaV is an ordinary `float64`, a `[]float64` needs
no companion mask, and `math.IsNaN` still recognizes it, so existing
NaN-aware code keeps working.

### 2.2 The three rules

1. **A gap never stops a computation.** It is skipped by aggregates and
   neutral in addition.
2. **An error always propagates.** When a gap and an error meet, the
   error wins.
3. **When there is nothing left to compute on, the result is a gap** —
   never zero.

Rule 3 deserves a line of its own. The mean of an empty series is NaV,
not 0. A zero mean is a statement about the data: it says the readings
averaged out to nothing. On an empty series there is no statement to
make, and saying zero would be an invention that no later reader could
detect.

### 2.3 What the rules produce

The consequences are not obvious, and they are the reason the policy is
worth stating:

| Situation | Result | Why |
|---|---|---|
| Mean of `[1, 2, NaV, 3]` | `2` | The gap is skipped; the divisor is 3, not 4 |
| Mean of `[1, 2, NaN, 3]` | `NaN` | An error upstream must stay visible |
| Mean of `[NaV, NaV]` | `NaV` | Nothing to say |
| `Sub(5, NaV)` | `NaV` | A difference needs both terms; returning 5 would read the gap as zero |
| `Add(NaV, 5)` | `5` | A sum tolerates an absent term |
| `Mul(NaV, 0)` | `NaV` | An unknown quantity of something stays unknown |
| Variation after a gap | `NaV` | A reading of 14 following a gap is not a rise of 14 |
| A rejected outlier | `NaV` | It was measured but is not believed: missing, not broken |
| Interpolating a `NaN` | left alone | Covering an error with a plausible number is how a bug stops being noticed |

The asymmetry between `Add` and `Sub` is the part people query most
often. A sum accumulates independent terms, and one absent term leaves
the others intact — which is why a monthly mean survives a missing day.
A difference compares two specific values; if one is unknown, the
difference is unknown, and any other answer manufactures a variation
nobody measured.

### 2.4 NaN boxing in detail

NaN boxing means storing information **inside** a floating-point
number, in bits the standard leaves free. To see where they are, a
`float64` has to be opened up.

#### The shape of a float

A `float64` is 64 bits, which IEEE-754 divides into three fields:

```
 ┌─┬───────────┬────────────────────────────────────────────────────┐
 │S│  exponent │                    mantissa                        │
 └─┴───────────┴────────────────────────────────────────────────────┘
  1     11 bits                      52 bits
```

An ordinary value reads as "mantissa × 2^exponent", signed. But the
standard reserves one configuration: **when all eleven exponent bits
are ones**, it is not a number any more.

- A zero mantissa means infinity, positive or negative by the sign.
- A non-zero mantissa means **NaN**.

And there the opportunity lies: the standard does not say *which*
value the mantissa must take. Any of them, as long as it is not zero,
makes a perfectly valid NaN. 2⁵² − 1 mantissas, two signs: **about nine
quadrillion bit patterns are NaN**, half of them quiet and safe to use.
All indistinguishable to arithmetic, and all wasted if nobody uses
them.

#### Quiet and signaling

The standard splits them again, by the top bit of the mantissa:

- **Quiet NaN** (bit set): travels through arithmetic silently. This is
  what `math.NaN()` returns and what any invalid operation produces on
  a current processor.
- **Signaling NaN** (bit clear): meant to raise a hardware exception.
  Go does not produce them, and most runtimes quiet them on first use.
  This package never deals with them.

That leaves **51 free bits** in the mantissa of a quiet NaN. Space
nobody uses.

#### What the package does with them

`NaV` is a quiet NaN with **one** of those free bits set:

```
math.NaN()  : 0 11111111111 1000000000000000000000000000000000000000000000000000
NaV         : 0 11111111111 1000000001000000000000000000000000000000000000000000
                            ↑        ↑
                            │        └── the NaV tag (bit 48)
                            └── the "quiet" bit
```

The test is one line: it is a NaN, **and** the tag bit is set.

```go
func IsNaV(x float64) bool {
    return math.IsNaN(x) && math.Float64bits(x)&navTag != 0
}
```

Four consequences follow, and they are what justifies the technique:

1. **No memory overhead.** A NaV is a `float64`. A slice of readings
   stays a `[]float64`, with no companion array.
2. **Existing code keeps working.** `math.IsNaN(NaV)` is true, so code
   already written to guard against NaN guards against NaV too. Nothing
   to recompile, nothing to audit.
3. **The distinction survives a binary round trip.** Copying,
   serializing in binary, going through `math.Float64bits`: the tag
   travels with the value, because it *is* the value.
4. **Fifty bits are still free.** "Never measured", "rejected as an
   outlier", "interpolated" could one day be told apart without
   breaking anything.

#### What it costs

The other side, stated plainly:

- **Arithmetic must go through the package's functions.** What becomes
  of the tag through a plain `-` is up to the processor (§2.6). That is
  the main constraint.
- **Text serialization loses the tag.** JSON has no NaN: every
  non-number becomes `null`, and only the counters carry the
  distinction across (§12.2).
- **It is an unfamiliar technique.** A reviewer meeting the library has
  to understand what they are reading first — which is what this
  chapter is for.

### 2.5 Why not a struct with a bool

It is the common answer, and the first one that comes to mind:

```go
type Valued struct {
    Value float64
    Valid bool
}
```

It is what `sql.NullFloat64` does in Go, `Option<f64>` in Rust,
`double?` in C#, `Optional<Double>` in Java. Why not use it? Four
reasons, measured on a million points.

#### 1. It doubles the memory

```
float64                       :  8 bytes
struct{ float64; bool }       : 16 bytes
```

The bool takes one byte, but alignment demands eight: seven bytes
wasted per point. Over a million readings, **8 MB become 16 MB**; over
ten million, 80 become 160.

A parallel `[]bool` mask does better — 9 MB — at the price of a second
slice to carry, to slice and to sort along with the first, and to
forget at the first refactoring.

#### 2. It cuts the library off from everything else

This is the decisive one. **Everything that computes on floats, in Go,
expects a `[]float64`**: gonum, Fourier transforms, statistics
packages, and `notavalue` itself.

With a struct, every call needs an extraction first — a loop and an
allocation proportional to the series:

| Mean over a million points | Time | Allocation |
|---|---|---|
| Struct, then extraction to `[]float64` | 1 368 µs | **8 MB per call** |
| NaN boxing, slice passed as it is | 623 µs | **none** |

Twice as slow, and 8 MB allocated per call. On a pipeline chaining
mean, median, deviation and percentiles, the bill is paid four times.

#### 3. A pointer is worse

`*float64`, with `nil` for absence, looks elegant. But every point
becomes an 8-byte pointer **plus** the value it points at, somewhere
else — and above all, a slice of a million pointers is walked by the
garbage collector at every cycle, where a `[]float64` holds no pointer
at all and stays invisible to it.

#### 4. A sentinel value is a trap

Coding absence as −999, or as 0, is the oldest answer. It works until
the day a real reading is −999 — and that day comes. A NaV cannot be
mistaken for a measurement: no operation on real numbers produces it.

#### The full table

| Approach | Bytes per point | Works as `[]float64` | Invisible to the GC | Can be confused with data |
|---|---|---|---|---|
| **NaN boxing** | 8 | yes | yes | no |
| `struct{float64; bool}` | 16 | no | yes | no |
| `[]float64` + `[]bool` | 9 | yes, but the mask travels apart | yes | no |
| `*float64` | 8 + the value | no | **no** | no |
| Sentinel −999 | 8 | yes | yes | **yes** |

Scanning speed does not settle it: the first four run between 0.6 and
0.8 ms per million points, and the gap is dominated by whatever else
the loop does. **Memory and interoperability decide**, not nanoseconds.

### 2.6 One warning

Do not use plain operators on values that may be missing. What payload
a NaN result carries is left to the processor, and processors disagree:
on some, `NaV - 5` comes out still tagged as a NaV, on others the tag is
lost. Go through `notavalue`'s `Add`, `Sub`, `Mul`, `Div` and the
aggregates, which make the outcome a property of the code rather than
of the machine it runs on.

---

## 3. Time

### 3.1 How an instant is stored

Timestamps are `time.Time`, kept exactly as given, location included. A
point therefore weighs 32 bytes rather than the 16 a count of
nanoseconds would take.

That is a deliberate price. A measurement separated from its instant is
no longer a measurement, and `time.Time` is the type every Go developer
already knows how to handle — zones, formatting, comparison, the
standard library. The package never converts a series to another zone
on its own: the zone a reading carries is a statement about where it
was taken.

### 3.2 Unix time, underneath

Underneath, every system that exchanges dates agrees on one origin:
**1 January 1970, 00:00:00 UTC**. An instant becomes a single number,
the count of what has elapsed since — seconds, milliseconds or
nanoseconds depending on the precision. That is *Unix time*: what
`time.Time` holds internally, what PostgreSQL stores, what sensors
report.

A few landmarks for reading such a number:

| Number | Unit | Instant |
|---|---|---|
| `0` | second | 1 January 1970, 00:00:00 UTC |
| `1 000 000 000` | seconds | 9 September 2001 |
| `1 767 225 600` | seconds | 1 January 2026 |
| `1 767 225 600 000 000 000` | nanoseconds | the same instant |

Three properties are worth knowing, because they explain choices made
here.

**Unix time knows nothing of zones.** It is a count from an origin, so
an absolute instant. Two sensors, one in Luxembourg and one in Tokyo,
measuring at the same moment produce the same number. A zone only
enters at display time — which is exactly why a regular grid aligns on
it (§3.6): it is the only reference two series share, wherever they
were recorded.

**It ignores leap seconds.** The Earth does not rotate regularly, and
UTC occasionally inserts a second — the last one in 2016. Unix time
pretends they do not exist: a day there is always exactly 86 400
seconds. Systems that must stay accurate absorb them by stretching
their clock imperceptibly over a few hours. In practice: a duration
spanning a leap second is wrong by one second, which matters only to
fine metrology.

**It says nothing about the precision of the measurement.** A timestamp
to the nanosecond does not mean the sensor knew what it was doing to
the nanosecond. The number is exact; the measurement may not be — and
the library keeps what it is given without pretending to improve it.

In Go, a `time.Time` carries more than a count: the absolute instant, a
location, and sometimes a monotonic clock reading — immune to system
clock changes — which the standard library uses to measure durations.
Timestamps coming from a database or a sensor carry none; that is of no
consequence here.

### 3.3 RFC 3339, the written form

Unix time is a number; a written form is needed too, readable by a
human and unambiguous to a machine. That is **RFC 3339**, and it is
what `ToJSON` produces:

```
2026-01-15T14:30:00Z           ← in UTC, "Z" for zero offset
2026-01-15T15:30:00+01:00      ← the same instant, seen from Paris
2026-01-15T14:30:00.123456789Z ← with its nanoseconds
```

The form is strict, and that is its value: a date, a `T`, a time, an
offset. Largest unit to smallest, always, zero-padded. It is a
deliberately narrow subset of ISO 8601, which allows a crowd of
variants — weeks, durations, partial dates, omitted separators — that
no implementation supports in full.

Three properties justify sticking to it:

**Lexicographic order is chronological order.** Two RFC 3339 timestamps
written with the same offset compare as ordinary text, character by
character, and the resulting order is the right one. That is what lets
a log file be sorted with `sort`, or a text column be indexed without
conversion.

**The offset is mandatory.** A timestamp without one — as so many
databases and APIs produce — does not designate an instant:
`2026-01-15 14:30:00` may be half past two in Paris, in Tokyo or in New
York, three instants hours apart. RFC 3339 forbids it. When data
arrives in that shape, a zone has to be supplied, and that is a
decision rather than a conversion.

**But an offset is not a zone.** `+01:00` says how far local time sits
from UTC at that instant; it does not say Paris, nor whether summer
time applied. A series serialized to RFC 3339 and read back therefore
loses the name of its zone: it keeps the exact instant, which is enough
for any computation, but a calendar grouping done after that trip falls
back on a frozen offset rather than a real zone. **When local days
matter, group before serializing**, not after.

### 3.4 A duration that does not exist

The first point of a series has no predecessor, so the interval before
it does not exist. That is `NaDuration`, the counterpart of NaV for
time. It prints as `NaDuration` rather than as the nonsense figure of
−2562047h47m16s, and `IsNaDuration` recognizes it.

### 3.5 Two conventions, deliberately different

| | Window | A reading at the boundary | The emitted instant |
|---|---|---|---|
| `Regularize` | closed on the right | belongs to the window that **ends** there | the tick |
| `Downscale*` | open on the left | **opens** the new day | the last instant of the period |

Regularize suits a computation: every window lasts exactly the same
duration, so two points always weigh the same. Downscale suits a
calendar: a reading at midnight belongs to the day that begins, as any
human would say.

Both emit their point at the *end* of the period, so both read as
"everything up to here".

### 3.6 A regular grid is aligned on UTC

The grid is not aligned on the first reading — that would make two
series incomparable — but on absolute time, which is to say on UTC.

In a zone offset by whole hours, this is invisible:

```
Paris (UTC+01:00), reading at 10:47 → hourly window at 10:00 local
```

In a zone offset by half an hour, it is not:

```
Kolkata (UTC+05:30), reading at 10:47 → hourly window at 10:30 local
```

Both are the same instants in UTC, which is the point: two series
regularized at the same step land on the very same instants, wherever
they were recorded. If local hour boundaries are what matters — a daily
report for a local team — use `DownscaleDaily`, which works in the
calendar, or shift the timestamps before regularizing.

### 3.7 Calendar periods, and the days that are not 24 hours long

Calendar boundaries only mean something in a place, so the Downscale
family computes them in the location of the series' first reading.

It handles the two days a year when the clocks change — 23 hours in
spring, 25 in autumn — and the zones where **midnight itself does not
exist**: in Santiago, Havana and the Azores the clocks spring forward
at midnight, and the day opens at 01:00. Building a period on a
midnight that never happened would close it an hour early, and every
period after it too.

### 3.8 Precision

Statistics on timestamps are computed on offsets from the first point,
not on nanoseconds since 1970.

A `float64` carries 53 bits of mantissa; a current Unix timestamp in
nanoseconds needs 61. Computing on absolute values rounds to a few
hundred nanoseconds — enough to make the mean instant of three readings
two nanoseconds apart land in the wrong place. Counting from the start
of the series keeps the numbers small and the result exact.

---

## 4. A series

### 4.1 The types

```go
type Datum struct {                 // a reading
    Chron time.Time
    Meas  float64
}

type DataUnit struct {              // a reading placed in a series
    Datum
    Dchron time.Duration            // elapsed since the previous point
    Dmeas  float64                  // variation since the previous point
}
```

A `TimeSeries` is a sequence of `DataUnit`, plus an `ID`, a `Name` for
humans and a `Comment` recording how it came to be.

### 4.2 The invariant

**At any moment, the points are in chronological order and every delta
agrees with that order.**

There is no method to sort a series, and none to recompute its deltas,
because there is never anything to repair. `Add` and `AddBatchData`
maintain the invariant as they insert — including when a reading
arrives out of order, which shifts the tail and recomputes exactly two
deltas.

This is why the points are not an exported field. Handing out the slice
would let a caller append out of order or sort by measurement, leaving
the deltas describing an order that no longer exists. An earlier
version of this library did exactly that and carried a boolean flag,
`deltasValid`, to warn that its own state might be a lie. Making the
state impossible is better than reporting it.

Reading goes through `Len`, `At`, `First`, `Last`, `Range`, `Meas` and
`MeasTo`.

### 4.3 Loading, and what it costs

| A million readings in random order | Time |
|---|---|
| `Add`, one at a time | ~2.5 minutes |
| `AddBatchData` | ~1 second |

`Add` shifts the tail for every reading that belongs earlier, which is
quadratic. Measured: 61 ms for 20 000 points, 255 ms for 40 000, 1 014
ms for 80 000 — a quadrupling at every doubling.

`AddBatchData` appends everything, sorts once and fills the deltas in a
single pass: 9, 21 and 45 ms on the same batches. When the batch
already extends the series in order — a query with `ORDER BY` — it
detects this in one pass and skips the sort entirely.

Use `Add` for the reading that arrives alone from a feed, and
`AddBatchData` for anything already in hand.

### 4.4 Feeding the aggregates without allocating

```go
var buf []float64
for _, ts := range all {
    buf = ts.MeasTo(buf[:0])           // no allocation after the first series
    fmt.Println(ts.Name, nav.Mean(buf))
}
```

`Meas()` allocates a fresh slice and is the convenient form; `MeasTo`
fills a buffer you keep, which matters when looping over hundreds of
series.

---

## 5. The summary

`Stats()` returns thirty fields, computed on demand and never cached —
so a summary can never describe a series that has since changed, and
two calls always agree.

The field names follow a key: `Ch*` concern timestamps, `Ms*` the
measurements, `DCh*` and `DMs*` the deltas, and `ChAt*` give the
instant at which another statistic occurs.

Grouped by what they answer:

| Question | Fields |
|---|---|
| Where does the series run? | `Chmin`, `Chmax`, with `ValAtChmin` and `ValAtChmax` |
| Where does the data start? | `ChFirstUsable`, `ValAtFirstUsable` |
| Where do the points sit? | `Chmean`, `Chmed` |
| What was measured? | `Msmin`, `Msmax` with their instants, `Msmean`, `Msmed`, `Msstd` |
| How often do readings arrive? | `DChmin`, `DChmax` with their instants, `DChmean`, `DChmed`, `DChstd` |
| How much does it move? | `DMsmin`, `DMsmax`, `DMsmean`, `DMsmed`, `DMsstd` |
| How good is the data? | `Len`, `NbreOfNaN`, `NbreOfNaV` |

Three of these are worth a comment.

**`Chmin` against `ChFirstUsable`.** The first is where the window
opens, the second where the data starts. On a feed whose sensor was
still warming up, they differ, and confusing them dates the series too
early.

**`NbreOfNaN` minus `NbreOfNaV`** is the number of genuine computation
errors. When the two are equal, every non-number in the series is a
gap and nothing is broken.

**`DChstd`** is the regularity of the sampling: near zero on a
disciplined feed, growing as soon as it stutters. It is often the most
informative number in the table, because a mean interval hides drift
completely — nineteen intervals of 56 to 66 minutes have a mean of
exactly one hour.

A statistic that cannot be computed is NaV, never zero. A single
reading has no standard deviation; a series of gaps has no mean.

---

## 6. Cleaning

### 6.1 What a rejection is

A rejected point is **not** an error and **not** a deletion. It is a
measurement the caller has decided to stop trusting, so it becomes NaV
and keeps its place in time.

The consequence is the whole point. The previous version of this
library replaced rejected points with a plain NaN, so one outlier in a
month turned every statistic of that month into NaN. With NaV the mean
carries on over what is left, and the counters say how much was
discarded.

The rejected readings come back in a series of their own, with their
original values, so nothing is lost and the decision can be reviewed.

### 6.2 What is never rejected

- **A gap**: there was nothing to judge.
- **A broken value**: cleaning is about implausible measurements, and
  disguising an error as a gap is a way of losing it.

Neither takes part in computing the fences.

### 6.3 The four methods

| Method | Fence | Use when |
|---|---|---|
| `RemoveOutbounds(min, max)` | Fixed | What is implausible is known in advance: a negative rainfall, a temperature above boiling |
| `RemovePercentileOutliers(low, high)` | Percentiles of the readings | The scale is unknown and the tails are suspect |
| `RemoveZScoreOutliers(level)` | Mean ± level × deviation | The readings scatter symmetrically; 3 is customary |
| `RemovePeirceOutliers()` | Peirce's criterion | No threshold can honestly be chosen |

Bounds are expressed as `float64`, with `NaV` meaning "no fence on that
side":

```go
ts.RemoveOutbounds(0, nav.NaV)        // nothing below zero
ts.RemovePercentileOutliers(nav.NaV, 95)  // trim the top only
```

**On percentiles:** they describe the readings at hand, so the fences
move with the data and something is always rejected. That is a
different tool from fixed bounds, not a better one.

**On Peirce's criterion:** it takes no threshold. It derives one from
the size of the sample, through a table published by Benjamin Peirce in
1852, and it decides how many readings a sample of that size may
legitimately lose. It is stricter on small samples, rejects at most
nine readings whatever the size, and — the test that matters — rejects
nothing at all from a well-behaved series. On the classical teaching
example it condemns exactly the two low readings, 90 and 89.

---

## 7. Regularization

### 7.1 The grid

```go
hourly, err := ts.Regularize(time.Hour, timeseries.AggMean)
```

Windows are closed on the right and aligned on the clock (§3.6). A
window that caught no reading is emitted as NaV: a regular grid must
have a point per step, and a step where nothing arrived is a gap, not
an absence of step. Nothing is emitted before the first reading or
after the last, since a series says nothing about what happened outside
its own span.

### 7.2 The tolerance

A logger meant to report on the hour that reports at 10:00:04 has not
skipped its window. Without a tolerance, every such reading lands one
window late:

| Window | No tolerance | 5 minutes of tolerance |
|---|---|---|
| 01:00 | 57 | 57 |
| 02:00 | **NaV** | 123 |
| 03:00 | **151.5** — the mean of two readings | 180 |
| 04:00 | **NaV** | 241 |

The hourly signal becomes an alternation of gaps and wrong means. The
tolerance must stay below the step, otherwise a reading would belong to
two windows.

### 7.3 The aggregators

| Aggregator | Returns | For |
|---|---|---|
| `AggMean` | Mean of the window | A physical quantity |
| `AggMedian` | Median | Same, when a stray reading must not drag the result |
| `AggMin`, `AggMax` | Extremes | Envelopes |
| `AggSum` | Sum | A quantity that accumulates: rainfall, energy, events |
| `AggFirst`, `AggLast` | The edge reading, gap included | A state signal, a counter |
| `AggFirstUsable`, `AggLastUsable` | The edge reading, stepping over gaps | A physical quantity |
| `AggCountUsable` | How many readings arrived | A coverage report |
| `AggSlope` | Slope of the fitted line | A trend per step |
| `AggIntegral(step)` | Area under the window | Turning a rate into a quantity |

All of them follow the NaV policy: a gap is skipped, an error
propagates. An empty window never reaches the aggregator — it is
emitted as NaV directly.

`Aggregator("maximum")` returns one by name, so a recipe stored in a
database can pick it. An unknown name is an error, never a silent
fallback on the mean.

---

## 8. Downscaling by calendar

`DownscaleDaily`, `DownscaleWeekly`, `DownscaleMonthly`,
`DownscaleYearly` group by calendar period.

**These are for presentation, not for computation.** Calendar periods
hide a variability in duration behind a familiar name:

- a month lasts 28 to 31 days — February holds about 10 % less time
  than March;
- a year lasts 365 or 366 days;
- a day lasts 24 hours, except the two when the clocks change.

Two monthly points therefore do not weigh the same. An aggregator that
grows with the length of the window — `AggSum`, `AggCountUsable`,
`AggIntegral` — produces numbers that differ partly because the periods
do, and February comes out lower for no reason but its length.

The rule of thumb: **regularize to compute, downscale last, to show.**

---

## 9. Compressing and restoring

### 9.1 Reduce

A signal that holds its value between changes — a door, a setpoint, a
state machine — stores the same number over and over. `Reduce` keeps
the first point, the last point, and the changes in between.

What counts as a change follows the policy rather than a plain
comparison, since no NaN equals itself: two consecutive gaps are not a
change, but going into a gap or coming out of one is — and that is
often the most interesting thing a series records.

`ReduceWithDeadband(band)` does the same with a tolerance: a reading is
kept only when it differs from **the last kept one** by more than the
band — not from the previous reading, so a slow drift is caught as soon
as it has moved by more than the band in total. That one loses
information, by a bounded amount, and says so in its name.

### 9.2 Expand

`Expand(start, end, step)` rebuilds a regular grid by holding each
value until the next change. Before the first known point it yields
NaV: the series says nothing about what the signal was doing then, and
an admitted gap is better than a made-up value.

### 9.3 MarkSilences

Reducing a raw series has a trap: if the instrument stops reporting for
a day, nothing in the reduced series records the silence — the last
value simply appears to hold.

`MarkSilences(maxGap)` inserts a NaV wherever two readings are further
apart than `maxGap`, dated `maxGap` after the last reading before the
silence — the moment an alarm watching the sensor would have fired. Up
to that moment the held value is legitimate: a sensor reporting every
ten minutes is not lost at the eleventh. The silence then survives
reduction, without going through a grid.

---

## 10. Interpolation

This is the one operation that invents data. What comes out is a
reading of what probably happened, not a measurement.

### 10.1 On time, not on rank

A gap is filled from where it sits **in time**. On a regularized series
this changes nothing; on a raw one it changes everything:

```
10 measured at 00:00, a gap at 00:01, 20 measured at 01:00

   by time (this library):  10.17
   by rank (the old one):   15
```

The gap is one minute from its left neighbour and fifty-nine from its
right one. Fifteen is not a defensible answer.

### 10.2 The seven methods

| Method | Fills with | For |
|---|---|---|
| `InterpLinear` | A straight line in time | A quantity that varies smoothly |
| `InterpNearest` | The closer neighbour | A signal that steps; keeps a measured value |
| `InterpForwardFill` | The last known value | A setpoint, a state, a counter |
| `InterpBackwardFill` | The next known value | The beginning of a series |
| `InterpLogLinear` | A constant growth rate | A quantity that compounds; both neighbours must be positive |
| `InterpCubicSpline` | A natural cubic spline | Smoothness above all |
| `InterpMonotoneSpline` | A PCHIP spline | Smoothness without overshoot |

**Why the monotone spline exists.** Through a step from 0 to 10, the
natural spline fills the following gap with **14.44** — a value above
every reading in the series, which no instrument saw. The monotone
spline stays at 10. On measured data, that makes it the safer of the
two.

### 10.3 Only gaps, and only short ones

Broken values are left alone (§2.3), and nothing extrapolates: a gap
with no reading on one side stays a gap, except for the two fills,
which lean on one neighbour by design.

No method knows how long an outage may reasonably be bridged. Half an
hour of missing temperature can be drawn through; three days cannot,
and a chart hiding the outage behind a smooth line is a lie told with a
straight face:

```go
filled, err := ts.InterpolateWithin(timeseries.InterpLinear, 2*time.Hour)
```

The limit is measured between the readings surrounding the gap, so it
means the same thing on a raw series and on a grid.

---

## 11. Containers

A `TsContainer` holds the variants of one signal: `raw`, `cleaned`,
`hourly`, whatever a recipe produced.

It keeps them **in the order they were filed**, which a Go map cannot:
map iteration is randomized, so an earlier version printed the variants
in a different order on every run and a chart legend reshuffled itself
between two calls. Replacing a variant keeps its rank.

`Get` distinguishes a name that was never filed from a name filed with
`nil`: the first was never asked for, the second could not be produced,
and the display says which.

---

## 12. Output

### 12.1 Terminal

```go
ts.PrettyPrint()      // the table of points
ts.PrintStats()       // the summary, in four sections
ts.PrettyPrint(90, 95) // a window, for a long series
```

Sentinels print by name — `NaV`, `NaN`, `NaDuration`, and a dash for an
unknown instant — because a gap that prints as `NaN`, or as a date in
year 1, is a gap nobody notices.

The `Fprint*` variants take an `io.Writer`, for a log or a test.

### 12.2 JSON

`ToJSON` produces the shape a front end reads: columns rather than
objects.

| Key | Content |
|---|---|
| `chron` | Instants, RFC 3339 |
| `meas`, `dmeas` | Measurements and variations, non-numbers as `null` |
| `dchron_ns` | Intervals in nanoseconds, `NaDuration` as `null` |
| `stats` | The summary, same field names as `BasicStats`, lowercased |

Every non-number becomes `null`, since JSON has no NaN. **The
distinction between a gap and an error does not survive the
conversion** — the counters `nbreOfNaV` and `nbreOfNaN` in the summary
are what carry it across.

---

## 13. Performance, measured

On an Apple M-series laptop.

| Operation | Cost |
|---|---|
| A point in memory | 32 bytes |
| Mean, min, max, sum | ~1.2 ns per point, no allocation |
| Standard deviation | ~2 ns per point, no allocation |
| Median, percentile | ~50 ns per point — they sort a copy |
| A full `Stats()` | ~60 ns per point |
| `AddBatchData`, in order | ~0.03 µs per point |
| `AddBatchData`, shuffled | ~0.5 µs per point |
| `Add`, one at a time, shuffled | quadratic — avoid |

Scaling is linear up to ten million points for everything but the
medians, which carry their `n log n`.

---

## 14. Coming from the previous version

The library was rewritten for `v0.2`. What changed, and why:

| Before | Now | Why |
|---|---|---|
| A rejected point became `NaN` | It becomes `NaV` | One outlier no longer destroys a month of statistics |
| Statistics stored in the series | `Stats()` computes on demand | Stored statistics went stale in silence |
| A `deltasValid` flag | An invariant | Making the bad state impossible beats reporting it |
| `DataSeries` public | Private, with accessors | That flag existed only because the slice was public |
| Tolerance accepted, never applied | Applied | It was silently ignored |
| Interpolation by rank | By time | Rank is wrong on any irregular series |
| Interpolation modified in place | Returns a new series | Consistent with cleaning and regularization |
| `MemId uint64` | `ID string` | It was never filled; a string takes a database key or a UUID |
| `Chstd` | Removed | The dispersion of timestamps says nothing; `DChstd` says the useful thing |
| — | `ChFirstUsable` | Where the window opens is not where the data starts |
| — | `NbreOfNaV` | Telling gaps from errors |
| — | `InterpolateWithin`, `MarkSilences`, `ReduceWithDeadband` | New |

For the JSON contract: `id` is now a string, `chstd` is gone, and
`nbreOfNaV`, `chFirstUsable` and `valAtFirstUsable` are added.

---

## 15. Where the reasoning lives

Every decision above is backed by a test that states it in words, and
by a commit message that records why it was taken. When a behaviour
here looks surprising, `git log` and `git blame` on the file will
usually explain it better than the code can.
