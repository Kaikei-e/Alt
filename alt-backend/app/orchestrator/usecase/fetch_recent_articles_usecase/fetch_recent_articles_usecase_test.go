package fetch_recent_articles_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestComputeRecentWindow(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		withinHours   int
		limit         int
		wantHours     int
		wantLimit     int
		wantSinceTime time.Time
	}{
		{
			name:          "defaults when withinHours is 0 and limit is 0",
			withinHours:   0,
			limit:         0,
			wantHours:     24,
			wantLimit:     0,
			wantSinceTime: now.Add(-24 * time.Hour),
		},
		{
			name:          "defaults when withinHours is negative and limit is negative",
			withinHours:   -5,
			limit:         -10,
			wantHours:     24,
			wantLimit:     100,
			wantSinceTime: now.Add(-24 * time.Hour),
		},
		{
			name:          "caps withinHours at 168 and limit at 500",
			withinHours:   200,
			limit:         1000,
			wantHours:     168,
			wantLimit:     500,
			wantSinceTime: now.Add(-168 * time.Hour),
		},
		{
			name:          "preserves valid withinHours and limit",
			withinHours:   48,
			limit:         50,
			wantHours:     48,
			wantLimit:     50,
			wantSinceTime: now.Add(-48 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSince, gotHours, gotLimit := computeRecentWindow(tt.withinHours, tt.limit, now)
			if !gotSince.Equal(tt.wantSinceTime) {
				t.Errorf("computeRecentWindow() since = %v, want %v", gotSince, tt.wantSinceTime)
			}
			if gotHours != tt.wantHours {
				t.Errorf("computeRecentWindow() hours = %d, want %d", gotHours, tt.wantHours)
			}
			if gotLimit != tt.wantLimit {
				t.Errorf("computeRecentWindow() limit = %d, want %d", gotLimit, tt.wantLimit)
			}
		})
	}
}

func TestFetchRecentArticlesUsecase_Execute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	t.Run("succeeds and returns output", func(t *testing.T) {
		mockPort := mocks.NewMockFetchRecentArticlesPort(ctrl)
		articles := []*domain.Article{
			{ID: uuid.New(), Title: "Article 1"},
		}
		mockPort.EXPECT().FetchRecentArticles(ctx, gomock.Any(), 100).Return(articles, nil)

		uc := NewFetchRecentArticlesUsecase(mockPort)
		out, err := uc.Execute(ctx, FetchRecentArticlesInput{WithinHours: 24, Limit: 100})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Count != 1 {
			t.Errorf("expected count 1, got %d", out.Count)
		}
		if len(out.Articles) != 1 {
			t.Errorf("expected 1 article, got %d", len(out.Articles))
		}
	})

	t.Run("returns error when gateway fails", func(t *testing.T) {
		mockPort := mocks.NewMockFetchRecentArticlesPort(ctrl)
		mockPort.EXPECT().FetchRecentArticles(ctx, gomock.Any(), 100).Return(nil, errors.New("db error"))

		uc := NewFetchRecentArticlesUsecase(mockPort)
		out, err := uc.Execute(ctx, FetchRecentArticlesInput{WithinHours: 24, Limit: 100})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if out != nil {
			t.Errorf("expected nil output, got %v", out)
		}
	})
}
