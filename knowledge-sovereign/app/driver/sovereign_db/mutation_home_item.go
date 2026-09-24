package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type homeItemWhyReason struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type homeItemMutation struct {
	UserID            uuid.UUID
	TenantID          uuid.UUID
	ItemKey           string
	ItemType          string
	PrimaryRefID      *uuid.UUID
	Title             string
	SummaryExcerpt    string
	TagsJSON          string
	WhyJSON           string
	Score             float64
	ScoreOp           string
	FreshnessAt       *time.Time
	PublishedAt       *time.Time
	LastInteractedAt  *time.Time
	GeneratedAt       time.Time
	UpdatedAt         time.Time
	DismissedAt       *time.Time
	ProjectionVersion int
	SummaryState      string
	SupersedeState    *string
	SupersededAt      *time.Time
	PreviousRefJSON   *string
	URL               string
}

func parseHomeItemMutation(payload json.RawMessage) (homeItemMutation, error) {
	var item struct {
		UserID         uuid.UUID           `json:"user_id"`
		TenantID       uuid.UUID           `json:"tenant_id"`
		ItemKey        string              `json:"item_key"`
		ItemType       string              `json:"item_type"`
		PrimaryRefID   *uuid.UUID          `json:"primary_ref_id"`
		Title          string              `json:"title"`
		SummaryExcerpt string              `json:"summary_excerpt"`
		Tags           []string            `json:"tags"`
		WhyReasons     []homeItemWhyReason `json:"why_reasons"`
		Score          float64             `json:"score"`
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
		return homeItemMutation{}, fmt.Errorf("UpsertKnowledgeHomeItem: unmarshal: %w", err)
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
		return homeItemMutation{}, fmt.Errorf("UpsertKnowledgeHomeItem: score_op is required")
	}
	scoreOp := *item.ScoreOp
	switch scoreOp {
	case "", scoreOpMax, scoreOpSet:
	default:
		return homeItemMutation{}, fmt.Errorf("UpsertKnowledgeHomeItem: unrecognized score_op %q (want \"\", %q, or %q)",
			scoreOp, scoreOpMax, scoreOpSet)
	}

	tags := item.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return homeItemMutation{}, fmt.Errorf("UpsertKnowledgeHomeItem: marshal tags: %w", err)
	}

	whyReasons := item.WhyReasons
	if whyReasons == nil {
		whyReasons = []homeItemWhyReason{}
	}
	whyJSON, err := json.Marshal(whyReasons)
	if err != nil {
		return homeItemMutation{}, fmt.Errorf("UpsertKnowledgeHomeItem: marshal why: %w", err)
	}

	var supersedeState *string
	if item.SupersedeState != "" {
		supersedeState = &item.SupersedeState
	}
	var previousRefJSON *string
	if item.PreviousRefJSON != "" {
		previousRefJSON = &item.PreviousRefJSON
	}

	return homeItemMutation{
		UserID:            item.UserID,
		TenantID:          item.TenantID,
		ItemKey:           item.ItemKey,
		ItemType:          item.ItemType,
		PrimaryRefID:      item.PrimaryRefID,
		Title:             item.Title,
		SummaryExcerpt:    item.SummaryExcerpt,
		TagsJSON:          string(tagsJSON),
		WhyJSON:           string(whyJSON),
		Score:             item.Score,
		ScoreOp:           scoreOp,
		FreshnessAt:       item.FreshnessAt,
		PublishedAt:       item.PublishedAt,
		LastInteractedAt:  item.LastInteractedAt,
		GeneratedAt:       item.GeneratedAt,
		UpdatedAt:         item.UpdatedAt,
		DismissedAt:       item.DismissedAt,
		ProjectionVersion: item.ProjectionVersion,
		SummaryState:      item.SummaryState,
		SupersedeState:    supersedeState,
		SupersededAt:      item.SupersededAt,
		PreviousRefJSON:   previousRefJSON,
		URL:               item.URL,
	}, nil
}

func buildUpsertKnowledgeHomeItemArgs(m homeItemMutation) []any {
	return []any{
		m.UserID, m.TenantID, m.ItemKey, m.ItemType, m.PrimaryRefID,
		m.Title, m.SummaryExcerpt, m.TagsJSON, m.WhyJSON, m.Score,
		m.FreshnessAt, m.PublishedAt, m.LastInteractedAt, m.GeneratedAt, m.UpdatedAt, m.DismissedAt,
		m.ProjectionVersion, m.SummaryState,
		m.SupersedeState, m.SupersededAt, m.PreviousRefJSON, m.URL,
		m.ScoreOp,
	}
}

// score_op values recognized by UpsertKnowledgeHomeItem's merge-safe UPSERT
// (see the score CASE below). Mirrored by knowledge_home_projector's
// scoreOpMax/scoreOpSet — duplicated rather than imported because driver/
// must not depend on usecase/ (Clean Architecture layer direction).
const (
	scoreOpMax = "max"
	scoreOpSet = "set"
)

