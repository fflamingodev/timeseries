package timeseries

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// TestWalkthrough goes the whole way: build a series of twenty points,
// insert one of them out of order, leave gaps where the sensor was
// offline, slip in one broken computation, then print the points and
// the summary.
//
// Run it with -v to read the tables:
//
//	go test -run TestWalkthrough -v
//
// The assertions at the end are deliberately few. The point of this
// test is what a human sees: that a gap shows as NaV and not as a zero,
// that the first point has no interval, that the mean survives the
// gaps, and that the broken value poisons what it should and nothing
// else.
func TestWalkthrough(t *testing.T) {
	var out bytes.Buffer

	// --- 1. Build ------------------------------------------------------
	//
	// An outdoor temperature, read every hour. The sensor went offline
	// at hours 7 and 8, hour 12 is missing too, and hour 15 carries the
	// result of a division by zero made upstream.
	readings := []float64{
		nav.NaV, // the sensor is not online yet at midnight
		12.1, 11.8, 11.5, 11.9, 13.2, 15.1,
		nav.NaV, nav.NaV, // offline
		19.8, 21.3, 22.0,
		nav.NaV, // one lost reading
		21.4, 20.2,
		math.NaN(), // a broken computation upstream
		17.6, 16.1, 14.9, 13.7,
	}

	ts := NewTimeSeries("Outdoor temperature")
	ts.ID = NewID()
	ts.Comment = "hourly readings; sensor offline at midnight and between 07:00 and 09:00"

	// Everything but hour 5, which arrives late, on purpose.
	batch := make([]Datum, 0, len(readings))
	for h, v := range readings {
		if h == 5 {
			continue
		}
		batch = append(batch, NewDatum(at(h), v))
	}
	ts.AddAll(batch)

	// --- 2. The late arrival -------------------------------------------
	ts.Add(NewDatum(at(5), readings[5]))

	if ts.Len() != len(readings) {
		t.Fatalf("Len = %d, want %d", ts.Len(), len(readings))
	}
	checkInvariant(t, ts)

	// --- 3. Look at it -------------------------------------------------
	ts.Fprint(&out)
	ts.FprintStats(&out)

	// --- 3b. The same series, once the broken value is called what it
	// is: unknown. Marking an error as missing is what a cleaning step
	// does, and it is what turns the Measure column back into
	// statements about the data.
	cleaned := NewTimeSeries(ts.Name + " (broken value marked as missing)")
	ts.Range(func(_ int, du DataUnit) bool {
		m := du.Meas
		if nav.IsStdNaN(m) {
			m = nav.NaV
		}
		cleaned.Add(NewDatum(du.Chron, m))
		return true
	})
	cleaned.FprintStats(&out)

	// --- 4. Compute on it ----------------------------------------------
	meas := ts.Meas()
	bs := ts.Stats()

	out.WriteString("\nWhat notavalue says about the measurements:\n")
	out.WriteString("  usable   : " + itoa(nav.CountUsable(meas)) + "\n")
	out.WriteString("  missing  : " + itoa(nav.CountNaV(meas)) + "\n")
	out.WriteString("  errors   : " + itoa(nav.CountNaN(meas)) + "\n")
	out.WriteString("  mean     : " + nav.Format(nav.Mean(meas)) + "   (poisoned by the error, as it must be)\n")

	// The same series with the broken value dropped: what the data
	// actually says once the upstream mistake is removed.
	clean := make([]float64, 0, len(meas))
	for _, m := range meas {
		if nav.IsStdNaN(m) {
			continue
		}
		clean = append(clean, m)
	}
	out.WriteString("  mean without the error : " + nav.Format(nav.Mean(clean)) + "\n")
	out.WriteString("  median                 : " + nav.Format(nav.Median(clean)) + "\n")
	out.WriteString("  spread                 : " + nav.Format(nav.StdDev(clean)) + "\n")

	t.Log("\n" + out.String())

	// --- 5. Check what the eye should see -------------------------------
	printed := out.String()

	if !strings.Contains(printed, "NaV") {
		t.Error("no NaV in the output: the gaps are invisible")
	}
	if !strings.Contains(printed, "NaDuration") {
		t.Error("no NaDuration in the output: the first point should have no interval")
	}
	if strings.Contains(printed, "0001-01-01") {
		t.Error("a zero time was printed as a date instead of a dash")
	}

	// Four gaps, one error.
	if bs.NbreOfNaV != 4 {
		t.Errorf("NbreOfNaV = %d, want 4", bs.NbreOfNaV)
	}

	// The window opens at midnight on a gap; the data starts an hour
	// later. Confusing the two would misdate the series.
	if !bs.Chmin.Equal(at(0)) || !nav.IsNaV(bs.ValAtChmin) {
		t.Errorf("the window should open at hour 0 on a gap, got %s measuring %s",
			bs.Chmin.Format("15:04"), nav.Format(bs.ValAtChmin))
	}
	if !bs.ChFirstUsable.Equal(at(1)) || bs.ValAtFirstUsable != 12.1 {
		t.Errorf("first usable = %s measuring %s, want hour 1 measuring 12.1",
			bs.ChFirstUsable.Format("15:04"), nav.Format(bs.ValAtFirstUsable))
	}
	if errs := bs.NbreOfNaN - bs.NbreOfNaV; errs != 1 {
		t.Errorf("errors = %d, want 1", errs)
	}
	if bs.Len != 20 {
		t.Errorf("Len = %d, want 20", bs.Len)
	}

	// The error contaminates the statistics of the measurements...
	if !nav.IsStdNaN(bs.Msmean) {
		t.Errorf("Msmean = %s, want a plain NaN: an error upstream must stay visible",
			nav.Format(bs.Msmean))
	}
	// ... and nothing else. The clock is untouched.
	if bs.DChmin != time.Hour || bs.DChmax != time.Hour {
		t.Errorf("intervals = [%v, %v], want an hour throughout", bs.DChmin, bs.DChmax)
	}
	if !bs.Chmin.Equal(at(0)) || !bs.Chmax.Equal(at(19)) {
		t.Error("the extent of the series is wrong")
	}

	// Dropping the error gives an honest mean over the sixteen readings
	// that exist.
	if m := nav.Mean(clean); nav.IsNaV(m) || math.IsNaN(m) {
		t.Errorf("mean without the error = %s, want a number", nav.Format(m))
	}
}

