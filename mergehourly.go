package timeseries

import (
	"math"
	"time"
)

// MergeHourlyFFill takes a set of input TimeSeries (typically one per
// signal to be summed — e.g. one per pollen species), regularizes each
// onto a shared hourly grid, forward-fills the gaps, and returns a
// single TimeSeries that is the pointwise sum of all aligned series.
//
// Algorithm:
//  1. Compute the hour-rounded envelope [min, max] covering all inputs.
//  2. Build an hourly grid spanning that envelope.
//  3. For each input series: drop each measurement into its nearest
//     hour bucket (averaging inside the bucket when multiple samples
//     land there), then forward-fill the NaN holes.
//  4. Sum all aligned series hour by hour; skip hours where no series
//     has a value.
func MergeHourlyFFill(series map[string]*TimeSeries) *TimeSeries {
	if len(series) == 0 {
		return &TimeSeries{Name: "pollen_sum", Comment: "no input series"}
	}

	// 1. Time envelope.
	var tMin, tMax time.Time
	first := true
	for _, ts := range series {
		if len(ts.DataSeries) == 0 {
			continue
		}
		ts.SortChronAsc()
		lo := ts.DataSeries[0].Chron
		hi := ts.DataSeries[len(ts.DataSeries)-1].Chron
		if first || lo.Before(tMin) {
			tMin = lo
		}
		if first || hi.After(tMax) {
			tMax = hi
		}
		first = false
	}
	if first {
		// All input series are empty.
		return &TimeSeries{Name: "pollen_sum", Comment: "all input series empty"}
	}

	// Round to whole hours; include the last hour.
	tMin = tMin.Truncate(time.Hour)
	tMax = tMax.Truncate(time.Hour).Add(time.Hour)

	// 2. Hourly grid.
	nHours := int(tMax.Sub(tMin)/time.Hour) + 1
	grid := make([]time.Time, nHours)
	for i := range grid {
		grid[i] = tMin.Add(time.Duration(i) * time.Hour)
	}

	// 3. Regularize + forward-fill each series onto a slice aligned on
	// grid.
	aligned := make([][]float64, 0, len(series))

	for _, ts := range series {
		vals := make([]float64, nHours)
		counts := make([]int, nHours)
		for i := range vals {
			vals[i] = NaV
		}

		for _, du := range ts.DataSeries {
			if math.IsNaN(du.Meas) {
				continue
			}
			idx := int(du.Chron.Sub(tMin) / time.Hour)
			if idx < 0 || idx >= nHours {
				continue
			}
			if counts[idx] == 0 {
				vals[idx] = du.Meas
			} else {
				// Running mean.
				vals[idx] = (vals[idx]*float64(counts[idx]) + du.Meas) / float64(counts[idx]+1)
			}
			counts[idx]++
		}

		// Forward-fill: NaN-class entries inherit the previous slot's value.
		for i := 1; i < nHours; i++ {
			if math.IsNaN(vals[i]) && !math.IsNaN(vals[i-1]) {
				vals[i] = vals[i-1]
			}
		}

		aligned = append(aligned, vals)
	}

	// 4. Pointwise sum.
	out := &TimeSeries{
		Name:    "pollen_sum",
		Comment: "hourly sum of all species (forward-filled)",
	}

	for i, t := range grid {
		sum := 0.0
		anyValid := false
		for _, a := range aligned {
			v := a[i]
			if !math.IsNaN(v) {
				sum += v
				anyValid = true
			}
		}
		if anyValid {
			out.AddData(t, sum)
		}
		// If no species has a value at this hour, the hour is skipped.
	}

	out.SortDeltasStats()
	return out
}

// MergeHourlyPerSpecies takes a set of input TimeSeries and regularizes
// each onto the SAME shared hourly grid with forward-fill, but unlike
// MergeHourlyFFill it does NOT sum them: it returns a map keyed by the
// input keys, each entry being a TimeSeries aligned on the common grid.
// This is the shape a frontend needs to render a stacked bar chart
// where each layer is one input signal.
//
// All returned TimeSeries share the same time vector (the hourly grid).
// Hours where NO series has a valid value are removed from every
// output so the grid stays compact.
func MergeHourlyPerSpecies(series map[string]*TimeSeries) map[string]*TimeSeries {
	if len(series) == 0 {
		return map[string]*TimeSeries{}
	}

	// 1. Time envelope.
	var tMin, tMax time.Time
	first := true
	for _, ts := range series {
		if ts == nil || len(ts.DataSeries) == 0 {
			continue
		}
		ts.SortChronAsc()
		lo := ts.DataSeries[0].Chron
		hi := ts.DataSeries[len(ts.DataSeries)-1].Chron
		if first || lo.Before(tMin) {
			tMin = lo
		}
		if first || hi.After(tMax) {
			tMax = hi
		}
		first = false
	}
	if first {
		return map[string]*TimeSeries{}
	}

	tMin = tMin.Truncate(time.Hour)
	tMax = tMax.Truncate(time.Hour).Add(time.Hour)

	nHours := int(tMax.Sub(tMin)/time.Hour) + 1
	grid := make([]time.Time, nHours)
	for i := range grid {
		grid[i] = tMin.Add(time.Duration(i) * time.Hour)
	}

	// 2. Regularize + forward-fill each series.
	aligned := make(map[string][]float64, len(series))
	for key, ts := range series {
		vals := make([]float64, nHours)
		counts := make([]int, nHours)
		for i := range vals {
			vals[i] = NaV
		}

		if ts != nil {
			for _, du := range ts.DataSeries {
				if math.IsNaN(du.Meas) {
					continue
				}
				idx := int(du.Chron.Sub(tMin) / time.Hour)
				if idx < 0 || idx >= nHours {
					continue
				}
				if counts[idx] == 0 {
					vals[idx] = du.Meas
				} else {
					vals[idx] = (vals[idx]*float64(counts[idx]) + du.Meas) / float64(counts[idx]+1)
				}
				counts[idx]++
			}
		}

		// Forward-fill.
		for i := 1; i < nHours; i++ {
			if math.IsNaN(vals[i]) && !math.IsNaN(vals[i-1]) {
				vals[i] = vals[i-1]
			}
		}
		aligned[key] = vals
	}

	// 3. Drop hours where no species has a valid value.
	keep := make([]bool, nHours)
	for i := 0; i < nHours; i++ {
		for _, vals := range aligned {
			if !math.IsNaN(vals[i]) {
				keep[i] = true
				break
			}
		}
	}

	// 4. Build the output TimeSeries aligned on the compacted grid.
	out := make(map[string]*TimeSeries, len(series))
	for key, vals := range aligned {
		ts := &TimeSeries{
			Name:    key,
			Comment: "hourly aligned + forward-filled",
		}
		for i, t := range grid {
			if !keep[i] {
				continue
			}
			v := vals[i]
			if math.IsNaN(v) {
				// Before a species' first observation, treat the value
				// as 0 so the stacked bar chart remains visually
				// consistent across layers.
				v = 0
			}
			ts.AddData(t, v)
		}
		ts.SortDeltasStats()
		out[key] = ts
	}

	return out
}
