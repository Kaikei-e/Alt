package sovereign_db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KnowledgeHomeItem is the domain representation of a home item.
type KnowledgeHomeItem struct {
	UserID            uuid.UUID
	TenantID          uuid.UUID
	ItemKey           string
	ItemType          string
	PrimaryRefID      *uuid.UUID
	Title             string
	SummaryExcerpt    string
	Tags              []string
	WhyReasons        []WhyReason
	Score             float64
	FreshnessAt       *time.Time
	PublishedAt       *time.Time
	LastInteractedAt  *time.Time
	GeneratedAt       time.Time
	UpdatedAt         time.Time
	ProjectionVersion int
	SummaryState      string
	DismissedAt       *time.Time
	SupersedeState    string
	SupersededAt      *time.Time
	PreviousRefJSON   string
	URL               string
}

// WhyReason explains why an item appears in the Knowledge Home.
type WhyReason struct {
	Code  string `json:"code"`
	RefID string `json:"ref_id,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// LensFilter defines filter criteria for home items.
type LensFilter struct {
	QueryText    string
	TagNames     []string
	SourceIDs    []string
	TimeWindow   string
	IncludeRecap bool
	IncludePulse bool
	SortMode     string
}

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

// GetKnowledgeHomeItems returns paginated items for a user.
// No articles JOIN — url is stored directly in knowledge_home_items.
func (r *Repository) GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *LensFilter) ([]KnowledgeHomeItem, string, bool, error) {
	var query strings.Builder
	args := []interface{}{userID}
	fetchLimit := limit + 1

	query.WriteString(`SELECT khi.user_id, khi.tenant_id, khi.item_key, khi.item_type, khi.primary_ref_id,
		khi.title, khi.summary_excerpt, khi.tags_json, khi.why_json, khi.score,
		khi.freshness_at, khi.published_at, khi.last_interacted_at, khi.generated_at, khi.updated_at,
		khi.dismissed_at, khi.summary_state, COALESCE(khi.url, '') AS url,
		khi.supersede_state, khi.superseded_at, khi.previous_ref_json
		FROM knowledge_home_items khi
		WHERE khi.user_id = $1
		  AND khi.projection_version = COALESCE((
		  	SELECT version FROM knowledge_projection_versions
		  	WHERE status = 'active'
		  	ORDER BY version DESC LIMIT 1
		  ), 1)
		  AND khi.dismissed_at IS NULL`)

	argPos := 2
	if filter != nil {
		if filter.QueryText != "" || len(filter.TagNames) > 0 || filter.TimeWindow != "" {
			query.WriteString(` AND khi.item_type = 'article'`)
		}
		if filter.QueryText != "" {
			query.WriteString(fmt.Sprintf(` AND (
				khi.title ILIKE $%d
				OR COALESCE(khi.summary_excerpt, '') ILIKE $%d
				OR EXISTS (
					SELECT 1 FROM jsonb_array_elements_text(khi.tags_json) AS tag_name
					WHERE tag_name ILIKE $%d
				)
			)`, argPos, argPos, argPos))
			args = append(args, "%"+filter.QueryText+"%")
			argPos++
		}
		if len(filter.TagNames) > 0 {
			query.WriteString(fmt.Sprintf(` AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(khi.tags_json) AS tag_name
				WHERE tag_name = ANY($%d)
			)`, argPos))
			args = append(args, filter.TagNames)
			argPos++
		}
		if filter.TimeWindow != "" {
			cutoff, err := cutoffFromTimeWindow(filter.TimeWindow)
			if err != nil {
				return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: %w", err)
			}
			query.WriteString(fmt.Sprintf(` AND khi.published_at >= $%d`, argPos))
			args = append(args, cutoff)
			argPos++
		}
	}

	if cursor != "" {
		cursorScore, cursorPublishedAt, cursorItemKey, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: invalid cursor: %w", err)
		}
		query.WriteString(fmt.Sprintf(` AND (khi.score, khi.published_at, khi.item_key) < ($%d, $%d, $%d)`,
			argPos, argPos+1, argPos+2))
		args = append(args, cursorScore, cursorPublishedAt, cursorItemKey)
		argPos += 3
	}

	query.WriteString(fmt.Sprintf(` ORDER BY khi.score DESC, khi.published_at DESC, khi.item_key DESC LIMIT $%d`, argPos))
	args = append(args, fetchLimit)

	rows, err := r.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: %w", err)
	}
	defer rows.Close()

	var items []KnowledgeHomeItem
	for rows.Next() {
		var item KnowledgeHomeItem
		var tagsJSON, whyJSON []byte
		var supersedeState, previousRefJSON *string
		if err := rows.Scan(
			&item.UserID, &item.TenantID, &item.ItemKey, &item.ItemType, &item.PrimaryRefID,
			&item.Title, &item.SummaryExcerpt, &tagsJSON, &whyJSON, &item.Score,
			&item.FreshnessAt, &item.PublishedAt, &item.LastInteractedAt, &item.GeneratedAt, &item.UpdatedAt,
			&item.DismissedAt, &item.SummaryState, &item.URL,
			&supersedeState, &item.SupersededAt, &previousRefJSON,
		); err != nil {
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems scan: %w", err)
		}
		_ = json.Unmarshal(tagsJSON, &item.Tags)
		_ = json.Unmarshal(whyJSON, &item.WhyReasons)
		if supersedeState != nil {
			item.SupersedeState = *supersedeState
		}
		if previousRefJSON != nil {
			item.PreviousRefJSON = *previousRefJSON
		}
		items = append(items, item)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.Score, last.PublishedAt, last.ItemKey)
	}

	return items, nextCursor, hasMore, nil
}

// ListDistinctUserIDs returns all distinct user IDs from knowledge_home_items.
func (r *Repository) ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	query := `SELECT DISTINCT user_id FROM knowledge_home_items`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListDistinctUserIDs: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("ListDistinctUserIDs scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// CountNeedToKnowItems returns the count of pulse_need_to_know items for today.
func (r *Repository) CountNeedToKnowItems(ctx context.Context, userID uuid.UUID, date time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM knowledge_home_items khi
		WHERE khi.user_id = $1
		  AND khi.projection_version = COALESCE((
		    SELECT version FROM knowledge_projection_versions WHERE status = 'active' ORDER BY version DESC LIMIT 1
		  ), 1)
		  AND khi.dismissed_at IS NULL
		  AND khi.published_at >= $2
		  AND khi.published_at < $3
		  AND EXISTS (
		    SELECT 1 FROM jsonb_array_elements(khi.why_json) AS r
		    WHERE r->>'code' = 'pulse_need_to_know'
		  )`

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var count int
	if err := r.pool.QueryRow(ctx, query, userID, startOfDay, endOfDay).Scan(&count); err != nil {
		return 0, fmt.Errorf("CountNeedToKnowItems: %w", err)
	}
	return count, nil
}

