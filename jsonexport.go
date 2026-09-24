package timeseries

import (
	"math"
	"strconv"
	"time"
)

// This file turns a series, its summary and a container into structures
// ready for encoding/json, for an HTTP handler to send as they are:
//
//	c.JSON(http.StatusOK, tsc.ToJSON())
//
// The format is the one the front end already reads, kept as it was so
// that the handlers and the TypeScript types keep working:
//
//   - a series is sent by columns — chron, meas, dchron_ns, dmeas —
//     rather than as a list of points, which is what a chart wants;
//   - instants are RFC 3339 strings, in the location they carry;
//   - durations are integers of nanoseconds, and their keys end in _ns;
//   - a measurement that is not a number becomes null.
//
// JSON has no NaN, and the format has one null for all of them: a gap
// (NaV) and a broken value (NaN) look the same once encoded. The
// summary keeps the difference in its two counts, nbreOfNaV and
// nbreOfNaN.

// JSONFloat64 is a float64 that encodes NaV, NaN and the infinities as
// null, where encoding/json would fail on them.
type JSONFloat64 float64

// MarshalJSON implements json.Marshaler.
func (f JSONFloat64) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatFloat(v, 'g', -1, 64)), nil
}

// JSONDurationNS is a duration that encodes as its number of
// nanoseconds, and NaDuration as null.
type JSONDurationNS time.Duration

// MarshalJSON implements json.Marshaler.
func (d JSONDurationNS) MarshalJSON() ([]byte, error) {
	v := time.Duration(d)
	if IsNaDuration(v) {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(v.Nanoseconds(), 10)), nil
}

// TimeSeriesJSON is a series as the front end receives it: four columns
// of the same length, one entry per point, and the summary.
type TimeSeriesJSON struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Comment string           `json:"comment,omitempty"`
	Chron   []time.Time      `json:"chron"`               // RFC 3339
	Meas    []JSONFloat64    `json:"meas"`                // NaV, NaN -> null
	Dchron  []JSONDurationNS `json:"dchron_ns,omitempty"` // NaDuration -> null
	Dmeas   []JSONFloat64    `json:"dmeas,omitempty"`     // NaV, NaN -> null
	Stats   *BasicStatsJSON  `json:"stats,omitempty"`     // absent on an empty series
}

// BasicStatsJSON is the summary of a series, flattened for the front
// end. The fields are those of BasicStats, under the keys it reads.
type BasicStatsJSON struct {
	Len        int         `json:"len"`
	Chmin      time.Time   `json:"chmin"`
	ValAtChmin JSONFloat64 `json:"valAtChmin"`
	Chmax      time.Time   `json:"chmax"`
	ValAtChmax JSONFloat64 `json:"valAtChmax"`

	Chmed            time.Time   `json:"chmed"`
	Chmean           time.Time   `json:"chmean"`
	ChFirstUsable    time.Time   `json:"chFirstUsable"`
	ValAtFirstUsable JSONFloat64 `json:"valAtFirstUsable"`

	Msmin     JSONFloat64 `json:"msmin"`
	ChAtMsmin time.Time   `json:"chAtMsmin"`
	Msmax     JSONFloat64 `json:"msmax"`
	ChAtMsmax time.Time   `json:"chAtMsmax"`
	Msmean    JSONFloat64 `json:"msmean"`
	Msmed     JSONFloat64 `json:"msmed"`
	Msstd     JSONFloat64 `json:"msstd"`

	DChminNS   JSONDurationNS `json:"dChmin_ns"`
	ChAtDChmin time.Time      `json:"chAtDChmin"`
	DChmaxNS   JSONDurationNS `json:"dChmax_ns"`
	ChAtDChmax time.Time      `json:"chAtDChmax"`
	DChmeanNS  JSONDurationNS `json:"dChmean_ns"`
	DChmedNS   JSONDurationNS `json:"dChmed_ns"`
	DChstdNS   JSONDurationNS `json:"dChstd_ns"`

	DMsmin  JSONFloat64 `json:"dMsmin"`
	DMsmax  JSONFloat64 `json:"dMsmax"`
	DMsmed  JSONFloat64 `json:"dMsmed"`
	DMsmean JSONFloat64 `json:"dMsmean"`
	DMsstd  JSONFloat64 `json:"dMsstd"`

	NbreOfNaN int `json:"nbreOfNaN"`
	NbreOfNaV int `json:"nbreOfNaV"`
}

