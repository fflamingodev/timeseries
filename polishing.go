package timeseries

import (
	"fmt"
	"runtime/debug"
	"time"
)

// ApplyPolishing runs the polishing pipeline described by rc on ts and
// stores the resulting TimeSeries in tsc.Ts under the rc.Variant key.
//
// The pipeline steps are executed in order, each conditioned on its
// corresponding recipe field being non-nil:
//
//  1. Reduce     — collapse runs of identical values.
//  2. Method1    — first cleaning pass (fixed bounds, percentile fences,
//                  z-score, or Peirce).
//  3. Regularize — resample on a fixed-step grid with the named aggregator.
//  4. Method2    — second cleaning pass, applied post-regularization.
//  5. Interp     — NaV/NaN filling strategy.
//
// If rc.Variant == "raw", no step runs and ts is stored as-is under
// the "raw" key.
//
// Errors and recovery:
//   - Any non-nil error returned by a step is wrapped in an *Error
//     with Op == "ApplyPolishing" and Kind preserved from the inner
//     error via Is propagation. Partial variants produced by earlier
//     successful steps (e.g. "Reduced") remain in tsc.Ts so callers
//     can still consume them.
//   - A panic raised by any step is recovered and converted to an
//     *Error with Kind == KindPanic, its Stack field populated via
//     runtime/debug.Stack, and Err set to the original panic value
//     (wrapped through fmt.Errorf to implement the error interface).
//
// Returns nil iff the pipeline ran to completion.
func (tsc *TsContainer) ApplyPolishing(ts *TimeSeries, rc *RecipesCatalogueRow) (err error) {
	if tsc == nil {
		return &Error{Op: "ApplyPolishing", Kind: KindInvalidArg, Msg: "nil receiver"}
	}
	if ts == nil {
		return &Error{Op: "ApplyPolishing", Kind: KindInvalidArg, Msg: "nil TimeSeries", Field: "ts"}
	}
	if rc == nil {
		return &Error{Op: "ApplyPolishing", Kind: KindInvalidArg, Msg: "nil recipe", Field: "rc"}
	}

	// Track which pipeline steps complete so the recover / error paths
	// can report partial progress to the caller.
	var completed []string

	defer func() {
		if r := recover(); r != nil {
			err = &Error{
				Op:    "ApplyPolishing",
				Kind:  KindPanic,
				Msg:   fmt.Sprintf("panic in pipeline step (completed: %v)", completed),
				Err:   fmt.Errorf("%v", r),
				Stack: string(debug.Stack()),
			}
		}
	}()

	ts.SortDeltasStats()
	workingTs := ts

	// Raw variant: no recipe, just pass the series through.
	if rc.Variant == "raw" {
		tsc.Ts["raw"] = ts
		return nil
	}

	if rc.Reduce != nil && *rc.Reduce == 1 {
		tsreduced := workingTs.Reduce()
		tsreduced.SortDeltasStats()
		tsc.Ts["Reduced"] = &tsreduced
		workingTs = &tsreduced
		completed = append(completed, "Reduce")
	}

	if rc.Method1 != nil {
		method := *rc.Method1

		var min1, max1, percent1, lvl1 float64
		if rc.Min1 != nil {
			min1 = *rc.Min1
		}
		if rc.Max1 != nil {
			max1 = *rc.Max1
		}
		if rc.Percent1 != nil {
			percent1 = *rc.Percent1
		}
		if rc.Lvl1 != nil {
			lvl1 = *rc.Lvl1
		}

		tsclean, tsreject, e := applyCleaning(workingTs, method, min1, max1, percent1, lvl1)
		if e != nil {
			return &Error{Op: "ApplyPolishing", Kind: KindBadState,
				Msg: "cleaning pass 1 failed", Field: "method1", Value: method, Err: e}
		}
		tsclean.SortDeltasStats()
		tsreject.SortDeltasStats()
		workingTs = &tsclean
		completed = append(completed, "Method1")
	}

	// Regularize.
	if rc.FreqSeconds != nil && rc.Agg != nil {
		freq := time.Duration(*rc.FreqSeconds) * time.Second
		agg, e := getAggFunc(*rc.Agg, freq)
		if e != nil {
			return &Error{Op: "ApplyPolishing", Kind: KindUnknownOption,
				Msg: "unknown aggregator for regularize", Field: "agg", Value: *rc.Agg, Err: e}
		}

		workingTs.SortDeltasStats()
		regularizedTs := workingTs.Regularize(freq, agg)
		regularizedTs.SortDeltasStats()
		workingTs = &regularizedTs
		completed = append(completed, "Regularize")
	}

	if rc.Method2 != nil {
		method2 := *rc.Method2

		var min2, max2, percent2, lvl2 float64
		if rc.Min2 != nil {
			min2 = *rc.Min2
		}
		if rc.Max2 != nil {
			max2 = *rc.Max2
		}
		if rc.Percent2 != nil {
			percent2 = *rc.Percent2
		}
		if rc.Lvl2 != nil {
			lvl2 = *rc.Lvl2
		}

		tsclean, _, e := applyCleaning(workingTs, method2, min2, max2, percent2, lvl2)
		if e != nil {
			return &Error{Op: "ApplyPolishing", Kind: KindBadState,
				Msg: "cleaning pass 2 failed", Field: "method2", Value: method2, Err: e}
		}
		tsclean.SortDeltasStats()
		workingTs = &tsclean
		completed = append(completed, "Method2")
	}

	// Interp.
	if rc.Interp != nil {
		method, e := getInterpMethod(*rc.Interp)
		if e != nil {
			return &Error{Op: "ApplyPolishing", Kind: KindUnknownOption,
				Msg: "unknown interpolation method", Field: "interp", Value: *rc.Interp, Err: e}
		}
		if e := workingTs.Interpolate(method); e != nil {
			return &Error{Op: "ApplyPolishing", Kind: KindBadState,
				Msg: "interpolation failed", Err: e}
		}
		workingTs.SortDeltasStats()
		completed = append(completed, "Interp")
	}

	// Final key = requested variant.
	tsc.Ts[rc.Variant] = workingTs
	return nil
}

