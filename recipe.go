package timeseries

// RecipesCatalogueRow describes a single "polishing" recipe — a named
// sequence of operations to apply to a raw series in order to produce a
// named variant (e.g. "hourly-mean-outliers-removed"). It is consumed by
// TsContainer.ApplyPolishing.
//
// The fields are pointer-typed so that "absent" (nil) is distinct from
// "present with zero value", mirroring how the recipe is typically
// loaded from a SQL row where many columns are NULL. The db and json
// tags match the conventions of the first backend to use this library
// and are preserved for backward compatibility.
//
// Semantics of the individual fields:
//   - Variant:      target key in the output TsContainer.Ts map.
//   - Reduce:       if *Reduce == 1, apply TimeSeries.Reduce before the
//                   rest of the pipeline.
//   - FreqSeconds:  if non-nil alongside Agg, regularize the series on
//                   a Freq grid with the named aggregator.
//   - Agg:          aggregator name passed to getAggFunc (e.g. "average",
//                   "maximum", "median", …).
//   - Method1/Min1/Max1/Percent1/Lvl1: first cleaning pass, applied
//     before regularization.
//   - Method2/Min2/Max2/Percent2/Lvl2: second cleaning pass, applied
//     after regularization.
//   - Interp:       interpolation method name passed to getInterpMethod
//     (e.g. "Linear", "MonotoneSpline").
//   - Scale:        multiplicative scale (currently informational;
//     reserved for a future "rescale" step).
//   - CreatedAtUTC: audit timestamp.
type RecipesCatalogueRow struct {
	Idx          string   `db:"idx" json:"idx"`
	Variant      string   `db:"variant" json:"variant"`
	Reduce       *int64   `db:"reduce" json:"reduce"`
	FreqSeconds  *int64   `db:"freqseconds" json:"freqseconds"`
	Agg          *string  `db:"agg" json:"agg"`
	Method1      *string  `db:"method1" json:"method1"`
	Min1         *float64 `db:"min1" json:"min1"`
	Max1         *float64 `db:"max1" json:"max1"`
	Percent1     *float64 `db:"percent1" json:"percent1"`
	Lvl1         *float64 `db:"lvl1" json:"lvl1"`
	Method2      *string  `db:"method2" json:"method2"`
	Min2         *float64 `db:"min2" json:"min2"`
	Max2         *float64 `db:"max2" json:"max2"`
	Percent2     *float64 `db:"percent2" json:"percent2"`
	Lvl2         *float64 `db:"lvl2" json:"lvl2"`
	Interp       *string  `db:"interp" json:"interp"`
	Scale        *float64 `db:"scale" json:"scale"`
	CreatedAtUTC *string  `db:"created_at_utc" json:"created_at_utc"`
}
