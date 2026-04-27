package timeseries

import (
	"fmt"
	"math"
	"sort"
)

// InterpolationMethod enumerates the strategies supported by the
// TimeSeries.Interpolate dispatcher.
type InterpolationMethod int

const (
	InterpNone           InterpolationMethod = iota
	InterpLinear                             // linear interpolation between valid neighbors
	InterpNearest                            // value of the nearest valid neighbor
	InterpForwardFill                        // propagate the last known value forward
	InterpBackwardFill                       // propagate the first known value backward
	InterpLogLinear                          // linear interpolation in log space
	InterpCubicSpline                        // natural cubic spline
	InterpMonotoneSpline                     // monotone cubic spline (PCHIP flavor)
)

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
	default:
		return fmt.Sprintf("InterpolationMethod(%d)", int(m))
	}
}

// Interpolate walks the TimeSeries and replaces the NaN-class Meas
// values according to the chosen method. The series is modified in
// place; Dmeas becomes stale (Chron/Dchron are untouched), so
// DeltasValid is cleared and callers should call SortDeltasStats to
// refresh deltas and stats.
//
// InterpNone is a no-op and returns nil. Any unrecognized method value
// returns an *Error with Kind == KindUnknownOption and leaves the
// series untouched.
func (ts *TimeSeries) Interpolate(method InterpolationMethod) error {
	switch method {
	case InterpNone:
		return nil
	case InterpLinear:
		interpolateLinear(ts)
	case InterpNearest:
		interpolateNearest(ts)
	case InterpForwardFill:
		interpolateForwardFill(ts)
	case InterpBackwardFill:
		interpolateBackwardFill(ts)
	case InterpLogLinear:
		interpolateLogLinear(ts)
	case InterpCubicSpline:
		interpolateCubicSpline(ts)
	case InterpMonotoneSpline:
		interpolateMonotoneSpline(ts)
	default:
		return &Error{
			Op:    "Interpolate",
			Kind:  KindUnknownOption,
			Msg:   "unknown interpolation method",
			Field: "method",
			Value: method,
		}
	}
	ts.invalidateDeltas()
	return nil
}

// ----------------------------------------------------------------------
// Generic helpers
// ----------------------------------------------------------------------

// isValid reports whether x is a non-NaN, non-NaV measurement.
func isValid(x float64) bool {
	return !math.IsNaN(x)
}

// computeNeighbors returns, for each index i,
//   - prev[i] = index of the last valid point at position <= i, or -1
//   - next[i] = index of the next valid point at position >= i, or -1
func computeNeighbors(ts *TimeSeries) (prev []int, next []int) {
	n := len(ts.DataSeries)
	prev = make([]int, n)
	next = make([]int, n)

	last := -1
	for i := 0; i < n; i++ {
		if isValid(ts.DataSeries[i].Meas) {
			last = i
		}
		prev[i] = last
	}

	last = -1
	for i := n - 1; i >= 0; i-- {
		if isValid(ts.DataSeries[i].Meas) {
			last = i
		}
		next[i] = last
	}

	return prev, next
}

// ----------------------------------------------------------------------
// 1) Linear interpolation
// ----------------------------------------------------------------------

func interpolateLinear(ts *TimeSeries) {
	n := len(ts.DataSeries)
	prev, next := computeNeighbors(ts)

	for i := 0; i < n; i++ {
		yi := ts.DataSeries[i].Meas
		if isValid(yi) {
			continue
		}

		pi := prev[i]
		ni := next[i]

		if pi == -1 || ni == -1 || pi == ni {
			// Not enough information to interpolate; leave NaN in place.
			continue
		}

		y0 := ts.DataSeries[pi].Meas
		y1 := ts.DataSeries[ni].Meas

		t := float64(i-pi) / float64(ni-pi)
		ts.DataSeries[i].Meas = y0 + t*(y1-y0)
	}
}

// ----------------------------------------------------------------------
// 2) Nearest neighbor
// ----------------------------------------------------------------------

func interpolateNearest(ts *TimeSeries) {
	n := len(ts.DataSeries)
	prev, next := computeNeighbors(ts)

	for i := 0; i < n; i++ {
		yi := ts.DataSeries[i].Meas
		if isValid(yi) {
			continue
		}

		pi := prev[i]
		ni := next[i]

		switch {
		case pi == -1 && ni == -1:
			continue
		case pi == -1:
			ts.DataSeries[i].Meas = ts.DataSeries[ni].Meas
		case ni == -1:
			ts.DataSeries[i].Meas = ts.DataSeries[pi].Meas
		default:
			leftDist := i - pi
			rightDist := ni - i
			if leftDist <= rightDist {
				ts.DataSeries[i].Meas = ts.DataSeries[pi].Meas
			} else {
				ts.DataSeries[i].Meas = ts.DataSeries[ni].Meas
			}
		}
	}
}

