package reading_queue_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

type fakeQueuePort struct {
	entries []domain.ReadingQueueEntry
	gotAsOf time.Time
	err     error
}

func (f *fakeQueuePort) GetReadingQueue(_ context.Context, _ uuid.UUID, asOf time.Time) ([]domain.ReadingQueueEntry, error) {
	f.gotAsOf = asOf
	return f.entries, f.err
}

type fakeCountPort struct {
	count    int
	gotSince time.Time
	err      error
}

func (f *fakeCountPort) CountReadingSourcesSince(_ context.Context, _ uuid.UUID, since time.Time) (int, error) {
	f.gotSince = since
	return f.count, f.err
}

type fakeAppendPort struct {
	events []domain.KnowledgeEvent
	seq    int64
	err    error
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, ke domain.KnowledgeEvent) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.events = append(f.events, ke)
	if f.seq != 0 || len(f.events) == 1 && f.seq == 0 && ke.EventType != "" {
		return f.seq, nil
	}
	return int64(len(f.events)), nil
}

type fakeArticleBatchPort struct {
	articles []*domain.Article
	err      error
}

func (f *fakeArticleBatchPort) FetchArticlesByIDs(_ context.Context, _ []uuid.UUID) ([]*domain.Article, error) {
	return f.articles, f.err
}

type fakeOgImagePort struct {
	urls map[string]string
	err  error
}

func (f *fakeOgImagePort) GetOgImageURLsByArticleIDs(_ context.Context, _ []string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.urls, nil
}

type fakeArticleLookupPort struct {
	sources map[string]domain.ArticleSource
	err     error
}

func (f *fakeArticleLookupPort) LookupArticleSource(_ context.Context, articleID string, _ uuid.UUID) (domain.ArticleSource, error) {
	if f.err != nil {
		return domain.ArticleSource{}, f.err
	}
	if f.sources == nil {
		return domain.ArticleSource{}, nil
	}
	return f.sources[articleID], nil
}

func TestNewQueueArticleUsecase_PanicsOnNilDeps(t *testing.T) {
	fixedNow := func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) }
	require.Panics(t, func() {
		NewQueueArticleUsecase(nil, &fakeAppendPort{}, &fakeArticleLookupPort{}, &fakeArticleBatchPort{}, &fakeOgImagePort{}, 7, 48, fixedNow)
	})
	require.Panics(t, func() {
		NewQueueArticleUsecase(&fakeQueuePort{}, nil, &fakeArticleLookupPort{}, &fakeArticleBatchPort{}, &fakeOgImagePort{}, 7, 48, fixedNow)
	})
	require.Panics(t, func() {
		NewQueueArticleUsecase(&fakeQueuePort{}, &fakeAppendPort{}, nil, &fakeArticleBatchPort{}, &fakeOgImagePort{}, 7, 48, fixedNow)
	})
	require.Panics(t, func() {
		NewQueueArticleUsecase(&fakeQueuePort{}, &fakeAppendPort{}, &fakeArticleLookupPort{}, nil, &fakeOgImagePort{}, 7, 48, fixedNow)
	})
	require.Panics(t, func() {
		NewQueueArticleUsecase(&fakeQueuePort{}, &fakeAppendPort{}, &fakeArticleLookupPort{}, &fakeArticleBatchPort{}, nil, 7, 48, fixedNow)
	})
}

