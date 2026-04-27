package timeseries

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Datum / DataUnit construction defaults
// ---------------------------------------------------------------------------

func TestNewDatum(t *testing.T) {
	d := NewDatum(tFromSec(10), 42)
	if !d.Chron.Equal(tFromSec(10)) {
		t.Errorf("Chron mismatch: %v", d.Chron)
	}
	if d.Meas != 42 {
		t.Errorf("Meas = %v, want 42", d.Meas)
	}
}

func TestNewDataUnit_deltaSentinelDefaults(t *testing.T) {
	// A freshly-constructed DataUnit must visibly say "I do not know my
	// deltas" via NaDuration / NaV — not pretend they are 0.
	du := NewDataUnit(tFromSec(10), 42)
	if !du.Chron.Equal(tFromSec(10)) {
		t.Errorf("Chron mismatch: %v", du.Chron)
	}
	if du.Meas != 42 {
		t.Errorf("Meas = %v, want 42", du.Meas)
	}
	if du.Dchron != NaDuration {
		t.Errorf("Dchron default = %v, want NaDuration", du.Dchron)
	}
	if !IsNaV(du.Dmeas) {
		t.Errorf("Dmeas default should be NaV, got %s", Format(du.Dmeas))
	}
}

func TestDataUnit_embedsDatum(t *testing.T) {
	// Field promotion: du.Chron and du.Meas must be reachable through the
	// outer DataUnit as if the Datum layer were transparent.
	du := NewDataUnit(tFromSec(5), 3.14)
	if du.Datum.Chron != du.Chron {
		t.Error("embedded Datum.Chron and promoted Chron disagree")
	}
	if du.Datum.Meas != du.Meas {
		t.Error("embedded Datum.Meas and promoted Meas disagree")
	}
}

// ---------------------------------------------------------------------------
// DeltasValid lifecycle
// ---------------------------------------------------------------------------

func TestDeltasValid_freshSeriesIsInvalid(t *testing.T) {
	var ts TimeSeries
	if ts.DeltasValid() {
		t.Error("fresh TimeSeries should have DeltasValid == false")
	}
}

func TestDeltasValid_setByAddData(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 1)
	if ts.DeltasValid() {
		t.Error("AddData must leave DeltasValid false until SortDeltasStats")
	}
}

func TestDeltasValid_validatedBySortDeltasStats(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 1)
	ts.AddData(tFromSec(10), 2)
	ts.SortDeltasStats()
	if !ts.DeltasValid() {
		t.Error("SortDeltasStats should set DeltasValid true on non-empty series")
	}
}

func TestDeltasValid_invalidatedBySort(t *testing.T) {
	var ts TimeSeries
	ts.AddData(tFromSec(0), 3)
	ts.AddData(tFromSec(10), 1)
	ts.AddData(tFromSec(20), 2)
	ts.SortDeltasStats()
	if !ts.DeltasValid() {
		t.Fatal("precondition: DeltasValid should be true after SortDeltasStats")
	}
	ts.SortMeasAsc()
	if ts.DeltasValid() {
		t.Error("SortMeasAsc must invalidate deltas")
	}
}

func TestDeltasValid_invalidatedByMutations(t *testing.T) {
	build := func() *TimeSeries {
		ts := &TimeSeries{}
		ts.AddData(tFromSec(0), 1)
		ts.AddData(tFromSec(10), 2)
		ts.SortDeltasStats()
		return ts
	}

	cases := []struct {
		name string
		act  func(*TimeSeries)
	}{
		{"AddData", func(ts *TimeSeries) { ts.AddData(tFromSec(20), 3) }},
		{"AddDataUnit", func(ts *TimeSeries) { ts.AddDataUnit(NewDataUnit(tFromSec(20), 3)) }},
		{"AddDatum", func(ts *TimeSeries) { ts.AddDatum(NewDatum(tFromSec(20), 3)) }},
		{"SortChronAsc", func(ts *TimeSeries) { ts.SortChronAsc() }},
		{"SortChronDesc", func(ts *TimeSeries) { ts.SortChronDesc() }},
		{"SortMeasAsc", func(ts *TimeSeries) { ts.SortMeasAsc() }},
		{"SortMeasDesc", func(ts *TimeSeries) { ts.SortMeasDesc() }},
		{"Reset", func(ts *TimeSeries) { ts.Reset() }},
		{"LimitSize", func(ts *TimeSeries) { ts.LimitSize(1) }},
		{"Interpolate", func(ts *TimeSeries) { _ = ts.Interpolate(InterpLinear) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := build()
			c.act(ts)
			if ts.DeltasValid() {
				t.Errorf("%s should invalidate DeltasValid", c.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AddDatum ingestion path
// ---------------------------------------------------------------------------

func TestAddDatum_populatesSeriesWithSentinelDeltas(t *testing.T) {
	var ts TimeSeries
	ts.AddDatum(
		NewDatum(tFromSec(0), 1),
		NewDatum(tFromSec(10), 2),
	)
	if len(ts.DataSeries) != 2 {
		t.Fatalf("len = %d, want 2", len(ts.DataSeries))
	}
	for i, du := range ts.DataSeries {
		if du.Dchron != NaDuration {
			t.Errorf("point %d: Dchron = %v, want NaDuration", i, du.Dchron)
		}
		if !IsNaV(du.Dmeas) {
			t.Errorf("point %d: Dmeas = %s, want NaV", i, Format(du.Dmeas))
		}
	}
}
