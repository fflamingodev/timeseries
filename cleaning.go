package timeseries

import (
	"errors"
	"fmt"
	"math"
	"sort"

	nav "github.com/fflamingodev/notavalue"
)

// Cleaning a series means deciding which readings not to believe.
//
// # What a rejection is
//
// A rejected point is not an error and not a deletion: it is a
// measurement the caller has decided to stop trusting. So it becomes
// NaV in the cleaned series — missing, exactly like a reading the
// sensor never took — and the point keeps its place in time.
//
// This matters more than it sounds. The previous version of this
// library replaced rejected points with a plain NaN, which meant that a
// single outlier in a month of readings turned every statistic of that
// month into NaN. With NaV, the mean carries on over what is left, and
// the counters say how much was discarded.
//
// The rejected points are returned in a series of their own, with their
// original values, so that nothing is lost and the decision can be
// reviewed.
//
// # What is never rejected
//
//   - A gap (NaV) is not rejected: there was nothing to judge. It stays
//     a gap, and does not appear among the rejects.
//   - A broken value (a plain NaN) is not rejected either, and is left
//     untouched. Cleaning is about implausible measurements; a
//     computation that failed upstream is a different problem, and
//     hiding it as a gap would be a way of losing it.
//
// Neither takes part in computing the fences: a mean is taken over what
// was actually measured.

// ErrCleaningArg reports an argument that cannot define a fence — a
// percentile outside (0, 100), a negative z-score level, bounds in the
// wrong order. Test for it with errors.Is.
var ErrCleaningArg = errors.New("timeseries: impossible cleaning argument")

// RemoveOutbounds splits the series on fixed bounds: a reading below
// min or above max is rejected. A bound set to notavalue.NaV means "no
// fence on that side", which is how a one-sided filter is expressed:
//
//	ts.RemoveOutbounds(0, nav.NaV)     // nothing below zero
//	ts.RemoveOutbounds(nav.NaV, 100)   // nothing above a hundred
//	ts.RemoveOutbounds(-40, 60)        // a plausible outdoor temperature
//
// The bounds are inclusive: a reading exactly equal to min or max is
// kept.
//
// It returns the cleaned series, where rejected readings are NaV, and
// the series of rejected readings with their original values.
func (ts *TimeSeries) RemoveOutbounds(min, max float64) (cleaned, rejected *TimeSeries, err error) {
	if !nav.IsNaV(min) && !nav.IsNaV(max) && min > max {
		return nil, nil, fmt.Errorf("%w: min %g is above max %g", ErrCleaningArg, min, max)
	}
	if math.IsNaN(min) && !nav.IsNaV(min) || math.IsNaN(max) && !nav.IsNaV(max) {
		return nil, nil, fmt.Errorf("%w: a bound cannot be NaN", ErrCleaningArg)
	}

	reject := func(v float64) bool {
		if !nav.IsNaV(min) && v < min {
			return true
		}
		if !nav.IsNaV(max) && v > max {
			return true
		}
		return false
	}
	return ts.split(reject, fmt.Sprintf("bounds [%s, %s]", nav.Format(min), nav.Format(max)))
}

// RemovePercentileOutliers splits the series on percentile fences
// computed from the readings themselves: everything below the low-th
// percentile and above the high-th is rejected.
//
// Each fence may be notavalue.NaV, meaning "no fence on that side":
//
//	ts.RemovePercentileOutliers(1, 99)        // trim both tails
//	ts.RemovePercentileOutliers(nav.NaV, 95)  // trim the top only
//
// Both must otherwise lie in (0, 100), and low must not exceed high.
//
// Note that this always rejects something on a series of any size:
// percentiles describe the readings at hand, so the fences move with
// the data. Use RemoveOutbounds when what is implausible is known in
// advance — a negative rainfall, a temperature above the boiling point.
func (ts *TimeSeries) RemovePercentileOutliers(low, high float64) (cleaned, rejected *TimeSeries, err error) {
	for _, p := range []struct {
		name  string
		value float64
	}{{"low", low}, {"high", high}} {
		if nav.IsNaV(p.value) {
			continue
		}
		if math.IsNaN(p.value) || p.value <= 0 || p.value >= 100 {
			return nil, nil, fmt.Errorf("%w: %s percentile %g is not in (0, 100)",
				ErrCleaningArg, p.name, p.value)
		}
	}
	if !nav.IsNaV(low) && !nav.IsNaV(high) && low > high {
		return nil, nil, fmt.Errorf("%w: low percentile %g is above high percentile %g",
			ErrCleaningArg, low, high)
	}

	usable := ts.usableMeas()
	if len(usable) == 0 {
		return ts.split(func(float64) bool { return false }, "percentiles on nothing to measure")
	}

	minBound, maxBound := nav.NaV, nav.NaV
	if !nav.IsNaV(low) {
		if minBound, err = nav.Percentile(usable, low); err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrCleaningArg, err)
		}
	}
	if !nav.IsNaV(high) {
		if maxBound, err = nav.Percentile(usable, high); err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrCleaningArg, err)
		}
	}
	return ts.RemoveOutbounds(minBound, maxBound)
}

