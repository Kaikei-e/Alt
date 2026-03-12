package user_read_state_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/cache"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type userReadStateDB interface {
	GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error)
	GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

type Gateway struct {
	db                userReadStateDB
	subscriptionCache *cache.SharedCache[uuid.UUID, []uuid.UUID]
}

func NewGateway(db *alt_db.AltDBRepository) *Gateway {
	g := &Gateway{db: db}
	g.subscriptionCache = cache.NewSharedCache(30*time.Second, 0, g.loadSubscriptions)
	return g
}

func newGateway(db userReadStateDB) *Gateway {
	g := &Gateway{db: db}
	g.subscriptionCache = cache.NewSharedCache(30*time.Second, 0, g.loadSubscriptions)
	return g
}

func (g *Gateway) GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(feedIDs) == 0 {
		return map[uuid.UUID]bool{}, nil
	}
	return g.db.GetReadFeedIDs(ctx, userID, feedIDs)
}

func (g *Gateway) GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	return g.db.GetAllReadFeedIDs(ctx, userID)
}

func (g *Gateway) GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return g.subscriptionCache.Get(ctx, userID)
}

func (g *Gateway) loadSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := g.db.GetUserSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user subscriptions: %w", err)
	}
	return ids, nil
}
