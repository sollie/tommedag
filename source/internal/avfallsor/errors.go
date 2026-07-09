package avfallsor

import "errors"

// Sentinel errors returned by this package. Callers should use errors.Is to
// check for these, since they may be wrapped with additional context.
var (
	// ErrAddressNotFound is returned when the address search returns no
	// candidate with a resolvable property ID.
	ErrAddressNotFound = errors.New("avfallsor: address not found")

	// ErrInvalidPropertyID is returned when a property ID does not match the
	// UUID shape the upstream API expects, or was extracted from a
	// malformed href.
	ErrInvalidPropertyID = errors.New("avfallsor: invalid property id")

	// ErrUpstreamUnavailable is returned when the upstream API cannot be
	// reached, times out, or returns a response this client cannot make
	// sense of (non-2xx status, non-JSON body, malformed JSON).
	ErrUpstreamUnavailable = errors.New("avfallsor: upstream unavailable")
)
