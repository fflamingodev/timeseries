// Package timeseries provides a float64 time series toolkit designed
// around a single idea: distinguish values that were never observed
// from values that are the result of a broken computation.
//
// Missing data is encoded in a tagged quiet NaN called NaV
// ("Not-a-Value"). NaV is a regular float64 that happens to be
// NaN-class — so any algorithm that already guards against NaN keeps
// working without changes — but carries a bit pattern that lets the
// library tell it apart from a plain math.NaN(). The distinction is
// used everywhere: aggregates (Sum, Mean, Median, StdDev, Min, Max)
// skip NaV/NaN by default, element-wise operators (Add, Sub, Mul, Div)
// propagate NaV/NaN strictly, and BasicStats reports NaV and NaN
// counts separately.
//
// For the full semantic contract, see the package-level comment in
// navdefinition.go. The short version:
//
//   - Element-wise: NaV propagates. Add(NaV, 5) == NaV, not 5.
//   - Aggregates:   NaV is skipped by default, following pandas/NumPy
//                   conventions. Use the *Strict variants for strict
//                   propagation at the aggregate level too.
//   - NaV vs NaN:   both are IEEE NaN, so math.IsNaN(x) is true for
//                   both. IsNaV(x) is true only for NaV.
//
// Time series are defined as an ordered collection of DataUnit
// (timestamp + value + deltas). Typical usage:
//
//   ts := timeseries.TimeSeries{Name: "sensor_42"}
//   ts.AddData(t0, 23.4)
//   ts.AddData(t1, 24.1)
//   ts.AddData(t2, timeseries.NaV) // reading missing
//   ts.SortDeltasStats()
//
//   hourly := ts.Regularize(time.Hour, timeseries.AggAverage)
//   if err := hourly.Interpolate(timeseries.InterpLinear); err != nil {
//       // handle the (rare) error
//   }
//
// # Errors
//
// All errors returned by this package are of type *timeseries.Error,
// which wraps an Op (operation name), a Kind (stable category, see
// Kind constants), a short lowercase Msg, optional Field/Value
// metadata for diagnostics, and an optional wrapped cause accessible
// via errors.Unwrap. Package-level sentinels match by Kind:
//
//   if errors.Is(err, timeseries.ErrEmptyInput) { ... }
//   if errors.Is(err, timeseries.ErrBounds)     { ... }
//   if errors.Is(err, timeseries.ErrInvalidArg) { ... }
//
// To access the structured fields, use errors.As:
//
//   var e *timeseries.Error
//   if errors.As(err, &e) {
//       log.Printf("%s failed on %s=%v", e.Op, e.Field, e.Value)
//   }
//
// Under normal use, the library does not panic: invalid arguments
// return an *Error with Kind == KindInvalidArg, empty or all-NaV
// input returns KindEmptyInput, and downstream step panics inside
// ApplyPolishing are recovered into an *Error with Kind == KindPanic
// that carries the runtime stack. Explicit panicking variants are
// offered via Must* wrappers (e.g. MustRegularizeWithTolerance) for
// callers who treat their arguments as coding invariants.
//
// The library targets Go 1.22+. See the README for a longer
// introduction and runnable examples.
package timeseries