// RemoveZScoreOutliers splits the series on a symmetric fence placed
// level standard deviations away from the mean: a reading further than
// that from the mean is rejected. Three is the customary level.
//
// The mean and the deviation are those of the usable readings. The
// method needs at least two of them, and a level above zero.
//
// Being built on a mean and a standard deviation, this fence assumes
// the readings scatter more or less symmetrically. On a skewed signal —
// rainfall, energy consumption — percentiles describe the data better.
func (ts *TimeSeries) RemoveZScoreOutliers(level float64) (cleaned, rejected *TimeSeries, err error) {
	if math.IsNaN(level) || level <= 0 {
		return nil, nil, fmt.Errorf("%w: z-score level %g must be above zero", ErrCleaningArg, level)
	}

	usable := ts.usableMeas()
	mean := nav.Mean(usable)
	deviation := nav.StdDev(usable)
	if nav.IsNaV(mean) || nav.IsNaV(deviation) {
		// Fewer than two readings: nothing to build a fence on, and
		// nothing to reject either.
		return ts.split(func(float64) bool { return false }, "z-score on too few readings")
	}

	low, high := mean-level*deviation, mean+level*deviation
	reject := func(v float64) bool { return v < low || v > high }
	return ts.split(reject, fmt.Sprintf("z-score at %g: [%s, %s]",
		level, nav.Format(low), nav.Format(high)))
}

// RemovePeirceOutliers splits the series according to Peirce's
// criterion, which decides how many readings a sample of that size may
// legitimately lose before the rejection itself becomes suspect.
//
// Unlike the z-score fence, it does not take a threshold: the criterion
// derives one from the number of readings, through a table published by
// Benjamin Peirce in 1852. It is stricter on small samples, where one
// value out of five can hardly be called an outlier, and it rejects at
// most nine readings whatever the size of the series.
//
// It needs at least three usable readings; below that it rejects
// nothing.
func (ts *TimeSeries) RemovePeirceOutliers() (cleaned, rejected *TimeSeries, err error) {
	usable := ts.usableMeas()
	doomed := peirceThreshold(usable)
	if nav.IsNaV(doomed) {
		return ts.split(func(float64) bool { return false }, "Peirce on too few readings")
	}

	mean := nav.Mean(usable)
	reject := func(v float64) bool { return math.Abs(v-mean) > doomed }
	return ts.split(reject, fmt.Sprintf("Peirce criterion: deviation above %s", nav.Format(doomed)))
}

// -----------------------------------------------------------------------
// The machinery
// -----------------------------------------------------------------------

// usableMeas returns the readings that are neither missing nor broken,
// which are the only ones a fence may be computed from.
func (ts *TimeSeries) usableMeas() []float64 {
	out := make([]float64, 0, ts.Len())
	ts.Range(func(_ int, du DataUnit) bool {
		if !math.IsNaN(du.Meas) {
			out = append(out, du.Meas)
		}
		return true
	})
	return out
}

// split walks the series once and applies reject to every usable
// reading. It returns the cleaned series, where a rejected reading
// becomes NaV and keeps its instant, and the series of the rejected
// readings with their original values.
//
// Gaps and broken values are never submitted to reject: there is
// nothing to judge in the first, and the second is a problem of its own.
func (ts *TimeSeries) split(reject func(float64) bool, how string) (cleaned, rejected *TimeSeries, err error) {
	cleaned = NewTimeSeries(ts.Name + " cleaned")
	cleaned.ID = ts.ID
	cleaned.Comment = how
	rejected = NewTimeSeries(ts.Name + " rejected")
	rejected.ID = ts.ID
	rejected.Comment = how

	keep := make([]Datum, 0, ts.Len())
	out := make([]Datum, 0, 8)

	ts.Range(func(_ int, du DataUnit) bool {
		switch {
		case math.IsNaN(du.Meas): // a gap or a broken value: left as it is
			keep = append(keep, du.Datum)
		case reject(du.Meas):
			keep = append(keep, NewDatum(du.Chron, nav.NaV))
			out = append(out, du.Datum)
		default:
			keep = append(keep, du.Datum)
		}
		return true
	})

	cleaned.AddBatchData(keep)
	rejected.AddBatchData(out)
	return cleaned, rejected, nil
}

