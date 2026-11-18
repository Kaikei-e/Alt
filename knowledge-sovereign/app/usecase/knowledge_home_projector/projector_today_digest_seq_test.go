package knowledge_home_projector

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// TestProjector_DigestWritesCarryTheirEventSeq is the producer half of
// today_digest_view's replay guard.
//
// sovereign_db.UpsertTodayDigest gates the additive counters on
// last_event_seq — the folding event's own knowledge_events.event_seq, the
// only discriminator that means "already folded" rather than "another
// producer's clock is ahead" — and refuses a payload that does not carry it.
// The three folds below are the only production writers of that payload, so a
// digestWrite without last_event_seq does not degrade today_digest_view, it
// stops it from being written at all, for every user, until a producer change
// lands. Consumer and producer therefore have to move in the same commit.
func TestProjector_DigestWritesCarryTheirEventSeq(t *testing.T) {
	articleID := uuid.New()

	tests := []struct {
		name      string
		eventType string
		seq       int64
		payload   map[string]any
	}{
		{
			name:      "ArticleCreated contributes the new/unsummarized deltas",
			eventType: "ArticleCreated",
			seq:       4711,
			payload: map[string]any{
				"article_id": articleID.String(),
				"title":      "Rust async runtimes compared",
				"url":        "https://example.com/rust-async",
			},
		},
		{
			name:      "SummaryVersionCreated moves an article from unsummarized to summarized",
			eventType: "SummaryVersionCreated",
			seq:       90210,
			payload: map[string]any{
				"article_id":   articleID.String(),
				"summary_text": "Tokio and async-std differ mainly in scheduler design.",
			},
		},
		{
			name:      "TagSetVersionCreated contributes top_tags",
			eventType: "TagSetVersionCreated",
			seq:       1337,
			payload: map[string]any{
				"article_id": articleID.String(),
				"tags":       []string{"rust", "async"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := uuid.New()
			user := userPtr()
			occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

			events := []sovereign_db.KnowledgeEvent{
				homeEvent(tt.seq, tt.eventType, articleID.String(), occurredAt, tenant, user, mustJSON(t, tt.payload)),
			}
			repo := newFakeRepo(events)
			// Start the checkpoint right below the event so the batch is
			// contiguous — the distinctive seq values above are there to
			// prove the fold forwards the event's own sequence rather than
			// some batch-local counter.
			repo.checkpoint = tt.seq - 1
			p := NewProjector(repo, nil, Config{})
			require.NoError(t, p.RunBatch(context.Background()))

			digest, ok := repo.digests[user.String()]
			require.True(t, ok, "%s must write today_digest_view", tt.eventType)
			require.NotNil(t, digest.LastEventSeq,
				"the marshalled digest payload must carry last_event_seq — the driver rejects it outright otherwise")
			assert.Equal(t, tt.seq, *digest.LastEventSeq,
				"last_event_seq must be the folding event's own event_seq, the same value the checkpoint advances on")
			assert.Equal(t, tt.seq, repo.checkpoint,
				"the checkpoint may only pass the event once its digest delta actually landed")
		})
	}
}
