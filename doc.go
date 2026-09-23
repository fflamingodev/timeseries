// Package timeseries conditions and computes irregular time series.
//
// # Missing values: NaV versus NaN
//
// The package distinguishes two kinds of non-numeric float64 values, and
// treats them in opposite ways:
//
//  1. NaV ("Not a Value") marks a missing observation: a sensor offline, a
//     day without data. A missing value
//     never stops a computation: NaV is skipped by aggregates and is
//     neutral in addition.
//
//  2. NaN marks a computation error (0/0, log(-1), Inf-Inf, ...). It always
//     propagates, so the error stays visible in the result. When a NaV and
//     a NaN meet, the NaN wins.
//
//  3. When a bucket or a series contains nothing but NaV, the result is NaV.
//
// The rationale: one missing day must not turn a monthly mean into an
// error, whereas a broken computation must never be silently hidden.
//
// # What the aggregates cost
//
// Skipping missing values must not make the library slow, because these
// functions are meant to run over long series and over many of them.
// So, as a rule, an aggregate reads its input once and allocates
// nothing. Sum, Mean, Min, Max and Bounds all work that way, out of a
// single shared traversal.
//
// Two exceptions, both deliberate:
//
//   - Median and Percentile must sort, so they copy the usable values
//     first. The caller's series keeps its order, which matters: in a
//     time series, order carries meaning.
//   - StdDev reads the input twice, since the mean must be known before
//     the deviations can be squared. It still copies nothing. The
//     one-pass alternatives were measured and rejected; the reasons are
//     in its documentation.
//
// Every one of these choices is backed by a benchmark kept in the
// repository, in aggregates_bench_test.go, together with the rejected
// alternatives and a test asserting that they all return the same
// result. Run them with:
//
//	go test -run XXX -bench . -benchmem
package timeseries
