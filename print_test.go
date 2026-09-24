package timeseries

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// reading is one line of the log used by the walkthroughs: minutes
// counted from midnight, and what the sensor reported.
type reading struct {
	minute int
	value  float64
}

// late is the index, in the log below, of the reading that reaches us
// out of order.
const late = 5

// driftingLog returns a day of hourly readings from a logger that is no
// metronome: its ticks land 56 to 66 minutes apart, which is what the
// interval statistics are there to reveal.
//
// The sensor is not online yet at midnight, goes offline again around
// 07:00 and 08:00, and one reading is lost around noon: four gaps that
// the library must step over rather than choke on.
//
// The reading at 14:59 is the caller's choice: pass math.NaN() to see
// what a broken computation upstream does to the summary, or a plain
// number for a log that only has gaps in it.
func driftingLog(at1459 float64) []reading {
	return []reading{
		{0, nav.NaV}, // not online yet
		{57, 12.1},
		{123, 11.8},
		{180, 11.5},
		{241, 11.9},
		{297, 13.2}, // arrives late
		{360, 15.1},
		{422, nav.NaV}, // offline
		{480, nav.NaV},
		{541, 19.8},
		{600, 21.3},
		{658, 22.0},
		{720, nav.NaV}, // lost reading
		{783, 21.4},
		{840, 20.2},
		{899, at1459},
		{960, 17.6},
		{1021, 16.1},
		{1080, 14.9},
		{1140, 13.7},
	}
}

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
	// An outdoor temperature, logged about every hour — about, because a
	// logger is not a metronome: its ticks drift by a few minutes, which
	// is what the interval statistics are there to reveal. The minutes
	// below are counted from midnight.
	//
	// The sensor is not online yet at midnight, goes offline again
	// around 07:00 and 08:00, one reading is lost around noon, and one
	// carries the result of a division by zero made upstream.
	readings := driftingLog(math.NaN()) // broken upstream at 14:59

	ts := NewTimeSeries("Outdoor temperature")
	ts.ID = NewID()
	ts.Comment = "logged roughly hourly; sensor offline at midnight and around 07:00-08:00"

	// The ordinary way to load a series: hand the whole batch over at
	// once. Add would give the same result, at a quadratic cost.
	batch := make([]Datum, 0, len(readings))
	for i, r := range readings {
		if i == late {
			continue
		}
		batch = append(batch, NewDatum(atMinute(r.minute), r.value))
	}
	ts.AddBatchData(batch)

	// --- 2. The late arrival -------------------------------------------
	//
	// One reading reaches us after the others, and belongs in the
	// middle. Add places it and fixes the two intervals it sits between.
	ts.Add(NewDatum(atMinute(readings[late].minute), readings[late].value))

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

	// The window opens at midnight on a gap; the data starts at 00:57.
	// Confusing the two would misdate the series.
	if !bs.Chmin.Equal(atMinute(0)) || !nav.IsNaV(bs.ValAtChmin) {
		t.Errorf("the window should open at midnight on a gap, got %s measuring %s",
			bs.Chmin.Format("15:04"), nav.Format(bs.ValAtChmin))
	}
	if !bs.ChFirstUsable.Equal(atMinute(57)) || bs.ValAtFirstUsable != 12.1 {
		t.Errorf("first usable = %s measuring %s, want 00:57 measuring 12.1",
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
	if !bs.Chmin.Equal(atMinute(0)) || !bs.Chmax.Equal(atMinute(1140)) {
		t.Error("the extent of the series is wrong")
	}

	// The drifting logger, read back from the intervals. The shortest
	// gap ends on the reading that arrived late, which is the proof that
	// inserting it in the middle recomputed both of its neighbours.
	if bs.DChmin != 56*time.Minute || !bs.ChAtDChmin.Equal(atMinute(297)) {
		t.Errorf("shortest interval = %v ending at %s, want 56m ending at 04:57",
			bs.DChmin, bs.ChAtDChmin.Format("15:04"))
	}
	if bs.DChmax != 66*time.Minute || !bs.ChAtDchmax.Equal(atMinute(123)) {
		t.Errorf("longest interval = %v ending at %s, want 1h6m ending at 02:03",
			bs.DChmax, bs.ChAtDchmax.Format("15:04"))
	}
	// Nineteen intervals spanning 1140 minutes: exactly an hour on
	// average, although not one of them is an hour.
	if bs.DChmean != float64(time.Hour) {
		t.Errorf("mean interval = %v, want exactly 1h", time.Duration(bs.DChmean))
	}
	// And that is what DChstd is for: the average hides the drift, the
	// dispersion shows it.
	if bs.DChstd <= 0 {
		t.Errorf("interval dispersion = %v, want a positive value on a drifting logger",
			time.Duration(bs.DChstd))
	}

	// Dropping the error gives an honest mean over the sixteen readings
	// that exist.
	if m := nav.Mean(clean); nav.IsNaV(m) || math.IsNaN(m) {
		t.Errorf("mean without the error = %s, want a number", nav.Format(m))
	}
}

// TestWalkthroughWithoutErrors is the same day of readings with nothing
// broken in it — only the three gaps where the sensor was silent.
//
// This is the case the library is built for, and the one to read first:
// every statistic of the measurements is a number, computed over the
// sixteen readings that exist, and the three gaps cost nothing but
// their own absence.
//
//	go test -run TestWalkthroughWithoutErrors -v
func TestWalkthroughWithoutErrors(t *testing.T) {
	var out bytes.Buffer

	readings := driftingLog(18.9) // a real reading at 14:59 this time

	ts := NewTimeSeries("Outdoor temperature, no error")
	ts.Comment = "logged roughly hourly; sensor offline at midnight and around 07:00-08:00"

	batch := make([]Datum, 0, len(readings))
	for i, r := range readings {
		if i == late {
			continue
		}
		batch = append(batch, NewDatum(atMinute(r.minute), r.value))
	}
	ts.AddBatchData(batch)
	ts.Add(NewDatum(atMinute(readings[late].minute), readings[late].value))

	checkInvariant(t, ts)
	ts.Fprint(&out)
	ts.FprintStats(&out)
	t.Log("\n" + out.String())

	bs := ts.Stats()

	// Twenty points, sixteen readings, four gaps, nothing broken. That
	// NbreOfNaN equals NbreOfNaV is the statement: every non-number here
	// is a gap, none is an error.
	if bs.Len != 20 || bs.NbreOfNaV != 4 || bs.NbreOfNaN != 4 {
		t.Errorf("Len = %d, missing = %d, non-numbers = %d; want 20, 4 and 4",
			bs.Len, bs.NbreOfNaV, bs.NbreOfNaN)
	}

	// Every statistic of the measurements is now a statement about the
	// data, which is the whole difference with the other walkthrough.
	for _, c := range []struct {
		name string
		got  float64
	}{
		{"Msmin", bs.Msmin}, {"Msmax", bs.Msmax}, {"Msmean", bs.Msmean},
		{"Msmed", bs.Msmed}, {"Msstd", bs.Msstd},
		{"DMsmin", bs.DMsmin}, {"DMsmax", bs.DMsmax}, {"DMsmean", bs.DMsmean},
		{"DMsmed", bs.DMsmed}, {"DMsstd", bs.DMsstd},
	} {
		if c.got != c.got { // NaN-class
			t.Errorf("%s = %s, want a number: nothing is broken in this log",
				c.name, nav.Format(c.got))
		}
	}

	// The extremes, and when they happened.
	if bs.Msmin != 11.5 || !bs.ChAtMsmin.Equal(atMinute(180)) {
		t.Errorf("lowest = %s at %s, want 11.5 at 03:00",
			nav.Format(bs.Msmin), bs.ChAtMsmin.Format("15:04"))
	}
	if bs.Msmax != 22 || !bs.ChAtMsmax.Equal(atMinute(658)) {
		t.Errorf("highest = %s at %s, want 22 at 10:58",
			nav.Format(bs.Msmax), bs.ChAtMsmax.Format("15:04"))
	}

	// The mean covers the sixteen readings that exist, not the twenty
	// slots: 261.5 / 16. Counting the gaps as zero would give 13.075,
	// and counting them as errors would give nothing at all.
	if want := 261.5 / 16.0; math.Abs(bs.Msmean-want) > 1e-12 {
		t.Errorf("mean = %s, want %v", nav.Format(bs.Msmean), want)
	}
	// Median of the sixteen sorted readings, lower of the two middles.
	if bs.Msmed != 15.1 {
		t.Errorf("median = %s, want 15.1", nav.Format(bs.Msmed))
	}

	// The measured day rose and fell: the largest single rise is bigger
	// than the largest single fall.
	if bs.DMsmax <= 0 || bs.DMsmin >= 0 {
		t.Errorf("variations = [%s, %s], want a fall and a rise",
			nav.Format(bs.DMsmin), nav.Format(bs.DMsmax))
	}

	printed := out.String()
	// The header states the count; "NaN" appears there as a label, so
	// the check is on the figure, not on the word.
	if !strings.Contains(printed, "0 broken (NaN)") {
		t.Errorf("the summary does not report a log free of errors:\n%s", printed)
	}
	if !strings.Contains(printed, "NaV") {
		t.Error("the four gaps are not visible in the output")
	}
}

// TestWalkthroughReduce follows a signal that holds its value between
// changes — a heating setpoint — from the raw log to its reduced form,
// three times: straight from the raw readings, from the raw readings
// with their silences marked, and through Regularize.
//
//	go test -run TestWalkthroughReduce -v
//
// The roads do not end at the same place, and the difference is the
// lesson. Reduce sees a gap only where the series says NaV. The raw log
// says nothing during the hour the logger was silent — it has no reading
// there at all — so the reduced series holds 21 °C straight across the
// silence, and Expand fills it in as if it had been measured.
//
// MarkSilences puts a NaV where an alarm watching the logger would have
// fired, fifteen minutes into the silence, and the reduction keeps it.
// The regularized series has a point every ten minutes, NaV where
// nothing arrived, and the silence survives the reduction too.
func TestWalkthroughReduce(t *testing.T) {
	var out bytes.Buffer
	section := func(title string) {
		out.WriteString("\n### " + title + "\n\n")
	}

	// --- 1. Generate ---------------------------------------------------
	//
	// A setpoint logged every ten minutes by a logger that drifts by up
	// to two minutes either way: 19 °C, raised to 21 °C around 01:30,
	// lowered to 17 °C around 03:40. One reading comes back empty at
	// 00:50, and the logger falls silent from 02:10 to 03:10 — seven
	// readings that never arrive.
	jitter := []int{0, 2, -1, 1, -2}
	raw := NewTimeSeries("Heating setpoint")
	raw.Comment = "logged every ten minutes, give or take two; silent 02:10-03:10"

	var batch []Datum
	for k := 0; k <= 25; k++ {
		if k >= 13 && k <= 19 {
			continue // the silence: nothing, not even a NaV
		}
		value := 19.0
		switch {
		case k == 5:
			value = nav.NaV // the reading that came back empty
		case k >= 22:
			value = 17
		case k >= 9:
			value = 21
		}
		batch = append(batch, NewDatum(atMinute(10*k+jitter[k%5]), value))
	}
	raw.AddBatchData(batch)
	checkInvariant(t, raw)

	section("1. The raw log")
	raw.Fprint(&out)

	// --- 2. Reduce the raw log -------------------------------------------
	//
	// Only the changes are kept, with both ends. The empty reading at
	// 00:50 is a change, going in and coming out. The silence is not: on
	// either side of it the setpoint reads 21, and nothing in between
	// says otherwise.
	rawReduced := raw.Reduce()
	checkInvariant(t, rawReduced)

	section("2. The raw log, reduced")
	rawReduced.Fprint(&out)

	// Expanded again on a ten-minute grid, the silence is filled with the
	// value held — a value nobody measured.
	rawBack, err := rawReduced.Expand(atMinute(0), atMinute(250), 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	section("2b. The raw reduction, expanded every ten minutes: the silence is gone")
	rawBack.Fprint(&out)

	// --- 2c. Watch the raw log, then reduce ------------------------------
	//
	// The logger reports every ten minutes, give or take two: readings
	// never lie more than twelve minutes apart, and fifteen minutes
	// without one means it is lost. MarkSilences dates the loss at the
	// moment the alarm would fire — 01:59 plus fifteen minutes, 02:14 —
	// and the reduction keeps it, without any grid.
	watched, err := raw.MarkSilences(15 * time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, watched)
	watchedReduced := watched.Reduce()
	checkInvariant(t, watchedReduced)

	section("2c. The raw log, silences beyond 15 minutes marked, then reduced")
	watchedReduced.Fprint(&out)

	watchedBack, err := watchedReduced.Expand(atMinute(0), atMinute(250), 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	section("2d. The watched reduction, expanded every ten minutes: the silence is back")
	watchedBack.Fprint(&out)

	// --- 3. Regularize first ---------------------------------------------
	//
	// Ten-minute windows, with three minutes of grace for the readings
	// that arrive late, and the last reading of each window: for a
	// setpoint, the value in force when the window closes. The windows
	// that caught nothing come out as NaV.
	regular, err := raw.RegularizeWithTolerance(10*time.Minute, 3*time.Minute, AggLast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, regular)

	section("3. The log, regularized every ten minutes")
	regular.Fprint(&out)

	// --- 4. Then reduce ---------------------------------------------------
	regReduced := regular.Reduce()
	checkInvariant(t, regReduced)

	section("4. The regularized log, reduced: the silence is kept")
	regReduced.Fprint(&out)

	// Expanded on the same grid, it gives the regularized series back.
	regBack, err := regReduced.Expand(atMinute(0), atMinute(250), 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	section("4b. The regularized reduction, expanded again: the regularized log, point for point")
	regBack.Fprint(&out)

	out.WriteString("\nSizes:\n")
	out.WriteString("  raw log                 : " + itoa(raw.Len()) + " points\n")
	out.WriteString("  raw, reduced            : " + itoa(rawReduced.Len()) + " points\n")
	out.WriteString("  raw, watched, reduced   : " + itoa(watchedReduced.Len()) + " points\n")
	out.WriteString("  regularized             : " + itoa(regular.Len()) + " points\n")
	out.WriteString("  regularized, reduced    : " + itoa(regReduced.Len()) + " points\n")

	t.Log("\n" + out.String())

	// --- 5. Check what the eye should see ---------------------------------

	// Straight from the raw log: the changes, and no trace of the silence.
	sameGrid(t, "raw, reduced", gridOf(rawReduced), []slot{
		{0, 19},
		{50, nav.NaV}, // the empty reading
		{62, 19},      // coming out of it
		{88, 21},
		{219, 17},
		{250, 17}, // the last reading, closing the observation
	})

	// And the price of it: during the silence, the expanded series claims
	// a setpoint of 21 that nobody read.
	for m := 130; m <= 190; m += 10 {
		if v := rawBack.At(m / 10).Meas; v != 21 {
			t.Errorf("raw, expanded, at minute %d: %s, want the 21 held across the silence",
				m, nav.Format(v))
		}
	}

	// Watched: the alarm at 02:14 is the only point added, and the
	// reduction keeps it.
	if watched.Len() != raw.Len()+1 {
		t.Errorf("watching added %d points, want the single alarm", watched.Len()-raw.Len())
	}
	sameGrid(t, "raw, watched, reduced", gridOf(watchedReduced), []slot{
		{0, 19},
		{50, nav.NaV},
		{62, 19},
		{88, 21},
		{134, nav.NaV}, // 01:59 + 15 minutes: the alarm
		{200, 21},      // the logger is back
		{219, 17},
		{250, 17},
	})

	// Expanded, the value holds until the alarm and not a minute longer:
	// still 21 at 02:10, unknown from 02:20 to 03:10. At 01:00 the grid
	// is NaV too, the logger's first reading after the empty one coming
	// at 01:02.
	wantWatched := make([]slot, 0, 26)
	for m := 0; m <= 250; m += 10 {
		v := 19.0
		switch {
		case m == 50, m == 60, m >= 140 && m <= 190:
			v = nav.NaV
		case m >= 220:
			v = 17
		case m >= 90:
			v = 21
		}
		wantWatched = append(wantWatched, slot{m, v})
	}
	sameGrid(t, "raw, watched, expanded", gridOf(watchedBack), wantWatched)

	// Through Regularize: a point every ten minutes, the silence as NaV.
	want := make([]slot, 0, 26)
	for m := 0; m <= 250; m += 10 {
		v := 19.0
		switch {
		case m == 50, m >= 130 && m <= 190:
			v = nav.NaV
		case m >= 220:
			v = 17
		case m >= 90:
			v = 21
		}
		want = append(want, slot{m, v})
	}
	sameGrid(t, "regularized", gridOf(regular), want)

	sameGrid(t, "regularized, reduced", gridOf(regReduced), []slot{
		{0, 19},
		{50, nav.NaV},
		{60, 19},
		{90, 21},
		{130, nav.NaV}, // the silence, kept as a change
		{200, 21},      // and the end of it
		{220, 17},
		{250, 17},
	})

	// The round trip is exact on the grid of the regularized series.
	sameGrid(t, "regularized, reduced, expanded", gridOf(regBack), gridOf(regular))
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