func TestQueueArticleUsecase_OutcomesAndPayload(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	fixedNow := func() time.Time { return now }
	userID := uuid.New()
	tenantID := uuid.New()
	art1 := uuid.New()
	art2 := uuid.New()

	t.Run("queues new article with fixed expires_at and policy metadata", func(t *testing.T) {
		qp := &fakeQueuePort{}
		ap := &fakeAppendPort{seq: 1}
		lk := &fakeArticleLookupPort{
			sources: map[string]domain.ArticleSource{
				art1.String(): {Title: "First Article", URL: "https://www.example.com/posts/1"},
			},
		}
		ab := &fakeArticleBatchPort{
			articles: []*domain.Article{{
				ID:    art1,
				Title: "First Article",
				URL:   "https://www.example.com/posts/1",
			}},
		}
		og := &fakeOgImagePort{
			urls: map[string]string{art1.String(): "https://cdn.example.com/1.png"},
		}
		uc := NewQueueArticleUsecase(qp, ap, lk, ab, og, 7, 48, fixedNow)

		clientEventID := "01960000-0000-7000-8000-000000000001"
		res, err := uc.Execute(context.Background(), userID, tenantID, art1.String(), clientEventID, now, domain.ReadingSurfaceSwipe)
		require.NoError(t, err)

		assert.Equal(t, domain.QueueArticleOutcomeQueued, res.Outcome)
		assert.Equal(t, 7, res.Cap)
		require.Len(t, res.Items, 1)
		assert.Equal(t, art1, res.Items[0].ArticleID)
		assert.Equal(t, "First Article", res.Items[0].Title)
		assert.Equal(t, "https://www.example.com/posts/1", res.Items[0].URL)
		assert.Equal(t, "example.com", res.Items[0].SourceName)
		assert.Equal(t, "https://cdn.example.com/1.png", res.Items[0].ImageURL)
		assert.Equal(t, now, res.Items[0].QueuedAt)
		assert.Equal(t, now.Add(48*time.Hour), res.Items[0].ExpiresAt)

		require.Len(t, ap.events, 1)
		ev := ap.events[0]
		assert.Equal(t, domain.EventReadingQueued, ev.EventType)
		assert.Equal(t, domain.EventReadingQueued+":"+clientEventID, ev.DedupeKey)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(ev.Payload, &payload))
		assert.Equal(t, art1.String(), payload["article_id"])
		assert.Equal(t, "swipe", payload["surface"])
		assert.Equal(t, "2026-11-18T12:00:00Z", payload["queued_at"])
		assert.Equal(t, "2026-11-20T12:00:00Z", payload["expires_at"])
		assert.Equal(t, float64(7), payload["cap"])
		assert.Equal(t, float64(48), payload["ttl_hours"])
		assert.Equal(t, float64(1), payload["queue_policy_version"])
	})

	t.Run("returns ALREADY_QUEUED without appending when already active", func(t *testing.T) {
		qp := &fakeQueuePort{
			entries: []domain.ReadingQueueEntry{{
				ArticleID:          art1,
				QueuedAt:           now.Add(-time.Hour),
				ExpiresAt:          now.Add(47 * time.Hour),
				QueuePolicyVersion: 1,
			}},
		}
		ap := &fakeAppendPort{seq: 1}
		lk := &fakeArticleLookupPort{
			sources: map[string]domain.ArticleSource{
				art1.String(): {Title: "A", URL: "https://a.test/1"},
			},
		}
		ab := &fakeArticleBatchPort{articles: []*domain.Article{{ID: art1, Title: "A", URL: "https://a.test/1"}}}
		og := &fakeOgImagePort{}
		uc := NewQueueArticleUsecase(qp, ap, lk, ab, og, 7, 48, fixedNow)

		res, err := uc.Execute(context.Background(), userID, tenantID, art1.String(), "01960000-0000-7000-8000-000000000002", now, domain.ReadingSurfaceSwipe)
		require.NoError(t, err)
		assert.Equal(t, domain.QueueArticleOutcomeAlreadyQueued, res.Outcome)
		assert.Empty(t, ap.events)
		require.Len(t, res.Items, 1)
	})

	t.Run("returns QUEUE_FULL without evicting or appending when at cap", func(t *testing.T) {
		qp := &fakeQueuePort{
			entries: []domain.ReadingQueueEntry{{
				ArticleID:          art1,
				QueuedAt:           now.Add(-time.Hour),
				ExpiresAt:          now.Add(47 * time.Hour),
				QueuePolicyVersion: 1,
			}},
		}
		ap := &fakeAppendPort{seq: 1}
		lk := &fakeArticleLookupPort{
			sources: map[string]domain.ArticleSource{
				art2.String(): {Title: "Second", URL: "https://a.test/2"},
			},
		}
		ab := &fakeArticleBatchPort{articles: []*domain.Article{{ID: art1, Title: "Existing", URL: "https://a.test/1"}}}
		og := &fakeOgImagePort{}
		uc := NewQueueArticleUsecase(qp, ap, lk, ab, og, 1, 48, fixedNow)

		res, err := uc.Execute(context.Background(), userID, tenantID, art2.String(), "01960000-0000-7000-8000-000000000003", now, domain.ReadingSurfaceSwipe)
		require.NoError(t, err)
		assert.Equal(t, domain.QueueArticleOutcomeQueueFull, res.Outcome)
		assert.Equal(t, 1, res.Cap)
		assert.Empty(t, ap.events)
		require.Len(t, res.Items, 1)
		assert.Equal(t, art1, res.Items[0].ArticleID)
	})

	t.Run("rejects unknown or foreign article without appending or reading queue", func(t *testing.T) {
		qp := &fakeQueuePort{err: errors.New("should not be called")}
		ap := &fakeAppendPort{seq: 1}
		lk := &fakeArticleLookupPort{sources: map[string]domain.ArticleSource{}}
		ab := &fakeArticleBatchPort{}
		og := &fakeOgImagePort{}
		uc := NewQueueArticleUsecase(qp, ap, lk, ab, og, 7, 48, fixedNow)

		_, err := uc.Execute(context.Background(), userID, tenantID, art2.String(), "01960000-0000-7000-8000-000000000004", now, domain.ReadingSurfaceSwipe)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidInput)
		assert.Empty(t, ap.events)
	})

	t.Run("rejects ErrArticleNotFound with ErrInvalidInput without appending", func(t *testing.T) {
		qp := &fakeQueuePort{}
		ap := &fakeAppendPort{seq: 1}
		lk := &fakeArticleLookupPort{err: domain.ErrArticleNotFound}
		ab := &fakeArticleBatchPort{}
		og := &fakeOgImagePort{}
		uc := NewQueueArticleUsecase(qp, ap, lk, ab, og, 7, 48, fixedNow)

		_, err := uc.Execute(context.Background(), userID, tenantID, art2.String(), "01960000-0000-7000-8000-000000000005", now, domain.ReadingSurfaceSwipe)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidInput)
		assert.Empty(t, ap.events)
	})

	t.Run("propagates transient lookup error without ErrInvalidInput and without appending", func(t *testing.T) {
		qp := &fakeQueuePort{}
		ap := &fakeAppendPort{seq: 1}
		lookupErr := errors.New("datahub unavailable")
		lk := &fakeArticleLookupPort{err: lookupErr}
		ab := &fakeArticleBatchPort{}
		og := &fakeOgImagePort{}
		uc := NewQueueArticleUsecase(qp, ap, lk, ab, og, 7, 48, fixedNow)

		_, err := uc.Execute(context.Background(), userID, tenantID, art2.String(), "01960000-0000-7000-8000-000000000006", now, domain.ReadingSurfaceSwipe)
		require.Error(t, err)
		assert.ErrorIs(t, err, lookupErr)
		assert.NotErrorIs(t, err, ErrInvalidInput)
		assert.Empty(t, ap.events)
	})
}

