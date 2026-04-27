package timeseries

import (
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Structural tests on the Error type
// ---------------------------------------------------------------------------

func TestError_ImplementsErrorInterface(t *testing.T) {
	var _ error = &Error{}
}

func TestError_FormatsMsg(t *testing.T) {
	e := &Error{Op: "Foo", Msg: "something went wrong"}
	got := e.Error()
	if got != "Foo: something went wrong" {
		t.Errorf("Error() = %q, want %q", got, "Foo: something went wrong")
	}
}

func TestError_FormatsFieldAndValue(t *testing.T) {
	e := &Error{Op: "Percentile", Msg: "p out of range", Field: "p", Value: 101.0}
	got := e.Error()
	if !strings.Contains(got, "Percentile:") {
		t.Errorf("missing Op prefix in %q", got)
	}
	if !strings.Contains(got, "(p=101") {
		t.Errorf("missing (field=value) in %q", got)
	}
}

func TestError_WrapsInnerError(t *testing.T) {
	inner := errors.New("inner cause")
	e := &Error{Op: "Outer", Msg: "wrap", Err: inner}
	if !strings.Contains(e.Error(), "inner cause") {
		t.Errorf("wrapped error not rendered: %q", e.Error())
	}
	if !errors.Is(e, inner) {
		t.Error("errors.Is should recognize the wrapped inner error")
	}
}

// ---------------------------------------------------------------------------
// errors.Is: sentinel matching by Kind
// ---------------------------------------------------------------------------

func TestErrorsIs_MatchesKind(t *testing.T) {
	enriched := &Error{Op: "Percentile", Kind: KindBounds, Msg: "p outside (0,100]", Field: "p", Value: 999.0}
	if !errors.Is(enriched, ErrBounds) {
		t.Error("enriched KindBounds should match ErrBounds sentinel")
	}
	if errors.Is(enriched, ErrEmptyInput) {
		t.Error("KindBounds must not match ErrEmptyInput")
	}
}

func TestErrorsIs_MatchesOpWhenSpecified(t *testing.T) {
	enriched := &Error{Op: "Percentile", Kind: KindBounds}

	if !errors.Is(enriched, &Error{Op: "Percentile", Kind: KindBounds}) {
		t.Error("same Op+Kind should match")
	}
	if errors.Is(enriched, &Error{Op: "StdDev", Kind: KindBounds}) {
		t.Error("different Op should not match")
	}
	if !errors.Is(enriched, &Error{Kind: KindBounds}) {
		t.Error("empty Op in target should act as wildcard")
	}
}

func TestErrorsIs_UnknownKindWildcard(t *testing.T) {
	// A target with KindUnknown acts as a wildcard on Kind — matches any
	// *Error (filtered only by Op).
	enriched := &Error{Op: "Regularize", Kind: KindInvalidArg}
	if !errors.Is(enriched, &Error{Kind: KindUnknown}) {
		t.Error("KindUnknown target should wildcard-match any *Error")
	}
}

// ---------------------------------------------------------------------------
// errors.As: extracting the structured fields
// ---------------------------------------------------------------------------

func TestErrorsAs_ExtractsFields(t *testing.T) {
	_, err := Percentile([]float64{1, 2, 3}, 200)
	if err == nil {
		t.Fatal("expected an error from Percentile with p=200")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Op != "Percentile" {
		t.Errorf("Op = %q, want %q", e.Op, "Percentile")
	}
	if e.Field != "p" {
		t.Errorf("Field = %q, want %q", e.Field, "p")
	}
	if e.Value != 200.0 {
		t.Errorf("Value = %v, want 200", e.Value)
	}
	if e.Kind != KindBounds {
		t.Errorf("Kind = %v, want KindBounds", e.Kind)
	}
}

// ---------------------------------------------------------------------------
// Kind.String
// ---------------------------------------------------------------------------

func TestKindString(t *testing.T) {
	cases := []struct {
		k    Kind
		want string
	}{
		{KindUnknown, "Unknown"},
		{KindEmptyInput, "EmptyInput"},
		{KindBounds, "Bounds"},
		{KindInvalidArg, "InvalidArg"},
		{KindBadState, "BadState"},
		{KindUnknownOption, "UnknownOption"},
		{KindPanic, "Panic"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("Kind(%d).String() = %q, want %q", c.k, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Panic → error: RegularizeWithTolerance
// ---------------------------------------------------------------------------

func TestRegularizeWithTolerance_ReturnsInvalidArgOnBadFreq(t *testing.T) {
	ts := &TimeSeries{}
	ts.AddData(tFromSec(0), 1)
	_, err := ts.RegularizeWithTolerance(0, AggAverage, 0)
	if err == nil {
		t.Fatal("expected error for freq=0")
	}
	if !errors.Is(err, ErrInvalidArg) {
		t.Errorf("err = %v, want InvalidArg", err)
	}
}

// ---------------------------------------------------------------------------
// Cleaners propagate structured errors
// ---------------------------------------------------------------------------

func TestRemoveOutbounds_InvalidArgWhenMinGTMax(t *testing.T) {
	ts := &TimeSeries{}
	ts.AddData(tFromSec(0), 1)
	mn, mx := 10.0, 5.0
	_, _, err := ts.RemoveOutbounds(&mn, &mx)
	if err == nil {
		t.Fatal("expected error when min > max")
	}
	if !errors.Is(err, ErrInvalidArg) {
		t.Errorf("err = %v, want InvalidArg", err)
	}
}

func TestPercCleaning_PropagatesPercentileError(t *testing.T) {
	ts := &TimeSeries{}
	ts.AddData(tFromSec(0), 1)
	_, _, err := ts.PercCleaning(200) // 200 is out of (0, 100]
	if err == nil {
		t.Fatal("expected error from underlying Percentile")
	}
	// Kind should bubble up as KindBounds because Percentile returns
	// KindBounds and PercCleaning wraps it with KindBounds too.
	if !errors.Is(err, ErrBounds) {
		t.Errorf("err = %v, want KindBounds", err)
	}
	// errors.As should recover the outer PercCleaning envelope.
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Op != "PercCleaning" {
		t.Errorf("Op = %q, want %q", e.Op, "PercCleaning")
	}
	// And the inner cause should also be recoverable.
	var inner *Error
	if !errors.As(e.Err, &inner) {
		t.Fatal("expected wrapped *Error from Percentile")
	}
	if inner.Op != "Percentile" {
		t.Errorf("inner Op = %q, want %q", inner.Op, "Percentile")
	}
}

// ---------------------------------------------------------------------------
// Interpolate returns an error on unknown method
// ---------------------------------------------------------------------------

func TestInterpolate_UnknownMethodReturnsError(t *testing.T) {
	ts := &TimeSeries{}
	ts.AddData(tFromSec(0), 1)
	ts.AddData(tFromSec(10), NaV)
	ts.AddData(tFromSec(20), 3)
	err := ts.Interpolate(InterpolationMethod(999))
	if err == nil {
		t.Fatal("expected error on unknown method")
	}
	if !errors.Is(err, ErrUnknownOption) {
		t.Errorf("err = %v, want UnknownOption", err)
	}
}

// ---------------------------------------------------------------------------
// Nil-safety
// ---------------------------------------------------------------------------

func TestMerge_NilSafe(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 1)

	// Each of (nil,nil), (nil,&), (&,nil) must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Merge panicked on nil argument: %v", r)
		}
	}()

	got := Merge(nil, nil)
	if len(got.DataSeries) != 0 {
		t.Errorf("Merge(nil,nil) len = %d, want 0", len(got.DataSeries))
	}
	got = Merge(nil, &ts)
	if len(got.DataSeries) != 1 {
		t.Errorf("Merge(nil,&ts) len = %d, want 1", len(got.DataSeries))
	}
	got = Merge(&ts, nil)
	if len(got.DataSeries) != 1 {
		t.Errorf("Merge(&ts,nil) len = %d, want 1", len(got.DataSeries))
	}
}

func TestCopy_NilSafe(t *testing.T) {
	var nilPtr *TimeSeries
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Copy panicked on nil receiver: %v", r)
		}
	}()
	out := nilPtr.Copy()
	if len(out.DataSeries) != 0 {
		t.Errorf("Copy() on nil = %v, want empty", out.DataSeries)
	}
}

func TestSortDeltasStats_NilSafe(t *testing.T) {
	var nilPtr *TimeSeries
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SortDeltasStats panicked on nil receiver: %v", r)
		}
	}()
	nilPtr.SortDeltasStats()
}