// peirceThreshold returns the greatest deviation from the mean that
// Peirce's criterion still tolerates, or NaV when the sample is too
// small to judge.
//
// The criterion works by trial: assume one reading is doomed, read the
// ratio R from the table, and see whether the largest deviation exceeds
// R times the standard deviation. If it does, assume two are doomed and
// start again, until the table stops condemning. The threshold returned
// is the one of the last accepted round.
func peirceThreshold(usable []float64) float64 {
	n := len(usable)
	if n < 3 {
		// The table starts at three readings, and below that a rejection
		// would say more about the criterion than about the data.
		return nav.NaV
	}
	mean := nav.Mean(usable)
	deviation := nav.StdDev(usable)
	if nav.IsNaV(mean) || nav.IsNaV(deviation) || deviation == 0 {
		return nav.NaV
	}

	deviations := make([]float64, n)
	for i, v := range usable {
		deviations[i] = math.Abs(v - mean)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(deviations)))

	row := n - 3
	if row > peirceMaxRow {
		row = peirceMaxRow
	}

	threshold := nav.NaV
	for doomed := 0; doomed < peirceMaxDoomed && doomed < n; doomed++ {
		ratio := peirceRatio(row, doomed)
		if ratio <= 0 {
			break
		}
		limit := ratio * deviation
		if deviations[doomed] <= limit {
			// This reading survives, and so does every smaller deviation.
			threshold = limit
			break
		}
		threshold = limit
	}
	return threshold
}

const (
	peirceMaxRow    = 57 // the table covers samples of 3 to 60 readings
	peirceMaxDoomed = 9  // and at most nine simultaneous rejections
)

// peirceRatio returns R(n, k), the critical ratio of Peirce's criterion
// for a sample whose size gives row = n-3, with k readings assumed
// doomed. A zero means the table has nothing to say for that pair.
func peirceRatio(row, doomed int) float64 {
	if row < 0 || row > peirceMaxRow || doomed < 0 || doomed >= peirceMaxDoomed {
		return 0
	}
	return peirceTable[row][doomed]
}

