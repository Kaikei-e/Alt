package user_read_state_gateway

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

type userReadStateDBStub struct {
	subscriptionLoads int32
	allReadLoads      int32
	subscriptions     []uuid.UUID
	readIDs           map[uuid.UUID]bool
	allReadIDs        map[uuid.UUID]bool
}

func (s *userReadStateDBStub) GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return s.readIDs, nil
}

func (s *userReadStateDBStub) GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	atomic.AddInt32(&s.allReadLoads, 1)
	return s.allReadIDs, nil
}

func (s *userReadStateDBStub) GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	atomic.AddInt32(&s.subscriptionLoads, 1)
	return s.subscriptions, nil
}

func TestGateway_GetUserSubscriptions_Caches(t *testing.T) {
	userID := uuid.New()
	db := &userReadStateDBStub{subscriptions: []uuid.UUID{uuid.New(), uuid.New()}}
	gateway := newGateway(db)

	first, err := gateway.GetUserSubscriptions(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserSubscriptions() error = %v", err)
	}
	second, err := gateway.GetUserSubscriptions(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserSubscriptions() second error = %v", err)
	}

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("unexpected subscription length")
	}
	if atomic.LoadInt32(&db.subscriptionLoads) != 1 {
		t.Fatalf("subscription loads = %d, want 1", db.subscriptionLoads)
	}
}

func TestGateway_GetAllReadFeedIDs(t *testing.T) {
	feedID := uuid.New()
	db := &userReadStateDBStub{allReadIDs: map[uuid.UUID]bool{feedID: true}}
	gateway := newGateway(db)
	got, err := gateway.GetAllReadFeedIDs(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetAllReadFeedIDs() error = %v", err)
	}
	if !got[feedID] {
		t.Fatal("expected feedID to be read")
	}
}

func TestGateway_GetAllReadFeedIDs_Caches(t *testing.T) {
	userID := uuid.New()
	feedID := uuid.New()
	db := &userReadStateDBStub{allReadIDs: map[uuid.UUID]bool{feedID: true}}
	gateway := newGateway(db)

	first, err := gateway.GetAllReadFeedIDs(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetAllReadFeedIDs() error = %v", err)
	}
	second, err := gateway.GetAllReadFeedIDs(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetAllReadFeedIDs() second error = %v", err)
	}

	if !first[feedID] || !second[feedID] {
		t.Fatal("expected feedID to be read in both calls")
	}
	if atomic.LoadInt32(&db.allReadLoads) != 1 {
		t.Fatalf("allReadLoads = %d, want 1 (should cache)", db.allReadLoads)
	}
}

func TestGateway_GetReadFeedIDs_Empty(t *testing.T) {
	gateway := newGateway(&userReadStateDBStub{})
	got, err := gateway.GetReadFeedIDs(context.Background(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("GetReadFeedIDs() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetReadFeedIDs() len = %d, want 0", len(got))
	}
}
