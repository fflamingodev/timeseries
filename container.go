package timeseries

import (
	"fmt"
	"io"
	"os"
)

// TsContainer holds several named TimeSeries that belong together:
// typically one signal in its successive states — the raw readings, a
// cleaned version, a regularized one, each variant a recipe produced.
//
// The names are the caller's: "raw", "hourly-mean", "outliers-removed".
// What they mean is a matter for the caller too; the container only
// guarantees that a name designates one series and that the order in
// which they were added is the order in which they come back.
//
// That last point is not a detail. A Go map iterates in a random order,
// so an earlier version of this library printed the variants of a
// container in a different order on every run, and a chart legend built
// from it reshuffled itself. The container keeps its own list of names.
type TsContainer struct {
	// Name identifies the container: the datasource it comes from, the
	// device, the query that produced it.
	Name string

	// Comment records anything worth knowing about the whole set — the
	// time window requested, the recipe applied, a warning.
	Comment string

	series map[string]*TimeSeries
	order  []string
}

// NewTsContainer returns an empty container with the given name.
func NewTsContainer(name string) *TsContainer {
	return &TsContainer{
		Name:   name,
		series: make(map[string]*TimeSeries),
	}
}

// Put files ts under name. A name already in use is overwritten in
// place, keeping its rank: replacing the cleaned version of a signal
// does not move it to the end of the list.
//
// A nil series is stored as such. It says "this variant was asked for
// and could not be produced", which is worth keeping, and the readers
// of a TimeSeries all tolerate nil.
func (tsc *TsContainer) Put(name string, ts *TimeSeries) {
	if tsc.series == nil {
		tsc.series = make(map[string]*TimeSeries)
	}
	if _, exists := tsc.series[name]; !exists {
		tsc.order = append(tsc.order, name)
	}
	tsc.series[name] = ts
}

// Get returns the series filed under name, and whether it was there at
// all. The two are worth telling apart: a name that was never filed and
// a name filed with nil are different statements.
func (tsc *TsContainer) Get(name string) (*TimeSeries, bool) {
	if tsc == nil {
		return nil, false
	}
	ts, ok := tsc.series[name]
	return ts, ok
}

// Series returns the series filed under name, or nil. Use it when the
// distinction Get makes does not matter, since every read method of
// TimeSeries is safe on a nil receiver.
func (tsc *TsContainer) Series(name string) *TimeSeries {
	ts, _ := tsc.Get(name)
	return ts
}

// Delete removes a name and its series.
func (tsc *TsContainer) Delete(name string) {
	if tsc == nil {
		return
	}
	if _, exists := tsc.series[name]; !exists {
		return
	}
	delete(tsc.series, name)
	for i, n := range tsc.order {
		if n == name {
			tsc.order = append(tsc.order[:i], tsc.order[i+1:]...)
			break
		}
	}
}

// Len returns how many series the container holds.
func (tsc *TsContainer) Len() int {
	if tsc == nil {
		return 0
	}
	return len(tsc.series)
}

// Names returns the names in the order they were first filed. The
// returned slice is a copy: reordering it does not reorder the
// container.
func (tsc *TsContainer) Names() []string {
	if tsc == nil {
		return nil
	}
	out := make([]string, len(tsc.order))
	copy(out, tsc.order)
	return out
}

// Range calls f for every series, in the order they were filed,
// stopping early if f returns false.
func (tsc *TsContainer) Range(f func(name string, ts *TimeSeries) bool) {
	if tsc == nil {
		return
	}
	for _, name := range tsc.order {
		if !f(name, tsc.series[name]) {
			return
		}
	}
}

// -----------------------------------------------------------------------
// Looking at a container
// -----------------------------------------------------------------------

// PrettyPrint writes every series of the container, with its summary,
// to the standard output. The optional arguments select a window of
// points, as in TimeSeries.PrettyPrint.
func (tsc *TsContainer) PrettyPrint(what ...int) {
	tsc.Fprint(os.Stdout, what...)
}

// Fprint writes every series of the container, with its summary, to w.
func (tsc *TsContainer) Fprint(w io.Writer, what ...int) {
	fmt.Fprintf(w, "\n########## Container: %v\n", tsc.Name)
	if tsc.Comment != "" {
		fmt.Fprintf(w, "########## %v\n", tsc.Comment)
	}
	if tsc.Len() == 0 {
		fmt.Fprintln(w, "(no series)")
		return
	}

	tsc.Range(func(name string, ts *TimeSeries) bool {
		fmt.Fprintf(w, "\n---------- %v\n", name)
		if ts == nil {
			fmt.Fprintln(w, "(not produced)")
			return true
		}
		ts.Fprint(w, what...)
		ts.FprintStats(w)
		return true
	})
}

// FprintStats writes only the summaries, one per series. On a container
// holding several variants of the same signal, this is the table that
// shows what each step of a recipe did to the data.
func (tsc *TsContainer) FprintStats(w io.Writer) {
	fmt.Fprintf(w, "\n########## Container: %v\n", tsc.Name)
	tsc.Range(func(name string, ts *TimeSeries) bool {
		if ts == nil {
			fmt.Fprintf(w, "\n%v: (not produced)\n", name)
			return true
		}
		ts.Stats().Fprint(w, name)
		return true
	})
}

// PrintStats writes the summaries of every series to the standard
// output.
func (tsc *TsContainer) PrintStats() {
	tsc.FprintStats(os.Stdout)
}
