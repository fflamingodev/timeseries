package timeseries

import (
	"bytes"
	"strings"
	"testing"

	nav "github.com/fflamingodev/notavalue"
)

func TestContainerPutAndGet(t *testing.T) {
	tsc := NewTsContainer("device 42")
	raw := seriesOf(1, 2, 3)
	tsc.Put("raw", raw)

	got, ok := tsc.Get("raw")
	if !ok || got != raw {
		t.Errorf("Get(raw) = %v, %v; want the series that was filed", got, ok)
	}
	if tsc.Len() != 1 {
		t.Errorf("Len = %d, want 1", tsc.Len())
	}

	if _, ok := tsc.Get("hourly"); ok {
		t.Error("Get reports a name that was never filed")
	}
	if tsc.Series("hourly") != nil {
		t.Error("Series returns something for a name that was never filed")
	}
}

// A name filed with nil and a name never filed are different
// statements: "this variant could not be produced" against "this
// variant was never asked for".
func TestContainerDistinguishesNilFromAbsent(t *testing.T) {
	tsc := NewTsContainer("device 42")
	tsc.Put("regularized", nil)

	ts, ok := tsc.Get("regularized")
	if !ok {
		t.Error("a name filed with nil is reported as absent")
	}
	if ts != nil {
		t.Error("the nil series came back as something else")
	}
	// And a nil series is still safe to read.
	if ts.Len() != 0 {
		t.Error("Len on the nil series did not return 0")
	}
}

// The order of the names is the order they were filed, not the random
// order of a map. A chart legend built from it must not reshuffle
// itself between two runs.
func TestContainerKeepsTheOrderOfInsertion(t *testing.T) {
	want := []string{"raw", "cleaned", "hourly", "daily"}

	// Run it several times: a map-backed implementation would pass once
	// in a while by luck.
	for round := 0; round < 50; round++ {
		tsc := NewTsContainer("device")
		for _, name := range want {
			tsc.Put(name, seriesOf(1, 2))
		}

		got := tsc.Names()
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("round %d: order = %v, want %v", round, got, want)
			}
		}

		var walked []string
		tsc.Range(func(name string, _ *TimeSeries) bool {
			walked = append(walked, name)
			return true
		})
		for i := range want {
			if walked[i] != want[i] {
				t.Fatalf("round %d: Range order = %v, want %v", round, walked, want)
			}
		}
	}
}

// Replacing a variant must not move it to the end: a recipe that
// recomputes its cleaned series should not reshuffle the display.
func TestContainerPutKeepsTheRankOfAName(t *testing.T) {
	tsc := NewTsContainer("device")
	tsc.Put("raw", seriesOf(1))
	tsc.Put("cleaned", seriesOf(2))
	tsc.Put("hourly", seriesOf(3))

	replacement := seriesOf(9, 9)
	tsc.Put("cleaned", replacement)

	names := tsc.Names()
	if len(names) != 3 || names[1] != "cleaned" {
		t.Errorf("order = %v, want cleaned still in second place", names)
	}
	if tsc.Series("cleaned") != replacement {
		t.Error("the replacement was not stored")
	}
	if tsc.Len() != 3 {
		t.Errorf("Len = %d, want 3: replacing is not adding", tsc.Len())
	}
}

func TestContainerDelete(t *testing.T) {
	tsc := NewTsContainer("device")
	tsc.Put("raw", seriesOf(1))
	tsc.Put("cleaned", seriesOf(2))
	tsc.Put("hourly", seriesOf(3))

	tsc.Delete("cleaned")

	if tsc.Len() != 2 {
		t.Errorf("Len = %d, want 2", tsc.Len())
	}
	if _, ok := tsc.Get("cleaned"); ok {
		t.Error("the deleted name is still there")
	}
	names := tsc.Names()
	if len(names) != 2 || names[0] != "raw" || names[1] != "hourly" {
		t.Errorf("order after deletion = %v, want [raw hourly]", names)
	}

	// Deleting twice, or deleting what was never there, is harmless.
	tsc.Delete("cleaned")
	tsc.Delete("never filed")
	if tsc.Len() != 2 {
		t.Errorf("Len = %d after harmless deletions, want 2", tsc.Len())
	}
}

// A nil container must not panic on the read-only operations: a lookup
// that found nothing is a common way to get one.
func TestNilContainerIsSafeToRead(t *testing.T) {
	var tsc *TsContainer

	if tsc.Len() != 0 {
		t.Error("Len on a nil container is not 0")
	}
	if tsc.Names() != nil {
		t.Error("Names on a nil container returned something")
	}
	if _, ok := tsc.Get("raw"); ok {
		t.Error("Get on a nil container found something")
	}
	if tsc.Series("raw") != nil {
		t.Error("Series on a nil container returned something")
	}
	tsc.Range(func(string, *TimeSeries) bool {
		t.Error("Range called f on a nil container")
		return true
	})
	tsc.Delete("raw")
}

// The zero value works too, without NewTsContainer: Put builds the map
// it needs.
func TestZeroContainerIsUsable(t *testing.T) {
	var tsc TsContainer
	tsc.Put("raw", seriesOf(1, 2))

	if tsc.Len() != 1 {
		t.Errorf("Len = %d, want 1", tsc.Len())
	}
}

// Names is a copy: reordering it must not reorder the container.
func TestContainerNamesIsACopy(t *testing.T) {
	tsc := NewTsContainer("device")
	tsc.Put("raw", seriesOf(1))
	tsc.Put("cleaned", seriesOf(2))

	names := tsc.Names()
	names[0], names[1] = names[1], names[0]

	if again := tsc.Names(); again[0] != "raw" {
		t.Errorf("the container follows its caller's edits: %v", again)
	}
}

// -----------------------------------------------------------------------
// Printing
// -----------------------------------------------------------------------

func TestContainerFprint(t *testing.T) {
	tsc := NewTsContainer("Outdoor temperature, device 42")
	tsc.Comment = "the day of 2026-01-01"
	tsc.Put("raw", seriesOf(12.4, nav.NaV, 13.1))
	tsc.Put("not produced yet", nil)

	var out bytes.Buffer
	tsc.Fprint(&out)
	printed := out.String()

	for _, want := range []string{
		"Outdoor temperature, device 42", // the container name
		"the day of 2026-01-01",          // its comment
		"raw",                            // each series name
		"not produced yet",
		"(not produced)",          // what a nil series prints as
		"NaV",                     // the gap, still visible
		"WHAT — the measurements", // the summary came along
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("the print lacks %q:\n%s", want, printed)
		}
	}
	t.Log("\n" + printed)
}

func TestContainerFprintStats(t *testing.T) {
	tsc := NewTsContainer("device 42")
	tsc.Put("raw", seriesOf(1, 2, 3))
	tsc.Put("cleaned", seriesOf(1, 2))

	var out bytes.Buffer
	tsc.FprintStats(&out)
	printed := out.String()

	// Summaries only: no table of points.
	if strings.Contains(printed, "index") {
		t.Errorf("FprintStats printed the points as well:\n%s", printed)
	}
	for _, want := range []string{"device 42", "raw", "cleaned", "3 points", "2 points"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the summaries lack %q:\n%s", want, printed)
		}
	}
}

func TestEmptyContainerFprint(t *testing.T) {
	var out bytes.Buffer
	NewTsContainer("nothing").Fprint(&out)

	if !strings.Contains(out.String(), "(no series)") {
		t.Errorf("an empty container printed as:\n%s", out.String())
	}
}
