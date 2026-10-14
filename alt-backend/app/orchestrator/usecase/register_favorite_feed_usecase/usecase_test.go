package register_favorite_feed_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// authedContext builds the request context the middleware installs. The
// usecase resolves the reader from it now: past this point the call leaves the
// process that knows who is logged in (capability catalog §2.I).
func authedContext(userID uuid.UUID) context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestRegisterFavoriteFeedUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
	userID := uuid.New()
	noopOpen := func(_ context.Context, _, _ uuid.UUID, _ string) error { return nil }

	tests := []struct {
		name      string
		url       string
		setupMock func()
		wantErr   bool
	}{
		{
			name: "success",
			url:  "https://example.com/rss.xml",
			setupMock: func() {
				mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/rss.xml", userID).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "gateway error",
			url:  "https://example.com/rss.xml",
			setupMock: func() {
				mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/rss.xml", userID).Return(context.DeadlineExceeded)
			},
			wantErr: true,
		},
		{
			name:      "empty url",
			url:       "",
			setupMock: func() {},
			wantErr:   true,
		},
	}

	// The unauthenticated case is separate because it must not reach the
	// gateway at all: with the tenant carried explicitly across the boundary,
	// a call with nobody logged in would otherwise send the zero UUID and star
	// a feed for a user that does not exist.
	t.Run("no authenticated user", func(t *testing.T) {
		u := NewRegisterFavoriteFeedUsecase(mockGateway).WithHomeOpen(&stubArticleLookup{}, noopOpen)
		if err := u.Execute(context.Background(), "https://example.com/rss.xml"); err == nil {
			t.Error("expected an authentication error")
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			u := NewRegisterFavoriteFeedUsecase(mockGateway).WithHomeOpen(&stubArticleLookup{}, noopOpen)
			err := u.Execute(authedContext(userID), tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type stubArticleLookup struct {
	article *domain.ArticleContent
	err     error
}

func (s *stubArticleLookup) FetchArticleByURL(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return s.article, s.err
}

func TestRegisterFavoriteFeedUsecase_WithHomeOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
	userID := uuid.New()
	mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/post-1", userID).Return(nil)

	var openedItemKey string
	var openedUser uuid.UUID
	u := NewRegisterFavoriteFeedUsecase(mockGateway).
		WithHomeOpen(&stubArticleLookup{article: &domain.ArticleContent{ID: "art-fav-1"}}, func(_ context.Context, uID, _ uuid.UUID, itemKey string) error {
			openedUser = uID
			openedItemKey = itemKey
			return nil
		})

	if err := u.Execute(authedContext(userID), "https://example.com/post-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if openedUser != userID || openedItemKey != "article:art-fav-1" {
		t.Fatalf("expected open of article:art-fav-1 for %s, got %s for %s", userID, openedItemKey, openedUser)
	}
}

func TestRegisterFavoriteFeedUsecase_PanicsWhenHomeOpenUnwired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := uuid.New()
	noopOpen := func(_ context.Context, _, _ uuid.UUID, _ string) error { return nil }

	t.Run("panics when WithHomeOpen was not called", func(t *testing.T) {
		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/post-1", userID).Return(nil)
		u := NewRegisterFavoriteFeedUsecase(mockGateway)
		assert.Panics(t, func() {
			_ = u.Execute(authedContext(userID), "https://example.com/post-1")
		})
	})

	t.Run("panics when articleLookup is nil", func(t *testing.T) {
		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/post-1", userID).Return(nil)
		u := NewRegisterFavoriteFeedUsecase(mockGateway).WithHomeOpen(nil, noopOpen)
		assert.Panics(t, func() {
			_ = u.Execute(authedContext(userID), "https://example.com/post-1")
		})
	})

	t.Run("panics when openFn is nil", func(t *testing.T) {
		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/post-1", userID).Return(nil)
		u := NewRegisterFavoriteFeedUsecase(mockGateway).WithHomeOpen(&stubArticleLookup{}, nil)
		assert.Panics(t, func() {
			_ = u.Execute(authedContext(userID), "https://example.com/post-1")
		})
	})
}

func TestRegisterFavoriteFeedUsecase_HomeOpenLookupErrorLogging(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := uuid.New()
	noopOpen := func(_ context.Context, _, _ uuid.UUID, _ string) error { return nil }

	t.Run("logs warn when FetchArticleByURL returns an error and still succeeds", func(t *testing.T) {
		var buf bytes.Buffer
		origLogger := logger.Logger
		logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		t.Cleanup(func() { logger.Logger = origLogger })

		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/post-1", userID).Return(nil)
		u := NewRegisterFavoriteFeedUsecase(mockGateway).
			WithHomeOpen(&stubArticleLookup{err: errors.New("datahub lookup failed")}, noopOpen)

		err := u.Execute(authedContext(userID), "https://example.com/post-1")
		require.NoError(t, err, "sync failure must stay best-effort")
		assert.Contains(t, buf.String(), `"level":"WARN"`)
		assert.Contains(t, buf.String(), "datahub lookup failed")
	})

	t.Run("does not log warn when FetchArticleByURL finds no archived article", func(t *testing.T) {
		var buf bytes.Buffer
		origLogger := logger.Logger
		logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		t.Cleanup(func() { logger.Logger = origLogger })

		mockGateway := mocks.NewMockRegisterFavoriteFeedPort(ctrl)
		mockGateway.EXPECT().RegisterFavoriteFeed(gomock.Any(), "https://example.com/post-1", userID).Return(nil)
		u := NewRegisterFavoriteFeedUsecase(mockGateway).
			WithHomeOpen(&stubArticleLookup{article: nil, err: nil}, noopOpen)

		err := u.Execute(authedContext(userID), "https://example.com/post-1")
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}
