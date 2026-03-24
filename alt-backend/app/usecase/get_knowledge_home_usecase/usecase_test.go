package get_knowledge_home_usecase

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
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
					QueryText:  "agents",
					TagNames:   []string{"AI"},
					SourceIDs:  []uuid.UUID{uuid.MustParse("22222222-2222-2222-2222-222222222222")},
					TimeWindow: "7d",
				},
			},
			wantDegraded:  false,
			wantItemCount: 1,
			wantFilter: &domain.KnowledgeHomeLensFilter{
				LensID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				QueryText:  "agents",
				TagNames:   []string{"AI"},
				SourceIDs:  []uuid.UUID{uuid.MustParse("22222222-2222-2222-2222-222222222222")},
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

func TestGetKnowledgeHomeUsecase_ServiceQuality(t *testing.T) {
	logger.InitLogger()
	userID := uuid.New()
	now := time.Now()
	recentTime := now.Add(-1 * time.Minute)

	t.Run("full when all sources succeed", func(t *testing.T) {
		uc := NewGetKnowledgeHomeUsecase(
			&mockHomeItemsPort{items: []domain.KnowledgeHomeItem{{ItemKey: "article:1", Title: "Test"}}},
			&mockTodayDigestPort{digest: domain.TodayDigest{NewArticles: 5}},
			nil, &mockProjectionFreshnessPort{updatedAt: &recentTime}, nil,
		)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)
		require.NoError(t, err)
		assert.Equal(t, "full", result.ServiceQuality)
	})

	t.Run("degraded when digest fails but items succeed", func(t *testing.T) {
		uc := NewGetKnowledgeHomeUsecase(
			&mockHomeItemsPort{items: []domain.KnowledgeHomeItem{{ItemKey: "article:1", Title: "Test"}}},
			&mockTodayDigestPort{err: errors.New("digest db error")},
			nil, nil, nil,
		)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)
		require.NoError(t, err)
		assert.Equal(t, "degraded", result.ServiceQuality)
	})

	t.Run("fallback when items fail but digest succeeds", func(t *testing.T) {
		uc := NewGetKnowledgeHomeUsecase(
			&mockHomeItemsPort{err: errors.New("items db error")},
			&mockTodayDigestPort{digest: domain.TodayDigest{NewArticles: 5}},
			nil, nil, nil,
		)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)
		require.NoError(t, err)
		assert.Equal(t, "fallback", result.ServiceQuality)
	})

	t.Run("fallback when both items and digest fail", func(t *testing.T) {
		uc := NewGetKnowledgeHomeUsecase(
			&mockHomeItemsPort{err: errors.New("items db error")},
			&mockTodayDigestPort{err: errors.New("digest db error")},
			nil, nil, nil,
		)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("degraded when projection is severely stale", func(t *testing.T) {
		staleTime := now.Add(-30 * time.Minute)
		uc := NewGetKnowledgeHomeUsecase(
			&mockHomeItemsPort{items: []domain.KnowledgeHomeItem{{ItemKey: "article:1", Title: "Test"}}},
			&mockTodayDigestPort{digest: domain.TodayDigest{NewArticles: 5}},
			nil, &mockProjectionFreshnessPort{updatedAt: &staleTime}, nil,
		)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)
		require.NoError(t, err)
		assert.Equal(t, "degraded", result.ServiceQuality)
	})

	t.Run("ServiceQuality zero value treated as full by handler", func(t *testing.T) {
		// When ServiceQuality is not set explicitly (legacy path), it should default
		result := &Result{GeneratedAt: time.Now()}
		assert.Equal(t, "", result.ServiceQuality, "zero value is empty string")
	})
}

// mockTagHotspotPort implements knowledge_home_port.TagHotspotPort.
type mockTagHotspotPort struct {
	tags []knowledge_home_port.TrendingTag
	err  error
}

func (m *mockTagHotspotPort) GetTrendingTags(_ context.Context, _ uuid.UUID) ([]knowledge_home_port.TrendingTag, error) {
	return m.tags, m.err
}

