package sovereign_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TodayDigest contains daily summary statistics.
type TodayDigest struct {
	UserID                uuid.UUID
	DigestDate            time.Time
	NewArticles           int
	SummarizedArticles    int
	UnsummarizedArticles  int
	TopTags               []string
	WeeklyRecapAvailable  bool
	EveningPulseAvailable bool
	UpdatedAt             time.Time
}

// RecallCandidate represents a candidate for the recall rail.
type RecallCandidate struct {
	UserID            uuid.UUID
	ItemKey           string
	RecallScore       float64
	Reasons           []RecallReason
	NextSuggestAt     *time.Time
	FirstEligibleAt   *time.Time
	SnoozedUntil      *time.Time
	UpdatedAt         time.Time
	ProjectionVersion int
	Item              *KnowledgeHomeItem
}

// RecallReason explains why an item is being recalled.
type RecallReason struct {
	Type          string `json:"type"`
	Description   string `json:"description"`
	SourceItemKey string `json:"source_item_key,omitempty"`
}

// rawTodayDigestRow holds columns scanned from today_digest_view.
type rawTodayDigestRow struct {
	UserID                uuid.UUID
	DigestDate            time.Time
	NewArticles           int
	SummarizedArticles    int
	UnsummarizedArticles  int
	TopTagsJSON           []byte
	UpdatedAt             time.Time
	WeeklyRecapAvailable  bool
	EveningPulseAvailable bool
}

// mapTodayDigestRow maps raw today_digest_view columns into TodayDigest.
func mapTodayDigestRow(raw rawTodayDigestRow) TodayDigest {
	d := TodayDigest{
		UserID:                raw.UserID,
		DigestDate:            raw.DigestDate,
		NewArticles:           raw.NewArticles,
		SummarizedArticles:    raw.SummarizedArticles,
		UnsummarizedArticles:  raw.UnsummarizedArticles,
		UpdatedAt:             raw.UpdatedAt,
		WeeklyRecapAvailable:  raw.WeeklyRecapAvailable,
		EveningPulseAvailable: raw.EveningPulseAvailable,
	}
	unmarshalJSONWarn(raw.TopTagsJSON, &d.TopTags, "top_tags_json")
	return d
}

// rawRecallCandidateRow represents scanned columns for a recall candidate joined with knowledge_home_items.
type rawRecallCandidateRow struct {
	UserID            uuid.UUID
	ItemKey           string
	RecallScore       float64
	ReasonJSON        []byte
	NextSuggestAt     *time.Time
	FirstEligibleAt   *time.Time
	SnoozedUntil      *time.Time
	UpdatedAt         time.Time
	ProjectionVersion int
	ItemTitle         *string
	ItemSummary       *string
	ItemTagsJSON      []byte
	ItemWhyJSON       []byte
	ItemScore         *float64
	ItemPublishedAt   *time.Time
	ItemSummaryState  *string
	ItemURL           *string
	ItemType          *string
	ItemPrimaryRefID  *uuid.UUID
}

// mapRecallCandidateRow converts scanned candidate and home item columns into RecallCandidate.
func mapRecallCandidateRow(raw rawRecallCandidateRow) RecallCandidate {
	c := RecallCandidate{
		UserID:            raw.UserID,
		ItemKey:           raw.ItemKey,
		RecallScore:       raw.RecallScore,
		NextSuggestAt:     raw.NextSuggestAt,
		FirstEligibleAt:   raw.FirstEligibleAt,
		SnoozedUntil:      raw.SnoozedUntil,
		UpdatedAt:         raw.UpdatedAt,
		ProjectionVersion: raw.ProjectionVersion,
	}
	unmarshalJSONWarn(raw.ReasonJSON, &c.Reasons, "reason_json")

	if raw.ItemTitle != nil {
		item := &KnowledgeHomeItem{
			UserID:       c.UserID,
			ItemKey:      c.ItemKey,
			Title:        *raw.ItemTitle,
			PrimaryRefID: raw.ItemPrimaryRefID,
		}
		if raw.ItemSummary != nil {
			item.SummaryExcerpt = *raw.ItemSummary
		}
		if raw.ItemScore != nil {
			item.Score = *raw.ItemScore
		}
		if raw.ItemPublishedAt != nil {
			item.PublishedAt = raw.ItemPublishedAt
		}
		if raw.ItemSummaryState != nil {
			item.SummaryState = *raw.ItemSummaryState
		}
		if raw.ItemURL != nil {
			item.URL = *raw.ItemURL
		}
		if raw.ItemType != nil {
			item.ItemType = *raw.ItemType
		}
		unmarshalJSONWarn(raw.ItemTagsJSON, &item.Tags, "tags_json")
		unmarshalJSONWarn(raw.ItemWhyJSON, &item.WhyReasons, "why_json")
		c.Item = item
	}

	return c
}

