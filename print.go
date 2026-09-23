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
//
// The table is laid out by subject rather than by statistic, because
// the columns of the previous layout invited a confusion this one
// prevents. "The series starts at 00:00, where 12.4 was measured" and
// "the lowest reading, 11.5, was taken at 03:00" are two different
// statements, and they now sit in two different sections, each value
// next to the instant it belongs to.
func (bs BasicStats) Fprint(w io.Writer, title string) {
	fmt.Fprintln(w, "================================================================")
	if title != "" {
		fmt.Fprintln(w, title)
	}

	usable := bs.Len - bs.NbreOfNaN
	errors := bs.NbreOfNaN - bs.NbreOfNaV
	fmt.Fprintf(w, "%d points   %d usable   %d missing (NaV)   %d broken (NaN)\n",
		bs.Len, usable, bs.NbreOfNaV, errors)
	if errors > 0 {
		fmt.Fprintln(w, "A broken measurement propagates: every statistic below that")
		fmt.Fprintln(w, "depends on the measurements is NaN, on purpose.")
	}

	// Left-aligned here: these rows are labelled statements, not columns
	// of figures to compare down the page.
	tw := new(tabwriter.Writer)
	tw.Init(w, 2, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "\nWHEN — the span of the series\t\t\t")
	fmt.Fprintf(tw, "  first point (Chmin)\t%v\tmeasuring (ValAtChmin)\t%v\t\n",
		formatTime(bs.Chmin), nav.Format(bs.ValAtChmin))
	fmt.Fprintf(tw, "  last point (Chmax)\t%v\tmeasuring (ValAtChmax)\t%v\t\n",
		formatTime(bs.Chmax), nav.Format(bs.ValAtChmax))
	fmt.Fprintf(tw, "  first usable (ChFirstUsable)\t%v\tmeasuring (ValAtFirstUsable)\t%v\t\n",
		formatTime(bs.ChFirstUsable), nav.Format(bs.ValAtFirstUsable))
	fmt.Fprintf(tw, "  mean instant (Chmean)\t%v\t\t\t\n", formatTime(bs.Chmean))
	fmt.Fprintf(tw, "  median instant (Chmed)\t%v\t\t\t\n", formatTime(bs.Chmed))

	fmt.Fprintln(tw, "\nWHAT — the measurements\t\t\t")
	fmt.Fprintf(tw, "  lowest (Msmin)\t%v\tmeasured at (ChAtMsmin)\t%v\t\n",
		nav.Format(bs.Msmin), formatTime(bs.ChAtMsmin))
	fmt.Fprintf(tw, "  highest (Msmax)\t%v\tmeasured at (ChAtMsmax)\t%v\t\n",
		nav.Format(bs.Msmax), formatTime(bs.ChAtMsmax))
	fmt.Fprintf(tw, "  mean (Msmean)\t%v\t\t\t\n", nav.Format(bs.Msmean))
	fmt.Fprintf(tw, "  median (Msmed)\t%v\t\t\t\n", nav.Format(bs.Msmed))
	fmt.Fprintf(tw, "  std deviation (Msstd)\t%v\t\t\t\n", nav.Format(bs.Msstd))

	fmt.Fprintln(tw, "\nHOW OFTEN — the intervals between points\t\t\t")
	fmt.Fprintf(tw, "  shortest (DChmin)\t%v\tending at (ChAtDChmin)\t%v\t\n",
		formatDuration(bs.DChmin), formatTime(bs.ChAtDChmin))
	fmt.Fprintf(tw, "  longest (DChmax)\t%v\tending at (ChAtDchmax)\t%v\t\n",
		formatDuration(bs.DChmax), formatTime(bs.ChAtDchmax))
	fmt.Fprintf(tw, "  mean (DChmean)\t%v\t\t\t\n", formatNanos(bs.DChmean))
	fmt.Fprintf(tw, "  median (DChmed)\t%v\t\t\t\n", formatNanos(bs.DChmed))
	fmt.Fprintf(tw, "  irregularity (DChstd)\t%v\t\t\t\n", formatNanos(bs.DChstd))

	fmt.Fprintln(tw, "\nBY HOW MUCH — the variations from one point to the next\t\t\t")
	fmt.Fprintf(tw, "  largest fall (DMsmin)\t%v\t\t\t\n", nav.Format(bs.DMsmin))
	fmt.Fprintf(tw, "  largest rise (DMsmax)\t%v\t\t\t\n", nav.Format(bs.DMsmax))
	fmt.Fprintf(tw, "  mean (DMsmean)\t%v\t\t\t\n", nav.Format(bs.DMsmean))
	fmt.Fprintf(tw, "  median (DMsmed)\t%v\t\t\t\n", nav.Format(bs.DMsmed))
	fmt.Fprintf(tw, "  std deviation (DMsstd)\t%v\t\t\t\n", nav.Format(bs.DMsstd))

	fmt.Fprintln(tw)
	tw.Flush()
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
