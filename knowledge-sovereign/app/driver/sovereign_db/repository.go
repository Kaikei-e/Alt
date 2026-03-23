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

// PgxIface defines the interface for pgx operations.
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
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

// UpsertKnowledgeHomeItem inserts or updates a knowledge home item.
func (r *Repository) UpsertKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error {
	var item struct {
		UserID           uuid.UUID  `json:"user_id"`
		TenantID         uuid.UUID  `json:"tenant_id"`
		ItemKey          string     `json:"item_key"`
		ItemType         string     `json:"item_type"`
		PrimaryRefID     *uuid.UUID `json:"primary_ref_id"`
		Title            string     `json:"title"`
		SummaryExcerpt   string     `json:"summary_excerpt"`
		Tags             []string   `json:"tags"`
		WhyReasons       []struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		} `json:"why_reasons"`
		Score             float64    `json:"score"`
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
		Link              string     `json:"link"`
	}
	if err := json.Unmarshal(payload, &item); err != nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: unmarshal: %w", err)
	}

	tags := item.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, _ := json.Marshal(tags)

	whyReasons := item.WhyReasons
	if whyReasons == nil {
		whyReasons = []struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		}{}
	}
	whyJSON, _ := json.Marshal(whyReasons)

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
		 supersede_state, superseded_at, previous_ref_json, link)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (user_id, item_key, projection_version) DO UPDATE SET
		 title = CASE WHEN EXCLUDED.title != '' THEN EXCLUDED.title ELSE knowledge_home_items.title END,
		 summary_excerpt = CASE WHEN EXCLUDED.summary_excerpt != '' THEN EXCLUDED.summary_excerpt ELSE knowledge_home_items.summary_excerpt END,
		 tags_json = CASE WHEN EXCLUDED.tags_json != '[]'::jsonb THEN EXCLUDED.tags_json ELSE knowledge_home_items.tags_json END,
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
		 score = GREATEST(EXCLUDED.score, knowledge_home_items.score),
		 freshness_at = COALESCE(EXCLUDED.freshness_at, knowledge_home_items.freshness_at),
		 published_at = COALESCE(EXCLUDED.published_at, knowledge_home_items.published_at),
		 last_interacted_at = COALESCE(EXCLUDED.last_interacted_at, knowledge_home_items.last_interacted_at),
		 updated_at = EXCLUDED.updated_at,
		 dismissed_at = COALESCE(knowledge_home_items.dismissed_at, EXCLUDED.dismissed_at),
		 projection_version = EXCLUDED.projection_version,
		 summary_state = CASE WHEN EXCLUDED.summary_state = 'ready' THEN 'ready' WHEN EXCLUDED.summary_state NOT IN ('', 'missing') THEN EXCLUDED.summary_state ELSE knowledge_home_items.summary_state END,
		 supersede_state = COALESCE(EXCLUDED.supersede_state, knowledge_home_items.supersede_state),
		 superseded_at = COALESCE(EXCLUDED.superseded_at, knowledge_home_items.superseded_at),
		 previous_ref_json = CASE
			 WHEN EXCLUDED.previous_ref_json IS NOT NULL THEN COALESCE(knowledge_home_items.previous_ref_json, '{}'::jsonb) || EXCLUDED.previous_ref_json
			 ELSE knowledge_home_items.previous_ref_json
		 END,
		 link = CASE WHEN EXCLUDED.link != '' THEN EXCLUDED.link ELSE knowledge_home_items.link END`

	_, err := r.pool.Exec(ctx, query,
		item.UserID, item.TenantID, item.ItemKey, item.ItemType, item.PrimaryRefID,
		item.Title, item.SummaryExcerpt, string(tagsJSON), string(whyJSON), item.Score,
		item.FreshnessAt, item.PublishedAt, item.LastInteractedAt, item.GeneratedAt, item.UpdatedAt, item.DismissedAt,
		item.ProjectionVersion, item.SummaryState,
		supersedeState, item.SupersededAt, previousRefJSON, item.Link,
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
	dismissedAt := time.Now()
	if params.DismissedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, params.DismissedAt)
		if err != nil {
			return fmt.Errorf("DismissKnowledgeHomeItem: parse dismissed_at: %w", err)
		}
		dismissedAt = parsed
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

// UpsertTodayDigest inserts or updates a today digest entry.
func (r *Repository) UpsertTodayDigest(ctx context.Context, payload json.RawMessage) error {
	var digest struct {
		UserID                 uuid.UUID `json:"user_id"`
		DigestDate             string    `json:"digest_date"`
		NewArticles            int       `json:"new_articles"`
		SummarizedArticles     int       `json:"summarized_articles"`
		UnsummarizedArticles   int       `json:"unsummarized_articles"`
		TopTags                []string  `json:"top_tags"`
		UpdatedAt              time.Time `json:"updated_at"`
		WeeklyRecapAvailable   bool      `json:"weekly_recap_available"`
		EveningPulseAvailable  bool      `json:"evening_pulse_available"`
	}
	if err := json.Unmarshal(payload, &digest); err != nil {
		return fmt.Errorf("UpsertTodayDigest: unmarshal: %w", err)
	}

	topTags := digest.TopTags
	if topTags == nil {
		topTags = []string{}
	}
	topTagsJSON, _ := json.Marshal(topTags)
	pulseRefsJSON := []byte("[]")

	query := `INSERT INTO today_digest_view
		(user_id, digest_date, new_articles, summarized_articles,
		 unsummarized_articles, top_tags_json, pulse_refs_json, updated_at,
		 weekly_recap_available, evening_pulse_available)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, digest_date) DO UPDATE SET
		 new_articles = today_digest_view.new_articles + EXCLUDED.new_articles,
		 summarized_articles = today_digest_view.summarized_articles + EXCLUDED.summarized_articles,
		 unsummarized_articles = GREATEST(0, today_digest_view.unsummarized_articles + EXCLUDED.unsummarized_articles),
		 top_tags_json = CASE WHEN EXCLUDED.top_tags_json != '[]'::jsonb THEN EXCLUDED.top_tags_json ELSE today_digest_view.top_tags_json END,
		 pulse_refs_json = CASE WHEN EXCLUDED.pulse_refs_json != '[]'::jsonb THEN EXCLUDED.pulse_refs_json ELSE today_digest_view.pulse_refs_json END,
		 updated_at = EXCLUDED.updated_at,
		 weekly_recap_available = EXCLUDED.weekly_recap_available OR today_digest_view.weekly_recap_available,
		 evening_pulse_available = EXCLUDED.evening_pulse_available OR today_digest_view.evening_pulse_available`

	_, err := r.pool.Exec(ctx, query,
		digest.UserID, digest.DigestDate,
		digest.NewArticles, digest.SummarizedArticles,
		digest.UnsummarizedArticles, string(topTagsJSON), string(pulseRefsJSON),
		digest.UpdatedAt,
		digest.WeeklyRecapAvailable, digest.EveningPulseAvailable,
	)
	if err != nil {
		return fmt.Errorf("UpsertTodayDigest: %w", err)
	}
	return nil
}

// UpsertRecallCandidate inserts or updates a recall candidate.
func (r *Repository) UpsertRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	var candidate struct {
		UserID            uuid.UUID  `json:"user_id"`
		ItemKey           string     `json:"item_key"`
		RecallScore       float64    `json:"recall_score"`
		Reasons           []struct {
			Code   string `json:"code"`
			Reason string `json:"reason"`
		} `json:"reasons"`
		NextSuggestAt     *time.Time `json:"next_suggest_at"`
		FirstEligibleAt   *time.Time `json:"first_eligible_at"`
		UpdatedAt         time.Time  `json:"updated_at"`
		ProjectionVersion int        `json:"projection_version"`
	}
	if err := json.Unmarshal(payload, &candidate); err != nil {
		return fmt.Errorf("UpsertRecallCandidate: unmarshal: %w", err)
	}

	reasonJSON, _ := json.Marshal(candidate.Reasons)

	query := `INSERT INTO recall_candidate_view
		(user_id, item_key, recall_score, reason_json, next_suggest_at, first_eligible_at, updated_at, projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		  recall_score = EXCLUDED.recall_score,
		  reason_json = EXCLUDED.reason_json,
		  next_suggest_at = EXCLUDED.next_suggest_at,
		  updated_at = EXCLUDED.updated_at,
		  projection_version = EXCLUDED.projection_version`

	_, err := r.pool.Exec(ctx, query,
		candidate.UserID, candidate.ItemKey, candidate.RecallScore, string(reasonJSON),
		candidate.NextSuggestAt, candidate.FirstEligibleAt, candidate.UpdatedAt, candidate.ProjectionVersion,
	)
	if err != nil {
		return fmt.Errorf("UpsertRecallCandidate: %w", err)
	}
	return nil
}

// SnoozeRecallCandidate snoozes a recall candidate until the given time.
func (r *Repository) SnoozeRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	var params struct {
		UserID  string `json:"user_id"`
		ItemKey string `json:"item_key"`
		Until   string `json:"until"`
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

	query := `UPDATE recall_candidate_view SET snoozed_until = $1, updated_at = now()
		WHERE user_id = $2 AND item_key = $3`
	_, err = r.pool.Exec(ctx, query, until, userID, params.ItemKey)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: %w", err)
	}
	return nil
}

// DismissRecallCandidate soft-deletes a recall candidate by setting dismissed_at.
// The candidate remains in the table so the projector's UPSERT preserves the dismissal.
// After a 30-day cooldown, the projector may clear dismissed_at to allow re-surfacing.
func (r *Repository) DismissRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	var params struct {
		UserID  string `json:"user_id"`
		ItemKey string `json:"item_key"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return fmt.Errorf("DismissRecallCandidate: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: parse user_id: %w", err)
	}

	query := `UPDATE recall_candidate_view SET dismissed_at = now(), updated_at = now()
		WHERE user_id = $1 AND item_key = $2`
	_, err = r.pool.Exec(ctx, query, userID, params.ItemKey)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: %w", err)
	}
	return nil
}
