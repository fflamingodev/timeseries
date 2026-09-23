package timeseries

import (
	"math"
	"strconv"
	"testing"
)

// want describes the expected outcome of an operation. Comparing float64
// results with == is not enough here: NaV and NaN are both NaN-class and
// never equal themselves, so the three cases are kept apart explicitly.
type want int

const (
	wantNaV want = iota // exactly the NaV sentinel
	wantNaN             // a plain NaN, i.e. a computation error
	wantNum             // an ordinary number, given by wantValue
)

// check reports whether got matches the expected outcome.
func check(t *testing.T, op string, got float64, w want, wantValue float64) {
	t.Helper()
	switch w {
	case wantNaV:
		if !IsNaV(got) {
			t.Errorf("%s = %v, want NaV", op, describe(got))
		}
	case wantNaN:
		if !IsStdNaN(got) {
			t.Errorf("%s = %v, want a plain NaN", op, describe(got))
		}
	case wantNum:
		if got != wantValue {
			t.Errorf("%s = %v, want %v", op, describe(got), wantValue)
		}
	}
}

// describe renders a float64 in a way that distinguishes NaV from NaN, so
// a failing test says which one it got.
func describe(x float64) string {
	switch {
	case IsNaV(x):
		return "NaV"
	case math.IsNaN(x):
		return "NaN"
	default:
		return strconv.FormatFloat(x, 'g', -1, 64)
	}
}

// -----------------------------------------------------------------------
// The sentinel itself
// -----------------------------------------------------------------------

func TestNaVIsNaNClass(t *testing.T) {
	if !math.IsNaN(NaV) {
		t.Fatal("math.IsNaN(NaV) = false, want true: NaV must stay NaN-class so " +
			"existing NaN-aware code keeps working")
	}
}

