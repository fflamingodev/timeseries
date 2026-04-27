package timeseries

import (
	"math"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers shared across *_test.go in this package.
// ---------------------------------------------------------------------------

// floatEqual compares two float64 with NaV-aware equality: NaV == NaV,
// plain NaN == plain NaN, NaV != plain NaN, everything else by ==.
func floatEqual(a, b float64) bool {
	switch {
	case IsNaV(a) && IsNaV(b):
		return true
	case IsNaV(a) != IsNaV(b):
		return false
	case math.IsNaN(a) && math.IsNaN(b):
		return true
	case math.IsNaN(a) != math.IsNaN(b):
		return false
	default:
		return a == b
	}
}

// assertFloat fails the test if got and want are not NaV-aware-equal.
func assertFloat(t *testing.T, got, want float64, msg string) {
	t.Helper()
	if !floatEqual(got, want) {
		t.Fatalf("%s: got %s, want %s", msg, Format(got), Format(want))
	}
}

// tFromSec returns a deterministic time.Time at the given UTC offset in
// seconds from a fixed epoch. Used to build fixtures without relying on
// time.Now().
func tFromSec(s int) time.Time {
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return epoch.Add(time.Duration(s) * time.Second)
}

// ---------------------------------------------------------------------------
// NaV identity and NaN classification
// ---------------------------------------------------------------------------

func TestNaV_identity(t *testing.T) {
	if !IsNaV(NaV) {
		t.Error("IsNaV(NaV) must be true")
	}
	if IsNaV(math.NaN()) {
		t.Error("IsNaV(math.NaN()) must be false")
	}
	if IsNaV(0) || IsNaV(1) || IsNaV(-1) || IsNaV(math.Inf(1)) || IsNaV(math.Inf(-1)) {
		t.Error("IsNaV must be false for regular numbers and infinities")
	}
}

func TestIsStdNaN(t *testing.T) {
	if IsStdNaN(NaV) {
		t.Error("IsStdNaN(NaV) must be false")
	}
	if !IsStdNaN(math.NaN()) {
		t.Error("IsStdNaN(math.NaN()) must be true")
	}
	if IsStdNaN(0) || IsStdNaN(math.Inf(1)) {
		t.Error("IsStdNaN must be false for numbers and infinities")
	}
}

func TestMathIsNaN_catchesBoth(t *testing.T) {
	if !math.IsNaN(NaV) {
		t.Error("math.IsNaN(NaV) must be true — NaV is a NaN class float")
	}
	if !math.IsNaN(math.NaN()) {
		t.Error("math.IsNaN(NaN) must be true")
	}
}

func TestNaV_survivesBitsRoundtrip(t *testing.T) {
	bits := math.Float64bits(NaV)
	back := math.Float64frombits(bits)
	if !IsNaV(back) {
		t.Errorf("NaV did not survive Float64bits/Float64frombits round-trip: %s", DebugBits(back))
	}
}

// ---------------------------------------------------------------------------
// Element-wise arithmetic: strict propagation
// ---------------------------------------------------------------------------

type binaryOpCase struct {
	name string
	a, b float64
	want float64
}

func runBinaryOp(t *testing.T, name string, op func(a, b float64) float64, cases []binaryOpCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(name+"/"+c.name, func(t *testing.T) {
			got := op(c.a, c.b)
			if !floatEqual(got, c.want) {
				t.Errorf("%s(%s, %s) = %s, want %s",
					name, Format(c.a), Format(c.b), Format(got), Format(c.want))
			}
		})
	}
}

func TestAdd_strictPropagation(t *testing.T) {
	runBinaryOp(t, "Add", Add, []binaryOpCase{
		{"num+num", 2, 3, 5},
		{"num+zero", 2, 0, 2},
		{"zero+zero", 0, 0, 0},
		{"NaV+num", NaV, 5, NaV},
		{"num+NaV", 5, NaV, NaV},
		{"NaV+NaV", NaV, NaV, NaV},
		{"NaN+num", math.NaN(), 5, math.NaN()},
		{"num+NaN", 5, math.NaN(), math.NaN()},
		{"NaV+NaN", NaV, math.NaN(), NaV}, // NaV wins
		{"NaN+NaV", math.NaN(), NaV, NaV},
	})
}

