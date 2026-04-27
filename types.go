package timeseries

import (
	"time"
)

// Datum is the "pure" observation type: a timestamp and a measured value,
// independent of any series context. Use it as the input type of code that
// ingests raw samples from a sensor, a CSV row, a database column, or an
// unmarshalled JSON payload — i.e. anywhere "what just arrived" is
// expressed without reference to a predecessor.
//
// A Datum has no notion of delta. Promoting a Datum to a DataUnit and
// placing it in a TimeSeries is what gives it a predecessor and, via
// DeltasFiller / SortDeltasStats, the corresponding Dchron and Dmeas.
type Datum struct {
	Chron time.Time
	Meas  float64
}

// DataUnit is a Datum enriched with its first-order deltas against the
// previous element of a sorted TimeSeries.
//
//   - The embedded Datum (Chron, Meas) is intrinsic to the observation.
//   - Dchron and Dmeas are derived fields. Their canonical values are set
//     by DeltasFiller (called by SortDeltasStats). Outside of a
//     chronologically sorted, delta-filled series they must be treated as
//     undefined; the sentinels NaDuration and NaV are the "I don't know"
//     markers and are what NewDataUnit returns by default.
//
// Field promotion means that existing reads like du.Chron or du.Meas keep
// working without change. Struct literals must name the embedded type,
// e.g. DataUnit{Datum: Datum{Chron: t, Meas: v}, Dchron: NaDuration,
// Dmeas: NaV}; in practice, prefer NewDataUnit.
type DataUnit struct {
	Datum
	Dchron time.Duration
	Dmeas  float64
}

// TimeSeries is an ordered collection of DataUnit, typically sorted by
// Chron. BasicStats is embedded so stat fields are addressable at the
// series level.
//
// The series tracks whether its Dchron/Dmeas fields are in sync with the
// current DataSeries order via a private deltasValid flag, exposed by
// DeltasValid. Any mutation (append, sort, truncate, reset) flips the
// flag to false; SortDeltasStats is what restores it to true.
type TimeSeries struct {
	MemId       uint64
	Name        string
	Comment     string
	DataSeries  []DataUnit
	BasicStats
	deltasValid bool
}

// BasicStats holds summary statistics for a series. All aggregates are
// computed on valid observations only (i.e. excluding NaV and NaN). The
// counts NbreOfNaN and NbreOfNaV let callers audit data quality without
// re-walking the series.
//
// Naming key:
//   - Ch* fields are time.Time (timestamps).
//   - Ms* fields are scalar stats on Meas.
//   - DCh* / DMs* fields are stats on the consecutive deltas (Dchron, Dmeas).
type BasicStats struct {
	Len        int
	Chmin      time.Time
	ValAtChmin float64
	Chmax      time.Time
	ValAtChmax float64
	Chmed      time.Time
	Chmean     time.Time
	Chstd      time.Time
	Msmin      float64
	ChAtMsmin  time.Time
	Msmax      float64
	ChAtMsmax  time.Time
	Msmean     float64
	Msmed      float64
	Msstd      float64
	DChmin     time.Duration
	ChAtDChmin time.Time
	DChmax     time.Duration
	ChAtDchmax time.Time
	DChmean    float64
	DChmed     float64
	DChstd     float64
	DMsmin     float64
	DMsmax     float64
	DMsmed     float64
	DMsmean    float64
	DMsstd     float64
	NbreOfNaN  int // total NaN-class Meas entries (includes NaV)
	NbreOfNaV  int // NaV-tagged Meas entries only (missing by nature)
}

// TsContainer is a named bag of TimeSeries accessed by string key. It is
// the typical output of a "polishing" pipeline that produces multiple
// variants (raw / reduced / regularized / cleaned / interpolated) of the
// same underlying signal.
type TsContainer struct {
	Name    string
	Comment string
	Ts      map[string]*TimeSeries
}

// NewDatum constructs a bare Datum. It is a thin convenience wrapper; the
// zero-argument form of Datum{chr, v} is equally acceptable.
func NewDatum(chr time.Time, meas float64) Datum {
	return Datum{Chron: chr, Meas: meas}
}

// NewDataUnit builds a DataUnit from a timestamp and a value. The derived
// delta fields are initialized to their "no predecessor known" sentinels:
// Dchron = NaDuration, Dmeas = NaV. This makes a freshly constructed
// DataUnit visibly honest about what it does and does not know — no zero
// value can be mistaken for a real delta. Once the DataUnit has been
// placed in a sorted TimeSeries, call SortDeltasStats (or DeltasFiller)
// to populate the deltas.
//
// A Meas value of 0.0 is significant and NOT treated as "unset". Callers
// that want to mark a value as missing should pass NaV explicitly.
func NewDataUnit(chr time.Time, meas float64) DataUnit {
	return DataUnit{
		Datum:  Datum{Chron: chr, Meas: meas},
		Dchron: NaDuration,
		Dmeas:  NaV,
	}
}

// DeltasValid reports whether Dchron and Dmeas are known to be consistent
// with the current order of DataSeries. The flag is set to true by
// SortDeltasStats and cleared by any mutation that could invalidate the
// pairing between points (append, sort by another key, truncate, reset).
// Code that relies on valid deltas should call SortDeltasStats first, or
// inspect this flag to decide whether to.
func (ts *TimeSeries) DeltasValid() bool {
	return ts.deltasValid
}

// invalidateDeltas marks the Dchron/Dmeas fields as untrusted. Called by
// any mutating method. Lowercase so only package code can flip it.
func (ts *TimeSeries) invalidateDeltas() {
	ts.deltasValid = false
}

// AddDataUnit appends one or more DataUnits to the series in the given
// order. It does not sort, compute deltas, or update stats; that is the
// caller's responsibility (typically via SortDeltasStats after all
// inserts). Calling AddDataUnit always invalidates DeltasValid.
func (ts *TimeSeries) AddDataUnit(dus ...DataUnit) {
	ts.DataSeries = append(ts.DataSeries, dus...)
	ts.invalidateDeltas()
}

// AddDatum appends a freshly-promoted Datum to the series. It is the
// preferred entry point for ingestion code that manipulates Datum as its
// currency type.
func (ts *TimeSeries) AddDatum(ds ...Datum) {
	for _, d := range ds {
		ts.DataSeries = append(ts.DataSeries, DataUnit{
			Datum:  d,
			Dchron: NaDuration,
			Dmeas:  NaV,
		})
	}
	ts.invalidateDeltas()
}
