# Contributing

Thanks for considering a contribution! This is a small, opinionated
library, so a few short conventions go a long way.

## Quick start

```bash
git clone https://github.com/fflamingodev/timeseries
cd timeseries
go test ./...
go vet ./...
```

If you have `golangci-lint` installed:

```bash
golangci-lint run
```

CI runs the same three commands on every push and pull request.

## What kind of changes are welcome

- **Bug reports and small fixes.** Open an issue with a minimal
  reproducer, or send a PR with a failing test alongside the fix.
- **New aggregators or interpolation methods.** Match the existing
  patterns in `aggregate_functions.go` and `interpolate.go`. NaV
  handling must be explicit.
- **Documentation improvements.** README and `NAV_SEMANTICS.md`
  benefit from concrete examples, especially in domains where
  missing-by-nature data has business meaning.
- **Performance work** with benchmark numbers. `Regularize`,
  `Interpolate` and the cleaners are the hot paths.

## What is out of scope, for now

- A full DataFrame API. We focus on a single-series toolkit.
- Streaming ingestion (planned for v0.2; an issue is open).
- Vendor-specific I/O (Influx line protocol, Parquet, etc.) — these
  belong in companion packages, not here.

## Coding conventions

- **Idiomatic Go.** `gofmt`, no panics in regular paths, error
  wrapping via `*Error` (see `errors.go`).
- **English in code and godoc comments.** Internal notes can be in
  any language but anything user-visible stays in English.
- **NaV-aware everywhere.** Any new function that consumes a
  `[]float64` must document its NaV behavior — propagate or skip,
  matching the rules in `NAV_SEMANTICS.md`.
- **One public API change per PR.** Keep diffs small enough to review
  in one sitting.

## Commit messages

We follow a relaxed [Conventional Commits](https://www.conventionalcommits.org/)
style:

```
feat(regularize): add tolerance for late samples
fix(stddev): use n-1 divisor for sample standard deviation
docs(readme): clarify the propagation vs skip rule
```

The prefix is informative, not enforced by hooks.

## Reporting a bug

Open an issue with:

- the Go version (`go version`);
- a minimal program that reproduces the bug;
- what you expected and what you got;
- if relevant, the bit pattern of the offending NaV / NaN
  (`timeseries.DebugBits(x)`).

## License

By contributing, you agree that your contributions are licensed under
the project's MIT license (see `LICENSE`).
