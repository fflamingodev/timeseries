package timeseries

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// Interpolating fills the gaps of a series with values inferred from
// their neighbours. It is the one operation in this library that
// invents data, and it is worth staying aware of that: what comes out
// is a reading of what probably happened, not a measurement.
//
// # What gets filled, and what does not
//
//   - A gap (NaV) is filled: nothing was measured, and the neighbours
//     are all there is to go on.
//   - A broken value (a plain NaN) is left alone. It is not missing, it
//     is wrong, and covering it with a plausible number is how a bug
//     upstream stops being noticed.
//   - Gaps before the first reading and after the last are left as
//     they are. Every method here interpolates, none extrapolates: the
//     series says nothing about what happened outside its own span.
//
// # On time, not on rank
//
// The value of a gap is computed from where it sits in time, not from
// its position in the slice. On a regularized series the two agree; on
// a raw one they do not, and the difference matters — a gap one minute
// after its left neighbour and one hour before its right one should
// take almost the value of the left one. The previous version of this
// library worked on ranks and gave it the middle value.
//
// # Gaps that should not be filled at all
//
// No method knows how long an outage may reasonably be bridged. Half
// an hour of missing temperature can be drawn through; three days
// cannot, and a chart that hides the outage behind a smooth line is a
// lie told with a straight face. InterpolateWithin sets that limit.

// InterpolationMethod names a way of filling a gap.
type InterpolationMethod int

const (
	// InterpNone leaves every gap as it is.
	InterpNone InterpolationMethod = iota

	// InterpLinear draws a straight line between the neighbours, in
	// proportion to time. The default choice for a physical quantity
	// that varies smoothly.
	InterpLinear

	// InterpNearest copies the closer neighbour in time. Keeps the
	// filled value one that was actually measured, which suits a signal
	// that steps rather than glides.
	InterpNearest

	// InterpForwardFill holds the last known value until a new one
	// arrives. The natural reading of a setpoint, a state, a counter:
	// nothing was reported because nothing changed.
	InterpForwardFill

	// InterpBackwardFill takes the next known value. Mostly useful to
	// fill the beginning of a series that starts with silence.
	InterpBackwardFill

	// InterpLogLinear draws a straight line through the logarithms of
	// the neighbours, which is a constant growth rate rather than a
	// constant increment. For a quantity that compounds. Both
	// neighbours must be above zero; otherwise the gap stays a gap.
	InterpLogLinear

	// InterpCubicSpline fits a natural cubic spline through the known
	// readings. Smooth, but it overshoots around a sharp change: the
	// filled value can leave the range of its neighbours.
	InterpCubicSpline

	// InterpMonotoneSpline fits a monotone cubic spline, of the PCHIP
	// kind. Smooth like the cubic spline but without the overshoot: it
	// never invents a peak between two readings, which makes it the
	// safer of the two on measured data.
	InterpMonotoneSpline
)

// String returns the name of the method, the one Interpolation accepts.
func (m InterpolationMethod) String() string {
	switch m {
	case InterpNone:
		return "None"
	case InterpLinear:
		return "Linear"
	case InterpNearest:
		return "Nearest"
	case InterpForwardFill:
		return "ForwardFill"
	case InterpBackwardFill:
		return "BackwardFill"
	case InterpLogLinear:
		return "LogLinear"
	case InterpCubicSpline:
		return "CubicSpline"
	case InterpMonotoneSpline:
		return "MonotoneSpline"
	}
	return fmt.Sprintf("InterpolationMethod(%d)", int(m))
}

// known reports whether the method is one this package implements,
// which a method value cast from an integer may well not be.
func (m InterpolationMethod) known() bool {
	return m >= InterpNone && m <= InterpMonotoneSpline
}

// ErrInterpolationArg reports an unknown method or an impossible limit.
// Test for it with errors.Is.
var ErrInterpolationArg = errors.New("timeseries: impossible interpolation argument")