// GetTodayDigest returns the today digest for a user and date.
func (r *Repository) GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (*TodayDigest, error) {
	query := `SELECT user_id, digest_date, new_articles, summarized_articles, unsummarized_articles,
		top_tags_json, updated_at, weekly_recap_available, evening_pulse_available
		FROM today_digest_view WHERE user_id = $1 AND digest_date = $2`

	var d TodayDigest
	var topTagsJSON []byte
	err := r.pool.QueryRow(ctx, query, userID, date.Format("2006-01-02")).Scan(
		&d.UserID, &d.DigestDate, &d.NewArticles, &d.SummarizedArticles, &d.UnsummarizedArticles,
		&topTagsJSON, &d.UpdatedAt, &d.WeeklyRecapAvailable, &d.EveningPulseAvailable,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetTodayDigest: %w", err)
	}
	_ = json.Unmarshal(topTagsJSON, &d.TopTags)
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
		  AND khi.projection_version = COALESCE((
		    SELECT version FROM knowledge_projection_versions WHERE status = 'active' ORDER BY version DESC LIMIT 1
		  ), 1)
		WHERE rcv.user_id = $1
		  AND rcv.dismissed_at IS NULL
		  AND rcv.snoozed_until IS NULL
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
		var c RecallCandidate
		var reasonJSON []byte
		var itemTitle, itemSummary, itemURL, itemType, itemSummaryState *string
		var itemTagsJSON, itemWhyJSON []byte
		var itemScore *float64
		var itemPublishedAt *time.Time
		var itemPrimaryRefID *uuid.UUID

		if err := rows.Scan(
			&c.UserID, &c.ItemKey, &c.RecallScore, &reasonJSON,
			&c.NextSuggestAt, &c.FirstEligibleAt, &c.SnoozedUntil, &c.UpdatedAt, &c.ProjectionVersion,
			&itemTitle, &itemSummary, &itemTagsJSON, &itemWhyJSON, &itemScore,
			&itemPublishedAt, &itemSummaryState, &itemURL,
			&itemType, &itemPrimaryRefID,
		); err != nil {
			return nil, fmt.Errorf("GetRecallCandidates scan: %w", err)
		}
		_ = json.Unmarshal(reasonJSON, &c.Reasons)

		if itemTitle != nil {
			item := &KnowledgeHomeItem{
				UserID:       c.UserID,
				ItemKey:      c.ItemKey,
				Title:        *itemTitle,
				PrimaryRefID: itemPrimaryRefID,
			}
			if itemSummary != nil {
				item.SummaryExcerpt = *itemSummary
			}
			if itemScore != nil {
				item.Score = *itemScore
			}
			if itemPublishedAt != nil {
				item.PublishedAt = itemPublishedAt
			}
			if itemSummaryState != nil {
				item.SummaryState = *itemSummaryState
			}
			if itemURL != nil {
				item.URL = *itemURL
			}
			if itemType != nil {
				item.ItemType = *itemType
			}
			_ = json.Unmarshal(itemTagsJSON, &item.Tags)
			_ = json.Unmarshal(itemWhyJSON, &item.WhyReasons)
			c.Item = item
		}

		candidates = append(candidates, c)
	}

	return candidates, nil
}

