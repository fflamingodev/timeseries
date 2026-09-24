package timeseries

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// encode marshals v and decodes it back into plain maps and slices, the
// way the front end will see it.
func encode(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, b)
	}
	return out
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal(%v): %v", v, err)
	}
	return string(b)
}

// -----------------------------------------------------------------------
// The values
// -----------------------------------------------------------------------

// JSON has no NaN: every non-number becomes null, where encoding/json
// would fail.
func TestJSONFloat64(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"a gap", nav.NaV, "null"},
		{"a broken value", math.NaN(), "null"},
		{"plus infinity", math.Inf(1), "null"},
		{"minus infinity", math.Inf(-1), "null"},
		{"a number", 3.5, "3.5"},
		{"zero", 0, "0"},
		{"a negative number", -12.25, "-12.25"},
		{"a tiny number", 1e-9, "1e-09"},
	}
	for _, c := range cases {
		if got := marshal(t, JSONFloat64(c.in)); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestJSONDurationNS(t *testing.T) {
	if got := marshal(t, JSONDurationNS(NaDuration)); got != "null" {
		t.Errorf("NaDuration: %s, want null", got)
	}
	if got := marshal(t, JSONDurationNS(90*time.Second)); got != "90000000000" {
		t.Errorf("90s: %s, want 90000000000", got)
	}
	if got := marshal(t, JSONDurationNS(0)); got != "0" {
		t.Errorf("zero: %s, want 0", got)
	}
}

// The delta statistics are floats of nanoseconds; they go out as whole
// nanoseconds, and a NaN as null rather than as a garbage integer.
func TestNanos(t *testing.T) {
	if got := nanos(nav.NaV); !IsNaDuration(got) {
		t.Errorf("nanos(NaV) = %v, want NaDuration", got)
	}
	if got := nanos(math.NaN()); !IsNaDuration(got) {
		t.Errorf("nanos(NaN) = %v, want NaDuration", got)
	}
	if got := nanos(1.6); got != 2 {
		t.Errorf("nanos(1.6) = %v, want 2ns", got)
	}
	if got := nanos(float64(time.Hour)); got != time.Hour {
		t.Errorf("nanos(1h) = %v", got)
	}
}

// -----------------------------------------------------------------------
// A series
// -----------------------------------------------------------------------

func TestTimeSeriesToJSON(t *testing.T) {
	ts := seriesOf(1, nav.NaV, 3, math.NaN())
	ts.ID = "device-42"
	ts.Name = "sensor"
	ts.Comment = "a test"

	got := encode(t, ts.ToJSON())

	if got["id"] != "device-42" || got["name"] != "sensor" || got["comment"] != "a test" {
		t.Errorf("identity: id=%v name=%v comment=%v", got["id"], got["name"], got["comment"])
	}

	// Four columns of the same length.
	for _, key := range []string{"chron", "meas", "dchron_ns", "dmeas"} {
		col, ok := got[key].([]any)
		if !ok || len(col) != 4 {
			t.Errorf("%s: %v, want a column of 4", key, got[key])
		}
	}

	// Gap and broken value alike are null.
	meas := got["meas"].([]any)
	if meas[0] != 1.0 || meas[1] != nil || meas[2] != 3.0 || meas[3] != nil {
		t.Errorf("meas = %v, want [1 null 3 null]", meas)
	}

	// The first point has no predecessor: no interval, no variation.
	dchron := got["dchron_ns"].([]any)
	if dchron[0] != nil || dchron[1] != float64(time.Hour) {
		t.Errorf("dchron_ns = %v, want [null 3.6e12 ...]", dchron)
	}
	if dmeas := got["dmeas"].([]any); dmeas[0] != nil {
		t.Errorf("dmeas[0] = %v, want null", dmeas[0])
	}

	// Instants are RFC 3339.
	chron := got["chron"].([]any)
	if chron[1] != "2026-01-01T01:00:00Z" {
		t.Errorf("chron[1] = %v, want 2026-01-01T01:00:00Z", chron[1])
	}
}

// An instant keeps the location it carries.
func TestTimeSeriesToJSONKeepsTheLocation(t *testing.T) {
	paris := mustLoad(t, "Europe/Paris")
	ts := seriesAt(tv{time.Date(2026, 7, 14, 9, 30, 0, 0, paris), 1})

	chron := encode(t, ts.ToJSON())["chron"].([]any)
	if chron[0] != "2026-07-14T09:30:00+02:00" {
		t.Errorf("chron[0] = %v, want 2026-07-14T09:30:00+02:00", chron[0])
	}
}

// The summary carries the difference that null erases: how many of the
// nulls are gaps, and how many are broken values.
func TestTimeSeriesToJSONStats(t *testing.T) {
	ts := seriesOf(nav.NaV, 2, 4, nav.NaV, math.NaN())

	stats, ok := encode(t, ts.ToJSON())["stats"].(map[string]any)
	if !ok {
		t.Fatal("no stats in the payload")
	}
	if stats["len"] != 5.0 {
		t.Errorf("len = %v, want 5", stats["len"])
	}
	if stats["nbreOfNaV"] != 2.0 || stats["nbreOfNaN"] != 3.0 {
		t.Errorf("nbreOfNaV = %v, nbreOfNaN = %v, want 2 and 3",
			stats["nbreOfNaV"], stats["nbreOfNaN"])
	}
	if stats["chFirstUsable"] != "2026-01-01T01:00:00Z" || stats["valAtFirstUsable"] != 2.0 {
		t.Errorf("first usable = %v measuring %v, want 01:00 measuring 2",
			stats["chFirstUsable"], stats["valAtFirstUsable"])
	}
	// The broken value poisons the aggregates, which go out as null.
	if stats["msmean"] != nil {
		t.Errorf("msmean = %v, want null", stats["msmean"])
	}
	// Hourly points: the mean interval is an hour, in whole nanoseconds.
	if stats["dChmean_ns"] != float64(time.Hour) {
		t.Errorf("dChmean_ns = %v, want %d", stats["dChmean_ns"], time.Hour)
	}
}

// The keys the front end reads, in Transports.ts, are all there — and
// chstd, removed from BasicStats, is gone.
func TestBasicStatsJSONKeys(t *testing.T) {
	stats := encode(t, seriesOf(1, 2, 3).ToJSON())["stats"].(map[string]any)

	for _, key := range []string{
		"len", "chmin", "valAtChmin", "chmax", "valAtChmax", "chmed", "chmean",
		"msmin", "chAtMsmin", "msmax", "chAtMsmax", "msmean", "msmed", "msstd",
		"dChmin_ns", "chAtDChmin", "dChmax_ns", "chAtDChmax",
		"dChmean_ns", "dChmed_ns", "dChstd_ns",
		"dMsmin", "dMsmax", "dMsmed", "dMsmean", "dMsstd",
		"nbreOfNaN",
		// Added in v0.2.
		"nbreOfNaV", "chFirstUsable", "valAtFirstUsable",
	} {
		if _, ok := stats[key]; !ok {
			t.Errorf("stats lacks %q", key)
		}
	}
	if _, ok := stats["chstd"]; ok {
		t.Error("stats still carries chstd")
	}
	if len(stats) != 30 {
		t.Errorf("stats has %d keys, want 30", len(stats))
	}
}

// An empty series has columns, empty, and no summary.
func TestTimeSeriesToJSONEmpty(t *testing.T) {
	got := encode(t, NewTimeSeries("empty").ToJSON())

	if col, ok := got["chron"].([]any); !ok || len(col) != 0 {
		t.Errorf("chron = %v, want []", got["chron"])
	}
	if col, ok := got["meas"].([]any); !ok || len(col) != 0 {
		t.Errorf("meas = %v, want []", got["meas"])
	}
	if _, ok := got["stats"]; ok {
		t.Error("an empty series has a summary")
	}
	if _, ok := got["comment"]; ok {
		t.Error("an empty comment is sent")
	}
}

// -----------------------------------------------------------------------
// A container
// -----------------------------------------------------------------------

func TestTsContainerToJSON(t *testing.T) {
	tsc := NewTsContainer("device-42")
	tsc.Comment = "last 24 hours"
	tsc.Put("Original", seriesOf(1, 2, 3))
	tsc.Put("Accepted", seriesOf(1, 3))
	tsc.Put("Rejected", nil) // asked for, could not be produced

	got := encode(t, tsc.ToJSON())
	if got["name"] != "device-42" || got["comment"] != "last 24 hours" {
		t.Errorf("identity: name=%v comment=%v", got["name"], got["comment"])
	}

	series, ok := got["series"].(map[string]any)
	if !ok {
		t.Fatalf("series = %v, want an object", got["series"])
	}
	if len(series) != 2 {
		t.Errorf("%d series, want 2: the nil one is left out", len(series))
	}
	original, ok := series["Original"].(map[string]any)
	if !ok {
		t.Fatal("no Original series")
	}
	if meas := original["meas"].([]any); len(meas) != 3 {
		t.Errorf("Original has %d points, want 3", len(meas))
	}
	if _, ok := original["stats"]; !ok {
		t.Error("a series of the container has no summary")
	}
}

func TestTsContainerToJSONEmpty(t *testing.T) {
	got := encode(t, NewTsContainer("nothing").ToJSON())
	if series, ok := got["series"].(map[string]any); !ok || len(series) != 0 {
		t.Errorf("series = %v, want {}", got["series"])
	}
}
