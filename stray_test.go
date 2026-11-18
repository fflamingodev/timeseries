package timeseries

import (
	"errors"
	"math"
	"testing"

	nav "github.com/fflamingodev/notavalue"
)

// The case that motivated the method: a window opened between two
// readings. One reading drops to zero, the next one is back.
func TestStrayWindowOpened(t *testing.T) {
	ts := seriesOf(20, 20.5, 0, 20, 21)

	cleaned, rejected, err := ts.RemoveStrayPoints(5, StrayDown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, cleaned)
	checkInvariant(t, rejected)

	if got := rejectedAt(rejected); !sameHours(got, []int{2}) {
		t.Fatalf("rejected at hours %v, want [2]", got)
	}
	if rejected.At(0).Meas != 0 {
		t.Errorf("the reject measures %s, want its original 0", nav.Format(rejected.At(0).Meas))
	}
	if cleaned.Len() != 5 || !nav.IsNaV(cleaned.At(2).Meas) {
		t.Error("the stray reading should become a gap in place")
	}
}

// What a stray point is not: an excursion of two readings, a change of
// level, a slope.
func TestStrayIsOneReadingOnly(t *testing.T) {
	cases := []struct {
		name string
		meas []float64
	}{
		{"two readings out", []float64{20, 20, 0, 0, 20, 20}},
		{"a change of level", []float64{20, 20, 20, 0, 0, 0}},
		{"a steep slope", []float64{0, 10, 20, 30, 40}},
		{"a jump back too small", []float64{20, 20, 0, 3, 3}},
	}
	for _, c := range cases {
		_, rejected, err := seriesOf(c.meas...).RemoveStrayPoints(5, StrayBoth)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if rejected.Len() != 0 {
			t.Errorf("%s: rejected at hours %v, want nothing", c.name, rejectedAt(rejected))
		}
	}
}

// The way back need not cancel the way in: only its size matters.
func TestStrayReturnOfTheSameOrder(t *testing.T) {
	_, rejected, err := seriesOf(20, 20, 0, 17, 17).RemoveStrayPoints(5, StrayBoth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rejectedAt(rejected); !sameHours(got, []int{2}) {
		t.Errorf("rejected at hours %v, want [2]", got)
	}
}

func TestStrayDirection(t *testing.T) {
	// A spike at hour 2, a dip at hour 5.
	ts := seriesOf(10, 10, 30, 10, 10, -10, 10, 10)

	for _, c := range []struct {
		direction StrayDirection
		want      []int
	}{
		{StrayBoth, []int{2, 5}},
		{StrayUp, []int{2}},
		{StrayDown, []int{5}},
	} {
		_, rejected, err := ts.RemoveStrayPoints(5, c.direction)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.direction, err)
		}
		if got := rejectedAt(rejected); !sameHours(got, c.want) {
			t.Errorf("%s: rejected at hours %v, want %v", c.direction, got, c.want)
		}
	}
}

// The threshold is exclusive, like the bounds of RemoveOutbounds.
func TestStrayThresholdIsExclusive(t *testing.T) {
	ts := seriesOf(10, 15, 10)
	if _, rejected, _ := ts.RemoveStrayPoints(5, StrayBoth); rejected.Len() != 0 {
		t.Error("a jump exactly equal to the threshold should not count")
	}
	if _, rejected, _ := ts.RemoveStrayPoints(4.9, StrayBoth); rejected.Len() != 1 {
		t.Error("a jump just above the threshold should count")
	}
}

