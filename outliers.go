package timeseries

import (
	"math"
	"sort"
	"time"
)

// RemoveOutbounds splits the series into (cleaned, rejected) by value
// bounds. It keeps points with min <= Meas <= max in the cleaned series
// and moves the others to the rejected series. In the cleaned series,
// rejected slots are materialized as NaV so index alignment with the
// original series is preserved. The input is temporarily sorted by Meas
// to evaluate bounds and then restored to chronological order; both
// outputs are returned in chronological order.
//
// Returns an *Error with Kind == KindInvalidArg if both bounds are
// provided and *min > *max.
func (tsin *TimeSeries) RemoveOutbounds(min, max *float64) (TimeSeries, TimeSeries, error) {
	if min != nil && max != nil && *min > *max {
		return TimeSeries{}, TimeSeries{}, &Error{
			Op:   "RemoveOutbounds",
			Kind: KindInvalidArg,
			Msg:  "min must be <= max",
		}
	}

	var tsout, tsrej TimeSeries

	tsin.SortMeasAsc()

	for _, du := range tsin.DataSeries {
		out := false
		if min != nil && du.Meas < *min {
			out = true
		}
		if max != nil && du.Meas > *max {
			out = true
		}

		if out {
			// Preserve the original Meas in the rejected series, but
			// reset the deltas: after SortMeasAsc above, the incoming
			// du.Dchron/du.Dmeas no longer refer to chronological
			// neighbors anyway. Callers should run SortDeltasStats on
			// the returned series to repopulate them.
			tsrej.AddDataUnit(NewDataUnit(du.Chron, du.Meas))
			tsout.AddDataUnit(NewDataUnit(du.Chron, NaV))
		} else {
			// Keep the unit but reset its (now-stale) deltas.
			tsout.AddDataUnit(NewDataUnit(du.Chron, du.Meas))
		}
	}

	tsin.SortChronAsc()
	tsout.SortChronAsc()
	tsrej.SortChronAsc()

	tsout.Name = tsin.Name + " Cleaned"
	tsrej.Name = tsin.Name + " Removed"

	return tsout, tsrej, nil
}

// PercCleaning removes outliers using symmetric percentile fences.
// It computes lower = Percentile(perc), upper = Percentile(100-perc) on
// the measurements and delegates to RemoveOutbounds. Returns
// (cleaned, rejected, nil) on success; any *Error from the underlying
// Percentile computation is propagated verbatim.
func (tsin *TimeSeries) PercCleaning(perc float64) (TimeSeries, TimeSeries, error) {
	tsin.SortMeasAsc()
	dt := tsin.MeasToArr()
	min, err := Percentile(dt, perc)
	if err != nil {
		return TimeSeries{}, TimeSeries{}, &Error{Op: "PercCleaning", Kind: KindBounds, Msg: "lower percentile failed", Err: err}
	}
	max, err := Percentile(dt, 100-perc)
	if err != nil {
		return TimeSeries{}, TimeSeries{}, &Error{Op: "PercCleaning", Kind: KindBounds, Msg: "upper percentile failed", Err: err}
	}
	return tsin.RemoveOutbounds(&min, &max)
}

// LowerPercCleaning removes only the lower-tail outliers (below the
// perc-th percentile). Same propagation contract as PercCleaning.
func (tsin *TimeSeries) LowerPercCleaning(perc float64) (TimeSeries, TimeSeries, error) {
	tsin.SortMeasAsc()
	dt := tsin.MeasToArr()
	min, err := Percentile(dt, perc)
	if err != nil {
		return TimeSeries{}, TimeSeries{}, &Error{Op: "LowerPercCleaning", Kind: KindBounds, Msg: "percentile failed", Err: err}
	}
	return tsin.RemoveOutbounds(&min, nil)
}

// UpperPercCleaning removes only the upper-tail outliers (above the
// (100-perc)-th percentile). Same propagation contract as PercCleaning.
func (tsin *TimeSeries) UpperPercCleaning(perc float64) (TimeSeries, TimeSeries, error) {
	tsin.SortMeasAsc()
	dt := tsin.MeasToArr()
	max, err := Percentile(dt, 100-perc)
	if err != nil {
		return TimeSeries{}, TimeSeries{}, &Error{Op: "UpperPercCleaning", Kind: KindBounds, Msg: "percentile failed", Err: err}
	}
	return tsin.RemoveOutbounds(nil, &max)
}

