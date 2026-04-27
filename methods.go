package timeseries

import (
	"math"
	"sort"
	"time"
)

// AddData appends a DataUnit built from chr and meas to ts. Dchron and
// Dmeas are initialized to their "no predecessor" sentinels via
// NewDataUnit. DeltasValid is cleared; call SortDeltasStats once all
// points have been inserted to populate deltas and stats.
func (ts *TimeSeries) AddData(chr time.Time, meas float64) {
	ts.DataSeries = append(ts.DataSeries, NewDataUnit(chr, meas))
	ts.invalidateDeltas()
}

// NewTsContainer returns an empty TsContainer with its inner map initialized,
// ready to hold named TimeSeries.
func NewTsContainer() (tsd TsContainer) {
	tsd.Ts = make(map[string]*TimeSeries)
	return
}

// SortChronAsc sorts the series by Chron in ascending order, in place. It
// invalidates DeltasValid (the deltas were computed against the previous
// order and must be rebuilt via DeltasFiller or SortDeltasStats).
func (ts *TimeSeries) SortChronAsc() {
	sort.Slice(ts.DataSeries, func(i, j int) bool {
		return ts.DataSeries[i].Chron.Before(ts.DataSeries[j].Chron)
	})
	ts.invalidateDeltas()
}

// SortChronDesc sorts the series by Chron in descending order, in place.
// It invalidates DeltasValid.
func (ts *TimeSeries) SortChronDesc() {
	sort.Slice(ts.DataSeries, func(i, j int) bool {
		return ts.DataSeries[i].Chron.After(ts.DataSeries[j].Chron)
	})
	ts.invalidateDeltas()
}

// SortMeasAsc sorts the series by Meas in ascending order, in place. NaV
// and NaN values are placed at the end (standard Go sort on NaN-bearing
// floats via the default less operator). It invalidates DeltasValid.
func (ts *TimeSeries) SortMeasAsc() {
	sort.Slice(ts.DataSeries, func(i, j int) bool {
		return ts.DataSeries[i].Meas < ts.DataSeries[j].Meas
	})
	ts.invalidateDeltas()
}

// SortMeasDesc sorts the series by Meas in descending order, in place. It
// invalidates DeltasValid.
func (ts *TimeSeries) SortMeasDesc() {
	sort.Slice(ts.DataSeries, func(i, j int) bool {
		return ts.DataSeries[i].Meas > ts.DataSeries[j].Meas
	})
	ts.invalidateDeltas()
}

// Reset truncates the DataSeries to zero length while preserving capacity
// and all series-level metadata (Name, Comment, MemId, BasicStats).
// DeltasValid is cleared.
func (ts *TimeSeries) Reset() {
	ts.DataSeries = ts.DataSeries[:0]
	ts.invalidateDeltas()
}

// Copy returns a deep copy of ts: Name, Comment, MemId are copied, and a
// fresh DataSeries backing array is allocated. SortDeltasStats is then
// invoked on the copy so the returned value has valid deltas and
// stats. Calling Copy on a nil receiver returns the zero TimeSeries.
func (ts *TimeSeries) Copy() TimeSeries {
	if ts == nil {
		return TimeSeries{}
	}
	out := TimeSeries{
		MemId:   ts.MemId,
		Name:    ts.Name,
		Comment: ts.Comment,
	}
	out.DataSeries = make([]DataUnit, len(ts.DataSeries))
	copy(out.DataSeries, ts.DataSeries)
	out.SortDeltasStats()
	return out
}

// DeltasFiller populates Dchron and Dmeas on each DataUnit from consecutive
// pairs. The first element is marked as "no predecessor" via NaDuration and
// NaV. If two consecutive Meas values include a NaN/NaV, the resulting
// Dmeas propagates NaV (strict propagation semantics; see navdefinition.go).
//
// DeltasFiller does not sort; call SortChronAsc first, or use
// Sort_Deltas_Stats which does sort + fill + stats in one call.
func (ts *TimeSeries) DeltasFiller() {
	n := len(ts.DataSeries)
	if n == 0 {
		return
	}
	ts.DataSeries[0].Dchron = NaDuration
	ts.DataSeries[0].Dmeas = NaV
	for i := 1; i < n; i++ {
		ts.DataSeries[i].Dchron = ts.DataSeries[i].Chron.Sub(ts.DataSeries[i-1].Chron)
		ts.DataSeries[i].Dmeas = Sub(ts.DataSeries[i].Meas, ts.DataSeries[i-1].Meas)
	}
}

// Sort_Deltas_Stats runs the full "fresh state" pipeline: sort ascending,
// fill deltas, compute BasicStats. Idempotent on stable input.
//
// Deprecated: kept for backward compatibility. Prefer SortDeltasStats.
func (ts *TimeSeries) Sort_Deltas_Stats() {
	ts.SortDeltasStats()
}

// SortDeltasStats is the idiomatic Go name for the Sort_Deltas_Stats
// pipeline. It sorts the series chronologically, fills Dchron/Dmeas,
// and (re)computes BasicStats. On successful completion DeltasValid is
// true. Calling SortDeltasStats on a nil receiver is a no-op (safe).
func (ts *TimeSeries) SortDeltasStats() {
	if ts == nil {
		return
	}
	if len(ts.DataSeries) == 0 {
		ts.Comment = "Warning: Empty Time Series"
		ts.deltasValid = false
		return
	}
	ts.SortChronAsc()
	ts.DeltasFiller()
	ts.ComputeBasicStats()
	ts.deltasValid = true
}

