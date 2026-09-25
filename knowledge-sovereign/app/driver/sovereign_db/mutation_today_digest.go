package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type todayDigestMutation struct {
	UserID                uuid.UUID
	DigestDate            string
	NewArticles           int
	SummarizedArticles    int
	UnsummarizedArticles  int
	TopTagsJSON           string
	PulseRefsJSON         string
	UpdatedAt             time.Time
	WeeklyRecapAvailable  bool
	EveningPulseAvailable bool
	LastEventSeq          int64
}

func parseTodayDigestMutation(payload json.RawMessage) (todayDigestMutation, error) {
	var digest struct {
		UserID               uuid.UUID `json:"user_id"`
		DigestDate           string    `json:"digest_date"`
		NewArticles          int       `json:"new_articles"`
		SummarizedArticles   int       `json:"summarized_articles"`
		UnsummarizedArticles int       `json:"unsummarized_articles"`
		TopTags              []string  `json:"top_tags"`
		PulseRefs            []string  `json:"pulse_refs"`
		UpdatedAt            time.Time `json:"updated_at"`
		// LastEventSeq is a pointer so a payload that omits the key
		// entirely (nil) can be told apart from one that sends an
		// explicit 0 — the two need different diagnostics, see below.
		LastEventSeq          *int64 `json:"last_event_seq"`
		WeeklyRecapAvailable  bool   `json:"weekly_recap_available"`
		EveningPulseAvailable bool   `json:"evening_pulse_available"`
	}
	if err := json.Unmarshal(payload, &digest); err != nil {
		return todayDigestMutation{}, fmt.Errorf("UpsertTodayDigest: unmarshal: %w", err)
	}

	// last_event_seq must be present and identify a real event. A payload
	// that omits the key comes from a producer built against the older
	// schema; silently falling back to the wall-clock guard would restore
	// the defect this column exists to close, with nothing anywhere
	// to say so (Alt Rule 8: no silent fallback for an unwired write path).
	// knowledge_events.event_seq is BIGSERIAL, hence always >= 1: a zero is
	// the Go/JSON zero value leaking through, and it would tie or lose
	// against every stored value and strand the row forever.
	if digest.LastEventSeq == nil {
		return todayDigestMutation{}, fmt.Errorf("UpsertTodayDigest: last_event_seq is required")
	}
	if *digest.LastEventSeq <= 0 {
		return todayDigestMutation{}, fmt.Errorf("UpsertTodayDigest: last_event_seq must be a positive event_seq, got %d", *digest.LastEventSeq)
	}

	topTags := digest.TopTags
	if topTags == nil {
		topTags = []string{}
	}
	topTagsJSON, err := json.Marshal(topTags)
	if err != nil {
		return todayDigestMutation{}, fmt.Errorf("UpsertTodayDigest: marshal top_tags: %w", err)
	}

	pulseRefs := digest.PulseRefs
	if pulseRefs == nil {
		pulseRefs = []string{}
	}
	pulseRefsJSON, err := json.Marshal(pulseRefs)
	if err != nil {
		return todayDigestMutation{}, fmt.Errorf("UpsertTodayDigest: marshal pulse_refs: %w", err)
	}

	return todayDigestMutation{
		UserID:                digest.UserID,
		DigestDate:            digest.DigestDate,
		NewArticles:           digest.NewArticles,
		SummarizedArticles:    digest.SummarizedArticles,
		UnsummarizedArticles:  digest.UnsummarizedArticles,
		TopTagsJSON:           string(topTagsJSON),
		PulseRefsJSON:         string(pulseRefsJSON),
		UpdatedAt:             digest.UpdatedAt,
		WeeklyRecapAvailable:  digest.WeeklyRecapAvailable,
		EveningPulseAvailable: digest.EveningPulseAvailable,
		LastEventSeq:          *digest.LastEventSeq,
	}, nil
}