// ---------------------------------------------------------------------------
// ApplyPolishing: nil guards + panic recovery + partial progress
// ---------------------------------------------------------------------------

func TestApplyPolishing_NilReceiver(t *testing.T) {
	var tsc *TsContainer
	rc := &RecipesCatalogueRow{Variant: "raw"}
	var ts TimeSeries
	err := tsc.ApplyPolishing(&ts, rc)
	if err == nil {
		t.Fatal("expected error on nil *TsContainer receiver")
	}
	if !errors.Is(err, ErrInvalidArg) {
		t.Errorf("err = %v, want InvalidArg", err)
	}
}

func TestApplyPolishing_NilTimeSeries(t *testing.T) {
	tsc := NewTsContainer()
	rc := &RecipesCatalogueRow{Variant: "raw"}
	err := (&tsc).ApplyPolishing(nil, rc)
	if err == nil {
		t.Fatal("expected error on nil ts")
	}
	if !errors.Is(err, ErrInvalidArg) {
		t.Errorf("err = %v, want InvalidArg", err)
	}
}

func TestApplyPolishing_NilRecipe(t *testing.T) {
	tsc := NewTsContainer()
	var ts TimeSeries
	err := (&tsc).ApplyPolishing(&ts, nil)
	if err == nil {
		t.Fatal("expected error on nil rc")
	}
	if !errors.Is(err, ErrInvalidArg) {
		t.Errorf("err = %v, want InvalidArg", err)
	}
}

func TestApplyPolishing_UnknownAggregatorIsReportedWithContext(t *testing.T) {
	tsc := NewTsContainer()
	ts := &TimeSeries{}
	ts.AddData(tFromSec(0), 1)
	ts.AddData(tFromSec(3600), 2)

	freq := int64(60)
	aggName := "no-such-agg"
	variant := "custom"
	rc := &RecipesCatalogueRow{
		Variant:     variant,
		FreqSeconds: &freq,
		Agg:         &aggName,
	}
	err := (&tsc).ApplyPolishing(ts, rc)
	if err == nil {
		t.Fatal("expected error on unknown aggregator")
	}
	if !errors.Is(err, ErrUnknownOption) {
		t.Errorf("err = %v, want UnknownOption", err)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if e.Op != "ApplyPolishing" {
		t.Errorf("Op = %q, want %q", e.Op, "ApplyPolishing")
	}
}

