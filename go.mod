module usefulrisk.com/timeseries

go 1.26

// v0.1.0 changed the NaV semantics: NaV propagates through Add/Sub, and
// plain NaN is silently skipped by aggregates. Use v0.2.0 or later.
retract v0.1.0

require github.com/fflamingodev/notavalue v0.1.0
