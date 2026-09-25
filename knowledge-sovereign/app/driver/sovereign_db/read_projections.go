package sovereign_db

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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

// homeItemCursor holds decoded keyset continuation parameters.
type homeItemCursor struct {
	RankScore   float64
	PublishedAt *time.Time
	ItemKey     string
	AsOf        time.Time
}

// mapKnowledgeHomeItemRow maps scanned raw columns into a domain KnowledgeHomeItem.
func mapKnowledgeHomeItemRow(item KnowledgeHomeItem, tagsJSON, whyJSON []byte, supersedeState, previousRefJSON *string) KnowledgeHomeItem {
	unmarshalJSONWarn(tagsJSON, &item.Tags, "tags_json")
	unmarshalJSONWarn(whyJSON, &item.WhyReasons, "why_json")
	if supersedeState != nil {
		item.SupersedeState = *supersedeState
	}
	if previousRefJSON != nil {
		item.PreviousRefJSON = *previousRefJSON
	}
	return item
}

// buildKnowledgeHomeFilterClauses constructs SQL predicates and binds parameters for LensFilter.
func buildKnowledgeHomeFilterClauses(filter *LensFilter, cutoff time.Time, startArgPos int) (string, []interface{}, int) {
	if filter == nil {
		return "", nil, startArgPos
	}
	var clause strings.Builder
	var args []interface{}
	argPos := startArgPos

	if filter.QueryText != "" || len(filter.TagNames) > 0 || filter.TimeWindow != "" {
		clause.WriteString(` AND khi.item_type = 'article'`)
	}
	if filter.QueryText != "" {
		fmt.Fprintf(&clause, ` AND (
				khi.title ILIKE $%d
				OR COALESCE(khi.summary_excerpt, '') ILIKE $%d
				OR EXISTS (
					SELECT 1 FROM jsonb_array_elements_text(khi.tags_json) AS tag_name
					WHERE tag_name ILIKE $%d
				)
			)`, argPos, argPos, argPos)
		args = append(args, "%"+filter.QueryText+"%")
		argPos++
	}
	if len(filter.TagNames) > 0 {
		fmt.Fprintf(&clause, ` AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(khi.tags_json) AS tag_name
				WHERE tag_name = ANY($%d)
			)`, argPos)
		args = append(args, filter.TagNames)
		argPos++
	}
	if filter.TimeWindow != "" {
		fmt.Fprintf(&clause, ` AND khi.published_at >= $%d`, argPos)
		args = append(args, cutoff)
		argPos++
	}

	return clause.String(), args, argPos
}

// buildKnowledgeHomeKeysetClause constructs the keyset pagination comparison predicate.
func buildKnowledgeHomeKeysetClause(rankScoreSQL string, cursor *homeItemCursor, startArgPos int) (string, []interface{}, int) {
	if cursor == nil {
		return "", nil, startArgPos
	}
	clause := fmt.Sprintf(
		` AND (`+rankScoreSQL+`, COALESCE(khi.published_at, '-infinity'), khi.item_key) < ($%d, COALESCE($%d::timestamptz, '-infinity'), $%d)`,
		startArgPos, startArgPos+1, startArgPos+2,
	)
	args := []interface{}{cursor.RankScore, cursor.PublishedAt, cursor.ItemKey}
	return clause, args, startArgPos + 3
}

