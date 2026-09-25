package datahub_capability_port

import (
	"context"
	"time"

	"alt/domain"
)

// PushSubscriptionPort is the push_subscriptions table: one row per device.
//
// userID is an argument on every method, never a context lookup, for the
// reason ReadStatePort gives — alt-data-hub sees a peer certificate naming
// alt-backend, so a context-scoped user would resolve to the service account
// and every caller would read the same devices.
//
// Every method is scoped by user including the ones the endpoint alone would
// identify. The endpoint is a capability URL; scoping the read to its owner is
// what stops a leaked one from being exchanged for its owner's settings here.
type PushSubscriptionPort interface {
	// Upsert stores a subscription by endpoint, replacing the key material
	// and preferences of one already registered. The bool reports a fresh
	// insert, which is how the caller tells a first subscription from a
	// browser re-prompting for permission.
	Upsert(ctx context.Context, sub domain.PushSubscription) (created bool, err error)
	// Get returns nil without error when this user has no subscription at
	// that endpoint — deliberately the same answer as for an endpoint that
	// does not exist at all.
	Get(ctx context.Context, userID, endpoint string) (*domain.PushSubscription, error)
	// UpdatePreferences writes all four booleans. There is no partial update:
	// the settings screen sends the whole set, and a per-field update would
	// need an absent field to mean "leave alone", which is the same bytes as
	// "turn off" over protoJSON.
	UpdatePreferences(ctx context.Context, userID, endpoint string, prefs domain.NotificationPreferences) (updated bool, err error)
	// Delete is idempotent; false means there was nothing to delete.
	Delete(ctx context.Context, userID, endpoint string) (deleted bool, err error)
	// ListForUser returns every device of one user — the fan-out list.
	ListForUser(ctx context.Context, userID string) ([]domain.PushSubscription, error)
}

// PushDeliveryPort is the Web Push delivery queue's state machine.
//
// push_deliveries is the dispatcher's queue, one row per (notification,
// device) — not a producer outbox. The producers' business state lives in
// pre-processor-db, acolyte-db and recap-db, so each keeps its own outbox
// beside the state it describes and a relay forwards to here.
//
// Claim is a capability rather than a read, like OutboxPort's: the lease and
// the state change are one statement, and the interface offers no way to take
// them apart. It also has no Reclaim method, and that absence is the design —
// the lease *is* next_attempt_at, so a row orphaned by a crashed dispatcher
// re-enters ClaimBatch on its own. A separate reclaim method would be a second
// path to the same transition and a second thing to schedule.
type PushDeliveryPort interface {
	// Enqueue fans one notification out to every subscription of the user
	// that still wants that kind, in one transaction. It reports how many
	// device rows were created and how many older unsent daily digests were
	// superseded; a duplicate relay creates none of either.
	Enqueue(ctx context.Context, in domain.NotificationEnqueue) (delivered, superseded int, err error)
	// ClaimBatch marks due rows SENDING, increments attempts and pushes
	// next_attempt_at forward by lease, in one statement.
	ClaimBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.PushDelivery, error)
	// MarkSent is terminal and stamps finalized_at.
	MarkSent(ctx context.Context, id string, statusCode int) error
	// Release returns a claimed row to PENDING at the caller's next attempt
	// time.
	Release(ctx context.Context, id string, nextAttemptAt time.Time, errorMessage string) error
	// MarkDead is terminal for a failure that will not improve.
	MarkDead(ctx context.Context, id string, statusCode int, errorMessage string) error
	// BacklogAge reports how stale the queue is, how deep it is, and how many
	// devices are registered to receive anything, for the dispatcher's gauges.
	// It counts rows in `sending` as well as `pending`: a row orphaned by a
	// crashed dispatcher stays `sending` until its lease elapses, and a reading
	// blind to those would answer 0 for a queue that is stuck. The device count
	// comes from the same pass because "nothing was sent" only means a broken
	// pipeline when there was somewhere to send it. An empty queue on an
	// unsubscribed deployment is (0, 0, 0) and not an error.
	BacklogAge(ctx context.Context) (oldest time.Duration, pending, subscriptions int64, err error)
}
