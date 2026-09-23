package timeseries

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// This file is for looking at a series with your own eyes: in a
// terminal, while exploring data or chasing a bug. Nothing here is on
// the path of a computation.
//
// Each printer comes in two forms. PrettyPrint and its kin write to the
// standard output, which is what you want from a command or a test.
// The Fprint* forms take an io.Writer, for a log, a file, or a buffer
// in a test that wants to inspect what was produced. A library has no
// business deciding that output goes to the terminal.
//
// Missing values print as "NaV", computation errors as "NaN" and absent
// durations as "NaDuration". Seeing a gap where there is one is the
// whole point of the exercise.

// formatDuration renders a duration, showing the sentinel by name
// rather than as the nonsense figure of -2562047h47m16s.
func formatDuration(d time.Duration) string {
	if IsNaDuration(d) {
		return "NaDuration"
	}
	return d.String()
}

// formatTime renders an instant, showing the zero time as a dash: on a
// statistic that could not be computed, "0001-01-01 00:00:00 UTC" is
// noise.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Round(0).Format("2006-01-02 15:04:05.999")
}

// newTabWriter returns the writer used by every printer here, so that
// the columns of a series and those of its summary line up.
func newTabWriter(w io.Writer) *tabwriter.Writer {
	tw := new(tabwriter.Writer)
	tw.Init(w, 5, 0, 3, ' ', tabwriter.AlignRight)
	return tw
}

// -----------------------------------------------------------------------
// A single point
// -----------------------------------------------------------------------

// PrettyPrint writes the point to the standard output, with a header.
func (du DataUnit) PrettyPrint() {
	du.Fprint(os.Stdout)
}

// Fprint writes the point to w, with a header.
func (du DataUnit) Fprint(w io.Writer) {
	tw := newTabWriter(w)
	fmt.Fprintf(tw, "%v|\t%v|\t%v|\t%v|\t\n", "Chron", "Measure", "Dchron", "Dmeas")
	fmt.Fprintln(tw, "------------------------|\t------------|\t------------|\t------------|\t")
	du.writeRow(tw)
	fmt.Fprintln(tw)
	tw.Flush()
}

// writeRow writes the four columns of a point, without a header.
func (du DataUnit) writeRow(tw *tabwriter.Writer) {
	fmt.Fprintf(tw, "%v|\t%v|\t%v|\t%v|\t\n",
		formatTime(du.Chron),
		nav.Format(du.Meas),
		formatDuration(du.Dchron),
		nav.Format(du.Dmeas),
	)
}

// -----------------------------------------------------------------------
// A series
// -----------------------------------------------------------------------

// PrettyPrint writes the points of the series to the standard output.
//
// The optional arguments select a window, which is what makes a long
// series readable:
//
//	ts.PrettyPrint()       // everything
//	ts.PrettyPrint(10)     // the first ten points
//	ts.PrettyPrint(90, 95) // points 90 to 94
//
// Indices outside the series are clamped rather than fatal: this is a
// debugging aid, and it should never be the thing that panics.
func (ts *TimeSeries) PrettyPrint(what ...int) {
	ts.Fprint(os.Stdout, what...)
}

// Fprint writes the points of the series to w. See PrettyPrint for the
// meaning of the optional arguments.
func (ts *TimeSeries) Fprint(w io.Writer, what ...int) {
	fmt.Fprintf(w, "Series     : %v\n", ts.Name)
	if ts.ID != "" {
		fmt.Fprintf(w, "ID         : %v\n", ts.ID)
	}
	if ts.Comment != "" {
		fmt.Fprintf(w, "Comment    : %v\n", ts.Comment)
	}

	from, to := window(ts.Len(), what)
	if from >= to {
		fmt.Fprintln(w, "(empty series, or indices out of range)")
		return
	}

	tw := newTabWriter(w)
	fmt.Fprintf(tw, "%v|\t%v|\t%v|\t%v|\t%v|\t\n", "index", "Chron", "Measure", "Dchron", "Dmeas")
	fmt.Fprintln(tw, "-----|\t------------------------|\t------------|\t------------|\t------------|\t")
	for i := from; i < to; i++ {
		fmt.Fprintf(tw, "%d|\t", i)
		ts.At(i).writeRow(tw)
	}
	fmt.Fprintln(tw)
	tw.Flush()

	if to < ts.Len() || from > 0 {
		fmt.Fprintf(w, "(points %d to %d of %d)\n", from, to-1, ts.Len())
	}
}

