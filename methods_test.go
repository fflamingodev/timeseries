package timeseries

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DeltasFiller
// ---------------------------------------------------------------------------

func TestDeltasFiller_firstElementIsSentinel(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 1)
	ts.AddData(tFromSec(10), 2)
	ts.SortChronAsc()
	ts.DeltasFiller()

	if ts.DataSeries[0].Dchron != NaDuration {
		t.Errorf("DataSeries[0].Dchron = %v, want NaDuration", ts.DataSeries[0].Dchron)
	}
	if !IsNaV(ts.DataSeries[0].Dmeas) {
		t.Errorf("DataSeries[0].Dmeas = %s, want NaV", Format(ts.DataSeries[0].Dmeas))
	}
}

func TestDeltasFiller_computesIntervals(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 10)
	ts.AddData(tFromSec(25), 13)
	ts.AddData(tFromSec(60), 12)
	ts.SortChronAsc()
	ts.DeltasFiller()

	want := []time.Duration{NaDuration, 25 * time.Second, 35 * time.Second}
	for i, w := range want {
		if ts.DataSeries[i].Dchron != w {
			t.Errorf("DataSeries[%d].Dchron = %v, want %v", i, ts.DataSeries[i].Dchron, w)
		}
	}
	wantDmeas := []float64{NaV, 3, -1}
	for i, w := range wantDmeas {
		if !floatEqual(ts.DataSeries[i].Dmeas, w) {
			t.Errorf("DataSeries[%d].Dmeas = %s, want %s", i, Format(ts.DataSeries[i].Dmeas), Format(w))
		}
	}
}

func TestDeltasFiller_propagatesNaVThroughSub(t *testing.T) {
	// If a Meas is NaV, Dmeas for the next point must be NaV too (strict
	// propagation via Sub).
	var ts TimeSeries
	ts.AddData(tFromSec(0), 10)
	ts.AddData(tFromSec(10), NaV)
	ts.AddData(tFromSec(20), 13)
	ts.SortChronAsc()
	ts.DeltasFiller()

	// DataSeries[1].Dmeas = 10 - NaV.Meas = NaV? Wait: Sub(Meas[i], Meas[i-1])
	// = Sub(NaV, 10) = NaV.
	if !IsNaV(ts.DataSeries[1].Dmeas) {
		t.Errorf("DataSeries[1].Dmeas = %s, want NaV (NaV - 10)", Format(ts.DataSeries[1].Dmeas))
	}
	// DataSeries[2].Dmeas = Sub(13, NaV) = NaV.
	if !IsNaV(ts.DataSeries[2].Dmeas) {
		t.Errorf("DataSeries[2].Dmeas = %s, want NaV (13 - NaV)", Format(ts.DataSeries[2].Dmeas))
	}
}

// ---------------------------------------------------------------------------
// ComputeBasicStats
// ---------------------------------------------------------------------------

func TestComputeBasicStats_counts(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 1)
	ts.AddData(tFromSec(1), 2)
	ts.AddData(tFromSec(2), NaV) // NaV → counts in both NbreOfNaN and NbreOfNaV
	ts.AddData(tFromSec(3), 4)
	ts.SortDeltasStats()

	if ts.Len != 4 {
		t.Errorf("Len = %d, want 4", ts.Len)
	}
	if ts.NbreOfNaN != 1 {
		t.Errorf("NbreOfNaN = %d, want 1 (NaV is a NaN)", ts.NbreOfNaN)
	}
	if ts.NbreOfNaV != 1 {
		t.Errorf("NbreOfNaV = %d, want 1", ts.NbreOfNaV)
	}
}