func TestIsNaV(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want bool
	}{
		{"NaV", NaV, true},
		{"plain NaN", math.NaN(), false},
		{"zero", 0, false},
		{"ordinary number", 42.5, false},
		{"+Inf", math.Inf(1), false},
		{"-Inf", math.Inf(-1), false},
	}
	for _, c := range cases {
		if got := IsNaV(c.in); got != c.want {
			t.Errorf("IsNaV(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsStdNaN(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want bool
	}{
		{"plain NaN", math.NaN(), true},
		{"NaV", NaV, false},
		{"ordinary number", 1, false},
		{"+Inf", math.Inf(1), false},
	}
	for _, c := range cases {
		if got := IsStdNaN(c.in); got != c.want {
			t.Errorf("IsStdNaN(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// A NaV must survive a trip through its bit pattern: the tag is what
// makes serialization and copying safe.
func TestNaVSurvivesBitsRoundtrip(t *testing.T) {
	back := math.Float64frombits(math.Float64bits(NaV))
	if !IsNaV(back) {
		t.Errorf("NaV lost its tag through Float64bits/Float64frombits, got %s", describe(back))
	}
}

// A NaV produced by arithmetic on a NaV must still be recognizable.
func TestNaVSurvivesArithmetic(t *testing.T) {
	if !IsNaV(Add(NaV, NaV)) {
		t.Error("Add(NaV, NaV) lost the NaV tag")
	}
}

// -----------------------------------------------------------------------
// The four operations
// -----------------------------------------------------------------------

func TestAdd(t *testing.T) {
	cases := []struct {
		name  string
		a, b  float64
		want  want
		value float64
	}{
		// NaV is the neutral element: a missing term does not stop a sum.
		{"NaV + 5", NaV, 5, wantNum, 5},
		{"5 + NaV", 5, NaV, wantNum, 5},
		{"NaV + 0", NaV, 0, wantNum, 0},
		{"NaV + (-5)", NaV, -5, wantNum, -5},
		// Nothing but missing values yields a missing value.
		{"NaV + NaV", NaV, NaV, wantNaV, 0},
		// An error wins over a missing value, and over everything else.
		{"NaN + 5", math.NaN(), 5, wantNaN, 0},
		{"5 + NaN", 5, math.NaN(), wantNaN, 0},
		{"NaN + NaV", math.NaN(), NaV, wantNaN, 0},
		{"NaV + NaN", NaV, math.NaN(), wantNaN, 0},
		{"NaN + NaN", math.NaN(), math.NaN(), wantNaN, 0},
		// Ordinary arithmetic is untouched.
		{"2 + 3", 2, 3, wantNum, 5},
	}
	for _, c := range cases {
		check(t, c.name, Add(c.a, c.b), c.want, c.value)
	}
}

func TestSub(t *testing.T) {
	cases := []struct {
		name  string
		a, b  float64
		want  want
		value float64
	}{
		// A difference needs both terms. Neither -5 nor 5 may be invented.
		{"NaV - 5", NaV, 5, wantNaV, 0},
		{"5 - NaV", 5, NaV, wantNaV, 0},
		{"NaV - NaV", NaV, NaV, wantNaV, 0},
		{"NaV - 0", NaV, 0, wantNaV, 0},
		// An error still wins.
		{"NaN - 5", math.NaN(), 5, wantNaN, 0},
		{"NaV - NaN", NaV, math.NaN(), wantNaN, 0},
		// Ordinary arithmetic is untouched.
		{"5 - 3", 5, 3, wantNum, 2},
	}
	for _, c := range cases {
		check(t, c.name, Sub(c.a, c.b), c.want, c.value)
	}
}

func TestMul(t *testing.T) {
	cases := []struct {
		name  string
		a, b  float64
		want  want
		value float64
	}{
		{"NaV * 5", NaV, 5, wantNaV, 0},
		{"5 * NaV", 5, NaV, wantNaV, 0},
		// An unknown quantity of something stays unknown, so this is not 0.
		{"NaV * 0", NaV, 0, wantNaV, 0},
		{"NaV * NaV", NaV, NaV, wantNaV, 0},
		{"NaN * NaV", math.NaN(), NaV, wantNaN, 0},
		{"3 * 4", 3, 4, wantNum, 12},
	}
	for _, c := range cases {
		check(t, c.name, Mul(c.a, c.b), c.want, c.value)
	}
}

func TestDiv(t *testing.T) {
	cases := []struct {
		name  string
		a, b  float64
		want  want
		value float64
	}{
		{"NaV / 5", NaV, 5, wantNaV, 0},
		{"5 / NaV", 5, NaV, wantNaV, 0},
		{"NaV / 0", NaV, 0, wantNaV, 0},
		{"NaN / NaV", math.NaN(), NaV, wantNaN, 0},
		{"6 / 3", 6, 3, wantNum, 2},
		// IEEE-754 is left untouched when no operand is NaN-class.
		{"5 / 0", 5, 0, wantNum, math.Inf(1)},
		{"-5 / 0", -5, 0, wantNum, math.Inf(-1)},
	}
	for _, c := range cases {
		check(t, c.name, Div(c.a, c.b), c.want, c.value)
	}
}

// 0/0 is a computation error, not a missing value: it must come out as a
// plain NaN so that the mistake stays visible.
func TestDivZeroByZeroIsNaNNotNaV(t *testing.T) {
	got := Div(0, 0)
	if IsNaV(got) {
		t.Fatal("Div(0, 0) = NaV, want a plain NaN: 0/0 is an error, not a missing value")
	}
	if !IsStdNaN(got) {
		t.Errorf("Div(0, 0) = %s, want a plain NaN", describe(got))
	}
}

// Inf - Inf is likewise an error introduced by the computation itself.
func TestSubInfinitiesIsNaNNotNaV(t *testing.T) {
	got := Sub(math.Inf(1), math.Inf(1))
	if !IsStdNaN(got) {
		t.Errorf("Sub(+Inf, +Inf) = %s, want a plain NaN", describe(got))
	}
}
