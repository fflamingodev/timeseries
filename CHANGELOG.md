# Changelog

All notable changes to this project are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the project adheres to [Semantic Versioning](https://semver.org/).

## [1.1.0] — 2026-11-18

### Added

- **Stray points.** `RemoveStrayPoints(jump, direction)` and
  `RemoveStrayPointsPercent(percent, direction)` reject a single
  reading that jumps away from its predecessor and straight back at
  its successor — a window opened between two samples, a sensor that
  misread once. Both jumps must exceed the threshold and go in
  opposite directions; an excursion of two readings, a change of level
  or a slope is kept. In percent, both jumps are measured against the
  predecessor. `StrayDirection` selects spikes (`StrayUp`), dips
  (`StrayDown`) or both (`StrayBoth`). Like the other cleaning methods,
  they return the cleaned series, where a stray reading becomes NaV,
  and the rejected readings with their original values.

## [1.0.0]

First stable release. From here on, no exported name, signature or
documented behavior changes before a v2; minor versions only add.