func TestSub_strictPropagation(t *testing.T) {
	// Regression guard: the old Sub returned -b when a was NaV, which is
	// semantically wrong. Under the new contract, any NaV input yields NaV.
	runBinaryOp(t, "Sub", Sub, []binaryOpCase{
		{"num-num", 5, 3, 2},
		{"num-zero", 5, 0, 5},
		{"NaV-num", NaV, 5, NaV},
		{"num-NaV", 5, NaV, NaV},
		{"NaV-NaV", NaV, NaV, NaV},
		{"NaN-num", math.NaN(), 5, math.NaN()},
		{"NaV-NaN", NaV, math.NaN(), NaV},
	})
}

func TestMul_strictPropagation(t *testing.T) {
	runBinaryOp(t, "Mul", Mul, []binaryOpCase{
		{"num*num", 2, 3, 6},
		{"num*zero", 5, 0, 0},
		{"NaV*num", NaV, 5, NaV},
		{"num*NaV", 5, NaV, NaV},
		{"NaV*zero", NaV, 0, NaV},
		{"NaN*num", math.NaN(), 5, math.NaN()},
		{"NaV*NaN", NaV, math.NaN(), NaV},
	})
}

func TestDiv_strictPropagation(t *testing.T) {
	runBinaryOp(t, "Div", Div, []binaryOpCase{
		{"num/num", 6, 3, 2},
		{"NaV/num", NaV, 5, NaV},
		{"num/NaV", 5, NaV, NaV},
		{"NaN/num", math.NaN(), 5, math.NaN()},
		{"NaV/NaN", NaV, math.NaN(), NaV},
	})
	// Special IEEE cases — Div does not intercept 0/0 or n/0.
	if !math.IsNaN(Div(0, 0)) || IsNaV(Div(0, 0)) {
		t.Error("Div(0,0) should be plain NaN, not NaV")
	}
	if !math.IsInf(Div(1, 0), 1) {
		t.Error("Div(1,0) should be +Inf")
	}
}

// ---------------------------------------------------------------------------
// Aggregates: skip-NaV default vs *Strict variants
// ---------------------------------------------------------------------------

func TestSum_skipNaV(t *testing.T) {
	assertFloat(t, Sum([]float64{1, 2, 3}), 6, "Sum of regular numbers")
	assertFloat(t, Sum([]float64{1, NaV, 3}), 4, "Sum skips NaV")
	assertFloat(t, Sum([]float64{1, math.NaN(), 3}), 4, "Sum skips plain NaN")
	assertFloat(t, Sum([]float64{NaV, NaV}), NaV, "Sum of all-NaV is NaV")
	assertFloat(t, Sum([]float64{}), NaV, "Sum of empty is NaV")
}

func TestMean_skipNaV(t *testing.T) {
	assertFloat(t, Mean([]float64{1, 2, 3}), 2, "Mean of {1,2,3}")
	assertFloat(t, Mean([]float64{1, NaV, 3}), 2, "Mean ignores NaV in denominator")
	assertFloat(t, Mean([]float64{NaV, NaV}), NaV, "Mean of all-NaV is NaV")
	assertFloat(t, Mean([]float64{}), NaV, "Mean of empty is NaV")
	assertFloat(t, Mean([]float64{42}), 42, "Mean of singleton")
}

