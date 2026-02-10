package subscription_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SubscriptionGateway struct {
	altDB *alt_db.AltDBRepository
}

func NewSubscriptionGateway(pool *pgxpool.Pool) *SubscriptionGateway {
	return &SubscriptionGateway{altDB: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *SubscriptionGateway) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	if g.altDB == nil {
		return nil, errors.New("database connection not available")
	}
	return g.altDB.FetchSubscriptions(ctx, userID)
}

func (g *SubscriptionGateway) Subscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	if g.altDB == nil {
		return errors.New("database connection not available")
	}
	return g.altDB.InsertSubscription(ctx, userID, feedLinkID)
}

func (g *SubscriptionGateway) Unsubscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	if g.altDB == nil {
		return errors.New("database connection not available")
	}
	return g.altDB.DeleteSubscription(ctx, userID, feedLinkID)
}
