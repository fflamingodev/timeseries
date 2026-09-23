package timeseries

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a fresh random identifier, formatted as a version 4
// UUID, for callers who have no identifier of their own to give a
// TimeSeries.
//
// Nothing forces the use of a UUID: TimeSeries.ID is a plain string, and
// a database key or a device name is usually more informative. NewID
// exists so that a series can be uniquely identified without dragging a
// dependency into the module for sixteen random bytes — the standard
// library provides them.
//
// NewID panics if the system source of randomness fails, which on any
// working machine does not happen. A caller who cannot afford a panic
// should supply its own identifier.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("timeseries: no randomness available for NewID: " + err.Error())
	}

	// Version 4, variant RFC 4122: the two markers that make these bytes
	// a well-formed UUID rather than plain hexadecimal.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}