// interpolationsByName maps the names a recipe uses to the methods.
// Keys are lowercase with no separators; Interpolation folds the name
// the same way, so "monotone spline", "Monotone-Spline" and
// "MonotoneSpline" all arrive here as one.
var interpolationsByName = map[string]InterpolationMethod{
	"none":           InterpNone,
	"linear":         InterpLinear,
	"nearest":        InterpNearest,
	"forwardfill":    InterpForwardFill,
	"ffill":          InterpForwardFill,
	"hold":           InterpForwardFill,
	"backwardfill":   InterpBackwardFill,
	"bfill":          InterpBackwardFill,
	"loglinear":      InterpLogLinear,
	"cubicspline":    InterpCubicSpline,
	"spline":         InterpCubicSpline,
	"monotonespline": InterpMonotoneSpline,
	"pchip":          InterpMonotoneSpline,
}

// Interpolation returns the method a recipe names. Case, spaces,
// hyphens and underscores are ignored, so a name coming from a database
// column or a form does not have to match exactly.
//
// An unknown name is an error rather than a silent fallback: a recipe
// asking for a method the library does not have must say so, not
// quietly draw a straight line.
func Interpolation(name string) (InterpolationMethod, error) {
	folded := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '_':
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(name)))

	method, ok := interpolationsByName[folded]
	if !ok {
		return InterpNone, fmt.Errorf("%w: unknown interpolation %q, known ones are %s",
			ErrInterpolationArg, name, strings.Join(InterpolationNames(), ", "))
	}
	return method, nil
}