func TestSumStrict(t *testing.T) {
	assertFloat(t, SumStrict([]float64{1, 2, 3}), 6, "SumStrict of regular numbers")
	assertFloat(t, SumStrict([]float64{1, NaV, 3}), NaV, "SumStrict propagates NaV")
	assertFloat(t, SumStrict([]float64{1, math.NaN(), 3}), math.NaN(), "SumStrict propagates NaN")
	assertFloat(t, SumStrict([]float64{NaV, math.NaN()}), NaV, "SumStrict: NaV beats NaN")
	assertFloat(t, SumStrict([]float64{}), NaV, "SumStrict of empty is NaV")
}

func TestMeanStrict(t *testing.T) {
	assertFloat(t, MeanStrict([]float64{1, 2, 3}), 2, "MeanStrict of {1,2,3}")
	assertFloat(t, MeanStrict([]float64{1, NaV, 3}), NaV, "MeanStrict propagates NaV")
	assertFloat(t, MeanStrict([]float64{1, math.NaN(), 3}), math.NaN(), "MeanStrict propagates NaN")
	assertFloat(t, MeanStrict([]float64{}), NaV, "MeanStrict of empty is NaV")
}

func TestCoalesce(t *testing.T) {
	assertFloat(t, Coalesce(NaV, NaV, 3, 4), 3, "Coalesce returns first non-NaN")
	assertFloat(t, Coalesce(NaV, math.NaN(), 3), 3, "Coalesce skips plain NaN too")
	assertFloat(t, Coalesce(NaV, NaV), NaV, "Coalesce of all-NaV is NaV")
	assertFloat(t, Coalesce(), NaV, "Coalesce of nothing is NaV")
	assertFloat(t, Coalesce(7), 7, "Coalesce of single value")
}

func TestLift(t *testing.T) {
	absent := func(x float64) bool { return x == -999 }
	assertFloat(t, Lift(42, absent), 42, "Lift passes normal value")
	assertFloat(t, Lift(-999, absent), NaV, "Lift returns NaV on sentinel")
}

func TestLiftSentinel(t *testing.T) {
	assertFloat(t, LiftSentinel(42, -999), 42, "LiftSentinel passes normal value")
	assertFloat(t, LiftSentinel(-999, -999), NaV, "LiftSentinel catches sentinel")
	assertFloat(t, LiftSentinel(-1, -999), -1, "LiftSentinel ignores non-sentinel")
}

// ---------------------------------------------------------------------------
// NaDate / NaDuration
// ---------------------------------------------------------------------------

func TestIsNaDate(t *testing.T) {
	if !IsNaDate(NaDate) {
		t.Error("IsNaDate(NaDate) must be true")
	}
	if IsNaDate(tFromSec(0)) {
		t.Error("IsNaDate of a real timestamp must be false")
	}
}

func TestSafeSub(t *testing.T) {
	a := tFromSec(100)
	b := tFromSec(60)
	if got := SafeSub(a, b); got != 40*time.Second {
		t.Errorf("SafeSub(a, b) = %v, want 40s", got)
	}
	if got := SafeSub(NaDate, b); got != NaDuration {
		t.Error("SafeSub(NaDate, b) must return NaDuration")
	}
	if got := SafeSub(a, NaDate); got != NaDuration {
		t.Error("SafeSub(a, NaDate) must return NaDuration")
	}
	if got := SafeSub(NaDate, NaDate); got != NaDuration {
		t.Error("SafeSub(NaDate, NaDate) must return NaDuration")
	}
}

// ---------------------------------------------------------------------------
// Format distinguishes NaV from NaN in pretty-print
// ---------------------------------------------------------------------------

func TestFormat_distinguishesNaVFromNaN(t *testing.T) {
	if Format(NaV) != "NaV" {
		t.Errorf("Format(NaV) = %q, want %q", Format(NaV), "NaV")
	}
	if Format(math.NaN()) != "NaN" {
		t.Errorf("Format(NaN) = %q, want %q", Format(math.NaN()), "NaN")
	}
	if Format(3.14) == "NaN" || Format(3.14) == "NaV" {
		t.Errorf("Format(3.14) = %q, expected a numeric rendering", Format(3.14))
	}
}
