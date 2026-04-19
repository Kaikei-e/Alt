package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"pre-processor/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo is a deterministic in-memory stand-in for ArticlesLanguageRepo used
// only in these tests. Keeping it inside _test.go avoids exporting a mock.
type fakeRepo struct {
	articles      []repository.ArticleForDetect
	updatesByRun  [][]repository.LanguageUpdate
	fetchErrAfter int // 0 = never; otherwise return err on the Nth fetch
	fetchCalls    int
	updateErr     error
	updateCalls   int
}

func (r *fakeRepo) FetchUndArticles(ctx context.Context, afterID string, limit int) ([]repository.ArticleForDetect, error) {
	r.fetchCalls++
	if r.fetchErrAfter > 0 && r.fetchCalls == r.fetchErrAfter {
		return nil, errors.New("fetch failed")
	}

	out := make([]repository.ArticleForDetect, 0, limit)
	for _, a := range r.articles {
		if a.ID <= afterID {
			continue
		}
		out = append(out, a)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateLanguageBulk(ctx context.Context, updates []repository.LanguageUpdate) (int, error) {
	r.updateCalls++
	if r.updateErr != nil {
		return 0, r.updateErr
	}
	if len(updates) == 0 {
		return 0, nil
	}
	// mimic real DB: updates applied in place on the fake dataset
	applied := make([]repository.LanguageUpdate, len(updates))
	copy(applied, updates)
	r.updatesByRun = append(r.updatesByRun, applied)
	return len(updates), nil
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLanguageBackfiller_DryRunDoesNotUpdate(t *testing.T) {
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "東京オリンピック 2028 開催地決定", Content: ""},
			{ID: "a2", Title: "OpenAI releases o3 in Q1 2026", Content: ""},
		},
	}
	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 10,
		Throttle:  0,
		DryRun:    true,
		Logger:    silentLogger(),
	})

	summary, err := b.Run(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, 2, summary.Scanned)
	assert.Equal(t, 2, summary.WouldUpdate)
	assert.Equal(t, 0, summary.Updated)
	assert.Equal(t, 0, repo.updateCalls, "dry-run must not call UpdateLanguageBulk")
	assert.Equal(t, 1, summary.ByLanguage["ja"])
	assert.Equal(t, 1, summary.ByLanguage["en"])
}

func TestLanguageBackfiller_LiveRunUpdatesAllBatches(t *testing.T) {
	// 3 articles with batch_size=2 forces two batches plus an empty-result probe.
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "東京オリンピック 2028 開催地決定", Content: ""},
			{ID: "a2", Title: "OpenAI releases o3 in Q1 2026", Content: ""},
			{ID: "a3", Title: "バッテリー技術の最前線", Content: ""},
		},
	}
	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 2,
		Throttle:  0,
		Logger:    silentLogger(),
	})

	summary, err := b.Run(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Scanned)
	assert.Equal(t, 3, summary.Updated)
	assert.Equal(t, 0, summary.WouldUpdate)
	assert.Equal(t, 2, repo.updateCalls)
	assert.Equal(t, 2, summary.ByLanguage["ja"])
	assert.Equal(t, 1, summary.ByLanguage["en"])
}

func TestLanguageBackfiller_ResumeFromID_SkipsProcessed(t *testing.T) {
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "東京オリンピック 2028 開催地決定", Content: ""},
			{ID: "a2", Title: "OpenAI releases o3 in Q1 2026", Content: ""},
			{ID: "a3", Title: "バッテリー技術の最前線", Content: ""},
		},
	}
	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 10,
		Throttle:  0,
		Logger:    silentLogger(),
	})

	summary, err := b.Run(context.Background(), "a1")
	require.NoError(t, err)
	assert.Equal(t, 2, summary.Scanned, "only a2, a3 fetched")
}

func TestLanguageBackfiller_SkipsUndResultFromDetector(t *testing.T) {
	// DetectLanguage returns "und" for whitespace / tiny inputs. Such rows must
	// not be updated — that would defeat the idempotency guard.
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "  ", Content: "  "},
			{ID: "a2", Title: "OpenAI releases o3 in Q1 2026", Content: ""},
		},
	}
	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 10,
		Throttle:  0,
		Logger:    silentLogger(),
	})

	summary, err := b.Run(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, 2, summary.Scanned)
	assert.Equal(t, 1, summary.Updated, "only a2 should be updated")
	assert.Equal(t, 1, summary.SkippedUnd)
	require.Len(t, repo.updatesByRun, 1)
	require.Len(t, repo.updatesByRun[0], 1)
	assert.Equal(t, "a2", repo.updatesByRun[0][0].ID)
}

func TestLanguageBackfiller_ThrottlesBetweenBatches(t *testing.T) {
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "東京 2028", Content: ""},
			{ID: "a2", Title: "OpenAI 2026", Content: ""},
			{ID: "a3", Title: "電気自動車市場", Content: ""},
		},
	}

	var sleeps []time.Duration
	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 1,
		Throttle:  250 * time.Millisecond,
		Logger:    silentLogger(),
		Sleep: func(d time.Duration) {
			sleeps = append(sleeps, d)
		},
	})

	_, err := b.Run(context.Background(), "")
	require.NoError(t, err)
	// 3 non-empty batches → 3 throttle pauses between batches
	// Implementation may also pause after the final empty probe; we only care
	// that at least N-1 pauses happened where N = number of non-empty batches.
	assert.GreaterOrEqual(t, len(sleeps), 2)
	assert.Equal(t, 250*time.Millisecond, sleeps[0])
}

func TestLanguageBackfiller_ContextCancellationStopsLoop(t *testing.T) {
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "東京 2028", Content: ""},
			{ID: "a2", Title: "OpenAI 2026", Content: ""},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before Run

	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 10,
		Throttle:  0,
		Logger:    silentLogger(),
	})

	_, err := b.Run(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLanguageBackfiller_PropagatesFetchError(t *testing.T) {
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "x", Content: "y"},
		},
		fetchErrAfter: 1,
	}
	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 10,
		Throttle:  0,
		Logger:    silentLogger(),
	})

	_, err := b.Run(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch failed")
}

func TestLanguageBackfiller_PropagatesUpdateError(t *testing.T) {
	repo := &fakeRepo{
		articles: []repository.ArticleForDetect{
			{ID: "a1", Title: "東京 2028", Content: ""},
		},
		updateErr: errors.New("update failed"),
	}
	b := NewLanguageBackfiller(repo, LanguageBackfillConfig{
		BatchSize: 10,
		Throttle:  0,
		Logger:    silentLogger(),
	})

	_, err := b.Run(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}