// buildKnowledgeHomeQuery builds the SELECT query and arguments for Knowledge Home items.
func buildKnowledgeHomeQuery(userID uuid.UUID, cursor *homeItemCursor, filter *LensFilter, cutoff time.Time, fetchLimit int) (string, []interface{}) {
	var query strings.Builder
	args := []interface{}{userID}
	argPos := 2

	rankAsOfExpr := "now()"
	if cursor != nil {
		rankAsOfExpr = fmt.Sprintf("$%d::timestamptz", argPos)
		args = append(args, cursor.AsOf)
		argPos++
	}
	rankScoreSQL := homeItemRankScoreSQL(rankAsOfExpr)

	query.WriteString(`SELECT khi.user_id, khi.tenant_id, khi.item_key, khi.item_type, khi.primary_ref_id,
		khi.title, khi.summary_excerpt, khi.tags_json, khi.why_json, khi.score,
		` + rankScoreSQL + ` AS rank_score, ` + rankAsOfExpr + ` AS rank_as_of,
		khi.freshness_at, khi.published_at, khi.last_interacted_at, khi.generated_at, khi.updated_at,
		khi.dismissed_at, khi.summary_state, COALESCE(khi.url, '') AS url,
		khi.supersede_state, khi.superseded_at, khi.previous_ref_json, khi.projection_version
		FROM knowledge_home_items khi
		WHERE khi.user_id = $1
		  AND khi.projection_version = ` + activeProjectionVersionSQL + `
		  AND khi.dismissed_at IS NULL`)

	filterSQL, filterArgs, nextArgPos := buildKnowledgeHomeFilterClauses(filter, cutoff, argPos)
	if filterSQL != "" {
		query.WriteString(filterSQL)
		args = append(args, filterArgs...)
		argPos = nextArgPos
	}

	keysetSQL, keysetArgs, nextArgPos := buildKnowledgeHomeKeysetClause(rankScoreSQL, cursor, argPos)
	if keysetSQL != "" {
		query.WriteString(keysetSQL)
		args = append(args, keysetArgs...)
		argPos = nextArgPos
	}

	fmt.Fprintf(&query,
		` ORDER BY rank_score DESC, COALESCE(khi.published_at, '-infinity') DESC, khi.item_key DESC LIMIT $%d`, argPos)
	args = append(args, fetchLimit)

	return query.String(), args
}

// paginateHomeItems is a pure function that trims fetch results to the requested limit and encodes the next cursor.
func paginateHomeItems(items []KnowledgeHomeItem, rankScores []float64, rankAsOf time.Time, limit int) ([]KnowledgeHomeItem, string, bool) {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
		rankScores = rankScores[:limit]
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(rankScores[len(rankScores)-1], last.PublishedAt, last.ItemKey, rankAsOf)
	}

	return items, nextCursor, hasMore
}

// GetKnowledgeHomeItems returns paginated items for a user.
// No articles JOIN — url is stored directly in knowledge_home_items.
//
// Ranking decays over time (homeItemRankScoreSQL), so the keyset cursor
// cannot simply carry a rank value computed by a previous call: "now" must
// be anchored to a single instant for the whole pagination session, or the
// page-boundary row's rank strictly drops between requests and re-satisfies
// its own keyset predicate on every later page (each_key_duplicate in the
// Knowledge Home stream). The first page lets Postgres's own `now()`
// (transaction start time, stable for the whole query) act as the anchor
// and reports it back via rankAsOf/nextCursor; every later page rebinds
// that exact instant as a query parameter instead of calling `now()` again.
func (r *Repository) GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *LensFilter) ([]KnowledgeHomeItem, string, bool, error) {
	var cursorPayload *homeItemCursor
	if cursor != "" {
		var err error
		cursorPayload, err = decodeCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: invalid cursor: %w", err)
		}
	}

	var cutoff time.Time
	if filter != nil && filter.TimeWindow != "" {
		var err error
		cutoff, err = cutoffFromTimeWindow(filter.TimeWindow)
		if err != nil {
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: %w", err)
		}
	}

	querySQL, args := buildKnowledgeHomeQuery(userID, cursorPayload, filter, cutoff, limit+1)

	rows, err := r.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: %w", err)
	}
	defer rows.Close()

	var items []KnowledgeHomeItem
	var rankScores []float64
	var rankAsOf time.Time
	for rows.Next() {
		var item KnowledgeHomeItem
		var tagsJSON, whyJSON []byte
		var supersedeState, previousRefJSON *string
		var rankScore float64
		if err := rows.Scan(
			&item.UserID, &item.TenantID, &item.ItemKey, &item.ItemType, &item.PrimaryRefID,
			&item.Title, &item.SummaryExcerpt, &tagsJSON, &whyJSON, &item.Score,
			&rankScore, &rankAsOf,
			&item.FreshnessAt, &item.PublishedAt, &item.LastInteractedAt, &item.GeneratedAt, &item.UpdatedAt,
			&item.DismissedAt, &item.SummaryState, &item.URL,
			&supersedeState, &item.SupersededAt, &previousRefJSON, &item.ProjectionVersion,
		); err != nil {
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems scan: %w", err)
		}
		item = mapKnowledgeHomeItemRow(item, tagsJSON, whyJSON, supersedeState, previousRefJSON)
		items = append(items, item)
		rankScores = append(rankScores, rankScore)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems rows: %w", err)
	}

	trimmedItems, nextCursor, hasMore := paginateHomeItems(items, rankScores, rankAsOf, limit)
	return trimmedItems, nextCursor, hasMore, nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListDistinctUserIDs rows: %w", err)
	}
	return ids, nil
}