// ----------------------------------------------------------------------
// 3) Forward fill
// ----------------------------------------------------------------------

func interpolateForwardFill(ts *TimeSeries) {
	n := len(ts.DataSeries)
	var lastVal float64
	hasLast := false

	for i := 0; i < n; i++ {
		y := ts.DataSeries[i].Meas
		if isValid(y) {
			lastVal = y
			hasLast = true
			continue
		}
		if hasLast {
			ts.DataSeries[i].Meas = lastVal
		}
	}
}

// ----------------------------------------------------------------------
// 4) Backward fill
// ----------------------------------------------------------------------

func interpolateBackwardFill(ts *TimeSeries) {
	n := len(ts.DataSeries)
	var nextVal float64
	hasNext := false

	for i := n - 1; i >= 0; i-- {
		y := ts.DataSeries[i].Meas
		if isValid(y) {
			nextVal = y
			hasNext = true
			continue
		}
		if hasNext {
			ts.DataSeries[i].Meas = nextVal
		}
	}
}

// ----------------------------------------------------------------------
// 5) Log-linear interpolation
// ----------------------------------------------------------------------

func interpolateLogLinear(ts *TimeSeries) {
	n := len(ts.DataSeries)
	prev, next := computeNeighbors(ts)

	for i := 0; i < n; i++ {
		yi := ts.DataSeries[i].Meas
		if isValid(yi) {
			continue
		}

		pi := prev[i]
		ni := next[i]
		if pi == -1 || ni == -1 || pi == ni {
			continue
		}

		y0 := ts.DataSeries[pi].Meas
		y1 := ts.DataSeries[ni].Meas

		if y0 <= 0 || y1 <= 0 {
			continue
		}

		logY0 := math.Log(y0)
		logY1 := math.Log(y1)

		t := float64(i-pi) / float64(ni-pi)
		logYi := logY0 + t*(logY1-logY0)
		ts.DataSeries[i].Meas = math.Exp(logYi)
	}
}

// ----------------------------------------------------------------------
// 6) Natural cubic spline
// ----------------------------------------------------------------------

// interpolateCubicSpline builds a natural cubic spline over the valid
// points (index -> value) and evaluates it at the NaN indices.
// Extrapolation is disabled: indices outside [firstValid, lastValid]
// are left as NaN.
func interpolateCubicSpline(ts *TimeSeries) {
	nTot := len(ts.DataSeries)
	if nTot == 0 {
		return
	}

	// 1) Collect valid points.
	var xs []float64
	var ys []float64

	for i, du := range ts.DataSeries {
		if isValid(du.Meas) {
			xs = append(xs, float64(i))
			ys = append(ys, du.Meas)
		}
	}

	m := len(xs)
	if m < 2 {
		// Not enough points to interpolate.
		return
	}
	if m == 2 {
		// With only 2 points, spline degenerates to linear.
		interpolateLinear(ts)
		return
	}

	// 2) Build the coefficients of the natural cubic spline:
	//    S_j(x) = a_j + b_j*(x-x_j) + c_j*(x-x_j)^2 + d_j*(x-x_j)^3 on
	//    the segment [x_j, x_{j+1}].
	a := make([]float64, m)
	b := make([]float64, m)
	c := make([]float64, m)
	d := make([]float64, m)

	copy(a, ys)

	h := make([]float64, m-1)
	for i := 0; i < m-1; i++ {
		h[i] = xs[i+1] - xs[i]
		if h[i] <= 0 {
			// xs must be strictly increasing.
			return
		}
	}

	alpha := make([]float64, m)
	for i := 1; i < m-1; i++ {
		alpha[i] = 3.0 * ((a[i+1]-a[i])/h[i] - (a[i]-a[i-1])/h[i-1])
	}

	l := make([]float64, m)
	mu := make([]float64, m)
	z := make([]float64, m)

	l[0] = 1.0
	mu[0] = 0.0
	z[0] = 0.0

	for i := 1; i < m-1; i++ {
		l[i] = 2.0*(xs[i+1]-xs[i-1]) - h[i-1]*mu[i-1]
		if l[i] == 0 {
			// Numerical safety: degenerate system, bail out.
			return
		}
		mu[i] = h[i] / l[i]
		z[i] = (alpha[i] - h[i-1]*z[i-1]) / l[i]
	}

	l[m-1] = 1.0
	z[m-1] = 0.0
	c[m-1] = 0.0

	for j := m - 2; j >= 0; j-- {
		c[j] = z[j] - mu[j]*c[j+1]
		b[j] = (a[j+1]-a[j])/h[j] - h[j]*(c[j+1]+2.0*c[j])/3.0
		d[j] = (c[j+1] - c[j]) / (3.0 * h[j])
	}

	// 3) Evaluate the spline at NaN positions.
	firstX := xs[0]
	lastX := xs[m-1]

	for i := 0; i < nTot; i++ {
		yi := ts.DataSeries[i].Meas
		if isValid(yi) {
			continue
		}

		x := float64(i)
		if x < firstX || x > lastX {
			// No extrapolation.
			continue
		}

		// Find the segment j such that xs[j] <= x <= xs[j+1].
		j := sort.Search(len(xs), func(k int) bool {
			return xs[k] >= x
		})
		if j == 0 {
			j = 0
		} else {
			j = j - 1
		}
		if j < 0 {
			j = 0
		}
		if j >= m-1 {
			j = m - 2
		}

		dx := x - xs[j]
		ts.DataSeries[i].Meas = a[j] + b[j]*dx + c[j]*dx*dx + d[j]*dx*dx*dx
	}
}

