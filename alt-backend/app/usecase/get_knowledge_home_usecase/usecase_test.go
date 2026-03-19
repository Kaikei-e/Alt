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
	lastFilter *domain.KnowledgeHomeLensFilter
}

func (m *mockHomeItemsPort) GetKnowledgeHomeItems(_ context.Context, _ uuid.UUID, _ string, _ int, filter *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error) {
	m.lastFilter = filter
	return m.items, m.nextCursor, m.hasMore, m.err
}

type mockResolveLensPort struct {
	filter *domain.KnowledgeHomeLensFilter
	err    error
}

func (m *mockResolveLensPort) ResolveKnowledgeHomeLens(_ context.Context, _ uuid.UUID, _ *uuid.UUID) (*domain.KnowledgeHomeLensFilter, error) {
	return m.filter, m.err
}

// mockTodayDigestPort implements today_digest_port.GetTodayDigestPort.
type mockTodayDigestPort struct {
	digest domain.TodayDigest
	err    error
}

func (m *mockTodayDigestPort) GetTodayDigest(_ context.Context, _ uuid.UUID, _ time.Time) (domain.TodayDigest, error) {
	return m.digest, m.err
}

// mockProjectionFreshnessPort implements today_digest_port.GetProjectionFreshnessPort.
type mockProjectionFreshnessPort struct {
	updatedAt *time.Time
	err       error
}

func (m *mockProjectionFreshnessPort) GetProjectionFreshness(_ context.Context, _ string) (*time.Time, error) {
	return m.updatedAt, m.err
}

// mockCountNeedToKnowPort implements today_digest_port.CountNeedToKnowItemsPort.
type mockCountNeedToKnowPort struct {
	count int
	err   error
}

func (m *mockCountNeedToKnowPort) CountNeedToKnowItems(_ context.Context, _ uuid.UUID, _ time.Time) (int, error) {
	return m.count, m.err
}

func TestGetKnowledgeHomeUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	now := time.Now()

	tests := []struct {
		name          string
		cursor        string
		limit         int
		date          time.Time
		homeItems     *mockHomeItemsPort
		todayDigest   *mockTodayDigestPort
		resolveLens   *mockResolveLensPort
		lensID        *uuid.UUID
		wantDegraded  bool
		wantItemCount int
		wantFilter    *domain.KnowledgeHomeLensFilter
		wantErr       bool
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
			resolveLens:   &mockResolveLensPort{},
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
			resolveLens:   &mockResolveLensPort{},
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
			resolveLens:   &mockResolveLensPort{},
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
			resolveLens:   &mockResolveLensPort{},
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
			resolveLens:   &mockResolveLensPort{},
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
			resolveLens:   &mockResolveLensPort{},
			wantDegraded:  false,
			wantItemCount: 0,
		},
		{
			name:   "applies resolved lens filter",
			cursor: "",
			limit:  20,
			date:   now,
			homeItems: &mockHomeItemsPort{
				items: []domain.KnowledgeHomeItem{
					{ItemKey: "article:1", Title: "Filtered", Score: 1.0},
				},
			},
			todayDigest: &mockTodayDigestPort{
				digest: domain.TodayDigest{},
			},
			resolveLens: &mockResolveLensPort{
				filter: &domain.KnowledgeHomeLensFilter{
					LensID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					TagNames:   []string{"AI"},
					FeedIDs:    []uuid.UUID{uuid.MustParse("22222222-2222-2222-2222-222222222222")},
					TimeWindow: "7d",
				},
			},
			wantDegraded:  false,
			wantItemCount: 1,
			wantFilter: &domain.KnowledgeHomeLensFilter{
				LensID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				TagNames:   []string{"AI"},
				FeedIDs:    []uuid.UUID{uuid.MustParse("22222222-2222-2222-2222-222222222222")},
				TimeWindow: "7d",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewGetKnowledgeHomeUsecase(tt.homeItems, tt.todayDigest, tt.resolveLens, nil, nil)
			result, err := uc.Execute(context.Background(), userID, tt.cursor, tt.limit, tt.date, tt.lensID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantDegraded, result.Degraded)
			assert.Len(t, result.Items, tt.wantItemCount)
			assert.Equal(t, tt.wantFilter, tt.homeItems.lastFilter)
		})
	}
}