// ZscoreCleaning removes outliers using a z-score envelope.
// It computes mean and (sample) standard deviation of the measurements,
// builds bounds [mean - lvl*std, mean + lvl*std], and delegates to
// RemoveOutbounds. Propagates any error from the underlying StdDev call.
func (tsin *TimeSeries) ZscoreCleaning(lvl float64) (TimeSeries, TimeSeries, error) {
	tsin.SortMeasAsc()
	dt := tsin.MeasToArr()
	dtmean := Mean(dt)
	if IsNaV(dtmean) {
		return TimeSeries{}, TimeSeries{}, &Error{Op: "ZscoreCleaning", Kind: KindEmptyInput, Msg: "no finite value in input"}
	}
	dtstd, err := StdDev(dt)
	if err != nil {
		return TimeSeries{}, TimeSeries{}, &Error{Op: "ZscoreCleaning", Kind: KindBadState, Msg: "standard deviation failed", Err: err}
	}
	min := dtmean - dtstd*lvl
	max := dtmean + dtstd*lvl
	return tsin.RemoveOutbounds(&min, &max)
}

// PeirceOutlierRemoval removes outliers according to Peirce's criterion.
// It calls Peirce on the measurement array to obtain the set of indices
// to drop and returns (cleaned, rejected, nil). Runs in O(N + k) where
// k is the number of rejected points. Currently never returns a
// non-nil error; the signature reserves one for forward compatibility.
func (tsin *TimeSeries) PeirceOutlierRemoval() (TimeSeries, TimeSeries, error) {
	var tsout, tsrej TimeSeries
	torem := peirce(tsin.MeasToArr())
	rejectIdx := make(map[int]struct{}, len(torem))
	for _, i := range torem {
		rejectIdx[i] = struct{}{}
	}
	for k, du := range tsin.DataSeries {
		if _, drop := rejectIdx[k]; drop {
			tsrej.AddDataUnit(du)
		} else {
			tsout.AddDataUnit(du)
		}
	}
	return tsout, tsrej, nil
}

// peirce returns the indices of observations rejected by Peirce's
// criterion. It ranks absolute deviations from the mean, then rejects
// the largest deviations while |dev| > R(N, r)*std, where R is given by
// rtable and r is the running count of suspects. The input slice is
// not modified. Unexported: external callers should use
// PeirceOutlierRemoval on a *TimeSeries.
func peirce(data []float64) []int {
	N := len(data)
	if N < 3 {
		// Peirce's table starts at N=3 (sampleLength index 0); smaller
		// samples are too short to reject any point meaningfully.
		return nil
	}
	avg := Mean(data)
	s, err := StdDev(data)
	if err != nil {
		return nil
	}

	type compdeviation struct {
		initialplace int
		value        float64
	}
	observedeviation := make([]compdeviation, N)
	for k, v := range data {
		observedeviation[k].value = math.Abs(v - avg)
		observedeviation[k].initialplace = k
	}
	sort.Slice(observedeviation, func(i, j int) bool {
		return observedeviation[i].value > observedeviation[j].value
	})

	// Table is 58 rows x 9 columns. sampleLength is clamped to 57 for
	// N > 60 (the critical ratios stabilize at high N). Suspect count is
	// clamped to 8 (table columns).
	NinTable := N - 3
	if NinTable > 57 {
		NinTable = 57
	}
	const maxSuspects = 9

	toremove := []int{}
	for i := 0; i < N && i < maxSuspects; i++ {
		r := rtable(NinTable, i)
		if r <= 0 {
			break
		}
		if observedeviation[i].value <= s*r {
			break
		}
		toremove = append(toremove, observedeviation[i].initialplace)
	}
	return toremove
}

