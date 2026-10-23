package subscription_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

type stubSubscriptionPort struct {
	sources        []*domain.FeedSource
	listErr        error
	subscribeErr   error
	unsubscribeErr error
	lastCtx        context.Context
	lastUserID     uuid.UUID
	lastFeedLinkID uuid.UUID
}

func (s *stubSubscriptionPort) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	s.lastCtx = ctx
	s.lastUserID = userID
	return s.sources, s.listErr
}

func (s *stubSubscriptionPort) Subscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	s.lastCtx = ctx
	s.lastUserID = userID
	s.lastFeedLinkID = feedLinkID
	return s.subscribeErr
}

func (s *stubSubscriptionPort) Unsubscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	s.lastCtx = ctx
	s.lastUserID = userID
	s.lastFeedLinkID = feedLinkID
	return s.unsubscribeErr
}

func contextWithUser(userID uuid.UUID) context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		TenantID:  userID,
		Email:     "subscriber@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestListSubscriptionsUsecase(t *testing.T) {
	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	t.Run("success returns subscriptions and preserves user context", func(t *testing.T) {
		now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
		expected := []*domain.FeedSource{
			{
				ID:           "feed-1",
				URL:          "https://example.com/rss.xml",
				Title:        "Example Feed",
				IsSubscribed: true,
				CreatedAt:    now,
			},
		}
		port := &stubSubscriptionPort{sources: expected}
		uc := NewListSubscriptionsUsecase(port)

		ctx := contextWithUser(userID)
		got, err := uc.Execute(ctx, userID)

		require.NoError(t, err)
		assert.Equal(t, expected, got)
		assert.Equal(t, userID, port.lastUserID)

		ctxUser, ctxErr := domain.GetUserFromContext(port.lastCtx)
		require.NoError(t, ctxErr)
		assert.Equal(t, userID, ctxUser.UserID)
	})

	t.Run("port error is propagated", func(t *testing.T) {
		wantErr := errors.New("datahub list subscriptions failed")
		port := &stubSubscriptionPort{listErr: wantErr}
		uc := NewListSubscriptionsUsecase(port)

		got, err := uc.Execute(contextWithUser(userID), userID)

		require.ErrorIs(t, err, wantErr)
		assert.Nil(t, got)
	})
}

func TestSubscribeUsecase(t *testing.T) {
	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	feedLinkID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	t.Run("success forwards userID, feedLinkID, and user context to port", func(t *testing.T) {
		port := &stubSubscriptionPort{}
		uc := NewSubscribeUsecase(port)

		ctx := contextWithUser(userID)
		err := uc.Execute(ctx, userID, feedLinkID)

		require.NoError(t, err)
		assert.Equal(t, userID, port.lastUserID)
		assert.Equal(t, feedLinkID, port.lastFeedLinkID)

		ctxUser, ctxErr := domain.GetUserFromContext(port.lastCtx)
		require.NoError(t, ctxErr)
		assert.Equal(t, userID, ctxUser.UserID)
	})

	t.Run("port error is propagated", func(t *testing.T) {
		wantErr := errors.New("subscribe failed")
		port := &stubSubscriptionPort{subscribeErr: wantErr}
		uc := NewSubscribeUsecase(port)

		err := uc.Execute(contextWithUser(userID), userID, feedLinkID)

		require.ErrorIs(t, err, wantErr)
	})
}

func TestUnsubscribeUsecase(t *testing.T) {
	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	feedLinkID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	t.Run("success forwards userID, feedLinkID, and user context to port", func(t *testing.T) {
		port := &stubSubscriptionPort{}
		uc := NewUnsubscribeUsecase(port)

		ctx := contextWithUser(userID)
		err := uc.Execute(ctx, userID, feedLinkID)

		require.NoError(t, err)
		assert.Equal(t, userID, port.lastUserID)
		assert.Equal(t, feedLinkID, port.lastFeedLinkID)

		ctxUser, ctxErr := domain.GetUserFromContext(port.lastCtx)
		require.NoError(t, ctxErr)
		assert.Equal(t, userID, ctxUser.UserID)
	})

	t.Run("port error is propagated", func(t *testing.T) {
		wantErr := errors.New("unsubscribe failed")
		port := &stubSubscriptionPort{unsubscribeErr: wantErr}
		uc := NewUnsubscribeUsecase(port)

		err := uc.Execute(contextWithUser(userID), userID, feedLinkID)

		require.ErrorIs(t, err, wantErr)
	})
}