// applyCleaning dispatches to the cleaning method selected by name.
// An unknown name yields an *Error with Kind == KindUnknownOption.
// Any error from the underlying cleaning method is propagated verbatim.
func applyCleaning(ts *TimeSeries, method string, min, max, percent, level float64) (TimeSeries, TimeSeries, error) {
	switch method {
	case "fixedOutbounds":
		return ts.RemoveOutbounds(&min, &max)
	case "outerPercentile":
		return ts.PercCleaning(percent)
	case "lowerPercentile":
		return ts.LowerPercCleaning(percent)
	case "upperPercentile":
		return ts.UpperPercCleaning(percent)
	case "zScore":
		return ts.ZscoreCleaning(level)
	case "peirce":
		return ts.PeirceOutlierRemoval()
	default:
		return TimeSeries{}, TimeSeries{}, &Error{
			Op:    "applyCleaning",
			Kind:  KindUnknownOption,
			Msg:   "unknown cleaning method",
			Field: "method",
			Value: method,
		}
	}
}

// getAggFunc maps a recipe's string-form aggregator name to the concrete
// AggFunc used by Regularize. An unknown name yields an *Error with
// Kind == KindUnknownOption.
func getAggFunc(aggName string, freq time.Duration) (AggFunc, error) {
	switch aggName {
	case "average":
		return AggAverage, nil
	case "maximum":
		return AggMaximum, nil
	case "minimum":
		return AggMinimum, nil
	case "last":
		return AggLast, nil
	case "open":
		return AggOpen, nil
	case "countValid":
		return AggCountValid, nil
	case "median":
		return AggMedian, nil
	case "slope":
		return AggSlope, nil
	case "integral":
		return AggIntegral(freq), nil
	case "incrementalCounter":
		return AggIncrementalCounter(), nil
	default:
		return nil, &Error{
			Op:    "getAggFunc",
			Kind:  KindUnknownOption,
			Msg:   "unknown aggregation function",
			Field: "aggName",
			Value: aggName,
		}
	}
}

// getInterpMethod maps a recipe's string-form interpolation name to the
// matching InterpolationMethod constant. An unknown name yields an
// *Error with Kind == KindUnknownOption.
func getInterpMethod(name string) (InterpolationMethod, error) {
	switch name {
	case "", "None", "none":
		return InterpNone, nil
	case "Linear":
		return InterpLinear, nil
	case "Nearest":
		return InterpNearest, nil
	case "ForwardFill":
		return InterpForwardFill, nil
	case "BackwardFill":
		return InterpBackwardFill, nil
	case "LogLinear":
		return InterpLogLinear, nil
	case "CubicSpline":
		return InterpCubicSpline, nil
	case "MonotoneSpline":
		return InterpMonotoneSpline, nil
	default:
		return InterpNone, &Error{
			Op:    "getInterpMethod",
			Kind:  KindUnknownOption,
			Msg:   "unknown interpolation method",
			Field: "name",
			Value: name,
		}
	}
}
