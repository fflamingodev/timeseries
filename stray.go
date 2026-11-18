package timeseries

import (
	"fmt"
	"math"

	nav "github.com/fflamingodev/notavalue"
)

// A stray point is a single reading that leaves the level of the series
// and is immediately brought back by the next one: a sharp jump from
// its predecessor, then a jump of comparable size the other way to its
// successor.
//
//	20  20  20   0  20  20     the 0 is a stray point
//	20  20   0   0  20  20     neither 0 is: the excursion lasts two readings
//	20  20  20   0   0   0     nor here: the level has changed
//
// This is not an outlier filter. A stray point need not be implausible
// in itself — 0 °C is a perfectly honest temperature — and a value far
// from the mean need not be stray. What gives it away is its shape in
// time: down then up, or up then down, on one reading only. A window
// opened and closed between two samples, a sensor that misread once, a
// cable that slipped and was pushed back.
//
// # The two jumps
//
// For a point v with predecessor p and successor n, the way in is v-p
// and the way back is n-v. The point is stray when:
//
//   - both jumps exceed the threshold, in magnitude;
//   - they have opposite signs;
//   - the way in goes in the direction asked for: up for a spike, down
//     for a dip, or either.
//
// The way back is not required to cancel the way in. 20, 0, 18 is a
// dip that came back slightly lower; it is still a stray point as long
// as 18 is far enough from 0. The threshold is what "of the same order"
// means here: both jumps must clear it.
//
// The threshold is exclusive, like the bounds of RemoveOutbounds: a
// jump exactly equal to it is not a sharp one.
//
// # What cannot be judged
//
// The first and the last point have only one neighbor and are never
// stray. Neither is a point whose predecessor or successor is a gap
// (NaV) or a broken value (NaN): there is no jump to measure across
// nothing. And, as with every cleaning method, a gap or a broken value
// is never rejected itself.
//
// The neighbors are the adjacent points in the series, whatever time
// separates them. On an irregular series, regularize first if a long
// silence should not count as "the next reading".
//
// # On oscillations
//
// The judgment is made on the original series, every point against its
// original neighbors. On a signal that alternates — 20, 0, 20, 0, 20 —
// every inner point is a turn in both directions: StrayBoth rejects all
// of them, StrayDown only the zeros, StrayUp only the inner twenties.
// Choosing a direction is how to say which of the two levels is the
// real one.

// StrayDirection says which kind of stray point to look for.
type StrayDirection int

const (
	// StrayBoth looks for spikes and dips alike.
	StrayBoth StrayDirection = iota
	// StrayUp looks for spikes only: a jump up, then back down.
	StrayUp
	// StrayDown looks for dips only: a jump down, then back up.
	StrayDown
)

// String returns the name of the direction, as it appears in the
// Comment of the series a cleaning returns.
func (d StrayDirection) String() string {
	switch d {
	case StrayBoth:
		return "up or down"
	case StrayUp:
		return "up"
	case StrayDown:
		return "down"
	default:
		return fmt.Sprintf("StrayDirection(%d)", int(d))
	}
}

// RemoveStrayPoints splits the series on its stray points, measuring the
// jumps in the unit of the readings: a point is stray when it moves by
// more than jump from its predecessor, then by more than jump the other
// way to its successor.
//
//	ts.RemoveStrayPoints(5, timeseries.StrayDown)  // a window opened, 5 °C or more
//	ts.RemoveStrayPoints(5, timeseries.StrayBoth)  // any one-reading excursion
//
// jump must be above zero.
//
// It returns the cleaned series, where stray readings are NaV, and the
// series of stray readings with their original values.
func (ts *TimeSeries) RemoveStrayPoints(jump float64, direction StrayDirection) (cleaned, rejected *TimeSeries, err error) {
	if math.IsNaN(jump) || jump <= 0 {
		return nil, nil, fmt.Errorf("%w: stray jump %s must be above zero", ErrCleaningArg, nav.Format(jump))
	}
	if err := direction.check(); err != nil {
		return nil, nil, err
	}

	sharp := func(delta, _ float64) bool { return math.Abs(delta) > jump }
	return ts.splitAt(ts.strayRule(sharp, direction),
		fmt.Sprintf("stray points, %s, jump above %s", direction, nav.Format(jump)))
}

// RemoveStrayPointsPercent is RemoveStrayPoints with the jumps measured
// in percent: a point is stray when it moves by more than percent of
// its predecessor's value, then by more than percent of that same value
// the other way.
//
// Both jumps are measured against the predecessor, the level the series
// was at before the excursion, and not against the stray point itself.
// Otherwise a dip to zero would make the way back infinitely large, and
// a spike would make it look small.
//
//	ts.RemoveStrayPointsPercent(50, timeseries.StrayBoth)  // half the level, or more
//
// A point whose predecessor is zero cannot be judged in percent, and is
// never stray. percent must be above zero; it may exceed a hundred.
func (ts *TimeSeries) RemoveStrayPointsPercent(percent float64, direction StrayDirection) (cleaned, rejected *TimeSeries, err error) {
	if math.IsNaN(percent) || percent <= 0 {
		return nil, nil, fmt.Errorf("%w: stray percent %s must be above zero", ErrCleaningArg, nav.Format(percent))
	}
	if err := direction.check(); err != nil {
		return nil, nil, err
	}

	sharp := func(delta, level float64) bool {
		if level == 0 {
			return false
		}
		return math.Abs(delta)/math.Abs(level)*100 > percent
	}
	return ts.splitAt(ts.strayRule(sharp, direction),
		fmt.Sprintf("stray points, %s, jump above %s%%", direction, nav.Format(percent)))
}

// check reports a direction that is none of the three constants.
func (d StrayDirection) check() error {
	switch d {
	case StrayBoth, StrayUp, StrayDown:
		return nil
	}
	return fmt.Errorf("%w: unknown stray direction %d", ErrCleaningArg, int(d))
}

// strayRule returns the rejection rule for splitAt. sharp tells whether
// a jump delta is large enough, given level, the reading before the
// excursion.
func (ts *TimeSeries) strayRule(sharp func(delta, level float64) bool, direction StrayDirection) func(int, float64) bool {
	return func(i int, v float64) bool {
		if i == 0 || i == ts.Len()-1 {
			return false
		}
		prev, next := ts.points[i-1].Meas, ts.points[i+1].Meas
		if math.IsNaN(prev) || math.IsNaN(next) {
			// A gap or a broken neighbor: no jump to measure.
			return false
		}

		in, back := v-prev, next-v
		switch {
		case in > 0 && back < 0:
			if direction == StrayDown {
				return false
			}
		case in < 0 && back > 0:
			if direction == StrayUp {
				return false
			}
		default:
			// Not a turn: the series goes on in the same direction, or
			// stays flat on one side.
			return false
		}
		return sharp(in, prev) && sharp(back, prev)
	}
}
