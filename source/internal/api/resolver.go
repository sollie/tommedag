package api

import (
	"context"

	"github.com/sollie/tommedag/internal/avfallsor"
)

// Resolver is the subset of the avfallsor client the handlers depend on. It
// is an interface so handlers can be tested against a fake without any
// network access.
type Resolver interface {
	ResolveAddress(ctx context.Context, term string) (avfallsor.Match, error)
	PickupCalendar(ctx context.Context, propertyID string) (avfallsor.PickupCalendar, error)
}
