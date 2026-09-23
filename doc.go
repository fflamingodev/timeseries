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
package timeseries