func TestComputeBasicStats_distinguishesNaNFromNaV(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 1)
	ts.AddData(tFromSec(1), NaNumber) // plain NaN
	ts.AddData(tFromSec(2), NaV)      // NaV
	ts.SortDeltasStats()

	if ts.NbreOfNaN != 2 {
		t.Errorf("NbreOfNaN = %d, want 2 (plain NaN + NaV)", ts.NbreOfNaN)
	}
	if ts.NbreOfNaV != 1 {
		t.Errorf("NbreOfNaV = %d, want 1 (only the NaV)", ts.NbreOfNaV)
	}
}

func TestComputeBasicStats_minMaxIndicesPreserved(t *testing.T) {
	// Minimum should carry its corresponding Chron even after the series
	// has been sorted chronologically (no stale index).
	var ts TimeSeries
	ts.AddData(tFromSec(0), 5)
	ts.AddData(tFromSec(10), 1) // min
	ts.AddData(tFromSec(20), 9) // max
	ts.AddData(tFromSec(30), 3)
	ts.SortDeltasStats()

	if !ts.ChAtMsmin.Equal(tFromSec(10)) {
		t.Errorf("ChAtMsmin = %v, want %v", ts.ChAtMsmin, tFromSec(10))
	}
	if !ts.ChAtMsmax.Equal(tFromSec(20)) {
		t.Errorf("ChAtMsmax = %v, want %v", ts.ChAtMsmax, tFromSec(20))
	}
	assertFloat(t, ts.Msmin, 1, "Msmin")
	assertFloat(t, ts.Msmax, 9, "Msmax")
}

func TestComputeBasicStats_allNaV(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), NaV)
	ts.AddData(tFromSec(1), NaV)
	ts.SortDeltasStats()

	if !IsNaV(ts.Msmin) || !IsNaV(ts.Msmax) || !IsNaV(ts.Msmean) {
		t.Errorf("all-NaV series: stats should be NaV, got min=%s max=%s mean=%s",
			Format(ts.Msmin), Format(ts.Msmax), Format(ts.Msmean))
	}
}

// ---------------------------------------------------------------------------
// Copy
// ---------------------------------------------------------------------------

func TestCopy_isDeepAndPreservesMemId(t *testing.T) {
	src := TimeSeries{
		MemId:   1234,
		Name:    "original",
		Comment: "first",
	}
	src.AddData(tFromSec(0), 1)
	src.AddData(tFromSec(10), 2)
	src.SortDeltasStats()

	cp := src.Copy()
	if cp.MemId != 1234 {
		t.Errorf("MemId not copied: got %d, want 1234", cp.MemId)
	}
	if cp.Name != "original" {
		t.Errorf("Name not copied: got %q", cp.Name)
	}
	if !cp.DeltasValid() {
		t.Error("Copy output should have DeltasValid == true (Sort_Deltas_Stats is called internally)")
	}

	// Mutate original, ensure copy is untouched.
	src.AddData(tFromSec(20), 3)
	if len(cp.DataSeries) != 2 {
		t.Errorf("Copy shared backing slice: len=%d after src.AddData", len(cp.DataSeries))
	}
}

// ---------------------------------------------------------------------------
// Sort invalidation then re-validation
// ---------------------------------------------------------------------------

func TestSort_thenSortDeltasStats_reValidates(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(30), 3)
	ts.AddData(tFromSec(10), 1)
	ts.AddData(tFromSec(20), 2)
	ts.SortDeltasStats()

	// SortMeasAsc breaks chronological order and stale-ifies Dchron/Dmeas.
	ts.SortMeasAsc()
	if ts.DeltasValid() {
		t.Error("SortMeasAsc did not invalidate deltas")
	}

	// Re-running SortDeltasStats restores chronological order and deltas.
	ts.SortDeltasStats()
	if !ts.DeltasValid() {
		t.Error("SortDeltasStats did not re-validate")
	}
	for i := 1; i < len(ts.DataSeries); i++ {
		if ts.DataSeries[i].Chron.Before(ts.DataSeries[i-1].Chron) {
			t.Errorf("series not chronologically sorted at %d", i)
		}
	}
}