// window turns the optional arguments of PrettyPrint into a [from, to)
// range clamped to the series.
func window(n int, what []int) (from, to int) {
	switch len(what) {
	case 0:
		from, to = 0, n
	case 1:
		from, to = 0, what[0]
	default:
		from, to = what[0], what[1]
	}
	if from < 0 {
		from = 0
	}
	if to > n {
		to = n
	}
	return from, to
}

// -----------------------------------------------------------------------
// The summary
// -----------------------------------------------------------------------

// PrintStats computes the summary of the series and writes it to the
// standard output as a table.
func (ts *TimeSeries) PrintStats() {
	ts.FprintStats(os.Stdout)
}

// FprintStats computes the summary of the series and writes it to w.
//
// The table reads by column: what the timestamps do, what the
// measurements do, then the same for the intervals between points and
// for the variations from one point to the next.
func (ts *TimeSeries) FprintStats(w io.Writer) {
	bs := ts.Stats()
	bs.Fprint(w, ts.Name)
}

// Fprint writes the summary to w, under the given title.
func (bs BasicStats) Fprint(w io.Writer, title string) {
	fmt.Fprintln(w, "------------------------------------------------------------")
	if title != "" {
		fmt.Fprintln(w, title)
	}
	fmt.Fprintf(w, "Points: %d", bs.Len)
	if bs.NbreOfNaV > 0 || bs.NbreOfNaN > 0 {
		fmt.Fprintf(w, "   Missing: %d   Errors: %d", bs.NbreOfNaV, bs.NbreOfNaN-bs.NbreOfNaV)
	}
	fmt.Fprintln(w)

	tw := newTabWriter(w)
	fmt.Fprintln(tw, " |\tChron|\tMeasure|\tDChron|\tDMeas|\t")
	fmt.Fprintln(tw, "-------|\t------------------------|\t------------|\t------------------|\t------------|\t")
	fmt.Fprintf(tw, "Min|\t%v|\t%v|\t%v|\t%v|\t\n",
		formatTime(bs.Chmin), nav.Format(bs.Msmin), formatDuration(bs.DChmin), nav.Format(bs.DMsmin))
	fmt.Fprintf(tw, "Max|\t%v|\t%v|\t%v|\t%v|\t\n",
		formatTime(bs.Chmax), nav.Format(bs.Msmax), formatDuration(bs.DChmax), nav.Format(bs.DMsmax))
	fmt.Fprintf(tw, "Mean|\t%v|\t%v|\t%v|\t%v|\t\n",
		formatTime(bs.Chmean), nav.Format(bs.Msmean), formatNanos(bs.DChmean), nav.Format(bs.DMsmean))
	fmt.Fprintf(tw, "Median|\t%v|\t%v|\t%v|\t%v|\t\n",
		formatTime(bs.Chmed), nav.Format(bs.Msmed), formatNanos(bs.DChmed), nav.Format(bs.DMsmed))
	// Chstd holds a dispersion encoded as an instant counted from the
	// zero time; printed as a date it reads as a year 1 timestamp, which
	// means nothing. Shown as the duration it is.
	fmt.Fprintf(tw, "StdDev|\t%v|\t%v|\t%v|\t%v|\t\n",
		formatDuration(bs.Chstd.Sub(time.Time{})), nav.Format(bs.Msstd),
		formatNanos(bs.DChstd), nav.Format(bs.DMsstd))
	fmt.Fprintln(tw)
	tw.Flush()

	fmt.Fprintf(w, "Value at first point: %v   at last point: %v\n",
		nav.Format(bs.ValAtChmin), nav.Format(bs.ValAtChmax))
	fmt.Fprintf(w, "Shortest interval at: %v   longest at: %v\n",
		formatTime(bs.ChAtDChmin), formatTime(bs.ChAtDchmax))
	fmt.Fprintf(w, "Minimum measured at : %v   maximum at: %v\n",
		formatTime(bs.ChAtMsmin), formatTime(bs.ChAtMsmax))
}

// formatNanos renders a count of nanoseconds held in a float64 — the
// form the delta statistics take — as a readable duration.
func formatNanos(ns float64) string {
	if nav.IsNaV(ns) || ns != ns {
		return nav.Format(ns)
	}
	return time.Duration(ns).String()
}

// PrettyPrintAll writes the points of the series and then its summary,
// to the standard output.
func (ts *TimeSeries) PrettyPrintAll(what ...int) {
	ts.PrettyPrint(what...)
	ts.PrintStats()
}