func TestGetKnowledgeHomeUsecase_DigestEnrichment(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	now := time.Now()
	recentTime := now.Add(-1 * time.Minute)
	staleTime := now.Add(-10 * time.Minute)

	t.Run("enriches needToKnowCount from backend query", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{items: []domain.KnowledgeHomeItem{}}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{NeedToKnowCount: 0}}
		countPort := &mockCountNeedToKnowPort{count: 7}
		freshnessPort := &mockProjectionFreshnessPort{updatedAt: &recentTime}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, freshnessPort, countPort)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		assert.Equal(t, 7, result.Digest.NeedToKnowCount)
	})

	t.Run("enriches freshness as fresh when projector is recent", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{items: []domain.KnowledgeHomeItem{}}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{}}
		freshnessPort := &mockProjectionFreshnessPort{updatedAt: &recentTime}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, freshnessPort, nil)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		assert.Equal(t, domain.FreshnessFresh, result.Digest.DigestFreshness)
		require.NotNil(t, result.Digest.LastProjectedAt)
	})

	t.Run("enriches freshness as stale when projector is old", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{items: []domain.KnowledgeHomeItem{}}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{}}
		freshnessPort := &mockProjectionFreshnessPort{updatedAt: &staleTime}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, freshnessPort, nil)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		assert.Equal(t, domain.FreshnessStale, result.Digest.DigestFreshness)
	})

	t.Run("freshness is unknown when checkpoint not found", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{items: []domain.KnowledgeHomeItem{}}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{}}
		freshnessPort := &mockProjectionFreshnessPort{updatedAt: nil}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, freshnessPort, nil)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		assert.Equal(t, domain.FreshnessUnknown, result.Digest.DigestFreshness)
	})

	t.Run("enrichment failure does not hard fail", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{
			items: []domain.KnowledgeHomeItem{{ItemKey: "article:1", Title: "Test"}},
		}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{NewArticles: 5}}
		freshnessPort := &mockProjectionFreshnessPort{err: errors.New("db error")}
		countPort := &mockCountNeedToKnowPort{err: errors.New("db error")}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, freshnessPort, countPort)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, 5, result.Digest.NewArticles)
		assert.Equal(t, domain.FreshnessUnknown, result.Digest.DigestFreshness)
		assert.Equal(t, 0, result.Digest.NeedToKnowCount)
	})

	t.Run("nil enrichment ports skip enrichment gracefully", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{items: []domain.KnowledgeHomeItem{}}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{NeedToKnowCount: 3}}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, nil, nil)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		// Without enrichment ports, digest keeps original values
		assert.Equal(t, 3, result.Digest.NeedToKnowCount)
		assert.Equal(t, "", result.Digest.DigestFreshness)
	})

	t.Run("enrichDigest preserves availability flags from read model", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{items: []domain.KnowledgeHomeItem{}}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{
			WeeklyRecapAvailable:  true,
			EveningPulseAvailable: true,
		}}
		countPort := &mockCountNeedToKnowPort{count: 2}
		freshnessPort := &mockProjectionFreshnessPort{updatedAt: &recentTime}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, freshnessPort, countPort)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		// enrichDigest must NOT overwrite availability flags
		assert.True(t, result.Digest.WeeklyRecapAvailable)
		assert.True(t, result.Digest.EveningPulseAvailable)
		assert.Equal(t, 2, result.Digest.NeedToKnowCount)
	})
}

func TestGetKnowledgeHomeUsecase_Execute_ReturnsCancellationErrors(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	now := time.Now()

	t.Run("returns context canceled when item fetch is canceled", func(t *testing.T) {
		uc := NewGetKnowledgeHomeUsecase(
			&mockHomeItemsPort{err: context.Canceled},
			&mockTodayDigestPort{digest: domain.TodayDigest{}},
			nil,
			nil,
			nil,
		)

		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	})

	t.Run("returns deadline exceeded when digest fetch times out", func(t *testing.T) {
		uc := NewGetKnowledgeHomeUsecase(
			&mockHomeItemsPort{items: []domain.KnowledgeHomeItem{{ItemKey: "article:1"}}},
			&mockTodayDigestPort{err: context.DeadlineExceeded},
			nil,
			nil,
			nil,
		)

		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, result)
	})
}
