package alt_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upsertSQL is the full UPSERT query including dismiss and supersede columns (21 args).
var upsertSQL = regexp.QuoteMeta(`INSERT INTO knowledge_home_items
		(user_id, tenant_id, item_key, item_type, primary_ref_id,
		 title, summary_excerpt, tags_json, why_json, score,
		 freshness_at, published_at, last_interacted_at, generated_at, updated_at, dismissed_at,
		 projection_version, summary_state,
		 supersede_state, superseded_at, previous_ref_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
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
		 END`)

func TestAltDBRepository_UpsertKnowledgeHomeItem_PassesJSONAsTextForPgBouncerCompat(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	userID := uuid.New()
	refID := uuid.New()

	mock.ExpectExec(upsertSQL).
		WithArgs(
			userID,
			userID,
			"article:"+refID.String(),
			domain.ItemArticle,
			&refID,
			"PgBouncer-safe item",
			"Summary",
			`["pg","jsonb"]`,
			`[{"code":"new_unread"},{"code":"summary_completed"}]`,
			0.9,
			&now,
			&now,
			&now,
			now,
			now,
			(*time.Time)(nil),
			2,
			domain.SummaryStateReady,
			// supersede fields: nil when not set
			(*string)(nil),
			(*time.Time)(nil),
			(*string)(nil),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertKnowledgeHomeItem(context.Background(), domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          userID,
		ItemKey:           "article:" + refID.String(),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &refID,
		Title:             "PgBouncer-safe item",
		SummaryExcerpt:    "Summary",
		SummaryState:      domain.SummaryStateReady,
		Tags:              []string{"pg", "jsonb"},
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyNewUnread}, {Code: domain.WhySummaryCompleted}},
		Score:             0.9,
		FreshnessAt:       &now,
		PublishedAt:       &now,
		LastInteractedAt:  &now,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: 2,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_UpsertKnowledgeHomeItem_MergesWhyJSONByCodeInQuery(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	userID := uuid.New()
	refID := uuid.New()

	mock.ExpectExec(`(?s)why_json = CASE.*jsonb_typeof\(EXCLUDED\.why_json\) = 'array'.*jsonb_typeof\(COALESCE\(knowledge_home_items\.why_json, '\[\]'::jsonb\)\) = 'array'`).
		WithArgs(
			userID,
			userID,
			"article:"+refID.String(),
			domain.ItemArticle,
			&refID,
			"Merge-safe item",
			"Summary",
			`["pg","jsonb"]`,
			`[{"code":"tag_hotspot","tag":"AI"},{"code":"summary_completed"}]`,
			0.9,
			&now,
			&now,
			&now,
			now,
			now,
			(*time.Time)(nil),
			2,
			domain.SummaryStateReady,
			// supersede fields: nil when not set
			(*string)(nil),
			(*time.Time)(nil),
			(*string)(nil),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertKnowledgeHomeItem(context.Background(), domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          userID,
		ItemKey:           "article:" + refID.String(),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &refID,
		Title:             "Merge-safe item",
		SummaryExcerpt:    "Summary",
		SummaryState:      domain.SummaryStateReady,
		Tags:              []string{"pg", "jsonb"},
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyTagHotspot, Tag: "AI"}, {Code: domain.WhySummaryCompleted}},
		Score:             0.9,
		FreshnessAt:       &now,
		PublishedAt:       &now,
		LastInteractedAt:  &now,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: 2,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetKnowledgeHomeItems_ExcludesDismissedItems(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()

	rows := pgxmock.NewRows([]string{
		"user_id", "tenant_id", "item_key", "item_type", "primary_ref_id",
		"title", "summary_excerpt", "tags_json", "why_json", "score",
		"freshness_at", "published_at", "last_interacted_at", "generated_at", "updated_at",
		"summary_state", "link", "supersede_state", "superseded_at", "previous_ref_json",
	})

	mock.ExpectQuery(`(?s)FROM knowledge_home_items khi.*WHERE khi.user_id = \$1.*khi\.projection_version = COALESCE\(\(.*knowledge_projection_versions.*status = 'active'.*\), 1\).*AND khi.dismissed_at IS NULL.*ORDER BY khi.score DESC, khi.published_at DESC, khi.item_key DESC LIMIT \$2`).
		WithArgs(userID, 21).
		WillReturnRows(rows)

	items, nextCursor, hasMore, err := repo.GetKnowledgeHomeItems(context.Background(), userID, "", 20, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Empty(t, items)
	assert.Empty(t, nextCursor)
	assert.False(t, hasMore)
}

func TestAltDBRepository_DismissKnowledgeHomeItem_UpdatesDismissedAt(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	now := time.Date(2026, 3, 18, 12, 30, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE knowledge_home_items
		SET dismissed_at = $1, updated_at = $1
		WHERE user_id = $2 AND item_key = $3 AND projection_version = $4`)).
		WithArgs(now, userID, "article:abc", 1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = repo.DismissKnowledgeHomeItem(context.Background(), userID, "article:abc", 1, now)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_UpsertKnowledgeHomeItem_WithSupersedeState(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	userID := uuid.New()
	refID := uuid.New()

	supersedeState := domain.SupersedeSummaryUpdated
	prevRef := `{"previous_summary_excerpt":"old text"}`

	mock.ExpectExec(upsertSQL).
		WithArgs(
			userID,
			userID,
			"article:"+refID.String(),
			domain.ItemArticle,
			&refID,
			"",
			"",
			`[]`,
			`[]`,
			0.0,
			(*time.Time)(nil),
			(*time.Time)(nil),
			(*time.Time)(nil),
			now,
			now,
			(*time.Time)(nil),
			0,
			"",
			&supersedeState,
			&now,
			&prevRef,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertKnowledgeHomeItem(context.Background(), domain.KnowledgeHomeItem{
		UserID:          userID,
		TenantID:        userID,
		ItemKey:         "article:" + refID.String(),
		ItemType:        domain.ItemArticle,
		PrimaryRefID:    &refID,
		Tags:            []string{},
		WhyReasons:      []domain.WhyReason{},
		SupersedeState:  domain.SupersedeSummaryUpdated,
		SupersededAt:    &now,
		PreviousRefJSON: prevRef,
		GeneratedAt:     now,
		UpdatedAt:       now,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, domain.SupersedeSummaryUpdated, supersedeState)
}

func TestAltDBRepository_UpsertKnowledgeHomeItem_PreservesDismissedAtOnConflict(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	now := time.Date(2026, 3, 18, 12, 45, 0, 0, time.UTC)
	userID := uuid.New()
	refID := uuid.New()

	mock.ExpectExec(`(?s)updated_at = EXCLUDED\.updated_at,\s*dismissed_at = COALESCE\(knowledge_home_items\.dismissed_at, EXCLUDED\.dismissed_at\),\s*projection_version = EXCLUDED\.projection_version`).
		WithArgs(
			userID,
			userID,
			"article:"+refID.String(),
			domain.ItemArticle,
			&refID,
			"Still hidden",
			"Summary refresh",
			`["ai"]`,
			`[{"code":"summary_completed"}]`,
			0.7,
			&now,
			&now,
			(*time.Time)(nil),
			now,
			now,
			(*time.Time)(nil),
			3,
			domain.SummaryStateReady,
			(*string)(nil),
			(*time.Time)(nil),
			(*string)(nil),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertKnowledgeHomeItem(context.Background(), domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          userID,
		ItemKey:           "article:" + refID.String(),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &refID,
		Title:             "Still hidden",
		SummaryExcerpt:    "Summary refresh",
		SummaryState:      domain.SummaryStateReady,
		Tags:              []string{"ai"},
		WhyReasons:        []domain.WhyReason{{Code: domain.WhySummaryCompleted}},
		Score:             0.7,
		FreshnessAt:       &now,
		PublishedAt:       &now,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: 3,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_UpsertKnowledgeHomeItem_PreservesReadySummaryStateWhenIncomingStateEmpty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	now := time.Date(2026, 3, 18, 13, 0, 0, 0, time.UTC)
	userID := uuid.New()
	refID := uuid.New()

	mock.ExpectExec(`(?s)summary_state = CASE WHEN EXCLUDED\.summary_state = 'ready' THEN 'ready' WHEN EXCLUDED\.summary_state NOT IN \('', 'missing'\) THEN EXCLUDED\.summary_state ELSE knowledge_home_items\.summary_state END`).
		WithArgs(
			userID,
			userID,
			"article:"+refID.String(),
			domain.ItemArticle,
			&refID,
			"",
			"",
			`["ai"]`,
			`[{"code":"tag_hotspot","tag":"AI"}]`,
			0.2,
			(*time.Time)(nil),
			(*time.Time)(nil),
			(*time.Time)(nil),
			now,
			now,
			(*time.Time)(nil),
			2,
			"",
			(*string)(nil),
			(*time.Time)(nil),
			(*string)(nil),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertKnowledgeHomeItem(context.Background(), domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          userID,
		ItemKey:           "article:" + refID.String(),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &refID,
		Tags:              []string{"ai"},
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyTagHotspot, Tag: "AI"}},
		Score:             0.2,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: 2,
		SummaryState:      "",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
