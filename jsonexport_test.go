package timeseries

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONFloat64_MarshalsNaVToNull(t *testing.T) {
	b, err := json.Marshal(JSONFloat64(NaV))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !bytes.Equal(b, []byte("null")) {
		t.Errorf("Marshal(NaV) = %q, want %q", string(b), "null")
	}
}

func TestJSONFloat64_MarshalsNaNToNull(t *testing.T) {
	b, err := json.Marshal(JSONFloat64(NaNumber))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !bytes.Equal(b, []byte("null")) {
		t.Errorf("Marshal(NaN) = %q, want %q", string(b), "null")
	}
}

func TestJSONFloat64_MarshalsRegularNumber(t *testing.T) {
	b, err := json.Marshal(JSONFloat64(3.5))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(b) != "3.5" {
		t.Errorf("Marshal(3.5) = %q, want %q", string(b), "3.5")
	}
}

func TestJSONDurationNS_MarshalsNaDurationToNull(t *testing.T) {
	b, err := json.Marshal(JSONDurationNS(NaDuration))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !bytes.Equal(b, []byte("null")) {
		t.Errorf("Marshal(NaDuration) = %q, want %q", string(b), "null")
	}
}

// ---------------------------------------------------------------------------
// End-to-end: a TimeSeries with NaV produces correct JSON
// ---------------------------------------------------------------------------

func TestTimeSeriesToJSON_exposesNaVAsNullAndCountsSeparately(t *testing.T) {
	var ts TimeSeries
	ts.Name = "sensor"
	ts.AddData(tFromSec(0), 1)
	ts.AddData(tFromSec(10), NaV)
	ts.AddData(tFromSec(20), 3)
	ts.SortDeltasStats()

	dto := ts.ToJSON()
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("Marshal err: %v", err)
	}
	s := string(b)

	// The middle Meas must be null in the JSON payload.
	if !strings.Contains(s, "null") {
		t.Errorf("expected null in JSON for NaV, got %s", s)
	}

	// NbreOfNaV must be 1 and NbreOfNaN must also be 1 (NaV is a NaN).
	if !strings.Contains(s, `"nbreOfNaV":1`) {
		t.Errorf("expected nbreOfNaV:1 in JSON, got %s", s)
	}
	if !strings.Contains(s, `"nbreOfNaN":1`) {
		t.Errorf("expected nbreOfNaN:1 in JSON, got %s", s)
	}
}