// TsContainerJSON is a container as the front end receives it: its
// series keyed by name.
type TsContainerJSON struct {
	Name    string                     `json:"name"`
	Comment string                     `json:"comment,omitempty"`
	Series  map[string]*TimeSeriesJSON `json:"series"`
}

// ToJSON returns the series ready for encoding, with its summary. The
// summary is computed here, once; on an empty series there is none, and
// the stats key is left out.
func (ts *TimeSeries) ToJSON() *TimeSeriesJSON {
	n := ts.Len()
	out := &TimeSeriesJSON{
		ID:      ts.ID,
		Name:    ts.Name,
		Comment: ts.Comment,
		Chron:   make([]time.Time, n),
		Meas:    make([]JSONFloat64, n),
		Dchron:  make([]JSONDurationNS, n),
		Dmeas:   make([]JSONFloat64, n),
	}

	ts.Range(func(i int, du DataUnit) bool {
		out.Chron[i] = du.Chron
		out.Meas[i] = JSONFloat64(du.Meas)
		out.Dchron[i] = JSONDurationNS(du.Dchron)
		out.Dmeas[i] = JSONFloat64(du.Dmeas)
		return true
	})

	if n > 0 {
		out.Stats = ts.Stats().ToJSON()
	}
	return out
}

// ToJSON returns the summary ready for encoding.
func (bs BasicStats) ToJSON() *BasicStatsJSON {
	return &BasicStatsJSON{
		Len:        bs.Len,
		Chmin:      bs.Chmin,
		ValAtChmin: JSONFloat64(bs.ValAtChmin),
		Chmax:      bs.Chmax,
		ValAtChmax: JSONFloat64(bs.ValAtChmax),

		Chmed:            bs.Chmed,
		Chmean:           bs.Chmean,
		ChFirstUsable:    bs.ChFirstUsable,
		ValAtFirstUsable: JSONFloat64(bs.ValAtFirstUsable),

		Msmin:     JSONFloat64(bs.Msmin),
		ChAtMsmin: bs.ChAtMsmin,
		Msmax:     JSONFloat64(bs.Msmax),
		ChAtMsmax: bs.ChAtMsmax,
		Msmean:    JSONFloat64(bs.Msmean),
		Msmed:     JSONFloat64(bs.Msmed),
		Msstd:     JSONFloat64(bs.Msstd),

		DChminNS:   JSONDurationNS(bs.DChmin),
		ChAtDChmin: bs.ChAtDChmin,
		DChmaxNS:   JSONDurationNS(bs.DChmax),
		ChAtDChmax: bs.ChAtDchmax,
		DChmeanNS:  JSONDurationNS(nanos(bs.DChmean)),
		DChmedNS:   JSONDurationNS(nanos(bs.DChmed)),
		DChstdNS:   JSONDurationNS(nanos(bs.DChstd)),

		DMsmin:  JSONFloat64(bs.DMsmin),
		DMsmax:  JSONFloat64(bs.DMsmax),
		DMsmed:  JSONFloat64(bs.DMsmed),
		DMsmean: JSONFloat64(bs.DMsmean),
		DMsstd:  JSONFloat64(bs.DMsstd),

		NbreOfNaN: bs.NbreOfNaN,
		NbreOfNaV: bs.NbreOfNaV,
	}
}

// nanos turns a count of nanoseconds held in a float64 — the form the
// delta statistics take — into a duration, rounded to the nanosecond.
// A NaN, gap or not, becomes NaDuration: converting it directly would
// give whatever the processor makes of it.
func nanos(ns float64) time.Duration {
	if math.IsNaN(ns) || math.IsInf(ns, 0) {
		return NaDuration
	}
	return time.Duration(math.Round(ns))
}

// ToJSON returns the container ready for encoding, each series with its
// summary. A JSON object has no order: the series come out keyed by
// name, and the order they were put in is not carried.
func (tsc *TsContainer) ToJSON() *TsContainerJSON {
	out := &TsContainerJSON{
		Name:    tsc.Name,
		Comment: tsc.Comment,
		Series:  make(map[string]*TimeSeriesJSON, tsc.Len()),
	}
	tsc.Range(func(name string, ts *TimeSeries) bool {
		if ts != nil {
			out.Series[name] = ts.ToJSON()
		}
		return true
	})
	return out
}