// ----------------------------------------------------------------------
// 7) Monotone cubic spline (PCHIP / MonotoneSpline)
// ----------------------------------------------------------------------

// interpolateMonotoneSpline builds a PCHIP (monotone cubic Hermite)
// spline. It guarantees no oscillations that would break local
// monotonicity when the input secant slopes are monotone.
func interpolateMonotoneSpline(ts *TimeSeries) {
	nTot := len(ts.DataSeries)
	if nTot == 0 {
		return
	}

	// 1) Collect valid points.
	var xs []float64
	var ys []float64

	for i, du := range ts.DataSeries {
		if isValid(du.Meas) {
			xs = append(xs, float64(i))
			ys = append(ys, du.Meas)
		}
	}

	n := len(xs)
	if n < 2 {
		return
	}
	if n == 2 {
		// With 2 points, monotone spline degenerates to linear.
		interpolateLinear(ts)
		return
	}

	// 2) h[i] = xs[i+1] - xs[i], delta[i] = secant slope.
	h := make([]float64, n-1)
	delta := make([]float64, n-1)

	for i := 0; i < n-1; i++ {
		h[i] = xs[i+1] - xs[i]
		if h[i] <= 0 {
			return // xs must be strictly increasing.
		}
		delta[i] = (ys[i+1] - ys[i]) / h[i]
	}

	// 3) Compute slopes m[i] (Fritsch-Carlson).
	m := make([]float64, n)

	// Interior points.
	for i := 1; i < n-1; i++ {
		if delta[i-1]*delta[i] <= 0 {
			m[i] = 0.0
		} else {
			w1 := 2.0*h[i] + h[i-1]
			w2 := h[i] + 2.0*h[i-1]
			m[i] = (w1 + w2) / (w1/delta[i-1] + w2/delta[i])
		}
	}

	// Left endpoint.
	m[0] = delta[0]
	if delta[0] != 0.0 {
		if math.Signbit(m[0]) != math.Signbit(delta[0]) {
			m[0] = 0.0
		} else if math.Abs(m[0]) > 2.0*math.Abs(delta[0]) {
			m[0] = 2.0 * delta[0]
		}
	}

	// Right endpoint.
	m[n-1] = delta[n-2]
	if delta[n-2] != 0.0 {
		if math.Signbit(m[n-1]) != math.Signbit(delta[n-2]) {
			m[n-1] = 0.0
		} else if math.Abs(m[n-1]) > 2.0*math.Abs(delta[n-2]) {
			m[n-1] = 2.0 * delta[n-2]
		}
	}

	// 4) Evaluate the Hermite spline at NaN indices.
	firstX := xs[0]
	lastX := xs[n-1]

	for i := 0; i < nTot; i++ {
		yi := ts.DataSeries[i].Meas
		if isValid(yi) {
			continue
		}

		x := float64(i)
		if x < firstX || x > lastX {
			// No extrapolation.
			continue
		}

		// Find the segment j such that xs[j] <= x <= xs[j+1].
		j := sort.Search(len(xs), func(k int) bool {
			return xs[k] >= x
		})
		if j == 0 {
			j = 0
		} else {
			j = j - 1
		}
		if j < 0 {
			j = 0
		}
		if j >= n-1 {
			j = n - 2
		}

		hj := h[j]
		t := (x - xs[j]) / hj

		t2 := t * t
		t3 := t2 * t

		h00 := 2*t3 - 3*t2 + 1
		h10 := t3 - 2*t2 + t
		h01 := -2*t3 + 3*t2
		h11 := t3 - t2

		y := h00*ys[j] + h10*hj*m[j] + h01*ys[j+1] + h11*hj*m[j+1]
		ts.DataSeries[i].Meas = y
	}
}
