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
		 projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		 title = CASE WHEN EXCLUDED.title != '' THEN EXCLUDED.title ELSE knowledge_home_items.title END,
		 summary_excerpt = CASE WHEN EXCLUDED.summary_excerpt != '' THEN EXCLUDED.summary_excerpt ELSE knowledge_home_items.summary_excerpt END,
		 tags_json = CASE WHEN EXCLUDED.tags_json != '[]'::jsonb THEN EXCLUDED.tags_json ELSE knowledge_home_items.tags_json END,
		 why_json = EXCLUDED.why_json,
		 score = GREATEST(EXCLUDED.score, knowledge_home_items.score),
		 freshness_at = COALESCE(EXCLUDED.freshness_at, knowledge_home_items.freshness_at),
		 published_at = COALESCE(EXCLUDED.published_at, knowledge_home_items.published_at),
		 last_interacted_at = COALESCE(EXCLUDED.last_interacted_at, knowledge_home_items.last_interacted_at),
		 updated_at = EXCLUDED.updated_at,
		 projection_version = EXCLUDED.projection_version`)).
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
