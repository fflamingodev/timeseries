package timeseries

import (
	"math"
	"time"
)

// AggFunc is the per-bucket aggregator used by Regularize and the
// Downscale* family. It receives the slice of Meas values falling in the
// current bucket (in chronological order) and returns a single float64.
//
// An AggFunc is responsible for NaV-awareness: the bucket may contain
// NaV/NaN entries and the aggregator decides whether to skip them,
// propagate them, or fold them in. All built-in aggregators in this file
// skip NaV/NaN, matching the aggregate semantics documented in
// navdefinition.go.
type AggFunc func(local []float64) float64

// AggAverage returns the NaV-skipping arithmetic mean of the bucket.
func AggAverage(local []float64) float64 {
	return Mean(local)
}

// AggMaximum returns the NaV-skipping maximum of the bucket.
func AggMaximum(local []float64) float64 {
	_, maxv := Bounds(local)
	return maxv
}

// AggMinimum returns the NaV-skipping minimum of the bucket.
func AggMinimum(local []float64) float64 {
	minv, _ := Bounds(local)
	return minv
}

// AggLast returns the chronologically last value in the bucket. If the
// bucket is empty, it returns NaV. NaV/NaN entries count as values and
// may be returned as-is; use AggLastValid if you want the last non-NaV.
func AggLast(local []float64) float64 {
	if len(local) == 0 {
		return NaV
	}
	return local[len(local)-1]
}

// AggOpen returns the chronologically first value in the bucket. If the
// bucket is empty, it returns NaV.
func AggOpen(local []float64) float64 {
	if len(local) == 0 {
		return NaV
	}
	return local[0]
}

// AggCountValid returns the number of non-NaN, non-NaV values in the
// bucket, as a float64.
func AggCountValid(local []float64) float64 {
	count := 0
	for _, v := range local {
		if math.IsNaN(v) {
			continue
		}
		count++
	}
	return float64(count)
}

// AggMedian returns the NaV-skipping median of the bucket. If the bucket
// contains no valid value, it returns NaV.
func AggMedian(local []float64) float64 {
	m, err := Median(local)
	if err != nil {
		return NaV
	}
	return m
}

// AggSlope returns the slope of the linear regression y = a + b*x fitted
// on the bucket, with x = 0, 1, ..., n-1 (index-based, not time-based:
// callers using AggSlope on an irregular series should regularize first).
// NaV and NaN values are skipped. If fewer than 2 valid points remain, or
// if the design matrix is degenerate, AggSlope returns NaV.
func AggSlope(local []float64) float64 {
	n := len(local)
	if n < 2 {
		return NaV
	}

	var (
		sumX, sumY, sumXY, sumX2 float64
		count                    float64
	)
	for i, v := range local {
		if math.IsNaN(v) {
			continue
		}
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumX2 += x * x
		count++
	}
	if count < 2 {
		return NaV
	}

	num := count*sumXY - sumX*sumY
	den := count*sumX2 - sumX*sumX
	if den == 0 {
		return NaV
	}
	return num / den
}

// AggIntegral builds an aggregator that returns a rectangle-rule estimate
// of the area under the bucket: sum(non-NaV values) * freq.Seconds().
// It assumes each sample represents one freq-wide slot; for irregular
// input, regularize first.
func AggIntegral(freq time.Duration) AggFunc {
	dt := freq.Seconds()
	return func(local []float64) float64 {
		sum := 0.0
		any := false
		for _, v := range local {
			if math.IsNaN(v) {
				continue
			}
			sum += v
			any = true
		}
		if !any {
			return NaV
		}
		return sum * dt
	}
}

// AggIncrementalCounter builds a stateful aggregator for monotonically
// increasing counters (e.g. electricity meters): for each bucket, it
// returns last(bucket) - last(previous bucket). The first emitted bucket
// has no previous reference and returns NaV.
//
// The returned AggFunc is not safe for concurrent use.
func AggIncrementalCounter() AggFunc {
	var prevLast *float64

	return func(local []float64) float64 {
		if len(local) == 0 {
			return NaV
		}
		curLast := local[len(local)-1]

		if prevLast == nil {
			val := curLast
			prevLast = &val
			return NaV
		}

		delta := Sub(curLast, *prevLast)
		*prevLast = curLast
		return delta
	}
}
