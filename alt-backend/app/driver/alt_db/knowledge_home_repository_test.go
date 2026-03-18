package alt_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_UpsertKnowledgeHomeItem_PassesJSONAsTextForPgBouncerCompat(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	userID := uuid.New()
	refID := uuid.New()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO knowledge_home_items
		(user_id, tenant_id, item_key, item_type, primary_ref_id,
		 title, summary_excerpt, tags_json, why_json, score,
		 freshness_at, published_at, last_interacted_at, generated_at, updated_at,
		 projection_version, summary_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
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
						 FROM jsonb_array_elements(EXCLUDED.why_json) AS reason
						 UNION ALL
						 SELECT reason->>'code' AS code, reason, 1 AS source_rank
						 FROM jsonb_array_elements(COALESCE(knowledge_home_items.why_json, '[]'::jsonb)) AS reason
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
		 projection_version = EXCLUDED.projection_version,
		 summary_state = CASE WHEN EXCLUDED.summary_state = 'ready' THEN 'ready' WHEN EXCLUDED.summary_state != 'missing' THEN EXCLUDED.summary_state ELSE knowledge_home_items.summary_state END`)).
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
			2,
			domain.SummaryStateReady,
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

	mock.ExpectExec(`(?s)why_json = CASE.*SELECT DISTINCT ON \(candidate\.code\).*source_rank.*jsonb_array_elements\(EXCLUDED\.why_json\).*jsonb_array_elements\(COALESCE\(knowledge_home_items\.why_json, '\[\]'::jsonb\)\)`).
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
			2,
			domain.SummaryStateReady,
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
