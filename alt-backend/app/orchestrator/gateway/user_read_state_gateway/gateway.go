package user_read_state_gateway

import (
	"alt/utils/cache"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// userReadStateDB is the alt-data-hub read state (capability catalog §2.I
// W3-I3 / W3-I4 / W3-I5). Method names match the driver they replace, so
// migrating was a DI change.
//
// The two caches stay on this side. A TTL is flow orchestration and belongs to
// the caller (ADR-000954 D4); putting them in the provider would make one
// process's staleness another process's problem, and the 5-second read-state
// window exists because this process re-reads it per page render.
type userReadStateDB interface {
	GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error)
	GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

type Gateway struct {
	db                userReadStateDB
	subscriptionCache *cache.SharedCache[uuid.UUID, []uuid.UUID]
	readStateCache    *cache.SharedCache[uuid.UUID, map[uuid.UUID]bool]
}

func NewGateway(db userReadStateDB) *Gateway {
	if db == nil {
		panic("user_read_state_gateway: a read state source is required — " +
			"a nil one would render every feed as unread for every user (see .claude/rules/di-wiring.md)")
	}
	g := &Gateway{db: db}
	g.subscriptionCache = cache.NewSharedCache(30*time.Second, 0, g.loadSubscriptions)
	g.readStateCache = cache.NewSharedCache(5*time.Second, 0, g.loadAllReadFeedIDs)
	return g
}

func newGateway(db userReadStateDB) *Gateway {
	g := &Gateway{db: db}
	g.subscriptionCache = cache.NewSharedCache(30*time.Second, 0, g.loadSubscriptions)
	g.readStateCache = cache.NewSharedCache(5*time.Second, 0, g.loadAllReadFeedIDs)
	return g
}

func (g *Gateway) GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(feedIDs) == 0 {
		return map[uuid.UUID]bool{}, nil
	}
	return g.db.GetReadFeedIDs(ctx, userID, feedIDs)
}

func (g *Gateway) GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	return g.readStateCache.Get(ctx, userID)
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

func (g *Gateway) loadAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	ids, err := g.db.GetAllReadFeedIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get all read feed ids: %w", err)
	}
	return ids, nil
}
