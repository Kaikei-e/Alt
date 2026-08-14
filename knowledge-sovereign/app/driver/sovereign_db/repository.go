package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface is the subset of the pgx pool the Repository depends on. It lets unit
// tests substitute a fake pool (see partition_test.go) without a live database.
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	// Begin starts a transaction. Required by any repository method that
	// must make more than one INSERT/UPDATE atomic (e.g., dedupe-key +
	// event append, or deactivate + activate a projection version) so a
	// mid-sequence crash can never leave the two statements half-applied.
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ PgxIface = (*pgxpool.Pool)(nil)

// Repository provides database operations for Knowledge Sovereign.
type Repository struct {
	pool PgxIface
}

// NewRepository creates a new sovereign DB repository.
func NewRepository(pool PgxIface) *Repository {
	return &Repository{pool: pool}
}

// ErrDismissTargetNotFound is returned when the dismiss target does not exist.
var ErrDismissTargetNotFound = fmt.Errorf("dismiss target not found")

// score_op values recognized by UpsertKnowledgeHomeItem's merge-safe UPSERT
// (see the score CASE below). Mirrored by knowledge_home_projector's
// scoreOpMax/scoreOpSet — duplicated rather than imported because driver/
// must not depend on usecase/ (Clean Architecture layer direction).
const (
	scoreOpMax = "max"
	scoreOpSet = "set"
)