// itoa keeps the walkthrough readable without importing strconv into
// its narrative.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for ; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits)
}

// -----------------------------------------------------------------------
// The printers themselves
// -----------------------------------------------------------------------

func TestFprintWindow(t *testing.T) {
	ts := seriesOf(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)

	var all, firstThree, middle bytes.Buffer
	ts.Fprint(&all)
	ts.Fprint(&firstThree, 3)
	ts.Fprint(&middle, 4, 6)

	if lines := strings.Count(all.String(), "\n"); lines < 10 {
		t.Errorf("the full print has %d lines, too few for ten points", lines)
	}
	// A window prints its bounds, so the reader knows it is a window.
	if !strings.Contains(firstThree.String(), "of 10") {
		t.Error("a partial print does not say how many points the series holds")
	}
	if strings.Contains(middle.String(), "of 10") == false {
		t.Error("the middle window does not report its bounds")
	}
	// Out-of-range indices are clamped, never fatal.
	var wild bytes.Buffer
	ts.Fprint(&wild, -5, 1000)
	if !strings.Contains(wild.String(), "index") {
		t.Error("clamping the window lost the table")
	}
}

func TestFprintEmptySeries(t *testing.T) {
	var out bytes.Buffer
	NewTimeSeries("nothing").Fprint(&out)

	if !strings.Contains(out.String(), "empty series") {
		t.Errorf("an empty series printed as:\n%s", out.String())
	}
}

func TestFprintStatsOfAnEmptySeries(t *testing.T) {
	var out bytes.Buffer
	NewTimeSeries("nothing").FprintStats(&out)

	printed := out.String()
	if !strings.Contains(printed, "NaV") {
		t.Errorf("the summary of an empty series shows no NaV:\n%s", printed)
	}
	if strings.Contains(printed, "0001-01-01") {
		t.Errorf("the summary of an empty series printed the zero time as a date:\n%s", printed)
	}
}

func TestDataUnitFprint(t *testing.T) {
	var out bytes.Buffer
	NewDataUnit(at(0), nav.NaV).Fprint(&out)

	printed := out.String()
	for _, want := range []string{"Chron", "Measure", "NaV", "NaDuration"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the print of a lone point lacks %q:\n%s", want, printed)
		}
	}
}

func TestFormatHelpers(t *testing.T) {
	if got := formatDuration(NaDuration); got != "NaDuration" {
		t.Errorf("formatDuration(NaDuration) = %q", got)
	}
	if got := formatDuration(90 * time.Second); got != "1m30s" {
		t.Errorf("formatDuration(90s) = %q", got)
	}
	if got := formatTime(time.Time{}); got != "—" {
		t.Errorf("formatTime(zero) = %q, want a dash", got)
	}
	if got := formatNanos(nav.NaV); got != "NaV" {
		t.Errorf("formatNanos(NaV) = %q", got)
	}
	if got := formatNanos(float64(3 * time.Hour)); got != "3h0m0s" {
		t.Errorf("formatNanos(3h) = %q", got)
	}
}
