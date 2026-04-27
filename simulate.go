package timeseries

import (
	"math/rand"
	"time"
)

// BulkSimul returns a synthetic TimeSeries of samplesize points starting
// at from. Each successive sample is spaced by period plus a Gaussian
// jitter with standard deviation jitter, so the output looks like
// real-world sensor data that arrives on a roughly-regular-but-not-quite
// cadence. The measured value follows a normal distribution with the
// given mean and stdDev.
//
// The random source is seeded from time.Now().UnixNano(); callers who
// need reproducibility should wrap the returned series or shift to a
// seeded variant.
func BulkSimul(
	name string,
	from time.Time,
	period time.Duration,
	samplesize int,
	mean float64,
	stdDev float64,
	jitter time.Duration, // standard deviation of the inter-arrival jitter
) TimeSeries {
	ts := TimeSeries{Name: name}
	t := from

	// Single RNG instance so the generated series is a proper stochastic
	// process and not a bundle of re-seeded sequences.
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < samplesize; i++ {
		// Gaussian jitter around 0, with standard deviation = jitter.
		jitterFactor := r.NormFloat64()                            // N(0, 1)
		jitterDur := time.Duration(jitterFactor * float64(jitter)) // ns, since jitter is already a Duration

		interval := period + jitterDur
		if interval < 0 {
			// Do not let the jitter push time backwards.
			interval = 0
		}

		t = t.Add(interval)

		value := r.NormFloat64()*stdDev + mean
		ts.AddDataUnit(NewDataUnit(t, value))
	}

	return ts
}
