package timeseries

import (
	"errors"
	"fmt"
	"time"

	nav "github.com/fflamingodev/notavalue"
)

// A sensor that falls silent leaves nothing behind: no reading, not even
// a NaV. Whatever comes after — a statistic, a chart, a reduction — sees
// the reading before the silence and the reading after it, and nothing
// in between to say that the signal was lost.
//
// A regularized series already says it: a window that caught nothing is
// NaV. A raw series has no windows, and must be told how long a silence
// may last before it means something. That is what this file does.

// ErrAlarmArg reports a delay that cannot define an alarm. Test for it
// with errors.Is.
var ErrAlarmArg = errors.New("timeseries: impossible alarm argument")

// MarkSilences returns the series with a NaV inserted wherever two
// consecutive readings are more than maxGap apart.
//
// The NaV is dated maxGap after the reading that preceded the silence:
// the moment an alarm watching the sensor would have fired. Up to that
// moment the value held is legitimate — a sensor reporting every ten
// minutes is not lost at the eleventh — and from it onwards the signal
// is unknown, until the next reading.
//
//	watched, err := raw.MarkSilences(15 * time.Minute)
//	reduced := watched.Reduce() // the silences survive the reduction
//
// Two readings exactly maxGap apart raise no alarm. A silence that
// follows a NaV raises none either: the signal is already unknown, and
// saying it twice adds nothing. Nothing is inserted after the last
// reading: the series ends there, and says nothing beyond.
//
// maxGap must be above zero.
func (ts *TimeSeries) MarkSilences(maxGap time.Duration) (*TimeSeries, error) {
	if maxGap <= 0 {
		return nil, fmt.Errorf("%w: the delay must be above zero, got %v", ErrAlarmArg, maxGap)
	}

	out := NewTimeSeries(ts.Name + " watched")
	out.ID = ts.ID
	out.Comment = fmt.Sprintf("silences beyond %v marked as NaV", maxGap)

	n := ts.Len()
	if n == 0 {
		return out, nil
	}

	data := make([]Datum, 0, n+8)
	data = append(data, ts.At(0).Datum)

	for i := 1; i < n; i++ {
		prev, cur := ts.At(i-1), ts.At(i)
		if cur.Chron.Sub(prev.Chron) > maxGap && !nav.IsNaV(prev.Meas) {
			data = append(data, NewDatum(prev.Chron.Add(maxGap), nav.NaV))
		}
		data = append(data, cur.Datum)
	}

	out.AddBatchData(data)
	return out, nil
}
