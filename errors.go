package timeseries

import (
	"fmt"
	"strings"
)

// Kind is the stable, programmatically-testable category of a library
// error. New kinds may be added over time; consumers should switch only
// on the kinds they know and treat the rest as KindUnknown.
type Kind int

const (
	// KindUnknown is the zero value; it matches any other Kind in the
	// Is-comparison, so it can be used as a wildcard sentinel. In
	// practice the library never emits this value directly.
	KindUnknown Kind = iota

	// KindEmptyInput: the input slice or series is empty, or contains
	// only NaN/NaV values when finite data was required.
	KindEmptyInput

	// KindBounds: a numeric argument or value falls outside the accepted
	// range (e.g. a percentile not in (0, 100], or a series value past a
	// user-supplied cleaning fence).
	KindBounds

	// KindInvalidArg: an argument is structurally wrong independently of
	// its numeric value (e.g. freq <= 0, tolerance >= freq, a nil AggFunc
	// or a nil receiver).
	KindInvalidArg

	// KindBadState: the operation cannot run given the current state of
	// the TimeSeries (e.g. deltas needed but DeltasValid() is false,
	// series not chronologically sorted when required).
	KindBadState

	// KindUnknownOption: a string-form option name (aggregator,
	// interpolation method, cleaning method) did not match any known
	// value.
	KindUnknownOption

	// KindPanic: a downstream step panicked and was recovered. The
	// attached Err typically carries the original panic value wrapped as
	// an error; the Stack field is populated when available.
	KindPanic
)

// String returns a human-readable name for the Kind.
func (k Kind) String() string {
	switch k {
	case KindEmptyInput:
		return "EmptyInput"
	case KindBounds:
		return "Bounds"
	case KindInvalidArg:
		return "InvalidArg"
	case KindBadState:
		return "BadState"
	case KindUnknownOption:
		return "UnknownOption"
	case KindPanic:
		return "Panic"
	default:
		return "Unknown"
	}
}

// Error is the structured error type raised by this package.
//
// A library function that produces a *Error guarantees:
//
//   - Op is the exported name of the operation that generated the error
//     (e.g. "Regularize", "Percentile", "ApplyPolishing"). It is never
//     empty for errors generated inside the library.
//   - Kind is one of the exported Kind constants; new kinds may be
//     introduced in minor releases.
//   - Msg is a short lower-case sentence without trailing punctuation
//     and without Go-style formatting verbs. It never exposes internal
//     variable names.
//   - Field and Value, when non-empty, identify the offending argument
//     or measurement so callers can build diagnostic UIs without parsing
//     the Msg.
//   - Err, when non-nil, is the wrapped underlying cause. Unwrap walks
//     the chain to recover it.
//
// Comparison is done via errors.Is against the package-level sentinels
// (ErrEmptyInput, ErrBounds, …) or against a hand-built *Error
// containing only the Op and Kind to match.
type Error struct {
	Op    string // operation name, e.g. "Regularize"
	Kind  Kind
	Msg   string // lowercase, no trailing punctuation
	Field string // optional: offending argument name
	Value any    // optional: offending value
	Err   error  // wrapped cause
	Stack string // optional: runtime stack trace for KindPanic
}

// Error implements the error interface. It assembles Op, Msg, the
// optional (field=value) context, and the wrapped chain in a single,
// predictable string. Callers that want to render the error
// differently should use errors.As to reach into the fields.
func (e *Error) Error() string {
	var sb strings.Builder
	if e.Op != "" {
		sb.WriteString(e.Op)
		sb.WriteString(": ")
	}
	if e.Msg != "" {
		sb.WriteString(e.Msg)
	} else if e.Kind != KindUnknown {
		sb.WriteString(e.Kind.String())
	}
	if e.Field != "" {
		sb.WriteString(" (")
		sb.WriteString(e.Field)
		if e.Value != nil {
			fmt.Fprintf(&sb, "=%v", e.Value)
		}
		sb.WriteByte(')')
	}
	if e.Err != nil {
		sb.WriteString(": ")
		sb.WriteString(e.Err.Error())
	}
	return sb.String()
}

// Unwrap returns the wrapped error, if any, so errors.Is and errors.As
// can walk the chain.
func (e *Error) Unwrap() error { return e.Err }

// Is reports whether the receiver matches target for errors.Is
// semantics. An *Error matches target when:
//
//   - target is also a *Error, and
//   - target.Kind is KindUnknown or equal to e.Kind, and
//   - target.Op is empty or equal to e.Op.
//
// This lets callers write errors.Is(err, ErrBounds) to match any bounds
// error, or errors.Is(err, &timeseries.Error{Op: "Percentile", Kind:
// KindBounds}) to match a specific operation.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	if t.Kind != KindUnknown && t.Kind != e.Kind {
		return false
	}
	if t.Op != "" && t.Op != e.Op {
		return false
	}
	return true
}

// -------------------------------------------------------------------
// Package-wide sentinels. These exist for errors.Is comparison:
//
//     if errors.Is(err, timeseries.ErrEmptyInput) { ... }
//
// They are intentionally lightweight (no Op, no Field/Value) so they
// match every operation-specific *Error with the same Kind. Do NOT
// mutate them; any enrichment must be done on a freshly allocated
// *Error whose Kind still matches the sentinel.
// -------------------------------------------------------------------

var (
	// ErrEmptyInput is returned when an operation requires at least one
	// valid (non-NaN) observation but receives none.
	ErrEmptyInput = &Error{Kind: KindEmptyInput, Msg: "input must not be empty"}

	// ErrBounds is returned when an argument or a measurement is outside
	// the accepted range.
	ErrBounds = &Error{Kind: KindBounds, Msg: "value outside of accepted range"}

	// ErrInvalidArg is returned when an argument is structurally wrong:
	// nil receiver or nil dependency, non-positive freq, and similar.
	ErrInvalidArg = &Error{Kind: KindInvalidArg, Msg: "invalid argument"}

	// ErrBadState is returned when a series cannot satisfy the
	// pre-conditions of the operation (typically when deltas are needed
	// but DeltasValid() is false).
	ErrBadState = &Error{Kind: KindBadState, Msg: "series is not in a valid state for this operation"}

	// ErrUnknownOption is returned when a string-form option name
	// (aggregator, interpolation method) does not match any known value.
	ErrUnknownOption = &Error{Kind: KindUnknownOption, Msg: "unknown option"}

	// ErrPanic is the Kind set on the error returned when ApplyPolishing
	// (or a future pipeline) recovers a panic from a downstream step.
	ErrPanic = &Error{Kind: KindPanic, Msg: "panic in pipeline step"}
)