// rtable returns the critical ratio R(N, k) used by Peirce's criterion.
// sampleLength is N (capped at 57 in this lookup), suspects is the
// current count of rejected points (0-based in this implementation).
// Unexported: external callers should use PeirceOutlierRemoval.
func rtable(sampleLength int, suspects int) float64 {
	Rtable := [58][9]float64{}
	Rtable[0] = [9]float64{1.196, 0, 0, 0, 0, 0, 0, 0, 0}
	Rtable[1] = [9]float64{1.383, 1.078, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	Rtable[2] = [9]float64{1.509, 1.2, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	Rtable[3] = [9]float64{1.61, 1.299, 1.099, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	Rtable[4] = [9]float64{1.693, 1.382, 1.187, 1.022, 0.0, 0.0, 0.0, 0.0, 0.0}
	Rtable[5] = [9]float64{1.763, 1.453, 1.261, 1.109, 0.0, 0.0, 0.0, 0.0, 0.0}
	Rtable[6] = [9]float64{1.824, 1.515, 1.324, 1.178, 1.045, 0.0, 0.0, 0.0, 0.0}
	Rtable[7] = [9]float64{1.878, 1.57, 1.38, 1.237, 1.114, 0.0, 0.0, 0.0, 0.0}
	Rtable[8] = [9]float64{1.925, 1.619, 1.43, 1.289, 1.172, 1.059, 0.0, 0.0, 0.0}
	Rtable[9] = [9]float64{1.969, 1.663, 1.475, 1.336, 1.221, 1.118, 1.009, 0.0, 0.0}
	Rtable[10] = [9]float64{2.007, 1.704, 1.516, 1.379, 1.266, 1.167, 1.07, 0.0, 0.0}
	Rtable[11] = [9]float64{2.043, 1.741, 1.554, 1.417, 1.307, 1.21, 1.12, 1.026, 0.0}
	Rtable[12] = [9]float64{2.076, 1.775, 1.589, 1.453, 1.344, 1.249, 1.164, 1.078, 0.0}
	Rtable[13] = [9]float64{2.106, 1.807, 1.622, 1.486, 1.378, 1.285, 1.202, 1.122, 1.039}
	Rtable[14] = [9]float64{2.134, 1.836, 1.652, 1.517, 1.409, 1.318, 1.237, 1.161, 1.084}
	Rtable[15] = [9]float64{2.161, 1.864, 1.68, 1.546, 1.438, 1.348, 1.268, 1.195, 1.123}
	Rtable[16] = [9]float64{2.185, 1.89, 1.707, 1.573, 1.466, 1.377, 1.298, 1.226, 1.158}
	Rtable[17] = [9]float64{2.209, 1.914, 1.732, 1.599, 1.492, 1.404, 1.326, 1.255, 1.19}
	Rtable[18] = [9]float64{2.23, 1.938, 1.756, 1.623, 1.517, 1.429, 1.352, 1.282, 1.218}
	Rtable[19] = [9]float64{2.251, 1.96, 1.779, 1.646, 1.54, 1.452, 1.376, 1.308, 1.245}
	Rtable[20] = [9]float64{2.271, 1.981, 1.8, 1.668, 1.563, 1.475, 1.399, 1.332, 1.27}
	Rtable[21] = [9]float64{2.29, 2, 1.821, 1.689, 1.584, 1.497, 1.421, 1.354, 1.293}
	Rtable[22] = [9]float64{2.307, 2.019, 1.84, 1.709, 1.604, 1.517, 1.442, 1.375, 1.315}
	Rtable[23] = [9]float64{2.324, 2.037, 1.859, 1.728, 1.624, 1.537, 1.462, 1.396, 1.336}
	Rtable[24] = [9]float64{2.341, 2.055, 1.877, 1.746, 1.642, 1.556, 1.481, 1.415, 1.356}
	Rtable[25] = [9]float64{2.356, 2.071, 1.894, 1.764, 1.66, 1.574, 1.5, 1.434, 1.375}
	Rtable[26] = [9]float64{2.371, 2.088, 1.911, 1.781, 1.677, 1.591, 1.517, 1.452, 1.393}
	Rtable[27] = [9]float64{2.385, 2.103, 1.927, 1.797, 1.694, 1.608, 1.534, 1.469, 1.411}
	Rtable[28] = [9]float64{2.399, 2.118, 1.942, 1.812, 1.71, 1.624, 1.55, 1.486, 1.428}
	Rtable[29] = [9]float64{2.412, 2.132, 1.957, 1.828, 1.725, 1.64, 1.567, 1.502, 1.444}
	Rtable[30] = [9]float64{2.425, 2.146, 1.971, 1.842, 1.74, 1.655, 1.582, 1.517, 1.459}
	Rtable[31] = [9]float64{2.438, 2.159, 1.985, 1.856, 1.754, 1.669, 1.597, 1.532, 1.475}
	Rtable[32] = [9]float64{2.45, 2.172, 1.998, 1.87, 1.768, 1.683, 1.611, 1.547, 1.489}
	Rtable[33] = [9]float64{2.461, 2.184, 2.011, 1.883, 1.782, 1.697, 1.624, 1.561, 1.504}
	Rtable[34] = [9]float64{2.472, 2.196, 2.024, 1.896, 1.795, 1.711, 1.638, 1.574, 1.517}
	Rtable[35] = [9]float64{2.483, 2.208, 2.036, 1.909, 1.807, 1.723, 1.651, 1.587, 1.531}
	Rtable[36] = [9]float64{2.494, 2.219, 2.047, 1.921, 1.82, 1.736, 1.664, 1.6, 1.544}
	Rtable[37] = [9]float64{2.504, 2.23, 2.059, 1.932, 1.832, 1.748, 1.676, 1.613, 1.556}
	Rtable[38] = [9]float64{2.514, 2.241, 2.07, 1.944, 1.843, 1.76, 1.688, 1.625, 1.568}
	Rtable[39] = [9]float64{2.524, 2.251, 2.081, 1.955, 1.855, 1.771, 1.699, 1.636, 1.58}
	Rtable[40] = [9]float64{2.533, 2.261, 2.092, 1.966, 1.866, 1.783, 1.711, 1.648, 1.592}
	Rtable[41] = [9]float64{2.542, 2.271, 2.102, 1.976, 1.876, 1.794, 1.722, 1.659, 1.603}
	Rtable[42] = [9]float64{2.551, 2.281, 2.112, 1.987, 1.887, 1.804, 1.733, 1.67, 1.614}
	Rtable[43] = [9]float64{2.56, 2.29, 2.122, 1.997, 1.897, 1.815, 1.743, 1.681, 1.625}
	Rtable[44] = [9]float64{2.568, 2.299, 2.131, 2.006, 1.907, 1.825, 1.754, 1.691, 1.636}
	Rtable[45] = [9]float64{2.577, 2.308, 2.14, 2.016, 1.917, 1.835, 1.764, 1.701, 1.646}
	Rtable[46] = [9]float64{2.585, 2.317, 2.149, 2.026, 1.927, 1.844, 1.773, 1.711, 1.656}
	Rtable[47] = [9]float64{2.592, 2.326, 2.158, 2.035, 1.936, 1.854, 1.783, 1.721, 1.666}
	Rtable[48] = [9]float64{2.6, 2.334, 2.167, 2.044, 1.945, 1.863, 1.792, 1.73, 1.675}
	Rtable[49] = [9]float64{2.608, 2.342, 2.175, 2.052, 1.954, 1.872, 1.802, 1.74, 1.685}
	Rtable[50] = [9]float64{2.615, 2.35, 2.184, 2.061, 1.963, 1.881, 1.811, 1.749, 1.694}
	Rtable[51] = [9]float64{2.622, 2.358, 2.192, 2.069, 1.972, 1.89, 1.82, 1.758, 1.703}
	Rtable[52] = [9]float64{2.629, 2.365, 2.2, 2.077, 1.98, 1.898, 1.828, 1.767, 1.711}
	Rtable[53] = [9]float64{2.636, 2.373, 2.207, 2.085, 1.988, 1.907, 1.837, 1.775, 1.72}
	Rtable[54] = [9]float64{2.643, 2.38, 2.215, 2.093, 1.996, 1.915, 1.845, 1.784, 1.729}
	Rtable[55] = [9]float64{2.65, 2.387, 2.223, 2.109, 2.012, 1.931, 1.861, 1.8, 1.745}
	Rtable[56] = [9]float64{2.656, 2.394, 2.237, 2.116, 2.019, 1.939, 1.869, 1.808, 1.753}
	Rtable[57] = [9]float64{2.663, 2.401, 2.223, 2.101, 2.004, 1.923, 1.853, 1.792, 1.737}
	return Rtable[sampleLength][suspects]
}

// Merge concatenates two series in their current order into a new
// one. Name and Comment are taken from tsa when non-empty, otherwise
// from tsb. The result is not automatically sorted; call SortChronAsc
// (or SortDeltasStats) if chronological order is required downstream.
//
// Merge is nil-safe: if either argument is nil, it is treated as an
// empty series. Merge(nil, nil) returns the zero TimeSeries.
func Merge(tsa *TimeSeries, tsb *TimeSeries) TimeSeries {
	var tsr TimeSeries
	na, nb := 0, 0
	if tsa != nil {
		na = len(tsa.DataSeries)
	}
	if tsb != nil {
		nb = len(tsb.DataSeries)
	}
	tsr.DataSeries = make([]DataUnit, 0, na+nb)
	if tsa != nil {
		tsr.DataSeries = append(tsr.DataSeries, tsa.DataSeries...)
	}
	if tsb != nil {
		tsr.DataSeries = append(tsr.DataSeries, tsb.DataSeries...)
	}
	switch {
	case tsa != nil && tsa.Name != "":
		tsr.Name = tsa.Name
	case tsb != nil:
		tsr.Name = tsb.Name
	}
	switch {
	case tsa != nil && tsa.Comment != "":
		tsr.Comment = tsa.Comment
	case tsb != nil:
		tsr.Comment = tsb.Comment
	}
	return tsr
}

// RemovedNonValid partitions the series into (cleaned, rejected) based on
// the validity of the Meas field. Points whose Meas is a finite, non-NaV
// float64 go to the cleaned series; points whose Meas is NaV, a plain NaN,
// or an infinity are returned in the rejected series. Relative order within
// each output is preserved; the input series is not modified.
//
// This is the canonical way to materialize the NaV-awareness of the library
// as two distinct slices — typically useful right before an external export
// that cannot represent missingness.
func (ts *TimeSeries) RemovedNonValid() (TimeSeries, TimeSeries) {
	var cleanedts, rejectedts TimeSeries
	cleanedts.Name = ts.Name
	cleanedts.Comment = ts.Comment
	rejectedts.Name = ts.Name + " (rejected)"
	for _, du := range ts.DataSeries {
		if math.IsNaN(du.Meas) || math.IsInf(du.Meas, 0) {
			rejectedts.DataSeries = append(rejectedts.DataSeries, du)
		} else {
			cleanedts.DataSeries = append(cleanedts.DataSeries, du)
		}
	}
	return cleanedts, rejectedts
}

// RemoveElements removes the elements at the provided indices.
// It returns (cleaned, rejected), where rejected contains exactly the
// points located at indices in idx (duplicates are ignored). Indices
// outside [0,len-1] are skipped. The relative order of remaining points
// is preserved in cleaned.
func (tsin *TimeSeries) RemoveElements(idx ...int) (TimeSeries, TimeSeries) {
	var cleanedts, rejectedts TimeSeries
	if len(tsin.DataSeries) == 0 || len(idx) == 0 {
		// Nothing to remove: everything passes through to cleaned.
		cleanedts.DataSeries = append(cleanedts.DataSeries, tsin.DataSeries...)
		return cleanedts, rejectedts
	}

	// Build a set of indices to reject: O(1) lookup, dedup for free.
	rejectIdx := make(map[int]struct{}, len(idx))
	for _, i := range idx {
		if i >= 0 && i < len(tsin.DataSeries) {
			rejectIdx[i] = struct{}{}
		}
	}

	for i, du := range tsin.DataSeries {
		if _, ok := rejectIdx[i]; ok {
			rejectedts.AddDataUnit(du)
		} else {
			cleanedts.AddDataUnit(du)
		}
	}
	return cleanedts, rejectedts
}

// LimitSize truncates the series to at most limit elements, in place.
// If limit >= len(DataSeries) the series is left unchanged; if limit <= 0
// the series is emptied (capacity is preserved). The truncation
// invalidates DeltasValid unless it was a no-op.
func (tsin *TimeSeries) LimitSize(limit int) {
	if limit < 0 {
		limit = 0
	}
	if limit > len(tsin.DataSeries) {
		return
	}
	tsin.DataSeries = tsin.DataSeries[:limit]
	tsin.invalidateDeltas()
}

// FindInIntSlice returns the index of val in slice, or -1 if not found.
// The search is linear and the slice is not modified.
func FindInIntSlice(slice []int, val int) int {
	for i, item := range slice {
		if item == val {
			return i
		}
	}
	return -1
}

// FindInFloatSlice returns the index of val in slice, or -1 if not found.
// The search is linear and the slice is not modified.
func FindInFloatSlice(slice []float64, val float64) int {
	for i, item := range slice {
		if item == val {
			return i
		}
	}
	return -1
}

// ChronToArr returns a newly allocated []time.Time with the Chron field
// of each DataUnit, in the current order of the series.
func (ts TimeSeries) ChronToArr() []time.Time {
	var measarr []time.Time
	for index, _ := range ts.DataSeries {
		measarr = append(measarr, ts.DataSeries[index].Chron)
	}
	return measarr
}

// MeasToArr returns a newly allocated []float64 with the Meas field
// of each DataUnit, in the current order of the series.
func (ts TimeSeries) MeasToArr() []float64 {
	var measarr []float64
	for index, _ := range ts.DataSeries {
		measarr = append(measarr, ts.DataSeries[index].Meas)
	}
	return measarr
}
