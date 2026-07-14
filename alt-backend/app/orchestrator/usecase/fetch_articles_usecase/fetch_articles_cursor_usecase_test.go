package fetch_articles_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

var (
	ErrMockDatabase = errors.New("mock database error")
)

func createMockArticles() []*domain.Article {
	now := time.Now()
	return []*domain.Article{
		{
			ID:          uuid.New(),
			Title:       "Test Article 1",
			URL:         "https://example.com/article1",
			Content:     "Test content 1",
			PublishedAt: now.Add(-1 * time.Hour),
			CreatedAt:   now.Add(-1 * time.Hour),
			Tags:        []string{"tag1", "tag2"},
		},
		{
			ID:          uuid.New(),
			Title:       "Test Article 2",
			URL:         "https://example.com/article2",
			Content:     "Test content 2",
			PublishedAt: now.Add(-2 * time.Hour),
			CreatedAt:   now.Add(-2 * time.Hour),
			Tags:        []string{"tag3"},
		},
	}
}

func createEmptyArticles() []*domain.Article {
	return []*domain.Article{}
}

func createCancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestFetchArticlesCursorUsecase_Execute(t *testing.T) {
	// Initialize logger to prevent nil pointer dereference
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGateway := mocks.NewMockFetchArticlesPort(ctrl)
	mockData := createMockArticles()

	// Create cursor time for testing
	cursorTime := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name      string
		ctx       context.Context
		cursor    *time.Time
		limit     int
		mockSetup func()
		want      []*domain.Article
		wantErr   bool
	}{
		{
			name:   "success - first page (no cursor)",
			ctx:    context.Background(),
			cursor: nil,
			limit:  20,
			mockSetup: func() {
				mockGateway.EXPECT().FetchArticlesWithCursor(gomock.Any(), nil, 20).Return(mockData, nil).Times(1)
			},
			want:    mockData,
			wantErr: false,
		},
		{
			name:   "success - with cursor",
			ctx:    context.Background(),
			cursor: &cursorTime,
			limit:  20,
			mockSetup: func() {
				mockGateway.EXPECT().FetchArticlesWithCursor(gomock.Any(), &cursorTime, 20).Return(mockData, nil).Times(1)
			},
			want:    mockData,
			wantErr: false,
		},
		{
			name:   "success - empty result",
			ctx:    context.Background(),
			cursor: &cursorTime,
			limit:  20,
			mockSetup: func() {
				mockGateway.EXPECT().FetchArticlesWithCursor(gomock.Any(), &cursorTime, 20).Return(createEmptyArticles(), nil).Times(1)
			},
			want:    createEmptyArticles(),
			wantErr: false,
		},
		{
			name:   "invalid limit - zero",
			ctx:    context.Background(),
			cursor: nil,
			limit:  0,
			mockSetup: func() {
				// Should not call gateway for invalid limit
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid limit - negative",
			ctx:    context.Background(),
			cursor: nil,
			limit:  -1,
			mockSetup: func() {
				// Should not call gateway for invalid limit
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "invalid limit - too large",
			ctx:    context.Background(),
			cursor: nil,
			limit:  101,
			mockSetup: func() {
				// Should not call gateway for limit > 100
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "database error",
			ctx:    context.Background(),
			cursor: nil,
			limit:  20,
			mockSetup: func() {
				mockGateway.EXPECT().FetchArticlesWithCursor(gomock.Any(), nil, 20).Return(nil, ErrMockDatabase).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:   "cancelled context",
			ctx:    createCancelledContext(),
			cursor: nil,
			limit:  20,
			mockSetup: func() {
				mockGateway.EXPECT().FetchArticlesWithCursor(gomock.Any(), nil, 20).Return(nil, context.Canceled).Times(1)
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()
			u := &FetchArticlesCursorUsecase{
				fetchArticlesGateway: mockGateway,
			}
			got, err := u.Execute(tt.ctx, tt.cursor, tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchArticlesCursorUsecase.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchArticlesCursorUsecase.Execute() = %v, want %v", got, tt.want)
			}
		})
	}
}
