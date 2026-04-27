package timeseries

import (
	"fmt"
	"math"
	"time"
)

// NaDuration is the sentinel time.Duration returned by operations that
// cannot produce a valid duration (e.g. when one of the operands is a
// NaDate). It is set to math.MinInt64 converted to time.Duration, a value
// extremely unlikely to arise from legitimate application code.
//
// Callers must treat NaDuration as "no valid duration" and avoid using it
// in further arithmetic. SafeSub propagates it automatically.
const NaDuration time.Duration = time.Duration(math.MinInt64)

// NaNumber is math.NaN(), exported for callers that want to produce a
// plain NaN explicitly without importing math. Note: it is NOT a NaV;
// use the NaV variable when the intent is "missing by nature" (see
// navdefinition.go).
var NaNumber float64 = math.NaN()

// NaDate is the sentinel time.Time returned by operations that cannot
// produce a valid timestamp. It is the zero-value of time.Time, which is
// the idiomatic way in Go to express "no valid time".
var NaDate = time.Time{}

// IsNaDate reports whether t is the NaDate sentinel. Equivalent to
// t.IsZero(); prefer IsNaDate in library code for intent clarity.
func IsNaDate(t time.Time) bool {
	return t.IsZero()
}

// SafeSub returns a.Sub(b), but if either a or b is a NaDate it returns
// NaDuration instead of an arbitrary interval.
//
// This NaD propagation mirrors the NaV propagation applied to Meas
// values: once one side of a difference is missing, the difference must
// be marked missing too. Without this, undefined intervals silently
// leak into downstream statistics (averages, sums, regularization) and
// make them dishonest.
func SafeSub(a, b time.Time) time.Duration {
	if IsNaDate(a) || IsNaDate(b) {
		return NaDuration
	}
	return a.Sub(b)
}

// formatDuration renders a time.Duration, mapping NaDuration to a
// readable "NaDuration" literal instead of the nonsense numeric value.
func formatDuration(d time.Duration) string {
	if d == NaDuration {
		return "NaDuration"
	}
	return d.String()
}

// formatFloat renders a float64, distinguishing NaV from plain NaN so
// debug/pretty-print output carries the same missingness distinction
// that the data model does.
func formatFloat(f float64) string {
	if IsNaV(f) {
		return "NaV"
	}
	if math.IsNaN(f) {
		return "NaN"
	}
	return fmt.Sprintf("%v", f)
}
