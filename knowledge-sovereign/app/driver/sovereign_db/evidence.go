package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Knowledge Loop evidence accumulator (ADR-000939).
//
// This is the durable backing store for the co-projected evidence accumulator
// that replaced EventLogSurfaceScoreResolver's per-entry 7-day window re-scan.
// The knowledge-loop-projector is the ONLY writer; it applies one fact per
// relevant event in the same ordered pass it projects entries, then derives an
// entry's SurfaceScoreInputs from the accumulator state of the prefix. See
// docs/ADR/000939.md §2 for the co-projection contract.
//
// Reproject-safety: every write carries the event's occurred_at + event_seq;
// the seq guard makes replay a no-op and the table is TRUNCATEd and rebuilt on
// every full reproject. No wall-clock is read or written here.

// KnowledgeLoopEvidenceFact is one raw event-time fact in a signal's bounded
// ring. Windowed counts are derived from these at read time (filter by
// occurred_at), never precomputed into a column.
type KnowledgeLoopEvidenceFact struct {
	OccurredAt time.Time `json:"occurred_at"`
	EventSeq   int64     `json:"event_seq"`
	// V is an opaque per-signal value (e.g. an outcome label or a tag name)
	// the deriver may inspect. Empty when the fact is a bare count tick.
	V string `json:"v,omitempty"`
}

// KnowledgeLoopEvidenceWrite is one accumulator update the projector applies for
// an event. Exactly one of NewFact (fact-style signals) or a Pinned* value
// (pin-style signals) is set; the other stays zero. The driver routes both
// through a single merge-safe UPSERT.
type KnowledgeLoopEvidenceWrite struct {
	UserID     uuid.UUID
	TenantID   uuid.UUID
	ScopeKind  string
	ScopeRef   string
	SignalKind string

	// NewFact, when non-nil, is appended to the signal's ring (trimmed to the
	// newest 32 by event_seq).
	NewFact *KnowledgeLoopEvidenceFact

	// PinnedText / PinnedPayload, when non-empty, replace the signal's latest
	// pinned value (url_pin / article_pin → text, tag_set_current → payload).
	PinnedText    string
	PinnedPayload []byte

	// OccurredAt / EventSeq are the projecting event's event-time + seq. The
	// seq guard (last_event_seq < EXCLUDED.last_event_seq) drops replays and
	// out-of-order deliveries.
	OccurredAt time.Time
	EventSeq   int64
}

// KnowledgeLoopEvidenceState is the stored accumulator cell read back by the
// deriver: the full ring plus any pinned value.
type KnowledgeLoopEvidenceState struct {
	ScopeKind      string
	ScopeRef       string
	SignalKind     string
	Facts          []KnowledgeLoopEvidenceFact
	FactsTotal     int64
	PinnedText     string
	PinnedPayload  []byte
	LastOccurredAt time.Time
	LastEventSeq   int64
}

// KnowledgeLoopEvidenceScope names a (scope_kind, scope_ref) pair the deriver
// wants every signal for. GetKnowledgeLoopEvidenceForScopes batches these so a
// single round-trip fetches the entry, article, and any tag/topic_term cells an
// entry's relations need.
type KnowledgeLoopEvidenceScope struct {
	ScopeKind string
	ScopeRef  string
}

// evidenceRingCap bounds the per-signal fact ring. 32 saturates any relation
// magnitude the Orient surface renders and keeps the JSONB column small. Keep
// this in sync with the `LIMIT 32` in upsertKnowledgeLoopEvidenceSQL and the
// CHECK constraint in migration 00025.
const evidenceRingCap = 32

// upsertKnowledgeLoopEvidenceSQL is a single statement so the read-modify-write
// of the ring is atomic and needs no separate SELECT. The ON CONFLICT branch:
//   - appends EXCLUDED.facts to the existing ring, keeps the newest 32 by
//     event_seq, and re-aggregates ascending so storage stays deterministic
//     (pin-only updates pass '[]' so the ring is untouched);
//   - advances facts_total by the supplied delta ($7);
//   - COALESCEs the pins so a fact update never clears a pin and vice versa;
//   - is gated by `last_event_seq < EXCLUDED.last_event_seq` so a replay or an
//     out-of-order delivery is a no-op (no double-count, no ring churn).
const upsertKnowledgeLoopEvidenceSQL = `
INSERT INTO knowledge_loop_evidence
  (user_id, tenant_id, scope_kind, scope_ref, signal_kind,
   facts, facts_total, pinned_text, pinned_payload, last_occurred_at, last_event_seq)
VALUES
  ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9::jsonb, $10, $11)
ON CONFLICT (user_id, scope_kind, scope_ref, signal_kind) DO UPDATE SET
  facts = (
    SELECT COALESCE(jsonb_agg(e ORDER BY (e->>'event_seq')::bigint), '[]'::jsonb)
    FROM (
      SELECT e
      FROM jsonb_array_elements(knowledge_loop_evidence.facts || EXCLUDED.facts) AS e
      ORDER BY (e->>'event_seq')::bigint DESC
      LIMIT 32
    ) AS keep
  ),
  facts_total      = knowledge_loop_evidence.facts_total + EXCLUDED.facts_total,
  pinned_text      = COALESCE(EXCLUDED.pinned_text, knowledge_loop_evidence.pinned_text),
  pinned_payload   = COALESCE(EXCLUDED.pinned_payload, knowledge_loop_evidence.pinned_payload),
  last_occurred_at = GREATEST(knowledge_loop_evidence.last_occurred_at, EXCLUDED.last_occurred_at),
  last_event_seq   = EXCLUDED.last_event_seq
WHERE knowledge_loop_evidence.last_event_seq < EXCLUDED.last_event_seq
`

