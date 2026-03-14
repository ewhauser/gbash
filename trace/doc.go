// Package trace provides the structured execution event model used by gbash.
//
// This is a supported public extension package for callers that need to
// consume, record, or test against gbash execution events. Most embedders
// should prefer the root `github.com/ewhauser/gbash` package unless they are
// integrating with tracing or telemetry systems directly. Runtime tracing is
// opt-in; most embedders should enable redacted tracing from the root package
// rather than wiring raw event capture by default.
package trace
