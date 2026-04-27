// Package apidto holds HTTP-handler request DTOs that happened to be
// attached to an early caller of the timeseries library. They are kept
// here as a convenience for that caller; a downstream project writing
// its own HTTP layer should define its own DTOs rather than import
// these. The types embed backend-specific conventions (uuid device IDs,
// UTC string timestamps) and are NOT part of the stable public surface
// of the time-series core.
package apidto

import "github.com/google/uuid"

// DetailsQuery is the payload of a single-container "details" endpoint.
// Variants names the list of polished variants the caller wants
// returned for the given [from, to] window.
type DetailsQuery struct {
	DeviceUUID uuid.UUID `json:"device_uuid"`
	DeviceName string    `json:"device_name"`
	Datasource string    `json:"datasource"`
	Variants   []string  `json:"variants"`
	From       string    `json:"from_utc"`
	To         string    `json:"to_utc"`
	Limit      *int      `json:"limit"`
}

// MultiDetailsQuery is the payload of a multi-datasource "details"
// endpoint. It lets the caller fetch raw data for several datasources
// in a single HTTP call; recipes are intentionally ignored here
// (multi-mode is raw-only for now).
type MultiDetailsQuery struct {
	DeviceName  string   `json:"device_name"`
	Datasources []string `json:"datasources"`
	Variants    []string `json:"variants"`
	From        string   `json:"from_utc"`
	To          string   `json:"to_utc"`
	Limit       *int     `json:"limit"`
}
