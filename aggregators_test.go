package timeseries

import (
	"math"
	"strings"
	"testing"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// Every aggregator on the same three windows: plain readings, readings
// with a gap among them, and readings with a broken value. The third
// column is where the rules of the library show.
func TestAggregators(t *testing.T) {
	plain := []float64{2, 4, 6}
	withGap := []float64{2, nav.NaV, 6}
	withError := []float64{2, math.NaN(), 6}

	cases := []struct {
		name               string
		agg                AggFunc
		wantPlain, wantGap float64
		wantErrorIsNaN     bool // a broken reading must propagate
	}{
		{name: "AggMean", agg: AggMean, wantPlain: 4, wantGap: 4, wantErrorIsNaN: true},
		{name: "AggMedian", agg: AggMedian, wantPlain: 4, wantGap: 2, wantErrorIsNaN: true},
		{name: "AggMin", agg: AggMin, wantPlain: 2, wantGap: 2, wantErrorIsNaN: true},
		{name: "AggMax", agg: AggMax, wantPlain: 6, wantGap: 6, wantErrorIsNaN: true},
		{name: "AggSum", agg: AggSum, wantPlain: 12, wantGap: 8, wantErrorIsNaN: true},
		{name: "AggFirst", agg: AggFirst, wantPlain: 2, wantGap: 2, wantErrorIsNaN: false},
		{name: "AggLast", agg: AggLast, wantPlain: 6, wantGap: 6, wantErrorIsNaN: false},
		{name: "AggFirstUsable", agg: AggFirstUsable, wantPlain: 2, wantGap: 2, wantErrorIsNaN: false},
		{name: "AggLastUsable", agg: AggLastUsable, wantPlain: 6, wantGap: 6, wantErrorIsNaN: false},
		{name: "AggCountUsable", agg: AggCountUsable, wantPlain: 3, wantGap: 2, wantErrorIsNaN: false},
	}
	for _, c := range cases {
		if got := c.agg(plain); got != c.wantPlain {
			t.Errorf("%s on plain readings = %s, want %v", c.name, nav.Format(got), c.wantPlain)
		}
		if got := c.agg(withGap); got != c.wantGap {
			t.Errorf("%s with a gap = %s, want %v", c.name, nav.Format(got), c.wantGap)
		}
		got := c.agg(withError)
		if c.wantErrorIsNaN && !nav.IsStdNaN(got) {
			t.Errorf("%s with a broken reading = %s, want a plain NaN", c.name, nav.Format(got))
		}
	}
}

// A window holding nothing usable has nothing to say, whichever
// aggregator is asked — except the counter, for which zero is a real
// answer.
func TestAggregatorsOnAWindowOfGaps(t *testing.T) {
	gaps := []float64{nav.NaV, nav.NaV}

	for _, c := range []struct {
		name string
		agg  AggFunc
	}{
		{"AggMean", AggMean}, {"AggMedian", AggMedian}, {"AggMin", AggMin},
		{"AggMax", AggMax}, {"AggSum", AggSum},
		{"AggFirstUsable", AggFirstUsable}, {"AggLastUsable", AggLastUsable},
		{"AggSlope", AggSlope},
	} {
		if got := c.agg(gaps); !nav.IsNaV(got) {
			t.Errorf("%s on a window of gaps = %s, want NaV", c.name, nav.Format(got))
		}
	}

	if got := AggCountUsable(gaps); got != 0 {
		t.Errorf("AggCountUsable on a window of gaps = %s, want 0: the window existed and held nothing",
			nav.Format(got))
	}
	// AggFirst and AggLast report what is there, gap included.
	if !nav.IsNaV(AggFirst(gaps)) || !nav.IsNaV(AggLast(gaps)) {
		t.Error("AggFirst and AggLast should report the gap they find")
	}
}

// AggFirst reports the first reading whatever it is; AggFirstUsable
// steps over a gap to find a measurement. The pair exists because a
// state signal and a physical quantity want different answers.
func TestAggFirstAndLastAgainstTheirUsableVariants(t *testing.T) {
	window := []float64{nav.NaV, 5, 7, nav.NaV}

	if !nav.IsNaV(AggFirst(window)) {
		t.Errorf("AggFirst = %s, want the gap it starts with", nav.Format(AggFirst(window)))
	}
	if got := AggFirstUsable(window); got != 5 {
		t.Errorf("AggFirstUsable = %s, want 5", nav.Format(got))
	}
	if !nav.IsNaV(AggLast(window)) {
		t.Errorf("AggLast = %s, want the gap it ends with", nav.Format(AggLast(window)))
	}
	if got := AggLastUsable(window); got != 7 {
		t.Errorf("AggLastUsable = %s, want 7", nav.Format(got))
	}
}

func TestAggSlope(t *testing.T) {
	// A line rising by two per step.
	if got := AggSlope([]float64{0, 2, 4, 6}); got != 2 {
		t.Errorf("slope of a straight rise = %s, want 2", nav.Format(got))
	}
	// A flat window has no slope, which is zero and not NaV.
	if got := AggSlope([]float64{5, 5, 5}); got != 0 {
		t.Errorf("slope of a flat window = %s, want 0", nav.Format(got))
	}
	// A gap is skipped, and the remaining points still describe the line.
	if got := AggSlope([]float64{0, nav.NaV, 4, 6}); got != 2 {
		t.Errorf("slope with a gap = %s, want 2", nav.Format(got))
	}
	// One reading says nothing about a trend.
	if got := AggSlope([]float64{3}); !nav.IsNaV(got) {
		t.Errorf("slope of a single reading = %s, want NaV", nav.Format(got))
	}
	// A broken reading propagates.
	if got := AggSlope([]float64{0, math.NaN(), 4}); !nav.IsStdNaN(got) {
		t.Errorf("slope with a broken reading = %s, want a plain NaN", nav.Format(got))
	}
}

func TestAggIntegral(t *testing.T) {
	// A power of 10 W sampled every minute over three samples: 1800 J.
	perMinute := AggIntegral(time.Minute)
	if got := perMinute([]float64{10, 10, 10}); got != 1800 {
		t.Errorf("integral = %s, want 1800", nav.Format(got))
	}
	// A gap contributes nothing rather than breaking the computation.
	if got := perMinute([]float64{10, nav.NaV, 10}); got != 1200 {
		t.Errorf("integral with a gap = %s, want 1200", nav.Format(got))
	}
	// Nothing usable, nothing to integrate.
	if got := perMinute([]float64{nav.NaV}); !nav.IsNaV(got) {
		t.Errorf("integral of a window of gaps = %s, want NaV", nav.Format(got))
	}
	// A broken reading propagates.
	if got := perMinute([]float64{10, math.NaN()}); !nav.IsStdNaN(got) {
		t.Errorf("integral with a broken reading = %s, want a plain NaN", nav.Format(got))
	}
}

// -----------------------------------------------------------------------
// Naming
// -----------------------------------------------------------------------

// A recipe stored in a database names its aggregator in words.
func TestAggregatorByName(t *testing.T) {
	window := []float64{2, 4, 6}

	cases := []struct {
		names []string
		want  float64
	}{
		{[]string{"mean", "average", "avg", "AVERAGE", "  Mean  "}, 4},
		{[]string{"median"}, 4},
		{[]string{"min", "minimum"}, 2},
		{[]string{"max", "maximum"}, 6},
		{[]string{"sum"}, 12},
		{[]string{"first", "open"}, 2},
		{[]string{"last", "close"}, 6},
		{[]string{"count", "countusable"}, 3},
	}
	for _, c := range cases {
		for _, name := range c.names {
			agg, err := Aggregator(name)
			if err != nil {
				t.Errorf("Aggregator(%q): %v", name, err)
				continue
			}
			if got := agg(window); got != c.want {
				t.Errorf("Aggregator(%q) computed %s, want %v", name, nav.Format(got), c.want)
			}
		}
	}
}

// An unknown name is an error, not a silent fallback on the mean: a
// recipe asking for something the library cannot do must say so.
func TestAggregatorRejectsAnUnknownName(t *testing.T) {
	agg, err := Aggregator("harmonic-mean")
	if err == nil {
		t.Fatal("no error for an unknown aggregator")
	}
	if agg != nil {
		t.Error("an aggregator was returned along with the error")
	}
	// The message lists what is available, so the caller can fix the recipe.
	if !strings.Contains(err.Error(), "median") {
		t.Errorf("the error does not say what is available: %v", err)
	}
}

func TestAggregatorNames(t *testing.T) {
	names := AggregatorNames()
	if len(names) < 10 {
		t.Errorf("only %d names: %v", len(names), names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("the names are not sorted: %v", names)
			break
		}
	}
	// Every name it advertises must work.
	for _, name := range names {
		if _, err := Aggregator(name); err != nil {
			t.Errorf("Aggregator(%q) is advertised but fails: %v", name, err)
		}
	}
}

// The whole point of naming them: regularizing from a recipe.
func TestRegularizeWithANamedAggregator(t *testing.T) {
	ts := seriesAtMinutes(slot{10, 1}, slot{20, 3}, slot{70, 10})

	agg, err := Aggregator("maximum")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := ts.Regularize(time.Hour, agg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sameGrid(t, "hourly maximum", gridOf(out), []slot{{60, 3}, {120, 10}})
}
