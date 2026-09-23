package timeseries

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// AggFunc condenses the readings that fall in one window of a
// regularized series into a single value.
//
// It receives the measurements of the window in chronological order,
// and never an empty slice: an empty window is emitted as NaV without
// consulting the aggregator, since there is nothing for it to decide.
//
// The slice it receives may hold gaps and broken values. Every
// aggregator in this file handles them the way the rest of the library
// does — a gap is skipped, an error propagates — because they are built
// on the notavalue package. An aggregator written elsewhere is free to
// decide otherwise, but should say so.
//
// The slice is reused between windows: an aggregator must not keep it.
type AggFunc func(window []float64) float64

// AggMean returns the mean of the window, over the readings it actually
// holds. It is the usual choice for a physical quantity: a temperature,
// a power, a concentration.
func AggMean(window []float64) float64 { return nav.Mean(window) }

// AggMedian returns the median of the window. Preferred to the mean
// when a stray reading must not drag the result: the median of a window
// is one of its readings.
func AggMedian(window []float64) float64 { return nav.Median(window) }

// AggMin returns the smallest reading of the window.
func AggMin(window []float64) float64 { return nav.Min(window) }

// AggMax returns the largest reading of the window.
func AggMax(window []float64) float64 { return nav.Max(window) }

// AggSum returns the sum of the window. The choice for a quantity that
// accumulates — rainfall, energy, a count of events — where a mean
// would answer a different question.
func AggSum(window []float64) float64 { return nav.Sum(window) }

// AggFirst returns the earliest reading of the window, gap or not. It
// is what a chart of a state signal wants: the value the system had
// when the window opened.
func AggFirst(window []float64) float64 {
	if len(window) == 0 {
		return nav.NaV
	}
	return window[0]
}

// AggLast returns the latest reading of the window, gap or not. The
// choice for a counter or a level: what the instrument showed at the
// end of the window.
func AggLast(window []float64) float64 {
	if len(window) == 0 {
		return nav.NaV
	}
	return window[len(window)-1]
}

// AggFirstUsable returns the earliest reading of the window that is
// neither missing nor broken, or NaV if the window holds none. Unlike
// AggFirst, it steps over a gap rather than reporting it.
func AggFirstUsable(window []float64) float64 {
	for _, v := range window {
		if !math.IsNaN(v) {
			return v
		}
	}
	return nav.NaV
}

// AggLastUsable returns the latest reading of the window that is
// neither missing nor broken, or NaV if the window holds none.
func AggLastUsable(window []float64) float64 {
	for i := len(window) - 1; i >= 0; i-- {
		if !math.IsNaN(window[i]) {
			return window[i]
		}
	}
	return nav.NaV
}

// AggCountUsable returns how many readings of the window are neither
// missing nor broken, as a float64 so that it fits an AggFunc.
//
// Regularizing with it turns a series into its own coverage report:
// how many readings arrived in each window. Zero is a real answer here,
// not a sentinel — the window existed and held nothing usable.
func AggCountUsable(window []float64) float64 {
	return float64(nav.CountUsable(window))
}

// AggSlope returns the slope of the least-squares line fitted through
// the window, per index and not per unit of time: the readings are
// taken as equally spaced, which they are once the series is
// regularized but not before.
//
// It needs two usable readings and a window that is not flat in time;
// otherwise it returns NaV. A broken reading propagates.
func AggSlope(window []float64) float64 {
	if hasBroken(window) {
		return math.NaN()
	}

	var sumX, sumY, sumXY, sumXX, n float64
	for i, v := range window {
		if math.IsNaN(v) {
			continue
		}
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumXX += x * x
		n++
	}
	if n < 2 {
		return nav.NaV
	}

	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return nav.NaV
	}
	return (n*sumXY - sumX*sumY) / denominator
}

// AggIntegral builds an aggregator that estimates the area under the
// window by the rectangle rule: the sum of its readings multiplied by
// step, the time each reading is taken to cover.
//
// Use it to turn a rate into a quantity — a power in watts sampled
// every minute into an energy — passing the sampling step of the input,
// not the width of the window.
func AggIntegral(step time.Duration) AggFunc {
	seconds := step.Seconds()
	return func(window []float64) float64 {
		return nav.Mul(nav.Sum(window), seconds)
	}
}

// hasBroken reports whether the window holds a plain NaN, which every
// aggregate must propagate.
func hasBroken(window []float64) bool {
	for _, v := range window {
		if nav.IsStdNaN(v) {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// Aggregators by name
// -----------------------------------------------------------------------

// aggregatorsByName maps the names a recipe uses to the aggregators
// themselves. The keys are lowercase; Aggregator does the folding.
var aggregatorsByName = map[string]AggFunc{
	"mean":        AggMean,
	"average":     AggMean,
	"avg":         AggMean,
	"median":      AggMedian,
	"min":         AggMin,
	"minimum":     AggMin,
	"max":         AggMax,
	"maximum":     AggMax,
	"sum":         AggSum,
	"first":       AggFirst,
	"open":        AggFirst,
	"last":        AggLast,
	"close":       AggLast,
	"firstusable": AggFirstUsable,
	"lastusable":  AggLastUsable,
	"count":       AggCountUsable,
	"countusable": AggCountUsable,
	"slope":       AggSlope,
}

// Aggregator returns the aggregator a recipe names, so that a
// regularization described in a database row or a JSON payload can be
// carried out. The name is matched without regard to case or
// surrounding spaces, and several spellings lead to the same function —
// "average", "mean" and "avg" among them.
//
// An unknown name is an error rather than a silent fallback: a recipe
// asking for something the library cannot do must say so, not quietly
// compute a mean.
func Aggregator(name string) (AggFunc, error) {
	agg, ok := aggregatorsByName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("timeseries: unknown aggregator %q, known ones are %s",
			name, strings.Join(AggregatorNames(), ", "))
	}
	return agg, nil
}

// AggregatorNames returns the names Aggregator accepts, sorted, for an
// error message or a form's drop-down list.
func AggregatorNames() []string {
	names := make([]string, 0, len(aggregatorsByName))
	for name := range aggregatorsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
