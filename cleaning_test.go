package timeseries

import (
	"errors"
	"math"
	"testing"

	nav "github.com/fflamingodev/notavalue"
)

// rejectedAt reports the hours at which the rejected series holds a
// point, so a test can state what was thrown out in one line.
func rejectedAt(rejected *TimeSeries) []int {
	var hours []int
	rejected.Range(func(_ int, du DataUnit) bool {
		hours = append(hours, int(du.Chron.Sub(origin).Hours()))
		return true
	})
	return hours
}

// sameHours compares two lists of hours.
func sameHours(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// -----------------------------------------------------------------------
// What a rejection does
// -----------------------------------------------------------------------

// The heart of the matter: a rejected reading becomes a gap, keeps its
// instant, and is handed back with its original value.
func TestRejectionTurnsAReadingIntoAGap(t *testing.T) {
	ts := seriesOf(10, 11, 900, 12) // 900 is the intruder

	cleaned, rejected, err := ts.RemoveOutbounds(0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, cleaned)
	checkInvariant(t, rejected)

	// The cleaned series keeps its four slots in time.
	if cleaned.Len() != 4 {
		t.Fatalf("cleaned has %d points, want 4: a rejection is not a deletion", cleaned.Len())
	}
	if !nav.IsNaV(cleaned.At(2).Meas) {
		t.Errorf("the rejected reading is %s in the cleaned series, want NaV",
			nav.Format(cleaned.At(2).Meas))
	}
	if !cleaned.At(2).Chron.Equal(at(2)) {
		t.Error("the rejected point lost its place in time")
	}

	// And nothing is lost: the reject keeps its value.
	if rejected.Len() != 1 || rejected.At(0).Meas != 900 {
		t.Errorf("rejected = %d points, first measuring %s; want one point measuring 900",
			rejected.Len(), nav.Format(rejected.At(0).Meas))
	}
	if !rejected.At(0).Chron.Equal(at(2)) {
		t.Error("the rejected point lost its instant")
	}
}

// Why NaV and not NaN: the statistics of the cleaned series must go on
// saying something. The previous version of this library used a plain
// NaN here, and one outlier in a month poisoned the whole month.
func TestCleanedSeriesStillHasStatistics(t *testing.T) {
	ts := seriesOf(10, 11, 900, 12)

	cleaned, _, err := ts.RemoveOutbounds(0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bs := cleaned.Stats()
	if nav.IsNaV(bs.Msmean) || math.IsNaN(bs.Msmean) {
		t.Fatalf("mean of the cleaned series = %s, want a number", nav.Format(bs.Msmean))
	}
	if want := 11.0; bs.Msmean != want { // (10+11+12)/3
		t.Errorf("mean = %s, want %v", nav.Format(bs.Msmean), want)
	}
	// And the summary says how much was discarded.
	if bs.NbreOfNaV != 1 || bs.NbreOfNaN != 1 {
		t.Errorf("missing = %d, non-numbers = %d; want 1 and 1: the rejection is a gap, not an error",
			bs.NbreOfNaV, bs.NbreOfNaN)
	}
}

// A gap has nothing to judge, and a broken value is another problem.
// Neither is rejected, and neither takes part in the fences.
func TestCleaningLeavesGapsAndErrorsAlone(t *testing.T) {
	ts := seriesOf(10, nav.NaV, math.NaN(), 900, 12)

	cleaned, rejected, err := ts.RemoveOutbounds(0, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !nav.IsNaV(cleaned.At(1).Meas) {
		t.Error("the gap did not stay a gap")
	}
	if !nav.IsStdNaN(cleaned.At(2).Meas) {
		t.Errorf("the broken value became %s: an error must not be disguised as a gap",
			nav.Format(cleaned.At(2).Meas))
	}
	// Only the intruder is rejected.
	if got := rejectedAt(rejected); !sameHours(got, []int{3}) {
		t.Errorf("rejected at hours %v, want only hour 3", got)
	}
}

// -----------------------------------------------------------------------
// Fixed bounds
// -----------------------------------------------------------------------

func TestRemoveOutbounds(t *testing.T) {
	cases := []struct {
		name     string
		meas     []float64
		min, max float64
		want     []int // hours rejected
	}{
		{"both fences", []float64{-5, 10, 20, 900}, 0, 100, []int{0, 3}},
		{"the bounds are inclusive", []float64{0, 50, 100}, 0, 100, nil},
		{"no lower fence", []float64{-5, 10, 900}, nav.NaV, 100, []int{2}},
		{"no upper fence", []float64{-5, 10, 900}, 0, nav.NaV, []int{0}},
		{"no fence at all", []float64{-5, 10, 900}, nav.NaV, nav.NaV, nil},
		{"nothing to reject", []float64{10, 20}, 0, 100, nil},
	}
	for _, c := range cases {
		cleaned, rejected, err := seriesOf(c.meas...).RemoveOutbounds(c.min, c.max)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		}
		if got := rejectedAt(rejected); !sameHours(got, c.want) {
			t.Errorf("%s: rejected at hours %v, want %v", c.name, got, c.want)
		}
		checkInvariant(t, cleaned)
		checkInvariant(t, rejected)
		if cleaned.Len() != len(c.meas) {
			t.Errorf("%s: cleaned has %d points, want %d", c.name, cleaned.Len(), len(c.meas))
		}
	}
}

func TestRemoveOutboundsRejectsImpossibleArguments(t *testing.T) {
	ts := seriesOf(1, 2, 3)
	cases := []struct {
		name     string
		min, max float64
	}{
		{"bounds in the wrong order", 100, 0},
		{"a NaN as lower bound", math.NaN(), 100},
		{"a NaN as upper bound", 0, math.NaN()},
	}
	for _, c := range cases {
		_, _, err := ts.RemoveOutbounds(c.min, c.max)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		if !errors.Is(err, ErrCleaningArg) {
			t.Errorf("%s: error is %v, want an ErrCleaningArg", c.name, err)
		}
	}
}

// The cleaned and rejected series carry the identity of their source,
// and say how they were produced.
func TestCleaningRecordsHowItWasDone(t *testing.T) {
	ts := seriesOf(10, 900)
	ts.ID = "device-42"

	cleaned, rejected, _ := ts.RemoveOutbounds(0, 100)

	if cleaned.ID != "device-42" || rejected.ID != "device-42" {
		t.Error("the identity of the source series was lost")
	}
	if cleaned.Comment == "" || rejected.Comment == "" {
		t.Error("nothing says how the split was made")
	}
	if cleaned.Name == ts.Name || rejected.Name == ts.Name {
		t.Error("the two outputs are not distinguishable from their source by name")
	}
}

// -----------------------------------------------------------------------
// Percentile fences
// -----------------------------------------------------------------------

func TestRemovePercentileOutliers(t *testing.T) {
	// Ten readings from 1 to 10. The 10th percentile is 1 and the 90th
	// is 9, by nearest rank.
	meas := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	cleaned, rejected, err := seriesOf(meas...).RemovePercentileOutliers(10, 90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, cleaned)

	// Only the reading above the upper fence goes: 10 > 9.
	if got := rejectedAt(rejected); !sameHours(got, []int{9}) {
		t.Errorf("rejected at hours %v, want only hour 9", got)
	}

	// One-sided, upper only.
	_, rejectedHigh, err := seriesOf(meas...).RemovePercentileOutliers(nav.NaV, 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rejectedAt(rejectedHigh); !sameHours(got, []int{8, 9}) {
		t.Errorf("rejected at hours %v, want hours 8 and 9", got)
	}
}

func TestRemovePercentileOutliersRejectsImpossibleArguments(t *testing.T) {
	ts := seriesOf(1, 2, 3)
	for _, c := range []struct {
		name      string
		low, high float64
	}{
		{"zero is not a percentile", 0, 90},
		{"a hundred is not a percentile either", 10, 100},
		{"negative", -5, 90},
		{"reversed", 90, 10},
		{"NaN", math.NaN(), 90},
	} {
		if _, _, err := ts.RemovePercentileOutliers(c.low, c.high); !errors.Is(err, ErrCleaningArg) {
			t.Errorf("%s: error is %v, want an ErrCleaningArg", c.name, err)
		}
	}
}

// -----------------------------------------------------------------------
// The z-score fence
// -----------------------------------------------------------------------

func TestRemoveZScoreOutliers(t *testing.T) {
	// Nine readings around ten, and one far away.
	ts := seriesOf(10, 10, 11, 9, 10, 11, 9, 10, 10, 500)

	cleaned, rejected, err := ts.RemoveZScoreOutliers(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, cleaned)

	if got := rejectedAt(rejected); !sameHours(got, []int{9}) {
		t.Errorf("rejected at hours %v, want only hour 9", got)
	}
	// What is left is the honest signal.
	if bs := cleaned.Stats(); bs.Msmax != 11 || bs.Msmin != 9 {
		t.Errorf("the cleaned series spans [%s, %s], want [9, 11]",
			nav.Format(bs.Msmin), nav.Format(bs.Msmax))
	}
}

func TestRemoveZScoreOutliersEdgeCases(t *testing.T) {
	if _, _, err := seriesOf(1, 2, 3).RemoveZScoreOutliers(0); !errors.Is(err, ErrCleaningArg) {
		t.Errorf("a level of zero gave %v, want an ErrCleaningArg", err)
	}
	if _, _, err := seriesOf(1, 2, 3).RemoveZScoreOutliers(-1); !errors.Is(err, ErrCleaningArg) {
		t.Errorf("a negative level gave %v, want an ErrCleaningArg", err)
	}

	// One reading says nothing about dispersion, so nothing can be
	// called an outlier.
	_, rejected, err := seriesOf(42).RemoveZScoreOutliers(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejected.Len() != 0 {
		t.Errorf("%d points rejected from a single reading", rejected.Len())
	}

	// Neither does a series of nothing but gaps.
	_, rejected, err = seriesOf(nav.NaV, nav.NaV).RemoveZScoreOutliers(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejected.Len() != 0 {
		t.Errorf("%d points rejected from a series of gaps", rejected.Len())
	}
}

// -----------------------------------------------------------------------
// Peirce's criterion
// -----------------------------------------------------------------------

func TestRemovePeirceOutliers(t *testing.T) {
	// The example Peirce's criterion is usually taught with: a series
	// whose last reading is plainly out of line.
	ts := seriesOf(101.2, 90.0, 99.0, 102.0, 103.0, 100.2, 89.0, 98.1, 101.5, 102.0)

	cleaned, rejected, err := ts.RemovePeirceOutliers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, cleaned)
	checkInvariant(t, rejected)

	// The criterion condemns the two low readings, 90 and 89, and
	// spares everything else.
	if got := rejectedAt(rejected); !sameHours(got, []int{1, 6}) {
		t.Errorf("rejected at hours %v, want hours 1 and 6 (the readings 90 and 89)", got)
	}
	if cleaned.Len() != ts.Len() {
		t.Error("the cleaned series lost points instead of turning them into gaps")
	}
	for i := 0; i < cleaned.Len(); i++ {
		du := cleaned.At(i)
		if i == 1 || i == 6 {
			if !nav.IsNaV(du.Meas) {
				t.Errorf("point %d should have become a gap, it measures %s", i, nav.Format(du.Meas))
			}
			continue
		}
		if nav.IsNaV(du.Meas) {
			t.Errorf("point %d was rejected although it belongs to the tight group", i)
		}
	}
}

// The criterion is able to condemn several readings at once, which is
// what distinguishes it from a fixed fence applied one point at a time.
func TestPeirceRejectsSeveralReadings(t *testing.T) {
	ts := seriesOf(10, 10, 11, 9, 10, 11, 9, 10, 500, 400)

	_, rejected, err := ts.RemovePeirceOutliers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rejectedAt(rejected); !sameHours(got, []int{8, 9}) {
		t.Errorf("rejected at hours %v, want both intruders at hours 8 and 9", got)
	}
}

// A well-behaved series must come out untouched: a criterion that
// always finds an outlier is of no use.
func TestPeirceLeavesACleanSeriesAlone(t *testing.T) {
	ts := seriesOf(10, 10.1, 9.9, 10.2, 9.8, 10, 10.1, 9.95, 10.05, 10)

	_, rejected, err := ts.RemovePeirceOutliers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejected.Len() != 0 {
		t.Errorf("%d readings rejected from a series that has no outlier: %v",
			rejected.Len(), rejectedAt(rejected))
	}
}

// The table starts at three readings; below that, a rejection would say
// more about the criterion than about the data.
func TestPeirceNeedsThreeReadings(t *testing.T) {
	for _, meas := range [][]float64{{}, {1}, {1, 100}} {
		_, rejected, err := seriesOf(meas...).RemovePeirceOutliers()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rejected.Len() != 0 {
			t.Errorf("on %v: %d readings rejected, want none", meas, rejected.Len())
		}
	}
}

// A flat series has no dispersion, so nothing can stand out of it.
func TestPeirceOnAFlatSeries(t *testing.T) {
	_, rejected, err := seriesOf(7, 7, 7, 7, 7).RemovePeirceOutliers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejected.Len() != 0 {
		t.Errorf("%d readings rejected from a flat series", rejected.Len())
	}
}

// -----------------------------------------------------------------------
// Empty series
// -----------------------------------------------------------------------

func TestCleaningAnEmptySeries(t *testing.T) {
	empty := NewTimeSeries("empty")

	for _, c := range []struct {
		name string
		run  func() (*TimeSeries, *TimeSeries, error)
	}{
		{"bounds", func() (*TimeSeries, *TimeSeries, error) { return empty.RemoveOutbounds(0, 100) }},
		{"percentiles", func() (*TimeSeries, *TimeSeries, error) { return empty.RemovePercentileOutliers(10, 90) }},
		{"z-score", func() (*TimeSeries, *TimeSeries, error) { return empty.RemoveZScoreOutliers(3) }},
		{"Peirce", func() (*TimeSeries, *TimeSeries, error) { return empty.RemovePeirceOutliers() }},
	} {
		cleaned, rejected, err := c.run()
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		}
		if cleaned.Len() != 0 || rejected.Len() != 0 {
			t.Errorf("%s: cleaned %d, rejected %d; want nothing at all",
				c.name, cleaned.Len(), rejected.Len())
		}
	}
}