// The ends have one neighbor only, and are never judged.
func TestStrayEndsAreNeverStray(t *testing.T) {
	_, rejected, err := seriesOf(0, 20, 20, 20, 0).RemoveStrayPoints(5, StrayBoth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejected.Len() != 0 {
		t.Errorf("rejected at hours %v, want nothing", rejectedAt(rejected))
	}
}

// No jump is measured across a gap or a broken value, and neither is
// ever rejected itself.
func TestStrayAroundGapsAndBrokenValues(t *testing.T) {
	ts := seriesOf(20, nav.NaV, 0, 20, math.NaN(), 0, 20)

	cleaned, rejected, err := ts.RemoveStrayPoints(5, StrayBoth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Hour 2 follows a gap, hour 3 precedes a broken value, hour 5
	// follows one: none can be judged.
	if rejected.Len() != 0 {
		t.Errorf("rejected at hours %v, want nothing", rejectedAt(rejected))
	}
	if !nav.IsNaV(cleaned.At(1).Meas) {
		t.Error("the gap should stay a gap")
	}
	if m := cleaned.At(4).Meas; !math.IsNaN(m) || nav.IsNaV(m) {
		t.Error("the broken value should stay a plain NaN")
	}
}

// The judgment is made on the original series: rejecting a point does
// not change how its neighbors are seen.
func TestStrayOscillation(t *testing.T) {
	ts := seriesOf(20, 0, 20, 0, 20)
	for _, c := range []struct {
		direction StrayDirection
		want      []int
	}{
		{StrayBoth, []int{1, 2, 3}},
		{StrayDown, []int{1, 3}},
		{StrayUp, []int{2}},
	} {
		_, rejected, _ := ts.RemoveStrayPoints(5, c.direction)
		if got := rejectedAt(rejected); !sameHours(got, c.want) {
			t.Errorf("%s: rejected at hours %v, want %v", c.direction, got, c.want)
		}
	}
}

// In percent, both jumps are measured against the level before the
// excursion.
func TestStrayPercent(t *testing.T) {
	// 100 → 60 is 40 %, back to 95 is 35 % of 100.
	ts := seriesOf(100, 100, 60, 95, 95)

	_, rejected, err := ts.RemoveStrayPointsPercent(30, StrayBoth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rejectedAt(rejected); !sameHours(got, []int{2}) {
		t.Errorf("at 30 %%: rejected at hours %v, want [2]", got)
	}

	if _, rejected, _ = ts.RemoveStrayPointsPercent(36, StrayBoth); rejected.Len() != 0 {
		t.Errorf("at 36 %%: rejected at hours %v, want nothing: the way back is 35 %%", rejectedAt(rejected))
	}
}

// A dip to zero is 100 % both ways, not an infinite way back.
func TestStrayPercentDipToZero(t *testing.T) {
	_, rejected, err := seriesOf(20, 20, 0, 20).RemoveStrayPointsPercent(90, StrayDown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rejectedAt(rejected); !sameHours(got, []int{2}) {
		t.Errorf("rejected at hours %v, want [2]", got)
	}
}

// A level of zero gives no reference for a percentage.
func TestStrayPercentFromZero(t *testing.T) {
	_, rejected, err := seriesOf(0, 0, 50, 0).RemoveStrayPointsPercent(10, StrayBoth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejected.Len() != 0 {
		t.Errorf("rejected at hours %v, want nothing", rejectedAt(rejected))
	}
}

// Negative readings are measured against the magnitude of the level.
func TestStrayPercentNegativeLevel(t *testing.T) {
	_, rejected, err := seriesOf(-20, -20, -2, -20).RemoveStrayPointsPercent(50, StrayUp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rejectedAt(rejected); !sameHours(got, []int{2}) {
		t.Errorf("rejected at hours %v, want [2]", got)
	}
}

func TestStrayArguments(t *testing.T) {
	ts := seriesOf(1, 2, 3)
	for _, c := range []struct {
		name string
		call func() error
	}{
		{"zero jump", func() error { _, _, err := ts.RemoveStrayPoints(0, StrayBoth); return err }},
		{"negative jump", func() error { _, _, err := ts.RemoveStrayPoints(-1, StrayBoth); return err }},
		{"NaN jump", func() error { _, _, err := ts.RemoveStrayPoints(math.NaN(), StrayBoth); return err }},
		{"NaV jump", func() error { _, _, err := ts.RemoveStrayPoints(nav.NaV, StrayBoth); return err }},
		{"zero percent", func() error { _, _, err := ts.RemoveStrayPointsPercent(0, StrayBoth); return err }},
		{"NaN percent", func() error { _, _, err := ts.RemoveStrayPointsPercent(math.NaN(), StrayBoth); return err }},
		{"unknown direction", func() error { _, _, err := ts.RemoveStrayPoints(1, StrayDirection(7)); return err }},
		{"unknown direction, percent", func() error { _, _, err := ts.RemoveStrayPointsPercent(1, StrayDirection(-1)); return err }},
	} {
		if err := c.call(); !errors.Is(err, ErrCleaningArg) {
			t.Errorf("%s: error %v, want ErrCleaningArg", c.name, err)
		}
	}
}

func TestStrayOnTinySeries(t *testing.T) {
	for _, ts := range []*TimeSeries{NewTimeSeries("empty"), seriesOf(5), seriesOf(5, 50)} {
		cleaned, rejected, err := ts.RemoveStrayPoints(1, StrayBoth)
		if err != nil {
			t.Fatalf("%d points: unexpected error: %v", ts.Len(), err)
		}
		if cleaned.Len() != ts.Len() || rejected.Len() != 0 {
			t.Errorf("%d points: cleaned %d, rejected %d; want everything kept",
				ts.Len(), cleaned.Len(), rejected.Len())
		}
	}
}

func TestStrayComment(t *testing.T) {
	cleaned, rejected, _ := seriesOf(1, 2, 3).RemoveStrayPointsPercent(25, StrayDown)
	want := "stray points, down, jump above 25%"
	if cleaned.Comment != want || rejected.Comment != want {
		t.Errorf("comments %q and %q, want %q", cleaned.Comment, rejected.Comment, want)
	}
}
