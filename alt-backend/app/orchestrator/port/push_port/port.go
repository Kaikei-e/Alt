// Package push_port declares what alt.push.v1.PushService needs from
// alt-data-hub.
//
// cmd/backend has no database pool (ADR-000954 Wave 3, asserted at the link
// level by di/import_boundary_test.go), so this port's only implementation is
// a Connect client over mutual TLS. It is declared here, beside its consumer,
// rather than borrowed from the data plane's capability port: that one is what
// alt-data-hub needs from alt_db, and the two happening to have the same five
// methods today is not a reason for the browser-facing handler to depend on
// the provider's contract.
package push_port

import (
	"context"

	"alt/domain"
)

// PushSubscriptionPort is the storage behind the four browser-facing
// procedures.
//
// userID is an argument on every method. The handler reads it from the
// X-Alt-Backend-Token subject and passes it down; nothing below this line
// re-derives it, so there is exactly one place where "whose devices are these"
// is decided.
type PushSubscriptionPort interface {
	// Upsert stores a subscription, replacing one already registered at the
	// same endpoint. The bool reports a fresh insert.
	Upsert(ctx context.Context, sub domain.PushSubscription) (created bool, err error)
	// Get returns nil without error when this user has no subscription at
	// that endpoint.
	Get(ctx context.Context, userID, endpoint string) (*domain.PushSubscription, error)
	// UpdatePreferences writes all four booleans; false means no row matched.
	UpdatePreferences(ctx context.Context, userID, endpoint string, prefs domain.NotificationPreferences) (updated bool, err error)
	// Delete is idempotent; false means there was nothing to delete.
	Delete(ctx context.Context, userID, endpoint string) (deleted bool, err error)
}