// InterpolationNames returns the names Interpolation accepts, sorted.
func InterpolationNames() []string {
	names := make([]string, 0, len(interpolationsByName))
	for name := range interpolationsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Interpolate returns a copy of the series with its gaps filled by the
// given method, without any limit on the length of a gap.
//
//	filled, err := ts.Interpolate(timeseries.InterpLinear)
func (ts *TimeSeries) Interpolate(method InterpolationMethod) (*TimeSeries, error) {
	return ts.InterpolateWithin(method, 0)
}

// InterpolateWithin fills only the gaps that can be bridged in at most
// maxGap of elapsed time, and leaves the longer ones alone.
//
// The limit is measured between the two readings surrounding the gap —
// the last one before and the first one after — and not between grid
// steps, so it means the same thing on a regular series and on a raw
// one. A maxGap of zero, as Interpolate passes, means no limit.
//
//	// Draw through a quarter of an hour of silence, not through an outage.
//	filled, err := ts.InterpolateWithin(timeseries.InterpLinear, 15*time.Minute)
//
// For ForwardFill and BackwardFill, which lean on one neighbour only,
// the limit is the distance to that neighbour.
func (ts *TimeSeries) InterpolateWithin(method InterpolationMethod, maxGap time.Duration) (*TimeSeries, error) {
	if maxGap < 0 {
		return nil, fmt.Errorf("%w: the gap limit cannot be negative, got %v",
			ErrInterpolationArg, maxGap)
	}
	if !method.known() {
		return nil, fmt.Errorf("%w: unknown interpolation method %d", ErrInterpolationArg, int(method))
	}

	out := NewTimeSeries(ts.Name + " interpolated")
	out.ID = ts.ID
	out.Comment = "gaps filled: " + method.String()
	if maxGap > 0 {
		out.Comment += fmt.Sprintf(", up to %v", maxGap)
	}

	n := ts.Len()
	if n == 0 || method == InterpNone {
		filled := make([]Datum, 0, n)
		ts.Range(func(_ int, du DataUnit) bool {
			filled = append(filled, du.Datum)
			return true
		})
		out.AddBatchData(filled)
		return out, nil
	}

	// The readings to lean on, and where they sit in time. Seconds from
	// the first point keep the numbers small enough for the spline
	// arithmetic to stay exact.
	base := ts.At(0).Chron
	seconds := func(t time.Time) float64 { return t.Sub(base).Seconds() }

	var anchorX, anchorY []float64
	var anchorIdx []int
	ts.Range(func(i int, du DataUnit) bool {
		if !math.IsNaN(du.Meas) {
			anchorX = append(anchorX, seconds(du.Chron))
			anchorY = append(anchorY, du.Meas)
			anchorIdx = append(anchorIdx, i)
		}
		return true
	})

	filled := make([]Datum, 0, n)
	ts.Range(func(_ int, du DataUnit) bool {
		filled = append(filled, du.Datum)
		return true
	})

	// Nothing to lean on, or nothing to fill.
	if len(anchorX) > 0 {
		var curve func(x float64) float64
		switch method {
		case InterpCubicSpline:
			curve = naturalCubicSpline(anchorX, anchorY)
		case InterpMonotoneSpline:
			curve = monotoneCubicSpline(anchorX, anchorY)
		}

		prev, next := neighbourAnchors(ts, anchorIdx)
		for i := range filled {
			if !nav.IsNaV(filled[i].Meas) {
				continue // a reading, or a broken value to be left alone
			}
			p, q := prev[i], next[i]
			x := seconds(filled[i].Chron)
			filled[i].Meas = fillOne(method, curve,
				anchorX, anchorY, p, q, x, maxGap)
		}
	}

	out.AddBatchData(filled)
	return out, nil
}

// neighbourAnchors returns, for every point of the series, the rank in
// the anchor slices of the nearest reading before it and after it, or
// -1 when there is none on that side.
func neighbourAnchors(ts *TimeSeries, anchorIdx []int) (prev, next []int) {
	n := ts.Len()
	prev = make([]int, n)
	next = make([]int, n)

	rank := -1
	for i, j := 0, 0; i < n; i++ {
		if j < len(anchorIdx) && anchorIdx[j] == i {
			rank = j
			j++
		}
		prev[i] = rank
	}

	rank = -1
	for i, j := n-1, len(anchorIdx)-1; i >= 0; i-- {
		if j >= 0 && anchorIdx[j] == i {
			rank = j
			j--
		}
		next[i] = rank
	}
	return prev, next
}

// fillOne computes the value of a single gap sitting at x seconds, with
// p and q the ranks of its surrounding readings. It returns NaV
// whenever the method has nothing to work with, or when the gap is
// wider than the caller allows.
func fillOne(method InterpolationMethod, curve func(float64) float64,
	anchorX, anchorY []float64, p, q int, x float64, maxGap time.Duration) float64 {

	limit := maxGap.Seconds()
	within := func(span float64) bool { return maxGap == 0 || span <= limit }

	switch method {
	case InterpForwardFill:
		if p < 0 || !within(x-anchorX[p]) {
			return nav.NaV
		}
		return anchorY[p]

	case InterpBackwardFill:
		if q < 0 || !within(anchorX[q]-x) {
			return nav.NaV
		}
		return anchorY[q]
	}

	// Every other method needs a reading on each side: no extrapolation.
	if p < 0 || q < 0 || p == q {
		return nav.NaV
	}
	if !within(anchorX[q] - anchorX[p]) {
		return nav.NaV
	}

	switch method {
	case InterpNearest:
		if x-anchorX[p] <= anchorX[q]-x {
			return anchorY[p]
		}
		return anchorY[q]

	case InterpLinear:
		t := (x - anchorX[p]) / (anchorX[q] - anchorX[p])
		return anchorY[p] + t*(anchorY[q]-anchorY[p])

	case InterpLogLinear:
		// A constant growth rate only means something between two
		// positive readings.
		if anchorY[p] <= 0 || anchorY[q] <= 0 {
			return nav.NaV
		}
		t := (x - anchorX[p]) / (anchorX[q] - anchorX[p])
		return math.Exp(math.Log(anchorY[p]) + t*(math.Log(anchorY[q])-math.Log(anchorY[p])))

	case InterpCubicSpline, InterpMonotoneSpline:
		if curve == nil {
			return nav.NaV
		}
		return curve(x)
	}
	return nav.NaV
}

// -----------------------------------------------------------------------
// The two splines
// -----------------------------------------------------------------------

// naturalCubicSpline returns a function evaluating the natural cubic
// spline through the given points — the curve of least total curvature
// passing through all of them, with a straight second derivative of
// zero at both ends.
//
// It returns nil when there are fewer than three points, where the
// spline degenerates into the straight line the linear method already
// draws.
func naturalCubicSpline(xs, ys []float64) func(float64) float64 {
	n := len(xs)
	if n < 3 {
		return nil
	}

	h := make([]float64, n-1)
	for i := 0; i < n-1; i++ {
		h[i] = xs[i+1] - xs[i]
		if h[i] <= 0 {
			return nil // readings must be strictly ordered in time
		}
	}

	// Thomas algorithm on the tridiagonal system of the second
	// derivatives.
	alpha := make([]float64, n)
	for i := 1; i < n-1; i++ {
		alpha[i] = 3*(ys[i+1]-ys[i])/h[i] - 3*(ys[i]-ys[i-1])/h[i-1]
	}

	l := make([]float64, n)
	mu := make([]float64, n)
	z := make([]float64, n)
	l[0] = 1
	for i := 1; i < n-1; i++ {
		l[i] = 2*(xs[i+1]-xs[i-1]) - h[i-1]*mu[i-1]
		if l[i] == 0 {
			return nil
		}
		mu[i] = h[i] / l[i]
		z[i] = (alpha[i] - h[i-1]*z[i-1]) / l[i]
	}
	l[n-1] = 1

	b := make([]float64, n)
	c := make([]float64, n)
	d := make([]float64, n)
	for j := n - 2; j >= 0; j-- {
		c[j] = z[j] - mu[j]*c[j+1]
		b[j] = (ys[j+1]-ys[j])/h[j] - h[j]*(c[j+1]+2*c[j])/3
		d[j] = (c[j+1] - c[j]) / (3 * h[j])
	}

	return func(x float64) float64 {
		if x < xs[0] || x > xs[n-1] {
			return nav.NaV // no extrapolation
		}
		j := segmentOf(xs, x)
		dx := x - xs[j]
		return ys[j] + b[j]*dx + c[j]*dx*dx + d[j]*dx*dx*dx
	}
}

// monotoneCubicSpline returns a function evaluating a PCHIP spline
// through the given points: cubic between readings, like the natural
// spline, but with the slopes chosen by the Fritsch-Carlson rule so
// that the curve never rises between two falling readings, nor falls
// between two rising ones.
//
// That is the difference that matters on measured data: a natural
// spline asked to pass smoothly through a step will overshoot and
// invent a peak that no instrument saw.
func monotoneCubicSpline(xs, ys []float64) func(float64) float64 {
	n := len(xs)
	if n < 3 {
		return nil
	}

	h := make([]float64, n-1)
	secant := make([]float64, n-1)
	for i := 0; i < n-1; i++ {
		h[i] = xs[i+1] - xs[i]
		if h[i] <= 0 {
			return nil
		}
		secant[i] = (ys[i+1] - ys[i]) / h[i]
	}

	slope := make([]float64, n)
	for i := 1; i < n-1; i++ {
		if secant[i-1]*secant[i] <= 0 {
			// A turning point: a flat slope is what keeps the curve from
			// overshooting on either side.
			slope[i] = 0
			continue
		}
		w1, w2 := 2*h[i]+h[i-1], h[i]+2*h[i-1]
		slope[i] = (w1 + w2) / (w1/secant[i-1] + w2/secant[i])
	}
	// The ends follow the secant of their own segment: no reading
	// beyond them says anything about where the curve should be going.
	slope[0] = secant[0]
	slope[n-1] = secant[n-2]

	return func(x float64) float64 {
		if x < xs[0] || x > xs[n-1] {
			return nav.NaV
		}
		j := segmentOf(xs, x)
		t := (x - xs[j]) / h[j]

		// Hermite basis on the segment.
		t2, t3 := t*t, t*t*t
		return ys[j]*(2*t3-3*t2+1) +
			h[j]*slope[j]*(t3-2*t2+t) +
			ys[j+1]*(-2*t3+3*t2) +
			h[j]*slope[j+1]*(t3-t2)
	}
}

// segmentOf returns the index j such that xs[j] <= x <= xs[j+1].
func segmentOf(xs []float64, x float64) int {
	j := sort.SearchFloat64s(xs, x)
	if j > 0 {
		j--
	}
	if j > len(xs)-2 {
		j = len(xs) - 2
	}
	return j
}
