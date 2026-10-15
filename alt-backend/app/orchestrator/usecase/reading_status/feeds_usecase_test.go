package reading_status

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestFeedsReadingStatusUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFeedURL := url.URL{
		Scheme: "https",
		Host:   "example.com",
		Path:   "/feed",
	}
	mockFeedURLError := url.URL{}
	mockUserID := uuid.MustParse("01020304-0506-0708-090a-0b0c0d0e0f10")
	mockCtx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    mockUserID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	mockUpdateFeedStatusGateway := mocks.NewMockUpdateFeedStatusPort(ctrl)
	noopDismiss := func(_ context.Context, _, _ uuid.UUID, _ string) error { return nil }

	type args struct {
		ctx     context.Context
		feedURL url.URL
	}
	tests := []struct {
		name    string
		u       *FeedsReadingStatusUsecase
		args    args
		wantErr bool
	}{
		{
			name: "success",
			u:    NewFeedsReadingStatusUsecase(mockUpdateFeedStatusGateway).WithHomeDismiss(&stubArticleLookup{}, noopDismiss),
			args: args{
				ctx:     mockCtx,
				feedURL: mockFeedURL,
			},
			wantErr: false,
		},
		{
			name: "error",
			u:    NewFeedsReadingStatusUsecase(mockUpdateFeedStatusGateway).WithHomeDismiss(&stubArticleLookup{}, noopDismiss),
			args: args{
				ctx:     mockCtx,
				feedURL: mockFeedURLError,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expectedError error
			if tt.wantErr {
				expectedError = errors.New("mock error")
			}
			mockUpdateFeedStatusGateway.EXPECT().UpdateFeedStatus(tt.args.ctx, tt.args.feedURL, mockUserID).Return(expectedError)

			u := tt.u
			if err := u.Execute(tt.args.ctx, tt.args.feedURL); (err != nil) != tt.wantErr {
				t.Errorf("FeedsReadingStatusUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFeedsReadingStatusUsecase_Execute_NoUserContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUpdateFeedStatusGateway := mocks.NewMockUpdateFeedStatusPort(ctrl)
	u := &FeedsReadingStatusUsecase{updateFeedStatusGateway: mockUpdateFeedStatusGateway}

	err := u.Execute(context.Background(), url.URL{Scheme: "https", Host: "example.com", Path: "/feed"})
	if err == nil {
		t.Fatal("expected error when user context is missing, got nil")
	}
}

type stubArticleLookup struct {
	article *domain.ArticleContent
	err     error
}

func (s *stubArticleLookup) FetchArticleByURL(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return s.article, s.err
}

type stubUpdateArticleStatus struct {
	err error
}

func (s *stubUpdateArticleStatus) MarkArticleAsRead(_ context.Context, _ url.URL, _ uuid.UUID) error {
	return s.err
}

func TestArticlesReadingStatusUsecase_WithHomeDismiss(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		TenantID:  tenantID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	var dismissedItemKey string
	var dismissedUser uuid.UUID
	u := NewArticlesReadingStatusUsecase(&stubUpdateArticleStatus{}).
		WithHomeDismiss(&stubArticleLookup{article: &domain.ArticleContent{ID: "art-123"}}, func(_ context.Context, uID, _ uuid.UUID, itemKey string) error {
			dismissedUser = uID
			dismissedItemKey = itemKey
			return nil
		})

	targetURL := url.URL{Scheme: "https", Host: "example.com", Path: "/post-123"}
	if err := u.Execute(ctx, targetURL); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dismissedUser != userID || dismissedItemKey != "article:art-123" {
		t.Fatalf("expected dismiss of article:art-123 for %s, got %s for %s", userID, dismissedItemKey, dismissedUser)
	}
}

func TestArticlesReadingStatusUsecase_PanicsWhenHomeDismissUnwired(t *testing.T) {
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	targetURL := url.URL{Scheme: "https", Host: "example.com", Path: "/post-123"}
	noopDismiss := func(_ context.Context, _, _ uuid.UUID, _ string) error { return nil }
	wantPanic := "reading_status: home-dismiss hook is not wired (use WithHomeDismiss)"

	t.Run("panics when WithHomeDismiss was not called", func(t *testing.T) {
		u := NewArticlesReadingStatusUsecase(&stubUpdateArticleStatus{})
		assert.PanicsWithValue(t, wantPanic, func() {
			_ = u.Execute(ctx, targetURL)
		})
	})

	t.Run("panics when articleLookup is nil", func(t *testing.T) {
		u := NewArticlesReadingStatusUsecase(&stubUpdateArticleStatus{}).WithHomeDismiss(nil, noopDismiss)
		assert.PanicsWithValue(t, wantPanic, func() {
			_ = u.Execute(ctx, targetURL)
		})
	})

	t.Run("panics when dismissFn is nil", func(t *testing.T) {
		u := NewArticlesReadingStatusUsecase(&stubUpdateArticleStatus{}).WithHomeDismiss(&stubArticleLookup{}, nil)
		assert.PanicsWithValue(t, wantPanic, func() {
			_ = u.Execute(ctx, targetURL)
		})
	})
}

func TestFeedsReadingStatusUsecase_PanicsWhenHomeDismissUnwired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := uuid.New()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		TenantID:  uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	targetURL := url.URL{Scheme: "https", Host: "example.com", Path: "/feed"}

	t.Run("panics when WithHomeDismiss was not called", func(t *testing.T) {
		mockGw := mocks.NewMockUpdateFeedStatusPort(ctrl)
		mockGw.EXPECT().UpdateFeedStatus(ctx, targetURL, userID).Return(nil)
		u := NewFeedsReadingStatusUsecase(mockGw)
		assert.PanicsWithValue(t, "reading_status: home-dismiss hook is not wired (use WithHomeDismiss)", func() {
			_ = u.Execute(ctx, targetURL)
		})
	})
}

func TestReadingStatus_HomeDismissLookupErrorLogging(t *testing.T) {
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	targetURL := url.URL{Scheme: "https", Host: "example.com", Path: "/post-123"}
	noopDismiss := func(_ context.Context, _, _ uuid.UUID, _ string) error { return nil }

	t.Run("logs warn without URL when FetchArticleByURL returns an error and still succeeds", func(t *testing.T) {
		var buf bytes.Buffer
		origLogger := logger.Logger
		logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		t.Cleanup(func() { logger.Logger = origLogger })

		u := NewArticlesReadingStatusUsecase(&stubUpdateArticleStatus{}).
			WithHomeDismiss(&stubArticleLookup{err: fmt.Errorf("get article by url %s: datahub connection refused", targetURL.String())}, noopDismiss)

		err := u.Execute(ctx, targetURL)
		require.NoError(t, err, "sync failure must stay best-effort")
		assert.Contains(t, buf.String(), `"level":"WARN"`)
		assert.Contains(t, buf.String(), `"error_kind":"article_lookup_failed"`)
		assert.NotContains(t, buf.String(), targetURL.String())
		assert.NotContains(t, buf.String(), "/post-123")
	})

	t.Run("logs warn without URL when dismissFn returns an error", func(t *testing.T) {
		var buf bytes.Buffer
		origLogger := logger.Logger
		logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		t.Cleanup(func() { logger.Logger = origLogger })

		u := NewArticlesReadingStatusUsecase(&stubUpdateArticleStatus{}).
			WithHomeDismiss(&stubArticleLookup{article: &domain.ArticleContent{ID: "art-123"}}, func(_ context.Context, _, _ uuid.UUID, _ string) error {
				return fmt.Errorf("dismiss %s: rpc failed", targetURL.String())
			})

		err := u.Execute(ctx, targetURL)
		require.NoError(t, err, "sync failure must stay best-effort")
		assert.Contains(t, buf.String(), `"level":"WARN"`)
		assert.Contains(t, buf.String(), `"article_id":"art-123"`)
		assert.NotContains(t, buf.String(), targetURL.String())
		assert.NotContains(t, buf.String(), "/post-123")
	})

	t.Run("does not log warn when FetchArticleByURL finds no archived article", func(t *testing.T) {
		var buf bytes.Buffer
		origLogger := logger.Logger
		logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		t.Cleanup(func() { logger.Logger = origLogger })

		u := NewArticlesReadingStatusUsecase(&stubUpdateArticleStatus{}).
			WithHomeDismiss(&stubArticleLookup{article: nil, err: nil}, noopDismiss)

		err := u.Execute(ctx, targetURL)
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}
