# Changelog

All notable changes to this project are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] — 2026-04-24

Initial public release.

### Added

- **NaN-boxing for missing data.** A tagged quiet NaN sentinel `NaV`
  ("Not a Value") that is `math.IsNaN()`-true and distinguishable from
  a plain `math.NaN()` via `IsNaV()`. Companion sentinels `NaDate` and
  `NaDuration` for `time.Time` and `time.Duration`, with a NaD-aware
  `SafeSub`.
- **Strict propagation in element-wise arithmetic.** `Add`, `Sub`,
  `Mul`, `Div` propagate NaV/NaN unconditionally. NaV wins over plain
  NaN on conflict.
- **Skip-NaV in aggregates.** `Sum`, `Mean`, `Median`, `StdDev`, `Min`,
  `Max` skip NaV and NaN by default; `*Strict` variants propagate.
- **Datum / DataUnit hierarchy.** `Datum{Chron, Meas}` is the pure
  observation type; `DataUnit` embeds it and adds `Dchron`, `Dmeas`.
  Field promotion makes existing reads work unchanged.
- **DeltasValid invariant.** Tracks whether `Dchron`/`Dmeas` are in
  sync with the current order of `DataSeries`. Set by
  `SortDeltasStats`, cleared by every mutation.
- **Regularization.** `Regularize(freq, agg)` and
  `RegularizeWithTolerance(freq, agg, tolerance)` resample on a
  freq-aligned grid in a single O(N+M) pass with buffer reuse and an
  `IsSorted` fast path. `MustRegularizeWithTolerance` for callers who
  treat arguments as coding invariants.
- **Calendar-bucket downscalers.** `DownscaleDaily`,
  `DownscaleWeekly`, `DownscaleMonthly`, `DownscaleYearly`.
- **Seven interpolation methods.** Linear, nearest, forward-fill,
  backward-fill, log-linear, natural cubic spline, monotone (PCHIP)
  cubic spline.
- **Outlier removal.** `RemoveOutbounds` (fixed bounds, with min<=max
  check), `PercCleaning` and `Lower`/`UpperPercCleaning` (percentile
  fences), `ZscoreCleaning` (z-score envelope), `PeirceOutlierRemoval`
  (Peirce's criterion with corrected `Rtable[57]`).
- **Polishing pipeline.** `TsContainer.ApplyPolishing` chains reduce →
  cleaning → regularize → cleaning → interpolation according to a
  declarative `RecipesCatalogueRow`. Recovers panics into a structured
  error and reports completed steps.
- **Structured errors.** All errors are `*Error` carrying `Op`,
  `Kind`, `Msg`, `Field`, `Value`, `Err`, `Stack`. `errors.Is` matches
  by `Kind`, `errors.As` extracts the fields. Sentinels:
  `ErrEmptyInput`, `ErrBounds`, `ErrInvalidArg`, `ErrBadState`,
  `ErrUnknownOption`, `ErrPanic`.
- **No panics in normal use.** Bad arguments return errors. Explicit
  `Must*` wrappers are provided for callers who prefer panicking.
- **JSON export.** `JSONFloat64` and `JSONDurationNS` marshal NaV/NaN
  and NaDuration as `null`. `BasicStats` exposes `NbreOfNaV` and
  `NbreOfNaN` separately.
- **Subpackage `apidto`.** Application-specific HTTP DTOs isolated
  from the core library.
- **82+ unit tests** covering NaV semantics, structured errors,
  Datum/DataUnit construction, `DeltasValid` lifecycle, statistics
  edge cases, regularization with and without tolerance, JSON
  roundtrip, aggregators.

### Documentation

- `README.md` with the NaV pitch, the `Datum`/`DataUnit` hierarchy,
  the regularize variants, the interpolation/cleaning catalogue, the
  Errors contract, and a positioning table against gonum/gota/flux/
  pandas.
- `NAV_SEMANTICS.md` — bilingual (FR/EN) deep dive on the propagation
  vs. skip-NaV rules, with truth tables, the "why not zero" trap
  (and the Go zero-initialization angle), and an FAQ.
- Package-level `doc.go` with the philosophy and the Errors section.

[0.1.0]: https://github.com/fflamingodev/timeseries/releases/tag/v0.1.0