// UpsertKnowledgeLoopEvidence applies one accumulator update. Merge-safe and
// idempotent: the seq guard means replaying the same event leaves the row
// unchanged. Single writer (the projector) so there is no read-modify-write
// race despite the in-statement ring rebuild.
func (r *Repository) UpsertKnowledgeLoopEvidence(ctx context.Context, w KnowledgeLoopEvidenceWrite) error {
	if w.UserID == uuid.Nil {
		return fmt.Errorf("UpsertKnowledgeLoopEvidence: nil user_id")
	}
	if w.ScopeKind == "" || w.ScopeRef == "" || w.SignalKind == "" {
		return fmt.Errorf("UpsertKnowledgeLoopEvidence: empty scope/signal (%q/%q/%q)", w.ScopeKind, w.ScopeRef, w.SignalKind)
	}

	factsJSON := []byte("[]")
	factsDelta := 0
	if w.NewFact != nil {
		b, err := json.Marshal([]KnowledgeLoopEvidenceFact{*w.NewFact})
		if err != nil {
			return fmt.Errorf("UpsertKnowledgeLoopEvidence: marshal fact: %w", err)
		}
		factsJSON = b
		factsDelta = 1
	}

	var pinnedText *string
	if w.PinnedText != "" {
		s := w.PinnedText
		pinnedText = &s
	}
	var pinnedPayload any
	if len(w.PinnedPayload) > 0 {
		pinnedPayload = string(w.PinnedPayload)
	}

	occurredAt := w.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Unix(0, 0).UTC()
	}

	_, err := r.pool.Exec(ctx, upsertKnowledgeLoopEvidenceSQL,
		w.UserID, w.TenantID, w.ScopeKind, w.ScopeRef, w.SignalKind,
		string(factsJSON), factsDelta, pinnedText, pinnedPayload,
		occurredAt, w.EventSeq,
	)
	if err != nil {
		return fmt.Errorf("UpsertKnowledgeLoopEvidence exec: %w", err)
	}
	return nil
}

// GetKnowledgeLoopEvidenceForScopes returns every signal cell for the given
// (scope_kind, scope_ref) pairs under one user. user_id is bound physically
// (F-001) and the (scope_kind, scope_ref) pairs are matched as a tuple set so
// one round-trip covers the entry + article + the article's tag/topic_term
// fan-in the deriver needs.
func (r *Repository) GetKnowledgeLoopEvidenceForScopes(
	ctx context.Context,
	userID uuid.UUID,
	scopes []KnowledgeLoopEvidenceScope,
) ([]KnowledgeLoopEvidenceState, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("GetKnowledgeLoopEvidenceForScopes: nil user_id")
	}
	if len(scopes) == 0 {
		return nil, nil
	}

	kinds := make([]string, len(scopes))
	refs := make([]string, len(scopes))
	for i, s := range scopes {
		kinds[i] = s.ScopeKind
		refs[i] = s.ScopeRef
	}

	const q = `
SELECT scope_kind, scope_ref, signal_kind,
       facts, facts_total, pinned_text, pinned_payload,
       last_occurred_at, last_event_seq
FROM knowledge_loop_evidence
WHERE user_id = $1
  AND (scope_kind, scope_ref) IN (
    SELECT k, r FROM unnest($2::text[], $3::text[]) AS t(k, r)
  )
`
	rows, err := r.pool.Query(ctx, q, userID, kinds, refs)
	if err != nil {
		return nil, fmt.Errorf("GetKnowledgeLoopEvidenceForScopes query: %w", err)
	}
	defer rows.Close()

	out := make([]KnowledgeLoopEvidenceState, 0, len(scopes))
	for rows.Next() {
		var (
			st            KnowledgeLoopEvidenceState
			factsRaw      []byte
			pinnedText    *string
			pinnedPayload []byte
		)
		if err := rows.Scan(
			&st.ScopeKind, &st.ScopeRef, &st.SignalKind,
			&factsRaw, &st.FactsTotal, &pinnedText, &pinnedPayload,
			&st.LastOccurredAt, &st.LastEventSeq,
		); err != nil {
			return nil, fmt.Errorf("GetKnowledgeLoopEvidenceForScopes scan: %w", err)
		}
		if len(factsRaw) > 0 {
			if err := json.Unmarshal(factsRaw, &st.Facts); err != nil {
				// Fail loud (ADR-000939 / CLAUDE.md #8): a malformed ring is a
				// data-quality bug, not an empty signal. Returning it silently
				// as zero evidence would empty the Orient surface while looking
				// like "no fuel". Surface the corruption.
				return nil, fmt.Errorf("GetKnowledgeLoopEvidenceForScopes: malformed facts ring for %s/%s/%s: %w",
					st.ScopeKind, st.ScopeRef, st.SignalKind, err)
			}
		}
		if pinnedText != nil {
			st.PinnedText = *pinnedText
		}
		st.PinnedPayload = pinnedPayload
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetKnowledgeLoopEvidenceForScopes rows: %w", err)
	}
	return out, nil
}

// TruncateKnowledgeLoopEvidence empties the accumulator. Called only by the
// reproject path, in the same transaction that truncates knowledge_loop_entries
// — the accumulator is disposable and rebuilt from the event log (ADR-000939
// §2c).
func (r *Repository) TruncateKnowledgeLoopEvidence(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, "TRUNCATE TABLE knowledge_loop_evidence"); err != nil {
		return fmt.Errorf("TruncateKnowledgeLoopEvidence: %w", err)
	}
	return nil
}