// UpsertKnowledgeHomeItem inserts or updates a knowledge home item.
func (r *Repository) UpsertKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error {
	var item struct {
		UserID         uuid.UUID  `json:"user_id"`
		TenantID       uuid.UUID  `json:"tenant_id"`
		ItemKey        string     `json:"item_key"`
		ItemType       string     `json:"item_type"`
		PrimaryRefID   *uuid.UUID `json:"primary_ref_id"`
		Title          string     `json:"title"`
		SummaryExcerpt string     `json:"summary_excerpt"`
		Tags           []string   `json:"tags"`
		WhyReasons     []struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		} `json:"why_reasons"`
		Score float64 `json:"score"`
		// ScoreOp is a pointer so a payload that omits the key entirely
		// (nil) can be told apart from one that sets it to the empty
		// string (a deliberate "leave score untouched", used by folds that
		// never affect score — see the validation below).
		ScoreOp           *string    `json:"score_op"`
		FreshnessAt       *time.Time `json:"freshness_at"`
		PublishedAt       *time.Time `json:"published_at"`
		LastInteractedAt  *time.Time `json:"last_interacted_at"`
		GeneratedAt       time.Time  `json:"generated_at"`
		UpdatedAt         time.Time  `json:"updated_at"`
		DismissedAt       *time.Time `json:"dismissed_at"`
		ProjectionVersion int        `json:"projection_version"`
		SummaryState      string     `json:"summary_state"`
		SupersedeState    string     `json:"supersede_state"`
		SupersededAt      *time.Time `json:"superseded_at"`
		PreviousRefJSON   string     `json:"previous_ref_json"`
		URL               string     `json:"url"`
	}
	if err := json.Unmarshal(payload, &item); err != nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: unmarshal: %w", err)
	}

	// score_op must be present and recognized. A payload that omits the key
	// entirely comes from a caller unaware of the field (e.g. a producer
	// built against an older schema) and cannot be told apart from one that
	// deliberately chose "leave score untouched" — silently falling back to
	// the latter would drop that caller's score writes forever with no
	// error anywhere (Alt Rule 8: no silent fallback for an unwired write
	// path). An unrecognized non-empty value is rejected the same way
	// rather than falling through to "untouched", so a typo doesn't
	// silently become a permanent no-op either.
	if item.ScoreOp == nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: score_op is required")
	}
	scoreOp := *item.ScoreOp
	switch scoreOp {
	case "", scoreOpMax, scoreOpSet:
	default:
		return fmt.Errorf("UpsertKnowledgeHomeItem: unrecognized score_op %q (want \"\", %q, or %q)",
			scoreOp, scoreOpMax, scoreOpSet)
	}

	tags := item.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: marshal tags: %w", err)
	}

	whyReasons := item.WhyReasons
	if whyReasons == nil {
		whyReasons = []struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		}{}
	}
	whyJSON, err := json.Marshal(whyReasons)
	if err != nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: marshal why: %w", err)
	}

	var supersedeState *string
	if item.SupersedeState != "" {
		supersedeState = &item.SupersedeState
	}
	var previousRefJSON *string
	if item.PreviousRefJSON != "" {
		previousRefJSON = &item.PreviousRefJSON
	}

	query := `INSERT INTO knowledge_home_items
		(user_id, tenant_id, item_key, item_type, primary_ref_id,
		 title, summary_excerpt, tags_json, why_json, score,
		 freshness_at, published_at, last_interacted_at, generated_at, updated_at, dismissed_at,
		 projection_version, summary_state,
		 supersede_state, superseded_at, previous_ref_json, url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (user_id, item_key, projection_version) DO UPDATE SET
		 -- $23 (score_op) is control metadata for the merge below, not a
		 -- business fact — it has no knowledge_home_items column and is
		 -- bound as a bare parameter rather than through EXCLUDED.
		 -- Merge-safe upsert (memory feedback_merge_safe_upsert.md +
		 -- .claude/rules/knowledge-home.md): "preserve previous on
		 -- empty new" is encoded with COALESCE/NULLIF rather than
		 -- SQL CASE so business logic stays in Go.
		 title = COALESCE(NULLIF(EXCLUDED.title, ''), knowledge_home_items.title),
		 summary_excerpt = COALESCE(NULLIF(EXCLUDED.summary_excerpt, ''), knowledge_home_items.summary_excerpt),
		 tags_json = COALESCE(NULLIF(EXCLUDED.tags_json, '[]'::jsonb), knowledge_home_items.tags_json),
		 why_json = CASE
			 WHEN EXCLUDED.why_json = '[]'::jsonb THEN knowledge_home_items.why_json
			 ELSE (
				 SELECT COALESCE(jsonb_agg(merged.reason ORDER BY merged.code), '[]'::jsonb)
				 FROM (
					 SELECT DISTINCT ON (candidate.code) candidate.code, candidate.reason
					 FROM (
						 SELECT reason->>'code' AS code, reason, 0 AS source_rank
						 FROM jsonb_array_elements(
						 	CASE
						 		WHEN jsonb_typeof(EXCLUDED.why_json) = 'array' THEN EXCLUDED.why_json
						 		ELSE '[]'::jsonb
						 	END
						 ) AS reason
						 UNION ALL
						 SELECT reason->>'code' AS code, reason, 1 AS source_rank
						 FROM jsonb_array_elements(
						 	CASE
						 		WHEN jsonb_typeof(COALESCE(knowledge_home_items.why_json, '[]'::jsonb)) = 'array' THEN COALESCE(knowledge_home_items.why_json, '[]'::jsonb)
						 		ELSE '[]'::jsonb
						 	END
						 ) AS reason
					 ) AS candidate
					 ORDER BY candidate.code, candidate.source_rank
				 ) AS merged
			 )
		 END,
		 -- Explicit per-write merge operator, not a blanket GREATEST: a
		 -- floor-only merge can never let a fold legitimately lower a score
		 -- (e.g. HomeItemOpened's suppressed 0.1 was unreachable once any
		 -- higher score had ever been written for the item). $23 carries
		 -- the fold's intent — 'set' overwrites unconditionally, 'max' keeps
		 -- the floor semantics the baseline/boost folds rely on, anything
		 -- else (including a fold that never touches score) leaves the
		 -- stored value untouched.
		 score = CASE
			 WHEN $23 = 'set' THEN EXCLUDED.score
			 WHEN $23 = 'max' THEN GREATEST(EXCLUDED.score, knowledge_home_items.score)
			 ELSE knowledge_home_items.score
		 END,
		 freshness_at = COALESCE(EXCLUDED.freshness_at, knowledge_home_items.freshness_at),
		 published_at = COALESCE(EXCLUDED.published_at, knowledge_home_items.published_at),
		 last_interacted_at = COALESCE(EXCLUDED.last_interacted_at, knowledge_home_items.last_interacted_at),
		 updated_at = EXCLUDED.updated_at,
		 dismissed_at = COALESCE(knowledge_home_items.dismissed_at, EXCLUDED.dismissed_at),
		 projection_version = EXCLUDED.projection_version,
		 -- summary_state monotonic latch via lexicographic ordering:
		 -- '' < missing < pending < ready (alphabetical). GREATEST preserves
		 -- the highest stage reached and forbids regression without
		 -- smuggling a CASE state machine into SQL. Same merge shape as
		 -- score below.
		 summary_state = GREATEST(knowledge_home_items.summary_state, EXCLUDED.summary_state),
		 supersede_state = COALESCE(EXCLUDED.supersede_state, knowledge_home_items.supersede_state),
		 superseded_at = COALESCE(EXCLUDED.superseded_at, knowledge_home_items.superseded_at),
		 previous_ref_json = CASE
			 WHEN EXCLUDED.previous_ref_json IS NOT NULL THEN COALESCE(knowledge_home_items.previous_ref_json, '{}'::jsonb) || EXCLUDED.previous_ref_json
			 ELSE knowledge_home_items.previous_ref_json
		 END,
		 url = COALESCE(NULLIF(EXCLUDED.url, ''), knowledge_home_items.url)`

	_, err = r.pool.Exec(ctx, query,
		item.UserID, item.TenantID, item.ItemKey, item.ItemType, item.PrimaryRefID,
		item.Title, item.SummaryExcerpt, string(tagsJSON), string(whyJSON), item.Score,
		item.FreshnessAt, item.PublishedAt, item.LastInteractedAt, item.GeneratedAt, item.UpdatedAt, item.DismissedAt,
		item.ProjectionVersion, item.SummaryState,
		supersedeState, item.SupersededAt, previousRefJSON, item.URL,
		scoreOp,
	)
	if err != nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: %w", err)
	}
	return nil
}

