package timeseries

import (
	"fmt"
	"os"
	"text/tabwriter"
)

type Series interface {
	PrettyPrint()
}

func (tsc *TsContainer) PrettyPrint(what ...int) {
	for k, v := range tsc.Ts {
		if v != nil {
			fmt.Printf("Container: %v\n", tsc.Name)
			fmt.Println("-------------------------------------------------")
			fmt.Printf("TimeSeries: %v\n", k)
			v.PrintTsStats()
			v.PrettyPrint(what...)
		}
	}
}

// --- DataUnit pretty-print helpers ---

// writeRow writes a single data row for the DataUnit into the given
// tabwriter; it does not write headers.
func (du *DataUnit) writeRow(w *tabwriter.Writer) {
	fmt.Fprintf(w, "%v|\t%v|\t%v|\t%v|\t\n",
		du.Chron.Round(0),         // Chron
		formatFloat(du.Meas),      // Measure
		formatDuration(du.Dchron), // Dchron
		formatFloat(du.Dmeas),     // Dmeas
	)
}

// PrettyPrint renders a single DataUnit on stdout in a terminal-friendly,
// right-aligned tabular form. NaV is printed "NaV", plain NaN is printed
// "NaN", and NaDuration is printed "NaDuration".
func (du *DataUnit) PrettyPrint() {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 5, 0, 3, ' ', tabwriter.AlignRight)

	fmt.Fprintf(w, "%v|\t%v|\t%v|\t%v|\t\n",
		"Chron", "Measure", "Dchron", "Dmeas")
	fmt.Fprintln(w, "------------|\t--------------------------------------|\t----------|\t------------|\t")

	du.writeRow(w)

	fmt.Fprintln(w)
	w.Flush()
}

// --- TimeSeries pretty-print ---

// PrettyPrint renders the TimeSeries on stdout in a terminal-friendly,
// right-aligned tabular form. The optional what argument selects the
// range of rows to print:
//
//   - no arg     : every row.
//   - what[0]    : rows [0, what[0]).
//   - what[0..1] : rows [what[0], what[1]).
//
// Out-of-range indices are clamped. If the selected range is empty or
// the series is empty, a one-line notice is printed instead.
func (ts *TimeSeries) PrettyPrint(what ...int) {
	fmt.Printf("Name       : %v\n", ts.Name)
	fmt.Println("----------------------------------------------------------------------------------------------------------------------------------------------")

	// Resolve the [j, k) range from the variadic argument.
	var j, k int
	switch len(what) {
	case 0:
		j = 0
		k = len(ts.DataSeries)
	case 1:
		j = 0
		k = what[0]
	case 2:
		j = what[0]
		k = what[1]
	default:
		j = 0
		k = len(ts.DataSeries)
	}

	if j < 0 {
		j = 0
	}
	if k > len(ts.DataSeries) {
		k = len(ts.DataSeries)
	}
	if j >= k || len(ts.DataSeries) == 0 {
		fmt.Println("(TimeSeries is empty or indices are out of range)")
		return
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 5, 0, 3, ' ', tabwriter.AlignRight)

	// Add an "index" column before the DataUnit columns.
	fmt.Fprintf(w, "%v|\t%v|\t%v|\t%v|\t%v|\t\n",
		"index", "Chron", "Measure", "Dchron", "Dmeas")
	fmt.Fprintln(w, "-----|\t------------|\t--------------------------------------|\t----------|\t------------|\t")

	for i := j; i < k; i++ {
		du := &ts.DataSeries[i]
		fmt.Fprintf(w, "%d|\t", i)
		du.writeRow(w)
	}

	fmt.Fprintln(w)
	w.Flush()
}

// PrintTsStats prints TsStats struct in a readable way in output terminal
func (ts *TimeSeries) PrintTsStats() {
	fmt.Println("------------------------------------------")
	fmt.Println(ts.Name)
	fmt.Printf("Warning: %v Missing Data\n", ts.NbreOfNaN)
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 5, 0, 3, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "Length| %v|\t\n", ts.Len)
	fmt.Fprintln(w, "\tChron|\tMeasure|\tDChron|\tDMeas|\t")
	fmt.Fprintln(w, "-\t-----------------\t------------\t------------\t------------\t")
	fmt.Fprintf(w, "Min|\t %v|\t%v|\t%v|\t%v|\t\n", ts.Chmin.Round(0), ts.Msmin, ts.DChmin, ts.DMsmin)
	fmt.Fprintf(w, "Max|\t %v|\t%v|\t%v|\t%v|\t\n", ts.Chmax.Round(0), ts.Msmax, ts.DChmax, ts.DMsmax)
	fmt.Fprintf(w, "Mean|\t %v|\t%v|\t%v|\t%v|\t\n", ts.Chmean, ts.Msmean, ts.DChmean, ts.DMsmean)
	fmt.Fprintf(w, "Median|\t %v|\t%v|\t%v|\t%v|\t\n", ts.Chmed, ts.Msmean, ts.DChmed, ts.DMsmed)
	fmt.Fprintf(w, "StdDev|\t %v|\t%v|\t%v|\t%v|\t\n", " ", ts.Msstd, ts.DChstd, ts.DMsstd)

	fmt.Fprintln(w)
	w.Flush()

}
func (ts *TimeSeries) PrettyPrintAll() {
	ts.PrettyPrint()
	ts.PrintTsStats()
}