const upsertKnowledgeHomeItemQuery = `INSERT INTO knowledge_home_items
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

// UpsertKnowledgeHomeItem inserts or updates a knowledge home item.
func (r *Repository) UpsertKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error {
	m, err := parseHomeItemMutation(payload)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, upsertKnowledgeHomeItemQuery, buildUpsertKnowledgeHomeItemArgs(m)...)
	if err != nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: %w", err)
	}
	return nil
}

type dismissHomeItemMutation struct {
	UserID            uuid.UUID
	ItemKey           string
	ProjectionVersion int
	DismissedAt       time.Time
}

func parseDismissHomeItemMutation(payload json.RawMessage) (dismissHomeItemMutation, error) {
	var params struct {
		UserID            string `json:"user_id"`
		ItemKey           string `json:"item_key"`
		ProjectionVersion int    `json:"projection_version"`
		DismissedAt       string `json:"dismissed_at"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return dismissHomeItemMutation{}, fmt.Errorf("DismissKnowledgeHomeItem: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return dismissHomeItemMutation{}, fmt.Errorf("DismissKnowledgeHomeItem: parse user_id: %w", err)
	}
	// dismissed_at is a business fact and must come from the event payload —
	// reproject-safe means replaying the same DismissedHomeItem event twice
	// produces the identical row. Falling back to wall-clock time here would
	// make each replay non-deterministic (immutable-design-guard: Event-time
	// purity). Loudly reject rather than fabricate a value.
	if params.DismissedAt == "" {
		return dismissHomeItemMutation{}, fmt.Errorf("DismissKnowledgeHomeItem: dismissed_at is required")
	}
	dismissedAt, err := time.Parse(time.RFC3339Nano, params.DismissedAt)
	if err != nil {
		return dismissHomeItemMutation{}, fmt.Errorf("DismissKnowledgeHomeItem: parse dismissed_at: %w", err)
	}
	return dismissHomeItemMutation{
		UserID:            userID,
		ItemKey:           params.ItemKey,
		ProjectionVersion: params.ProjectionVersion,
		DismissedAt:       dismissedAt,
	}, nil
}

const (
	dismissHomeItemAllVersionsQuery = `UPDATE knowledge_home_items
			SET dismissed_at = $1, updated_at = $1
			WHERE user_id = $2 AND item_key = $3 AND dismissed_at IS NULL`
	dismissHomeItemExactVersionQuery = `UPDATE knowledge_home_items
			SET dismissed_at = $1, updated_at = $1
			WHERE user_id = $2 AND item_key = $3 AND projection_version = $4`
)

func buildDismissHomeItemQueryAndArgs(m dismissHomeItemMutation) (string, []any) {
	if m.ProjectionVersion == 0 {
		return dismissHomeItemAllVersionsQuery, []any{m.DismissedAt, m.UserID, m.ItemKey}
	}
	return dismissHomeItemExactVersionQuery, []any{m.DismissedAt, m.UserID, m.ItemKey, m.ProjectionVersion}
}

// DismissKnowledgeHomeItem marks an item as dismissed.
func (r *Repository) DismissKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error {
	m, err := parseDismissHomeItemMutation(payload)
	if err != nil {
		return err
	}
	query, args := buildDismissHomeItemQueryAndArgs(m)
	commandTag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("DismissKnowledgeHomeItem: %w", err)
	}
	if m.ProjectionVersion != 0 && commandTag.RowsAffected() == 0 {
		return ErrDismissTargetNotFound
	}
	return nil
}

type clearSupersedeStateMutation struct {
	UserID            uuid.UUID
	ItemKey           string
	ProjectionVersion int
}

func parseClearSupersedeStateMutation(payload json.RawMessage) (clearSupersedeStateMutation, error) {
	var params struct {
		UserID            string `json:"user_id"`
		ItemKey           string `json:"item_key"`
		ProjectionVersion int    `json:"projection_version"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return clearSupersedeStateMutation{}, fmt.Errorf("ClearSupersedeState: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return clearSupersedeStateMutation{}, fmt.Errorf("ClearSupersedeState: parse user_id: %w", err)
	}
	return clearSupersedeStateMutation{
		UserID:            userID,
		ItemKey:           params.ItemKey,
		ProjectionVersion: params.ProjectionVersion,
	}, nil
}

func buildClearSupersedeStateArgs(m clearSupersedeStateMutation) []any {
	return []any{m.UserID, m.ItemKey, m.ProjectionVersion}
}

const clearSupersedeStateQuery = `UPDATE knowledge_home_items
		SET supersede_state = NULL, superseded_at = NULL, previous_ref_json = NULL
		WHERE user_id = $1 AND item_key = $2 AND projection_version = $3 AND supersede_state IS NOT NULL`

// ClearSupersedeState clears the supersede state for a specific item.
func (r *Repository) ClearSupersedeState(ctx context.Context, payload json.RawMessage) error {
	m, err := parseClearSupersedeStateMutation(payload)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, clearSupersedeStateQuery, buildClearSupersedeStateArgs(m)...)
	if err != nil {
		return fmt.Errorf("ClearSupersedeState: %w", err)
	}
	return nil
}
