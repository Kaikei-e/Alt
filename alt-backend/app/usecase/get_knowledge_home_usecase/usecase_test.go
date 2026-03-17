package get_knowledge_home_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHomeItemsPort implements knowledge_home_port.GetKnowledgeHomeItemsPort.
type mockHomeItemsPort struct {
	items      []domain.KnowledgeHomeItem
	nextCursor string
	hasMore    bool
	err        error
}

func (m *mockHomeItemsPort) GetKnowledgeHomeItems(_ context.Context, _ uuid.UUID, _ string, _ int) ([]domain.KnowledgeHomeItem, string, bool, error) {
	return m.items, m.nextCursor, m.hasMore, m.err
}

// mockTodayDigestPort implements today_digest_port.GetTodayDigestPort.
type mockTodayDigestPort struct {
	digest domain.TodayDigest
	err    error
}

func (m *mockTodayDigestPort) GetTodayDigest(_ context.Context, _ uuid.UUID, _ time.Time) (domain.TodayDigest, error) {
	return m.digest, m.err
}

func TestGetKnowledgeHomeUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	now := time.Now()

	tests := []struct {
		name           string
		cursor         string
		limit          int
		date           time.Time
		homeItems      *mockHomeItemsPort
		todayDigest    *mockTodayDigestPort
		wantDegraded   bool
		wantItemCount  int
		wantErr        bool
	}{
		{
			name:   "success - returns items and digest",
			cursor: "",
			limit:  20,
			date:   now,
			homeItems: &mockHomeItemsPort{
				items: []domain.KnowledgeHomeItem{
					{ItemKey: "article:1", Title: "Test Article", Score: 1.0},
					{ItemKey: "article:2", Title: "Another Article", Score: 0.5},
				},
				nextCursor: "abc",
				hasMore:    true,
			},
			todayDigest: &mockTodayDigestPort{
				digest: domain.TodayDigest{
					UserID:      userID,
					NewArticles: 5,
				},
			},
			wantDegraded:  false,
			wantItemCount: 2,
		},
		{
			name:   "degraded mode - items fetch fails",
			cursor: "",
			limit:  20,
			date:   now,
			homeItems: &mockHomeItemsPort{
				err: errors.New("db error"),
			},
			todayDigest: &mockTodayDigestPort{
				digest: domain.TodayDigest{NewArticles: 3},
			},
			wantDegraded:  true,
			wantItemCount: 0,
		},
		{
			name:   "degraded mode - digest fetch fails",
			cursor: "",
			limit:  20,
			date:   now,
			homeItems: &mockHomeItemsPort{
				items: []domain.KnowledgeHomeItem{
					{ItemKey: "article:1", Title: "Test"},
				},
			},
			todayDigest: &mockTodayDigestPort{
				err: errors.New("db error"),
			},
			wantDegraded:  true,
			wantItemCount: 1,
		},
		{
			name:   "empty result",
			cursor: "",
			limit:  20,
			date:   now,
			homeItems: &mockHomeItemsPort{
				items: nil,
			},
			todayDigest: &mockTodayDigestPort{
				digest: domain.TodayDigest{},
			},
			wantDegraded:  false,
			wantItemCount: 0,
		},
		{
			name:   "limit clamped to 100",
			cursor: "",
			limit:  200,
			date:   now,
			homeItems: &mockHomeItemsPort{
				items: nil,
			},
			todayDigest: &mockTodayDigestPort{
				digest: domain.TodayDigest{},
			},
			wantDegraded:  false,
			wantItemCount: 0,
		},
		{
			name:   "default limit when zero",
			cursor: "",
			limit:  0,
			date:   now,
			homeItems: &mockHomeItemsPort{
				items: nil,
			},
			todayDigest: &mockTodayDigestPort{
				digest: domain.TodayDigest{},
			},
			wantDegraded:  false,
			wantItemCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewGetKnowledgeHomeUsecase(tt.homeItems, tt.todayDigest)
			result, err := uc.Execute(context.Background(), userID, tt.cursor, tt.limit, tt.date)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantDegraded, result.Degraded)
			assert.Len(t, result.Items, tt.wantItemCount)
		})
	}
}