// CountNeedToKnowItems returns the count of pulse_need_to_know items for today.
func (r *Repository) CountNeedToKnowItems(ctx context.Context, userID uuid.UUID, date time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM knowledge_home_items khi
		WHERE khi.user_id = $1
		  AND khi.projection_version = ` + activeProjectionVersionSQL + `
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

// --- cursor helpers ---
//
// The float64 carried by the cursor is the read-time decayed rank_score
// (homeItemRankScoreSQL), not the stored knowledge_home_items.score — it
// must match whatever GetKnowledgeHomeItems' ORDER BY / keyset WHERE clause
// actually ranks on for pagination to stay consistent page to page.
//
// asOf is the instant homeItemRankScoreSQL decayed against to produce
// rankScore. It must round-trip through every page of one pagination
// session unchanged (see GetKnowledgeHomeItems's rankAsOfExpr) — decay
// strictly shrinks with elapsed time, so re-deriving "now" independently on
// each page would make the boundary row's rank drop below its own cursor
// value and re-satisfy the keyset predicate against itself.

func encodeCursor(rankScore float64, publishedAt *time.Time, itemKey string, asOf time.Time) string {
	pub := ""
	if publishedAt != nil {
		pub = publishedAt.Format(time.RFC3339Nano)
	}
	raw := fmt.Sprintf("%v|%s|%s|%s", rankScore, pub, itemKey, asOf.UTC().Format(time.RFC3339Nano))
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (*homeItemCursor, error) {
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	parts := strings.SplitN(string(raw), "|", 4)
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid cursor format")
	}
	var rankScore float64
	if _, err := fmt.Sscanf(parts[0], "%g", &rankScore); err != nil {
		return nil, fmt.Errorf("parse rank_score: %w", err)
	}
	var publishedAt *time.Time
	if parts[1] != "" {
		t, err := time.Parse(time.RFC3339Nano, parts[1])
		if err != nil {
			return nil, fmt.Errorf("parse published_at: %w", err)
		}
		publishedAt = &t
	}
	asOf, err := time.Parse(time.RFC3339Nano, parts[3])
	if err != nil {
		return nil, fmt.Errorf("parse rank_as_of: %w", err)
	}
	return &homeItemCursor{
		RankScore:   rankScore,
		PublishedAt: publishedAt,
		ItemKey:     parts[2],
		AsOf:        asOf,
	}, nil
}

// cutoffFromTimeWindowAt computes the cutoff timestamp relative to reference instant now.
func cutoffFromTimeWindowAt(now time.Time, window string) (time.Time, error) {
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

func cutoffFromTimeWindow(window string) (time.Time, error) {
	return cutoffFromTimeWindowAt(time.Now().UTC(), window)
}