// DismissKnowledgeHomeItem marks an item as dismissed.
func (r *Repository) DismissKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error {
	var params struct {
		UserID            string `json:"user_id"`
		ItemKey           string `json:"item_key"`
		ProjectionVersion int    `json:"projection_version"`
		DismissedAt       string `json:"dismissed_at"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return fmt.Errorf("DismissKnowledgeHomeItem: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return fmt.Errorf("DismissKnowledgeHomeItem: parse user_id: %w", err)
	}
	// dismissed_at is a business fact and must come from the event payload —
	// reproject-safe means replaying the same DismissedHomeItem event twice
	// produces the identical row. Falling back to wall-clock time here would
	// make each replay non-deterministic (immutable-design-guard: Event-time
	// purity). Loudly reject rather than fabricate a value.
	if params.DismissedAt == "" {
		return fmt.Errorf("DismissKnowledgeHomeItem: dismissed_at is required")
	}
	dismissedAt, err := time.Parse(time.RFC3339Nano, params.DismissedAt)
	if err != nil {
		return fmt.Errorf("DismissKnowledgeHomeItem: parse dismissed_at: %w", err)
	}

	var commandTag pgconn.CommandTag
	if params.ProjectionVersion == 0 {
		// Curation path: version not specified → dismiss across all versions (idempotent).
		query := `UPDATE knowledge_home_items
			SET dismissed_at = $1, updated_at = $1
			WHERE user_id = $2 AND item_key = $3 AND dismissed_at IS NULL`
		commandTag, err = r.pool.Exec(ctx, query, dismissedAt, userID, params.ItemKey)
	} else {
		// Projector path: version specified → dismiss exact version.
		query := `UPDATE knowledge_home_items
			SET dismissed_at = $1, updated_at = $1
			WHERE user_id = $2 AND item_key = $3 AND projection_version = $4`
		commandTag, err = r.pool.Exec(ctx, query, dismissedAt, userID, params.ItemKey, params.ProjectionVersion)
	}
	if err != nil {
		return fmt.Errorf("DismissKnowledgeHomeItem: %w", err)
	}
	if params.ProjectionVersion != 0 && commandTag.RowsAffected() == 0 {
		return ErrDismissTargetNotFound
	}
	return nil
}

// ClearSupersedeState clears the supersede state for a specific item.
func (r *Repository) ClearSupersedeState(ctx context.Context, payload json.RawMessage) error {
	var params struct {
		UserID            string `json:"user_id"`
		ItemKey           string `json:"item_key"`
		ProjectionVersion int    `json:"projection_version"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return fmt.Errorf("ClearSupersedeState: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return fmt.Errorf("ClearSupersedeState: parse user_id: %w", err)
	}

	query := `UPDATE knowledge_home_items
		SET supersede_state = NULL, superseded_at = NULL, previous_ref_json = NULL
		WHERE user_id = $1 AND item_key = $2 AND projection_version = $3 AND supersede_state IS NOT NULL`
	_, err = r.pool.Exec(ctx, query, userID, params.ItemKey, params.ProjectionVersion)
	if err != nil {
		return fmt.Errorf("ClearSupersedeState: %w", err)
	}
	return nil
}

// UpsertTodayDigest inserts or updates a today digest entry. The counters
// (new_articles / summarized_articles / unsummarized_articles) are additive
// deltas contributed by the projector on each source event (see
// event-stream-consumer.md: "projection の UPSERT は絶対値上書き... 加算
// マージは禁止" and immutable-design-guard's Merge-safe upsert principle).
// A naive unconditional `col = col + delta` double-counts on an at-least-once
// resend or a full reprojection replay of the same event.
//
// Guard: the fold's own knowledge_events.event_seq, carried as
// `last_event_seq`. The WHERE clause on the UPDATE only applies the delta
// when the incoming event sits strictly above the highest event_seq already
// folded into the row, so replaying an event the row has already seen is a
// no-op instead of a second addition.
//
// It deliberately is *not* guarded on updated_at. updated_at is the source
// event's OccurredAt, and OccurredAt is stamped with time.Now() by whichever
// producer emitted the event before the append RPC — six-plus independent
// producers, six-plus independent clocks — while the projector folds in
// event_seq order. When the two orders disagree for the same user and day,
// a wall-clock guard throws away the whole DO UPDATE of the older-looking
// event, counter deltas included, and TodayBar stays permanently short.
// event_seq is monotonic in exactly the order the fold runs, so it is the
// only discriminator that means "already folded" rather than "another
// machine's clock is ahead". Same role last_event_seq plays in
// knowledge_projection_checkpoints.
func (r *Repository) UpsertTodayDigest(ctx context.Context, payload json.RawMessage) error {
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
		return fmt.Errorf("UpsertTodayDigest: unmarshal: %w", err)
	}

	// last_event_seq must be present and identify a real event. A payload
	// that omits the key comes from a producer built against the older
	// schema; silently falling back to the wall-clock guard would restore
	// the very defect this column exists to close, with nothing anywhere
	// to say so (Alt Rule 8: no silent fallback for an unwired write path).
	// knowledge_events.event_seq is BIGSERIAL, hence always >= 1: a zero is
	// the Go/JSON zero value leaking through, and it would tie or lose
	// against every stored value and strand the row forever.
	if digest.LastEventSeq == nil {
		return fmt.Errorf("UpsertTodayDigest: last_event_seq is required")
	}
	if *digest.LastEventSeq <= 0 {
		return fmt.Errorf("UpsertTodayDigest: last_event_seq must be a positive event_seq, got %d", *digest.LastEventSeq)
	}

	topTags := digest.TopTags
	if topTags == nil {
		topTags = []string{}
	}
	topTagsJSON, err := json.Marshal(topTags)
	if err != nil {
		return fmt.Errorf("UpsertTodayDigest: marshal top_tags: %w", err)
	}

	pulseRefs := digest.PulseRefs
	if pulseRefs == nil {
		pulseRefs = []string{}
	}
	pulseRefsJSON, err := json.Marshal(pulseRefs)
	if err != nil {
		return fmt.Errorf("UpsertTodayDigest: marshal pulse_refs: %w", err)
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
	query := `INSERT INTO today_digest_view
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

	_, err = r.pool.Exec(ctx, query,
		digest.UserID, digest.DigestDate,
		digest.NewArticles, digest.SummarizedArticles,
		digest.UnsummarizedArticles, string(topTagsJSON), string(pulseRefsJSON),
		digest.UpdatedAt,
		digest.WeeklyRecapAvailable, digest.EveningPulseAvailable,
		*digest.LastEventSeq,
	)
	if err != nil {
		return fmt.Errorf("UpsertTodayDigest: %w", err)
	}
	return nil
}

// UpsertRecallCandidate inserts or updates a recall candidate.
func (r *Repository) UpsertRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	var candidate struct {
		UserID            uuid.UUID      `json:"user_id"`
		ItemKey           string         `json:"item_key"`
		RecallScore       float64        `json:"recall_score"`
		Reasons           []RecallReason `json:"reasons"`
		NextSuggestAt     *time.Time     `json:"next_suggest_at"`
		FirstEligibleAt   *time.Time     `json:"first_eligible_at"`
		UpdatedAt         time.Time      `json:"updated_at"`
		ProjectionVersion int            `json:"projection_version"`
	}
	if err := json.Unmarshal(payload, &candidate); err != nil {
		return fmt.Errorf("UpsertRecallCandidate: unmarshal: %w", err)
	}

	reasonJSON, err := json.Marshal(candidate.Reasons)
	if err != nil {
		return fmt.Errorf("UpsertRecallCandidate: marshal reasons: %w", err)
	}

	query := `INSERT INTO recall_candidate_view
		(user_id, item_key, recall_score, reason_json, next_suggest_at, first_eligible_at, updated_at, projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		  recall_score = EXCLUDED.recall_score,
		  reason_json = EXCLUDED.reason_json,
		  next_suggest_at = EXCLUDED.next_suggest_at,
		  updated_at = EXCLUDED.updated_at,
		  projection_version = EXCLUDED.projection_version`

	_, err = r.pool.Exec(ctx, query,
		candidate.UserID, candidate.ItemKey, candidate.RecallScore, string(reasonJSON),
		candidate.NextSuggestAt, candidate.FirstEligibleAt, candidate.UpdatedAt, candidate.ProjectionVersion,
	)
	if err != nil {
		return fmt.Errorf("UpsertRecallCandidate: %w", err)
	}
	return nil
}

// SnoozeRecallCandidate snoozes a recall candidate until the given time.
// updated_at is written from the caller-supplied occurred_at rather than SQL
// now() — recall_candidate_view is a disposable projection (immutable-design-
// guard: Event-time purity), and now() would make ApplyRecallMutation's
// resend/replay non-deterministic.
func (r *Repository) SnoozeRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	var params struct {
		UserID     string `json:"user_id"`
		ItemKey    string `json:"item_key"`
		Until      string `json:"until"`
		OccurredAt string `json:"occurred_at"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: parse user_id: %w", err)
	}
	until, err := time.Parse(time.RFC3339Nano, params.Until)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: parse until: %w", err)
	}
	if params.OccurredAt == "" {
		return fmt.Errorf("SnoozeRecallCandidate: occurred_at is required")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, params.OccurredAt)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: parse occurred_at: %w", err)
	}

	query := `UPDATE recall_candidate_view SET snoozed_until = $1, updated_at = $2
		WHERE user_id = $3 AND item_key = $4`
	_, err = r.pool.Exec(ctx, query, until, occurredAt, userID, params.ItemKey)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: %w", err)
	}
	return nil
}

// DismissRecallCandidate soft-deletes a recall candidate by setting dismissed_at.
// The candidate remains in the table so the projector's UPSERT preserves the dismissal.
// After a 30-day cooldown, the projector may clear dismissed_at to allow re-surfacing.
// dismissed_at/updated_at are written from the caller-supplied occurred_at
// rather than SQL now() for the same reproject-determinism reason as
// SnoozeRecallCandidate above.
func (r *Repository) DismissRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	var params struct {
		UserID     string `json:"user_id"`
		ItemKey    string `json:"item_key"`
		OccurredAt string `json:"occurred_at"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return fmt.Errorf("DismissRecallCandidate: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: parse user_id: %w", err)
	}
	if params.OccurredAt == "" {
		return fmt.Errorf("DismissRecallCandidate: occurred_at is required")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, params.OccurredAt)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: parse occurred_at: %w", err)
	}

	query := `UPDATE recall_candidate_view SET dismissed_at = $1, updated_at = $1
		WHERE user_id = $2 AND item_key = $3`
	_, err = r.pool.Exec(ctx, query, occurredAt, userID, params.ItemKey)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: %w", err)
	}
	return nil
}