// peirceTable is the table of critical ratios published by Benjamin
// Peirce in 1852, as reproduced in Ross, "Peirce's criterion for the
// elimination of suspect experimental data", Journal of Engineering
// Technology, 2003. Row i is a sample of i+3 readings; column j is the
// case of j+1 doomed readings.
var peirceTable = [peirceMaxRow + 1][peirceMaxDoomed]float64{
	{1.196, 0, 0, 0, 0, 0, 0, 0, 0},
	{1.383, 1.078, 0, 0, 0, 0, 0, 0, 0},
	{1.509, 1.2, 0, 0, 0, 0, 0, 0, 0},
	{1.61, 1.299, 1.099, 0, 0, 0, 0, 0, 0},
	{1.693, 1.382, 1.187, 1.022, 0, 0, 0, 0, 0},
	{1.763, 1.453, 1.261, 1.109, 0, 0, 0, 0, 0},
	{1.824, 1.515, 1.324, 1.178, 1.045, 0, 0, 0, 0},
	{1.878, 1.57, 1.38, 1.237, 1.114, 0, 0, 0, 0},
	{1.925, 1.619, 1.43, 1.289, 1.172, 1.059, 0, 0, 0},
	{1.969, 1.663, 1.475, 1.336, 1.221, 1.118, 1.009, 0, 0},
	{2.007, 1.704, 1.516, 1.379, 1.266, 1.167, 1.07, 0, 0},
	{2.043, 1.741, 1.554, 1.417, 1.307, 1.21, 1.12, 1.026, 0},
	{2.076, 1.775, 1.589, 1.453, 1.344, 1.249, 1.164, 1.078, 0},
	{2.106, 1.807, 1.622, 1.486, 1.378, 1.285, 1.202, 1.122, 1.039},
	{2.134, 1.836, 1.652, 1.517, 1.409, 1.318, 1.237, 1.161, 1.084},
	{2.161, 1.864, 1.68, 1.546, 1.438, 1.348, 1.268, 1.195, 1.123},
	{2.185, 1.89, 1.707, 1.573, 1.466, 1.377, 1.298, 1.226, 1.158},
	{2.209, 1.914, 1.732, 1.599, 1.492, 1.404, 1.326, 1.255, 1.19},
	{2.23, 1.938, 1.756, 1.623, 1.517, 1.429, 1.352, 1.282, 1.218},
	{2.251, 1.96, 1.779, 1.646, 1.54, 1.452, 1.376, 1.308, 1.245},
	{2.271, 1.981, 1.8, 1.668, 1.563, 1.475, 1.399, 1.332, 1.27},
	{2.29, 2, 1.821, 1.689, 1.584, 1.497, 1.421, 1.354, 1.293},
	{2.307, 2.019, 1.84, 1.709, 1.604, 1.517, 1.442, 1.375, 1.315},
	{2.324, 2.037, 1.859, 1.728, 1.624, 1.537, 1.462, 1.396, 1.336},
	{2.341, 2.055, 1.877, 1.746, 1.642, 1.556, 1.481, 1.415, 1.356},
	{2.356, 2.071, 1.894, 1.764, 1.66, 1.574, 1.5, 1.434, 1.375},
	{2.371, 2.088, 1.911, 1.781, 1.677, 1.591, 1.517, 1.452, 1.393},
	{2.385, 2.103, 1.927, 1.797, 1.694, 1.608, 1.534, 1.469, 1.411},
	{2.399, 2.118, 1.942, 1.812, 1.71, 1.624, 1.55, 1.486, 1.428},
	{2.412, 2.132, 1.957, 1.828, 1.725, 1.64, 1.567, 1.502, 1.444},
	{2.425, 2.146, 1.971, 1.842, 1.74, 1.655, 1.582, 1.517, 1.459},
	{2.438, 2.159, 1.985, 1.856, 1.754, 1.669, 1.597, 1.532, 1.475},
	{2.45, 2.172, 1.998, 1.87, 1.768, 1.683, 1.611, 1.547, 1.489},
	{2.461, 2.184, 2.011, 1.883, 1.782, 1.697, 1.624, 1.561, 1.504},
	{2.472, 2.196, 2.024, 1.896, 1.795, 1.711, 1.638, 1.574, 1.517},
	{2.483, 2.208, 2.036, 1.909, 1.807, 1.723, 1.651, 1.587, 1.531},
	{2.494, 2.219, 2.047, 1.921, 1.82, 1.736, 1.664, 1.6, 1.544},
	{2.504, 2.23, 2.059, 1.932, 1.832, 1.748, 1.676, 1.613, 1.556},
	{2.514, 2.241, 2.07, 1.944, 1.843, 1.76, 1.688, 1.625, 1.568},
	{2.524, 2.251, 2.081, 1.955, 1.855, 1.771, 1.699, 1.636, 1.58},
	{2.533, 2.261, 2.092, 1.966, 1.866, 1.783, 1.711, 1.648, 1.592},
	{2.542, 2.271, 2.102, 1.976, 1.876, 1.794, 1.722, 1.659, 1.603},
	{2.551, 2.281, 2.112, 1.987, 1.887, 1.804, 1.733, 1.67, 1.614},
	{2.56, 2.29, 2.122, 1.997, 1.897, 1.815, 1.743, 1.681, 1.625},
	{2.568, 2.299, 2.131, 2.006, 1.907, 1.825, 1.754, 1.691, 1.636},
	{2.577, 2.308, 2.14, 2.016, 1.917, 1.835, 1.764, 1.701, 1.646},
	{2.585, 2.317, 2.149, 2.026, 1.927, 1.844, 1.773, 1.711, 1.656},
	{2.592, 2.326, 2.158, 2.035, 1.936, 1.854, 1.783, 1.721, 1.666},
	{2.6, 2.334, 2.167, 2.044, 1.945, 1.863, 1.792, 1.73, 1.675},
	{2.608, 2.342, 2.175, 2.052, 1.954, 1.872, 1.802, 1.74, 1.685},
	{2.615, 2.35, 2.184, 2.061, 1.963, 1.881, 1.811, 1.749, 1.694},
	{2.622, 2.358, 2.192, 2.069, 1.972, 1.89, 1.82, 1.758, 1.703},
	{2.629, 2.365, 2.2, 2.077, 1.98, 1.898, 1.828, 1.767, 1.711},
	{2.636, 2.373, 2.207, 2.085, 1.988, 1.907, 1.837, 1.775, 1.72},
	{2.643, 2.38, 2.215, 2.093, 1.996, 1.915, 1.845, 1.784, 1.729},
	{2.65, 2.387, 2.223, 2.109, 2.012, 1.931, 1.861, 1.8, 1.745},
	{2.656, 2.394, 2.237, 2.116, 2.019, 1.939, 1.869, 1.808, 1.753},
	{2.663, 2.401, 2.223, 2.101, 2.004, 1.923, 1.853, 1.792, 1.737},
}