func TestGetKnowledgeHomeUsecase_TagHotspotEnrichment(t *testing.T) {
	logger.InitLogger()
	userID := uuid.New()
	now := time.Now()

	t.Run("adds tag_hotspot only to items with trending tags", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{
			items: []domain.KnowledgeHomeItem{
				{ItemKey: "article:1", Title: "AI Article", Tags: []string{"AI", "ML"}, WhyReasons: []domain.WhyReason{{Code: domain.WhyNewUnread}}},
				{ItemKey: "article:2", Title: "Go Article", Tags: []string{"Go", "Backend"}, WhyReasons: []domain.WhyReason{{Code: domain.WhyNewUnread}}},
				{ItemKey: "article:3", Title: "No Tags", Tags: nil, WhyReasons: []domain.WhyReason{{Code: domain.WhyNewUnread}}},
			},
		}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{}}
		hotspotPort := &mockTagHotspotPort{
			tags: []knowledge_home_port.TrendingTag{
				{TagName: "AI", RecentCount: 10, SurgeRatio: 3.0},
			},
		}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, nil, nil)
		uc.SetTagHotspotPort(hotspotPort)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		require.Len(t, result.Items, 3)

		// AI Article should have tag_hotspot
		aiReasons := result.Items[0].WhyReasons
		hasHotspot := false
		for _, r := range aiReasons {
			if r.Code == domain.WhyTagHotspot {
				hasHotspot = true
				assert.Equal(t, "AI", r.Tag)
			}
		}
		assert.True(t, hasHotspot, "AI article should have tag_hotspot")

		// Go Article should NOT have tag_hotspot
		for _, r := range result.Items[1].WhyReasons {
			assert.NotEqual(t, domain.WhyTagHotspot, r.Code, "Go article should not have tag_hotspot")
		}

		// No Tags article should NOT have tag_hotspot
		for _, r := range result.Items[2].WhyReasons {
			assert.NotEqual(t, domain.WhyTagHotspot, r.Code, "tagless article should not have tag_hotspot")
		}
	})

	t.Run("nil hotspot port skips enrichment gracefully", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{
			items: []domain.KnowledgeHomeItem{
				{ItemKey: "article:1", Tags: []string{"AI"}, WhyReasons: []domain.WhyReason{{Code: domain.WhyNewUnread}}},
			},
		}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{}}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, nil, nil)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		assert.Len(t, result.Items[0].WhyReasons, 1, "should keep original reasons unchanged")
	})

	t.Run("hotspot port error does not fail request", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{
			items: []domain.KnowledgeHomeItem{
				{ItemKey: "article:1", Tags: []string{"AI"}, WhyReasons: []domain.WhyReason{{Code: domain.WhyNewUnread}}},
			},
		}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{}}
		hotspotPort := &mockTagHotspotPort{err: errors.New("cache miss")}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, nil, nil)
		uc.SetTagHotspotPort(hotspotPort)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
	})

	t.Run("does not duplicate tag_hotspot if already present", func(t *testing.T) {
		homeItems := &mockHomeItemsPort{
			items: []domain.KnowledgeHomeItem{
				{ItemKey: "article:1", Tags: []string{"AI"}, WhyReasons: []domain.WhyReason{
					{Code: domain.WhyNewUnread},
					{Code: domain.WhyTagHotspot, Tag: "AI"},
				}},
			},
		}
		digestPort := &mockTodayDigestPort{digest: domain.TodayDigest{}}
		hotspotPort := &mockTagHotspotPort{
			tags: []knowledge_home_port.TrendingTag{{TagName: "AI", RecentCount: 10, SurgeRatio: 3.0}},
		}

		uc := NewGetKnowledgeHomeUsecase(homeItems, digestPort, nil, nil, nil)
		uc.SetTagHotspotPort(hotspotPort)
		result, err := uc.Execute(context.Background(), userID, "", 20, now, nil)

		require.NoError(t, err)
		hotspotCount := 0
		for _, r := range result.Items[0].WhyReasons {
			if r.Code == domain.WhyTagHotspot {
				hotspotCount++
			}
		}
		assert.Equal(t, 1, hotspotCount, "should not duplicate tag_hotspot")
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
