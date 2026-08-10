package datahub_capability_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"
)

// ---------------------------------------------------------------------------
// Web Push subscriptions
// ---------------------------------------------------------------------------

type pushSubscriptionDriver interface {
	UpsertPushSubscription(ctx context.Context, sub domain.PushSubscription) (bool, error)
	GetPushSubscription(ctx context.Context, userID, endpoint string) (*domain.PushSubscription, error)
	UpdatePushSubscriptionPreferences(ctx context.Context, userID, endpoint string, prefs domain.NotificationPreferences) (bool, error)
	DeletePushSubscription(ctx context.Context, userID, endpoint string) (bool, error)
	ListPushSubscriptionsForUser(ctx context.Context, userID string) ([]domain.PushSubscription, error)
}

// PushSubscriptionGateway implements
// datahub_capability_port.PushSubscriptionPort.
//
// The wrapped errors here name the operation and the user, never the endpoint:
// an endpoint is a capability URL, and an error string is one of the places it
// would otherwise reach a log.
type PushSubscriptionGateway struct {
	db pushSubscriptionDriver
}

func NewPushSubscriptionGateway(db *alt_db.AltDBRepository) *PushSubscriptionGateway {
	return &PushSubscriptionGateway{db: db}
}

func (g *PushSubscriptionGateway) Upsert(ctx context.Context, sub domain.PushSubscription) (bool, error) {
	created, err := g.db.UpsertPushSubscription(ctx, sub)
	if err != nil {
		return false, fmt.Errorf("upsert push subscription for user %s: %w", sub.UserID, err)
	}
	return created, nil
}

func (g *PushSubscriptionGateway) Get(ctx context.Context, userID, endpoint string) (*domain.PushSubscription, error) {
	sub, err := g.db.GetPushSubscription(ctx, userID, endpoint)
	if err != nil {
		return nil, fmt.Errorf("get push subscription for user %s: %w", userID, err)
	}
	return sub, nil
}

func (g *PushSubscriptionGateway) UpdatePreferences(ctx context.Context, userID, endpoint string, prefs domain.NotificationPreferences) (bool, error) {
	updated, err := g.db.UpdatePushSubscriptionPreferences(ctx, userID, endpoint, prefs)
	if err != nil {
		return false, fmt.Errorf("update push subscription preferences for user %s: %w", userID, err)
	}
	return updated, nil
}

func (g *PushSubscriptionGateway) Delete(ctx context.Context, userID, endpoint string) (bool, error) {
	deleted, err := g.db.DeletePushSubscription(ctx, userID, endpoint)
	if err != nil {
		return false, fmt.Errorf("delete push subscription for user %s: %w", userID, err)
	}
	return deleted, nil
}

func (g *PushSubscriptionGateway) ListForUser(ctx context.Context, userID string) ([]domain.PushSubscription, error) {
	subs, err := g.db.ListPushSubscriptionsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions for user %s: %w", userID, err)
	}
	return subs, nil
}

// ---------------------------------------------------------------------------
// Push delivery queue
// ---------------------------------------------------------------------------

type pushDeliveryDriver interface {
	EnqueueNotification(ctx context.Context, in domain.NotificationEnqueue) (int, int, error)
	ClaimPushDeliveryBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.PushDelivery, error)
	MarkPushDeliverySent(ctx context.Context, id string, statusCode int) error
	ReleasePushDelivery(ctx context.Context, id string, nextAttemptAt time.Time, errorMessage string) error
	MarkPushDeliveryDead(ctx context.Context, id string, statusCode int, errorMessage string) error
	PushDeliveryBacklogAge(ctx context.Context) (time.Duration, int64, int64, error)
}

// PushDeliveryGateway implements datahub_capability_port.PushDeliveryPort.
type PushDeliveryGateway struct {
	db pushDeliveryDriver
}

func NewPushDeliveryGateway(db *alt_db.AltDBRepository) *PushDeliveryGateway {
	return &PushDeliveryGateway{db: db}
}

func (g *PushDeliveryGateway) Enqueue(ctx context.Context, in domain.NotificationEnqueue) (int, int, error) {
	delivered, superseded, err := g.db.EnqueueNotification(ctx, in)
	if err != nil {
		return 0, 0, fmt.Errorf("enqueue notification %s: %w", in.DedupeKey, err)
	}
	return delivered, superseded, nil
}

func (g *PushDeliveryGateway) ClaimBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.PushDelivery, error) {
	deliveries, err := g.db.ClaimPushDeliveryBatch(ctx, lockedBy, limit, lease)
	if err != nil {
		return nil, fmt.Errorf("claim push delivery batch for %s: %w", lockedBy, err)
	}
	return deliveries, nil
}

func (g *PushDeliveryGateway) MarkSent(ctx context.Context, id string, statusCode int) error {
	if err := g.db.MarkPushDeliverySent(ctx, id, statusCode); err != nil {
		return fmt.Errorf("mark push delivery %s sent: %w", id, err)
	}
	return nil
}

func (g *PushDeliveryGateway) Release(ctx context.Context, id string, nextAttemptAt time.Time, errorMessage string) error {
	if err := g.db.ReleasePushDelivery(ctx, id, nextAttemptAt, errorMessage); err != nil {
		return fmt.Errorf("release push delivery %s: %w", id, err)
	}
	return nil
}

func (g *PushDeliveryGateway) MarkDead(ctx context.Context, id string, statusCode int, errorMessage string) error {
	if err := g.db.MarkPushDeliveryDead(ctx, id, statusCode, errorMessage); err != nil {
		return fmt.Errorf("mark push delivery %s dead: %w", id, err)
	}
	return nil
}

func (g *PushDeliveryGateway) BacklogAge(ctx context.Context) (time.Duration, int64, int64, error) {
	oldest, pending, subscriptions, err := g.db.PushDeliveryBacklogAge(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read push delivery backlog age: %w", err)
	}
	return oldest, pending, subscriptions, nil
}
