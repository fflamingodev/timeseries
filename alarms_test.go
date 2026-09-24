package timeseries

import (
	"errors"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

func markSilences(t *testing.T, ts *TimeSeries, maxGap time.Duration) *TimeSeries {
	t.Helper()
	out, err := ts.MarkSilences(maxGap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checkInvariant(t, out)
	return out
}

// A silence longer than the delay gets a NaV, dated when the alarm
// would have fired.
func TestMarkSilencesDatesTheAlarm(t *testing.T) {
	ts := seriesAtMinutes(slot{0, 1}, slot{10, 1}, slot{60, 2}, slot{70, 2})

	sameGrid(t, "one silence", gridOf(markSilences(t, ts, 15*time.Minute)), []slot{
		{0, 1}, {10, 1},
		{25, nav.NaV}, // 10 + 15: the alarm
		{60, 2}, {70, 2},
	})
}

// Every silence gets its own alarm.
func TestMarkSilencesSeveralSilences(t *testing.T) {
	ts := seriesAtMinutes(slot{0, 1}, slot{30, 2}, slot{40, 3}, slot{100, 4})

	sameGrid(t, "two silences", gridOf(markSilences(t, ts, 15*time.Minute)), []slot{
		{0, 1}, {15, nav.NaV}, {30, 2}, {40, 3}, {55, nav.NaV}, {100, 4},
	})
}

// Two readings exactly the delay apart raise no alarm.
func TestMarkSilencesExactlyTheDelay(t *testing.T) {
	ts := seriesAtMinutes(slot{0, 1}, slot{15, 2}, slot{31, 3})

	sameGrid(t, "at the delay", gridOf(markSilences(t, ts, 15*time.Minute)), []slot{
		{0, 1}, {15, 2}, // exactly 15: nothing
		{30, nav.NaV}, {31, 3}, // 16: an alarm
	})
}

// A silence after a NaV adds nothing: the signal is already unknown.
func TestMarkSilencesAfterAGap(t *testing.T) {
	ts := seriesAtMinutes(slot{0, 1}, slot{10, nav.NaV}, slot{60, 2})

	sameGrid(t, "after a gap", gridOf(markSilences(t, ts, 15*time.Minute)), []slot{
		{0, 1}, {10, nav.NaV}, {60, 2},
	})
}

// A regular series within the delay is left as it was.
func TestMarkSilencesNothingToMark(t *testing.T) {
	ts := everyMinute(1, 2, 3, 4)
	sameGrid(t, "no silence", gridOf(markSilences(t, ts, 2*time.Minute)), gridOf(ts))
}

func TestMarkSilencesEdges(t *testing.T) {
	empty := markSilences(t, NewTimeSeries("empty"), time.Minute)
	if empty.Len() != 0 {
		t.Errorf("%d points from an empty series", empty.Len())
	}

	// Nothing after the last reading, however long ago it was.
	one := markSilences(t, seriesAtMinutes(slot{0, 7}), time.Minute)
	sameGrid(t, "one reading", gridOf(one), []slot{{0, 7}})
}

func TestMarkSilencesRejectsImpossibleDelays(t *testing.T) {
	ts := everyMinute(1, 2)
	for name, d := range map[string]time.Duration{
		"a delay of zero":  0,
		"a negative delay": -time.Minute,
	} {
		if _, err := ts.MarkSilences(d); !errors.Is(err, ErrAlarmArg) {
			t.Errorf("%s: error is %v, want an ErrAlarmArg", name, err)
		}
	}
}

func TestMarkSilencesProducesAProperSeries(t *testing.T) {
	ts := seriesAtMinutes(slot{0, 1}, slot{60, 2})
	ts.ID = "device-42"

	out := markSilences(t, ts, 15*time.Minute)
	if out.ID != "device-42" {
		t.Error("the watched series lost the identity of its source")
	}
	if out.Comment == "" {
		t.Error("nothing says how the silences were marked")
	}
	if ts.Len() != 2 {
		t.Errorf("the source now holds %d points", ts.Len())
	}
}

// The point of it all: once marked, a silence survives Reduce, and
// Expand no longer holds the value across it.
func TestMarkSilencesThenReduce(t *testing.T) {
	ts := seriesAtMinutes(slot{0, 5}, slot{10, 5}, slot{60, 5}, slot{70, 5})

	blind := ts.Reduce()
	sameGrid(t, "reduced blind", gridOf(blind), []slot{{0, 5}, {70, 5}})

	watched := markSilences(t, ts, 15*time.Minute).Reduce()
	sameGrid(t, "reduced watched", gridOf(watched), []slot{
		{0, 5}, {25, nav.NaV}, {60, 5}, {70, 5},
	})

	back, err := watched.Expand(atMinute(0), atMinute(70), 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "expanded", gridOf(back), []slot{
		{0, 5}, {10, 5}, {20, 5}, // held until the alarm at 25
		{30, nav.NaV}, {40, nav.NaV}, {50, nav.NaV},
		{60, 5}, {70, 5},
	})
}
