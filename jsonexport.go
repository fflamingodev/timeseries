package timeseries

import (
	"math"
	"strconv"
	"time"
)

type JSONFloat64 float64

func (f JSONFloat64) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []byte("null"), nil
	}
	// Nombre JSON normal
	return []byte(strconv.FormatFloat(v, 'g', -1, 64)), nil
}

type JSONDurationNS time.Duration

func (d JSONDurationNS) MarshalJSON() ([]byte, error) {
	v := time.Duration(d)
	if v == NaDuration {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(v.Nanoseconds(), 10)), nil
}

type TimeSeriesJSON struct {
	Name    string           `json:"name"`
	MemId   uint64           `json:"id"`
	Comment string           `json:"comment,omitempty"`
	Chron   []time.Time      `json:"chron"`               // RFC3339
	Meas    []JSONFloat64    `json:"meas"`                // NaN -> null
	Dchron  []JSONDurationNS `json:"dchron_ns,omitempty"` // NaDuration -> null
	Dmeas   []JSONFloat64    `json:"dmeas,omitempty"`     // NaN -> null
	Stats   *BasicStatsJSON  `json:"stats,omitempty"`
}
type BasicStatsJSON struct {
	Len        int         `json:"len"`
	Chmin      time.Time   `json:"chmin"`
	ValAtChmin JSONFloat64 `json:"valAtChmin"`
	Chmax      time.Time   `json:"chmax"`
	ValAtChmax JSONFloat64 `json:"valAtChmax"`

	Chmed     time.Time   `json:"chmed"`
	Chmean    time.Time   `json:"chmean"`
	Chstd     time.Time   `json:"chstd"`
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

	DMsmin    JSONFloat64 `json:"dMsmin"`
	DMsmax    JSONFloat64 `json:"dMsmax"`
	DMsmed    JSONFloat64 `json:"dMsmed"`
	DMsmean   JSONFloat64 `json:"dMsmean"`
	DMsstd    JSONFloat64 `json:"dMsstd"`
	NbreOfNaN int         `json:"nbreOfNaN"`
	NbreOfNaV int         `json:"nbreOfNaV"`
}
type TsContainerJSON struct {
	Name    string                     `json:"name"`
	Comment string                     `json:"comment,omitempty"`
	Series  map[string]*TimeSeriesJSON `json:"series"`
}

func (ts *TimeSeries) ToJSON() *TimeSeriesJSON {
	n := len(ts.DataSeries)
	chron := make([]time.Time, n)
	meas := make([]JSONFloat64, n)
	dchron := make([]JSONDurationNS, n)
	dmeas := make([]JSONFloat64, n)

	for i, du := range ts.DataSeries {
		chron[i] = du.Chron
		meas[i] = JSONFloat64(du.Meas)
		dchron[i] = JSONDurationNS(du.Dchron)
		dmeas[i] = JSONFloat64(du.Dmeas)
	}

	var statsJSON *BasicStatsJSON
	if ts.Len > 0 {
		statsJSON = basicStatsToJSON(&ts.BasicStats)
	}

	return &TimeSeriesJSON{
		Name:    ts.Name,
		MemId:   ts.MemId,
		Comment: ts.Comment,
		Chron:   chron,
		Meas:    meas,
		Dchron:  dchron,
		Dmeas:   dmeas,
		Stats:   statsJSON,
	}
}

func basicStatsToJSON(bs *BasicStats) *BasicStatsJSON {
	if bs == nil {
		return nil
	}

	return &BasicStatsJSON{
		Len:        bs.Len,
		Chmin:      bs.Chmin,
		ValAtChmin: JSONFloat64(bs.ValAtChmin),
		Chmax:      bs.Chmax,
		ValAtChmax: JSONFloat64(bs.ValAtChmax),

		Chmed:     bs.Chmed,
		Chmean:    bs.Chmean,
		Chstd:     bs.Chstd,
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
		DChmeanNS:  JSONDurationNS(bs.DChmean),
		DChmedNS:   JSONDurationNS(bs.DChmed),
		DChstdNS:   JSONDurationNS(bs.DChstd),

		DMsmin:    JSONFloat64(bs.DMsmin),
		DMsmax:    JSONFloat64(bs.DMsmax),
		DMsmed:    JSONFloat64(bs.DMsmed),
		DMsmean:   JSONFloat64(bs.DMsmean),
		DMsstd:    JSONFloat64(bs.DMsstd),
		NbreOfNaN: bs.NbreOfNaN,
		NbreOfNaV: bs.NbreOfNaV,
	}
}
func (c *TsContainer) ToJSON() *TsContainerJSON {
	out := &TsContainerJSON{
		Name:    c.Name,
		Comment: c.Comment,
		Series:  make(map[string]*TimeSeriesJSON, len(c.Ts)),
	}
	for key, ts := range c.Ts {
		if ts == nil {
			continue
		}
		out.Series[key] = ts.ToJSON()
	}
	return out
}
