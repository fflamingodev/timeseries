package timeseries

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// interpolated runs a method over a series built at successive hours
// and returns the measurements that came out.
func interpolated(t *testing.T, method InterpolationMethod, meas ...float64) []float64 {
	t.Helper()
	out, err := seriesOf(meas...).Interpolate(method)
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", method, err)
	}
	checkInvariant(t, out)
	return out.Meas()
}

// closeEnough compares two measurements with the tolerance the spline
// arithmetic calls for.
func closeEnough(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

// -----------------------------------------------------------------------
// Where the gap sits in time
// -----------------------------------------------------------------------

// The correction that matters most: a gap is filled from its place in
// time, not from its rank in the slice.
func TestInterpolateUsesTimeNotRank(t *testing.T) {
	// A reading of 10, a gap one minute later, and a reading of 20
	// fifty-nine minutes after that.
	ts := NewTimeSeries("irregular")
	ts.AddBatchData([]Datum{
		NewDatum(atMinute(0), 10),
		NewDatum(atMinute(1), nav.NaV),
		NewDatum(atMinute(60), 20),
	})

	out, err := ts.Interpolate(InterpLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// One sixtieth of the way from 10 to 20.
	if want := 10 + 10.0/60.0; !closeEnough(out.At(1).Meas, want) {
		t.Errorf("the gap was filled with %s, want %v", nav.Format(out.At(1).Meas), want)
	}
	// Counting in ranks, as the previous version did, would have put the
	// gap halfway and answered 15.
	if closeEnough(out.At(1).Meas, 15) {
		t.Error("the gap was filled by rank: it sits one minute from its left neighbour, not halfway")
	}

	// Nearest reads the same clock.
	out, err = ts.Interpolate(InterpNearest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.At(1).Meas != 10 {
		t.Errorf("Nearest chose %s, want the reading one minute away", nav.Format(out.At(1).Meas))
	}
}

// -----------------------------------------------------------------------
// One method at a time
// -----------------------------------------------------------------------

func TestInterpolateMethods(t *testing.T) {
	// A gap in the middle, with a reading on each side.
	cases := []struct {
		method InterpolationMethod
		want   float64
	}{
		{InterpLinear, 15},
		{InterpNearest, 10},      // equidistant: the earlier one wins
		{InterpForwardFill, 10},  // hold the last known value
		{InterpBackwardFill, 20}, // take the next one
		{InterpLogLinear, math.Sqrt(200)},
	}
	for _, c := range cases {
		got := interpolated(t, c.method, 10, nav.NaV, 20)
		if !closeEnough(got[1], c.want) {
			t.Errorf("%s filled the gap with %s, want %v", c.method, nav.Format(got[1]), c.want)
		}
		// The readings themselves are never touched.
		if got[0] != 10 || got[2] != 20 {
			t.Errorf("%s changed the readings: %v", c.method, got)
		}
	}
}

func TestInterpolateNone(t *testing.T) {
	got := interpolated(t, InterpNone, 10, nav.NaV, 20)
	if !nav.IsNaV(got[1]) {
		t.Errorf("InterpNone filled the gap with %s", nav.Format(got[1]))
	}
}

// Log-linear means a constant growth rate, which only exists between
// two positive readings.
func TestLogLinearNeedsPositiveReadings(t *testing.T) {
	for _, meas := range [][]float64{
		{0, nav.NaV, 20},
		{-5, nav.NaV, 20},
		{10, nav.NaV, 0},
	} {
		got := interpolated(t, InterpLogLinear, meas...)
		if !nav.IsNaV(got[1]) {
			t.Errorf("on %v: filled with %s, want NaV — a growth rate needs positive readings",
				meas, nav.Format(got[1]))
		}
	}
}

// -----------------------------------------------------------------------
// The edges
// -----------------------------------------------------------------------

// Nothing extrapolates: a gap with no reading on one side stays a gap,
// except for the two fills, which lean on one neighbour by design.
func TestInterpolateDoesNotExtrapolate(t *testing.T) {
	for _, method := range []InterpolationMethod{
		InterpLinear, InterpNearest, InterpLogLinear,
		InterpCubicSpline, InterpMonotoneSpline,
	} {
		got := interpolated(t, method, nav.NaV, 10, 20, 30, nav.NaV)
		if !nav.IsNaV(got[0]) {
			t.Errorf("%s invented a value before the first reading: %s", method, nav.Format(got[0]))
		}
		if !nav.IsNaV(got[4]) {
			t.Errorf("%s invented a value after the last reading: %s", method, nav.Format(got[4]))
		}
	}

	// ForwardFill holds the last value to the end, and says nothing
	// about the beginning; BackwardFill is its mirror.
	got := interpolated(t, InterpForwardFill, nav.NaV, 10, 20, nav.NaV)
	if !nav.IsNaV(got[0]) || got[3] != 20 {
		t.Errorf("ForwardFill gave %v, want a gap at the start and 20 at the end", got)
	}
	got = interpolated(t, InterpBackwardFill, nav.NaV, 10, 20, nav.NaV)
	if got[0] != 10 || !nav.IsNaV(got[3]) {
		t.Errorf("BackwardFill gave %v, want 10 at the start and a gap at the end", got)
	}
}

// -----------------------------------------------------------------------
// Errors are not gaps
// -----------------------------------------------------------------------

// A broken value is not filled. Covering it with a plausible number is
// how a bug upstream stops being noticed.
func TestInterpolateLeavesBrokenValuesAlone(t *testing.T) {
	for _, method := range []InterpolationMethod{
		InterpLinear, InterpNearest, InterpForwardFill, InterpBackwardFill,
		InterpLogLinear, InterpCubicSpline, InterpMonotoneSpline,
	} {
		got := interpolated(t, method, 10, math.NaN(), 20, 30)
		if !nav.IsStdNaN(got[1]) {
			t.Errorf("%s replaced a broken value with %s", method, nav.Format(got[1]))
		}
	}
}

// -----------------------------------------------------------------------
// The limit on a gap
// -----------------------------------------------------------------------

func TestInterpolateWithin(t *testing.T) {
	// Readings every hour, with one gap of an hour and one of three.
	ts := NewTimeSeries("outage")
	ts.AddBatchData([]Datum{
		NewDatum(at(0), 10),
		NewDatum(at(1), nav.NaV), // bridged by an hour
		NewDatum(at(2), 20),
		NewDatum(at(3), nav.NaV), // an outage of three hours
		NewDatum(at(4), nav.NaV),
		NewDatum(at(5), 30),
	})

	out, err := ts.InterpolateWithin(InterpLinear, 2*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.At(1).Meas != 15 {
		t.Errorf("the short gap was left at %s, want 15", nav.Format(out.At(1).Meas))
	}
	for _, i := range []int{3, 4} {
		if !nav.IsNaV(out.At(i).Meas) {
			t.Errorf("the outage was drawn through at point %d: %s — "+
				"a chart hiding it behind a smooth line would be a lie",
				i, nav.Format(out.At(i).Meas))
		}
	}

	// Without a limit, the same series is filled throughout.
	out, err = ts.Interpolate(InterpLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nav.IsNaV(out.At(3).Meas) || nav.IsNaV(out.At(4).Meas) {
		t.Error("without a limit, every gap should have been filled")
	}
}

// The two fills measure the limit against the one neighbour they use.
func TestInterpolateWithinOnTheFills(t *testing.T) {
	ts := NewTimeSeries("held")
	ts.AddBatchData([]Datum{
		NewDatum(at(0), 10),
		NewDatum(at(1), nav.NaV), // one hour after the reading
		NewDatum(at(5), nav.NaV), // five hours after it
	})

	out, err := ts.InterpolateWithin(InterpForwardFill, 2*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.At(1).Meas != 10 {
		t.Errorf("the value was not held one hour on: %s", nav.Format(out.At(1).Meas))
	}
	if !nav.IsNaV(out.At(2).Meas) {
		t.Errorf("the value was held five hours on: %s — beyond the limit given",
			nav.Format(out.At(2).Meas))
	}
}

// -----------------------------------------------------------------------
// The two splines
// -----------------------------------------------------------------------

// The reason the monotone spline exists: through a step, the natural
// spline overshoots and invents a reading no instrument saw.
func TestSplineOvershoot(t *testing.T) {
	// A step from 0 to 10, with two gaps just after it.
	meas := []float64{0, 0, 0, 0, 10, nav.NaV, nav.NaV, 10, 10, 10}

	natural := interpolated(t, InterpCubicSpline, meas...)
	monotone := interpolated(t, InterpMonotoneSpline, meas...)

	if natural[5] <= 10 {
		t.Errorf("the natural spline filled the gap with %s; it is expected to overshoot above 10 here",
			nav.Format(natural[5]))
	}
	for _, i := range []int{5, 6} {
		if monotone[i] > 10.000001 || monotone[i] < 0 {
			t.Errorf("the monotone spline left the range of its neighbours at point %d: %s",
				i, nav.Format(monotone[i]))
		}
	}
}

// On a straight line, both splines agree with the linear method: a
// smooth curve through aligned points is that line.
func TestSplinesOnAStraightLine(t *testing.T) {
	meas := []float64{0, 2, 4, nav.NaV, 8, 10}

	for _, method := range []InterpolationMethod{InterpCubicSpline, InterpMonotoneSpline} {
		got := interpolated(t, method, meas...)
		if !closeEnough(got[3], 6) {
			t.Errorf("%s filled the gap with %s, want 6", method, nav.Format(got[3]))
		}
	}
}

// With fewer than three readings there is no curve to fit, and the
// splines have nothing to say.
func TestSplinesNeedThreeReadings(t *testing.T) {
	for _, method := range []InterpolationMethod{InterpCubicSpline, InterpMonotoneSpline} {
		got := interpolated(t, method, 10, nav.NaV, 20)
		if !nav.IsNaV(got[1]) {
			t.Errorf("%s filled a gap between two lone readings with %s",
				method, nav.Format(got[1]))
		}
	}
}

// -----------------------------------------------------------------------
// Series that give nothing to lean on
// -----------------------------------------------------------------------

func TestInterpolateEdgeSeries(t *testing.T) {
	// Nothing at all.
	out, err := NewTimeSeries("empty").Interpolate(InterpLinear)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("%d points came out of an empty series", out.Len())
	}

	// Nothing but gaps: no reading to lean on, so they stay.
	got := interpolated(t, InterpLinear, nav.NaV, nav.NaV, nav.NaV)
	for i, v := range got {
		if !nav.IsNaV(v) {
			t.Errorf("point %d was filled with %s although the series holds no reading",
				i, nav.Format(v))
		}
	}

	// A series without a single gap comes out unchanged.
	got = interpolated(t, InterpLinear, 1, 2, 3)
	for i, want := range []float64{1, 2, 3} {
		if got[i] != want {
			t.Errorf("point %d = %s, want %v", i, nav.Format(got[i]), want)
		}
	}
}

// The interpolated series is a series like any other, and says how it
// was filled.
func TestInterpolateProducesAProperSeries(t *testing.T) {
	ts := seriesOf(10, nav.NaV, 20)
	ts.ID = "device-42"

	out, err := ts.InterpolateWithin(InterpLinear, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checkInvariant(t, out)
	if out.ID != "device-42" {
		t.Error("the interpolated series lost the identity of its source")
	}
	if !strings.Contains(out.Comment, "Linear") {
		t.Errorf("the comment does not say how the gaps were filled: %q", out.Comment)
	}
	// The source is untouched: interpolation returns a new series.
	if !nav.IsNaV(ts.At(1).Meas) {
		t.Error("the source series was modified")
	}
}

// -----------------------------------------------------------------------
// Naming
// -----------------------------------------------------------------------

func TestInterpolationByName(t *testing.T) {
	cases := []struct {
		names []string
		want  InterpolationMethod
	}{
		{[]string{"linear", "Linear", " LINEAR "}, InterpLinear},
		{[]string{"nearest"}, InterpNearest},
		{[]string{"forwardfill", "ForwardFill", "forward_fill", "forward-fill", "ffill", "hold"}, InterpForwardFill},
		{[]string{"backwardfill", "bfill"}, InterpBackwardFill},
		{[]string{"loglinear", "Log Linear"}, InterpLogLinear},
		{[]string{"cubicspline", "spline", "Cubic Spline"}, InterpCubicSpline},
		{[]string{"monotonespline", "Monotone-Spline", "pchip"}, InterpMonotoneSpline},
		{[]string{"none"}, InterpNone},
	}
	for _, c := range cases {
		for _, name := range c.names {
			got, err := Interpolation(name)
			if err != nil {
				t.Errorf("Interpolation(%q): %v", name, err)
				continue
			}
			if got != c.want {
				t.Errorf("Interpolation(%q) = %s, want %s", name, got, c.want)
			}
		}
	}
}

func TestInterpolationRejectsAnUnknownName(t *testing.T) {
	_, err := Interpolation("kriging")
	if !errors.Is(err, ErrInterpolationArg) {
		t.Errorf("error is %v, want an ErrInterpolationArg", err)
	}
	if !strings.Contains(err.Error(), "linear") {
		t.Errorf("the error does not say what is available: %v", err)
	}
}

func TestInterpolateRejectsImpossibleArguments(t *testing.T) {
	ts := seriesOf(1, nav.NaV, 3)

	if _, err := ts.InterpolateWithin(InterpLinear, -time.Hour); !errors.Is(err, ErrInterpolationArg) {
		t.Errorf("a negative limit gave %v, want an ErrInterpolationArg", err)
	}
	if _, err := ts.Interpolate(InterpolationMethod(99)); !errors.Is(err, ErrInterpolationArg) {
		t.Errorf("an unknown method gave %v, want an ErrInterpolationArg", err)
	}
}

func TestInterpolationNames(t *testing.T) {
	names := InterpolationNames()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("the names are not sorted: %v", names)
			break
		}
	}
	for _, name := range names {
		if _, err := Interpolation(name); err != nil {
			t.Errorf("Interpolation(%q) is advertised but fails: %v", name, err)
		}
	}
}

// Every method prints as the name Interpolation accepts, which is what
// a recipe stores and a message shows.
func TestInterpolationMethodString(t *testing.T) {
	for _, method := range []InterpolationMethod{
		InterpNone, InterpLinear, InterpNearest, InterpForwardFill,
		InterpBackwardFill, InterpLogLinear, InterpCubicSpline, InterpMonotoneSpline,
	} {
		back, err := Interpolation(method.String())
		if err != nil {
			t.Errorf("%s does not read back: %v", method, err)
			continue
		}
		if back != method {
			t.Errorf("%s read back as %s", method, back)
		}
	}
	if got := InterpolationMethod(42).String(); !strings.Contains(got, "42") {
		t.Errorf("an unknown method prints as %q, which hides which one it was", got)
	}
}