func buildUpsertTodayDigestArgs(m todayDigestMutation) []any {
	return []any{
		m.UserID, m.DigestDate,
		m.NewArticles, m.SummarizedArticles,
		m.UnsummarizedArticles, m.TopTagsJSON, m.PulseRefsJSON,
		m.UpdatedAt,
		m.WeeklyRecapAvailable, m.EveningPulseAvailable,
		m.LastEventSeq,
	}
}

// $5 (the unsummarized_articles delta) is the one signed counter:
// SummaryVersionCreated contributes -1. The floor has to sit on the
// INSERT operand too, not only in the ON CONFLICT branch — a midnight
// batch summarizing yesterday's articles makes that -1 the first digest
// write of the new day, and the plain INSERT path would store it as-is
// (the column has no CHECK and GetTodayDigest returns it verbatim).
// The conflict branch then has to read the delta back from the bare
// parameter rather than EXCLUDED: EXCLUDED is the row *after* the VALUES
// expressions are evaluated, so it carries the floored 0 and the
// decrement of an existing row would silently become a no-op.
const upsertTodayDigestQuery = `INSERT INTO today_digest_view
		(user_id, digest_date, new_articles, summarized_articles,
		 unsummarized_articles, top_tags_json, pulse_refs_json, updated_at,
		 weekly_recap_available, evening_pulse_available, last_event_seq)
		VALUES ($1, $2, $3, $4, GREATEST(0, $5), $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id, digest_date) DO UPDATE SET
		 new_articles = today_digest_view.new_articles + EXCLUDED.new_articles,
		 summarized_articles = today_digest_view.summarized_articles + EXCLUDED.summarized_articles,
		 unsummarized_articles = GREATEST(0, today_digest_view.unsummarized_articles + $5),
		 top_tags_json = COALESCE(NULLIF(EXCLUDED.top_tags_json, '[]'::jsonb), today_digest_view.top_tags_json),
		 pulse_refs_json = COALESCE(NULLIF(EXCLUDED.pulse_refs_json, '[]'::jsonb), today_digest_view.pulse_refs_json),
		 updated_at = EXCLUDED.updated_at,
		 weekly_recap_available = EXCLUDED.weekly_recap_available OR today_digest_view.weekly_recap_available,
		 evening_pulse_available = EXCLUDED.evening_pulse_available OR today_digest_view.evening_pulse_available,
		 last_event_seq = EXCLUDED.last_event_seq
		WHERE EXCLUDED.last_event_seq > today_digest_view.last_event_seq`

// UpsertTodayDigest inserts or updates a today digest entry. The counters
// (new_articles / summarized_articles / unsummarized_articles) are additive
// deltas contributed by the projector on each source event.
//
// Guard: the fold's own knowledge_events.event_seq, carried as
// `last_event_seq`. The WHERE clause on the UPDATE only applies the delta
// when the incoming event sits strictly above the highest event_seq already
// folded into the row, so replaying an event the row has already seen is a
// no-op instead of a second addition.
//
// It deliberately is not guarded on updated_at. updated_at is the source
// event's OccurredAt, and OccurredAt is stamped by several producers with
// separate clocks, while the projector folds in event_seq order. When the
// two orders disagree for the same user and day, a wall-clock guard throws
// away the whole DO UPDATE of the older-looking event, counter deltas included,
// and TodayBar stays permanently short. event_seq is monotonic in exactly the
// order the fold runs, so it is the only discriminator that means "already folded"
// rather than "another machine's clock is ahead". Same role last_event_seq
// plays in knowledge_projection_checkpoints.
func (r *Repository) UpsertTodayDigest(ctx context.Context, payload json.RawMessage) error {
	m, err := parseTodayDigestMutation(payload)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, upsertTodayDigestQuery, buildUpsertTodayDigestArgs(m)...)
	if err != nil {
		return fmt.Errorf("UpsertTodayDigest: %w", err)
	}
	return nil
}