func TestUnqueueArticleUsecase_AppendsEvent(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	ap := &fakeAppendPort{seq: 1}
	uc := NewUnqueueArticleUsecase(ap, func() time.Time { return now })

	artID := uuid.New()
	clientEventID := "01960000-0000-7000-8000-000000000004"
	err := uc.Execute(context.Background(), uuid.New(), uuid.New(), artID.String(), clientEventID, now)
	require.NoError(t, err)
	require.Len(t, ap.events, 1)
	assert.Equal(t, domain.EventReadingUnqueued, ap.events[0].EventType)
	assert.Equal(t, domain.EventReadingUnqueued+":"+clientEventID, ap.events[0].DedupeKey)
}

func TestGetReadTodayQueueUsecase_ReturnsHydratedQueueAndCount(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	since := time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)
	artID := uuid.New()

	qp := &fakeQueuePort{
		entries: []domain.ReadingQueueEntry{{
			ArticleID:          artID,
			QueuedAt:           now.Add(-2 * time.Hour),
			ExpiresAt:          now.Add(46 * time.Hour),
			QueuePolicyVersion: 1,
		}},
	}
	cp := &fakeCountPort{count: 3}
	ab := &fakeArticleBatchPort{
		articles: []*domain.Article{{ID: artID, Title: "Read Later", URL: "https://blog.example.org/post"}},
	}
	og := &fakeOgImagePort{err: errors.New("og lookup timeout")}

	uc := NewGetReadTodayQueueUsecase(qp, cp, ab, og, 7, func() time.Time { return now })
	res, err := uc.Execute(context.Background(), uuid.New(), since)
	require.NoError(t, err)
	assert.Equal(t, now, qp.gotAsOf)
	assert.Equal(t, since, cp.gotSince)
	assert.Equal(t, 7, res.Cap)
	assert.Equal(t, int32(3), res.ReadCountSince)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "Read Later", res.Items[0].Title)
	assert.Equal(t, "blog.example.org", res.Items[0].SourceName)
	assert.Empty(t, res.Items[0].ImageURL)
}
