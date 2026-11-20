package feed_inflow_stats_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubFeedInflowPort struct {
	inflows           []domain.FeedInflow
	articleToFeedLink map[uuid.UUID]uuid.UUID
	inflowErr         error
	batchErr          error
	gotFrom, gotTo    time.Time
	batchCalls        int
}

func (s *stubFeedInflowPort) GetFeedInflowByUser(_ context.Context, _ uuid.UUID, from, to time.Time) ([]domain.FeedInflow, error) {
	s.gotFrom, s.gotTo = from, to
	return s.inflows, s.inflowErr
}

func (s *stubFeedInflowPort) BatchGetFeedLinkIDsByArticleIDs(_ context.Context, _ uuid.UUID, _ []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	s.batchCalls++
	return s.articleToFeedLink, s.batchErr
}

type stubReadingSourcesPort struct {
	entries        []domain.ReadingSourceEntry
	err            error
	gotFrom, gotTo time.Time
}

func (s *stubReadingSourcesPort) ListReadingSourcesInWindow(_ context.Context, _ uuid.UUID, from, to time.Time) ([]domain.ReadingSourceEntry, error) {
	s.gotFrom, s.gotTo = from, to
	return s.entries, s.err
}

func userCtx(userID uuid.UUID) context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		TenantID:  userID,
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestFeedInflowStatsUsecase_Execute(t *testing.T) {
	userID := uuid.New()
	linkA := uuid.New()
	linkB := uuid.New()
	artA1, artA2, artALegacy := uuid.New(), uuid.New(), uuid.New()
	windowEnd := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	expectedStart := windowEnd.Add(-30 * 24 * time.Hour)
	capA := 5

	t.Run("aggregates reads separately from legacy opens and handles zero inflow", func(t *testing.T) {
		inflowPort := &stubFeedInflowPort{
			inflows: []domain.FeedInflow{
				{FeedLinkID: linkA, FeedURL: "https://a.example.com/rss", DailyCap: &capA, InflowCount: 10},
				{FeedLinkID: linkB, FeedURL: "https://b.example.com/rss", InflowCount: 0},
			},
			articleToFeedLink: map[uuid.UUID]uuid.UUID{
				artA1:      linkA,
				artA2:      linkA,
				artALegacy: linkA,
			},
		}
		readingPort := &stubReadingSourcesPort{
			entries: []domain.ReadingSourceEntry{
				{ArticleID: artA1, ReadRuleVersion: 1},
				{ArticleID: artA1, ReadRuleVersion: 1}, // duplicate ignored
				{ArticleID: artA2, ReadRuleVersion: 2},
				{ArticleID: artALegacy, ReadRuleVersion: 0}, // legacy opened only
			},
		}

		uc := NewFeedInflowStatsUsecase(inflowPort, readingPort)
		stats, err := uc.Execute(userCtx(userID), windowEnd)
		require.NoError(t, err)
		require.Len(t, stats, 2)

		assert.Equal(t, expectedStart, inflowPort.gotFrom)
		assert.Equal(t, windowEnd, inflowPort.gotTo)
		assert.Equal(t, expectedStart, readingPort.gotFrom)
		assert.Equal(t, windowEnd, readingPort.gotTo)

		assert.Equal(t, linkA.String(), stats[0].FeedLinkID)
		assert.Equal(t, int64(10), stats[0].InflowCount)
		assert.Equal(t, int64(2), stats[0].ReadCount)
		assert.Equal(t, int64(1), stats[0].OpenedCountLegacy)
		assert.InDelta(t, 0.2, stats[0].ReadRate, 1e-9)
		require.NotNil(t, stats[0].DailyCap)
		assert.Equal(t, 5, *stats[0].DailyCap)

		assert.Equal(t, linkB.String(), stats[1].FeedLinkID)
		assert.Equal(t, int64(0), stats[1].InflowCount)
		assert.Equal(t, int64(0), stats[1].ReadCount)
		assert.Equal(t, int64(0), stats[1].OpenedCountLegacy)
		assert.InDelta(t, 0.0, stats[1].ReadRate, 1e-9)
	})

	t.Run("skips article batch resolution when no reading sources exist", func(t *testing.T) {
		inflowPort := &stubFeedInflowPort{
			inflows: []domain.FeedInflow{{FeedLinkID: linkA, FeedURL: "https://a.example.com/rss", InflowCount: 4}},
		}
		readingPort := &stubReadingSourcesPort{}

		uc := NewFeedInflowStatsUsecase(inflowPort, readingPort)
		stats, err := uc.Execute(userCtx(userID), windowEnd)
		require.NoError(t, err)
		require.Len(t, stats, 1)
		assert.Equal(t, 0, inflowPort.batchCalls)
		assert.InDelta(t, 0.0, stats[0].ReadRate, 1e-9)
	})

	t.Run("propagates sovereign error without silent fallback", func(t *testing.T) {
		wantErr := errors.New("sovereign down")
		inflowPort := &stubFeedInflowPort{}
		readingPort := &stubReadingSourcesPort{err: wantErr}

		uc := NewFeedInflowStatsUsecase(inflowPort, readingPort)
		_, err := uc.Execute(userCtx(userID), windowEnd)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("version-0 rows never increment read_count or read_rate", func(t *testing.T) {
		artL1, artL2 := uuid.New(), uuid.New()
		inflowPort := &stubFeedInflowPort{
			inflows: []domain.FeedInflow{
				{FeedLinkID: linkA, FeedURL: "https://a.example.com/rss", InflowCount: 8},
			},
			articleToFeedLink: map[uuid.UUID]uuid.UUID{
				artL1: linkA,
				artL2: linkA,
			},
		}
		readingPort := &stubReadingSourcesPort{
			entries: []domain.ReadingSourceEntry{
				{ArticleID: artL1, ReadRuleVersion: 0},
				{ArticleID: artL2, ReadRuleVersion: 0},
			},
		}

		uc := NewFeedInflowStatsUsecase(inflowPort, readingPort)
		stats, err := uc.Execute(userCtx(userID), windowEnd)
		require.NoError(t, err)
		require.Len(t, stats, 1)
		assert.Equal(t, int64(0), stats[0].ReadCount)
		assert.Equal(t, int64(2), stats[0].OpenedCountLegacy)
		assert.InDelta(t, 0.0, stats[0].ReadRate, 1e-9)
	})

	t.Run("panics when required ports are nil", func(t *testing.T) {
		assert.Panics(t, func() { NewFeedInflowStatsUsecase(nil, &stubReadingSourcesPort{}) })
		assert.Panics(t, func() { NewFeedInflowStatsUsecase(&stubFeedInflowPort{}, nil) })
	})
}
