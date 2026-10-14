package reading_status

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
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
			u:    &FeedsReadingStatusUsecase{updateFeedStatusGateway: mockUpdateFeedStatusGateway},
			args: args{
				ctx:     mockCtx,
				feedURL: mockFeedURL,
			},
			wantErr: false,
		},
		{
			name: "error",
			u:    &FeedsReadingStatusUsecase{updateFeedStatusGateway: mockUpdateFeedStatusGateway},
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