// GetProjectionFreshness returns the updated_at timestamp from the projection checkpoint.
func (r *Repository) GetProjectionFreshness(ctx context.Context, projectorName string) (*time.Time, error) {
	query := `SELECT updated_at FROM knowledge_projection_checkpoints WHERE projector_name = $1`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, query, projectorName).Scan(&updatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetProjectionFreshness: %w", err)
	}
	return &updatedAt, nil
}

// --- cursor helpers ---

func encodeCursor(score float64, publishedAt *time.Time, itemKey string) string {
	pub := ""
	if publishedAt != nil {
		pub = publishedAt.Format(time.RFC3339Nano)
	}
	raw := fmt.Sprintf("%v|%s|%s", score, pub, itemKey)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (float64, *time.Time, string, error) {
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, nil, "", fmt.Errorf("decode base64: %w", err)
	}
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 {
		return 0, nil, "", fmt.Errorf("invalid cursor format")
	}
	var score float64
	if _, err := fmt.Sscanf(parts[0], "%g", &score); err != nil {
		return 0, nil, "", fmt.Errorf("parse score: %w", err)
	}
	var publishedAt *time.Time
	if parts[1] != "" {
		t, err := time.Parse(time.RFC3339Nano, parts[1])
		if err != nil {
			return 0, nil, "", fmt.Errorf("parse published_at: %w", err)
		}
		publishedAt = &t
	}
	return score, publishedAt, parts[2], nil
}

func cutoffFromTimeWindow(window string) (time.Time, error) {
	now := time.Now().UTC()
	switch window {
	case "7d":
		return now.Add(-7 * 24 * time.Hour), nil
	case "30d":
		return now.Add(-30 * 24 * time.Hour), nil
	case "90d":
		return now.Add(-90 * 24 * time.Hour), nil
	case "":
		return time.Time{}, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time window: %s", window)
	}
}