// GetTodayDigest returns the today digest for a user and date.
func (r *Repository) GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (*TodayDigest, error) {
	query := `SELECT user_id, digest_date, new_articles, summarized_articles, unsummarized_articles,
		top_tags_json, updated_at, weekly_recap_available, evening_pulse_available
		FROM today_digest_view WHERE user_id = $1 AND digest_date = $2`

	var raw rawTodayDigestRow
	err := r.pool.QueryRow(ctx, query, userID, date.Format("2006-01-02")).Scan(
		&raw.UserID, &raw.DigestDate, &raw.NewArticles, &raw.SummarizedArticles, &raw.UnsummarizedArticles,
		&raw.TopTagsJSON, &raw.UpdatedAt, &raw.WeeklyRecapAvailable, &raw.EveningPulseAvailable,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetTodayDigest: %w", err)
	}
	d := mapTodayDigestRow(raw)
	return &d, nil
}

// GetRecallCandidates returns recall candidates for a user.
// No articles JOIN — returns candidates with embedded home items from sovereign DB only.
func (r *Repository) GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]RecallCandidate, error) {
	query := `SELECT rcv.user_id, rcv.item_key, rcv.recall_score, rcv.reason_json,
		rcv.next_suggest_at, rcv.first_eligible_at, rcv.snoozed_until, rcv.updated_at, rcv.projection_version,
		khi.title, khi.summary_excerpt, khi.tags_json, khi.why_json, khi.score,
		khi.published_at, khi.summary_state, COALESCE(khi.url, '') AS url,
		khi.item_type, khi.primary_ref_id
		FROM recall_candidate_view rcv
		LEFT JOIN knowledge_home_items khi ON rcv.user_id = khi.user_id AND rcv.item_key = khi.item_key
		  AND khi.projection_version = ` + activeProjectionVersionSQL + `
		WHERE rcv.user_id = $1
		  AND rcv.dismissed_at IS NULL
		  AND (rcv.snoozed_until IS NULL OR rcv.snoozed_until <= now())
		  AND rcv.next_suggest_at IS NOT NULL
		  AND rcv.next_suggest_at <= now()
		ORDER BY rcv.recall_score DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetRecallCandidates: %w", err)
	}
	defer rows.Close()

	var candidates []RecallCandidate
	for rows.Next() {
		var raw rawRecallCandidateRow
		if err := rows.Scan(
			&raw.UserID, &raw.ItemKey, &raw.RecallScore, &raw.ReasonJSON,
			&raw.NextSuggestAt, &raw.FirstEligibleAt, &raw.SnoozedUntil, &raw.UpdatedAt, &raw.ProjectionVersion,
			&raw.ItemTitle, &raw.ItemSummary, &raw.ItemTagsJSON, &raw.ItemWhyJSON, &raw.ItemScore,
			&raw.ItemPublishedAt, &raw.ItemSummaryState, &raw.ItemURL,
			&raw.ItemType, &raw.ItemPrimaryRefID,
		); err != nil {
			return nil, fmt.Errorf("GetRecallCandidates scan: %w", err)
		}
		candidates = append(candidates, mapRecallCandidateRow(raw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetRecallCandidates rows: %w", err)
	}

	return candidates, nil
}

// GetProjectionFreshness returns the updated_at timestamp from the projection checkpoint.
func (r *Repository) GetProjectionFreshness(ctx context.Context, projectorName string) (*time.Time, error) {
	query := `SELECT updated_at FROM knowledge_projection_checkpoints WHERE projector_name = $1`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, query, projectorName).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetProjectionFreshness: %w", err)
	}
	return &updatedAt, nil
}