// ComputeBasicStats recomputes the summary statistics on the series. NaN
// and NaV values are excluded from numeric aggregates; both are counted in
// NbreOfNaN (for backward compatibility), with NbreOfNaV tracking only NaV
// separately so callers can distinguish "missing" from "invalid".
func (ts *TimeSeries) ComputeBasicStats() {
	n := len(ts.DataSeries)
	if n == 0 {
		ts.Comment = "Warning: Empty Time Series"
		return
	}

	// --- Counters on Meas: total NaN and NaV specifically --------------
	ts.NbreOfNaN = 0
	ts.NbreOfNaV = 0
	for _, v := range ts.DataSeries {
		if math.IsNaN(v.Meas) {
			ts.NbreOfNaN++
			if IsNaV(v.Meas) {
				ts.NbreOfNaV++
			}
		}
	}

	// --- Chron stats ----------------------------------------------------
	ts.Chmin = ts.DataSeries[0].Chron
	ts.ValAtChmin = ts.DataSeries[0].Meas
	ts.Chmax = ts.DataSeries[n-1].Chron
	ts.ValAtChmax = ts.DataSeries[n-1].Meas

	chr := make([]float64, n)
	for i, v := range ts.DataSeries {
		chr[i] = float64(v.Chron.UnixNano())
	}
	ts.Len = n
	ts.Chmean = time.Unix(0, int64(Mean(chr)))
	med, _ := Median(chr)
	ts.Chmed = time.Unix(0, int64(med))

	// --- Dchron stats (second element onwards) --------------------------
	var (
		dch        []float64
		haveDch    bool
		minDch     time.Duration
		maxDch     time.Duration
		chAtMinDch time.Time
		chAtMaxDch time.Time
	)
	for i := 1; i < n; i++ {
		d := ts.DataSeries[i].Dchron
		if d == NaDuration {
			continue
		}
		dch = append(dch, float64(d))
		if !haveDch {
			haveDch = true
			minDch, maxDch = d, d
			chAtMinDch = ts.DataSeries[i].Chron
			chAtMaxDch = ts.DataSeries[i].Chron
			continue
		}
		if d < minDch {
			minDch = d
			chAtMinDch = ts.DataSeries[i].Chron
		}
		if d > maxDch {
			maxDch = d
			chAtMaxDch = ts.DataSeries[i].Chron
		}
	}
	if haveDch {
		ts.DChmin = minDch
		ts.ChAtDChmin = chAtMinDch
		ts.DChmax = maxDch
		ts.ChAtDchmax = chAtMaxDch
		ts.DChmean = Mean(dch)
		ts.DChmed, _ = Median(dch)
		ts.DChstd, _ = StdDev(dch)
	}

	// --- Meas stats: single pass with index capture --------------------
	var (
		meas       []float64
		seenMs     bool
		msMin      float64
		msMax      float64
		chAtMsMin  time.Time
		chAtMsMax  time.Time
	)
	for _, v := range ts.DataSeries {
		if math.IsNaN(v.Meas) {
			continue
		}
		meas = append(meas, v.Meas)
		if !seenMs {
			seenMs = true
			msMin, msMax = v.Meas, v.Meas
			chAtMsMin = v.Chron
			chAtMsMax = v.Chron
			continue
		}
		if v.Meas < msMin {
			msMin = v.Meas
			chAtMsMin = v.Chron
		}
		if v.Meas > msMax {
			msMax = v.Meas
			chAtMsMax = v.Chron
		}
	}
	if seenMs {
		ts.Msmin = msMin
		ts.Msmax = msMax
		ts.ChAtMsmin = chAtMsMin
		ts.ChAtMsmax = chAtMsMax
		ts.Msmean = Mean(meas)
		ts.Msmed, _ = Median(meas)
		ts.Msstd, _ = StdDev(meas)
	} else {
		ts.Msmin, ts.Msmax = NaV, NaV
		ts.Msmean, ts.Msmed, ts.Msstd = NaV, NaV, NaV
	}

	// --- Dmeas stats (skip the first point whose Dmeas is NaV) ----------
	var dmeas []float64
	for i := 1; i < n; i++ {
		if math.IsNaN(ts.DataSeries[i].Dmeas) {
			continue
		}
		dmeas = append(dmeas, ts.DataSeries[i].Dmeas)
	}
	if len(dmeas) > 0 {
		ts.DMsmin, _ = Min(dmeas)
		ts.DMsmax, _ = Max(dmeas)
		ts.DMsmean = Mean(dmeas)
		ts.DMsmed, _ = Median(dmeas)
		ts.DMsstd, _ = StdDev(dmeas)
		ts.Comment = " Time Series ok."
	} else {
		ts.DMsmin, ts.DMsmax = NaV, NaV
		ts.DMsmean, ts.DMsmed, ts.DMsstd = NaV, NaV, NaV
	}
}
