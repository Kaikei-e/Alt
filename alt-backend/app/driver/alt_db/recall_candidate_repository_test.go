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

func TestAltDBRepository_GetRecallCandidates_EnrichesWithHomeItem(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	firstEligibleAt := now.Add(-48 * time.Hour)
	nextSuggestAt := now.Add(-1 * time.Hour)
	updatedAt := now.Add(-30 * time.Minute)
	publishedAt := now.Add(-72 * time.Hour)

	rows := pgxmock.NewRows([]string{
		"user_id", "item_key", "recall_score", "reason_json", "next_suggest_at",
		"first_eligible_at", "snoozed_until", "updated_at", "projection_version",
		"home_item_key", "tenant_id", "item_type", "primary_ref_id", "title",
		"summary_excerpt", "tags_json", "why_json", "item_score", "published_at",
		"summary_state", "link", "fb_title", "fb_url", "fb_published_at",
	}).AddRow(
		userID,
		"article:"+articleID.String(),
		0.91,
		[]byte(`[{"type":"related_to_recent_search","description":"Recent search overlap"}]`),
		&nextSuggestAt,
		&firstEligibleAt,
		nil,
		updatedAt,
		3,
		"article:"+articleID.String(),
		tenantID.String(),
		domain.ItemArticle,
		articleID.String(),
		"Enriched recall title",
		"Enriched recall summary",
		[]byte(`["AI","Go"]`),
		[]byte(`[{"code":"summary_completed"},{"code":"tag_hotspot","tag":"AI"}]`),
		0.77,
		publishedAt,
		domain.SummaryStateReady,
		"https://example.com/article",
		nil,
		nil,
		nil,
	)

	mock.ExpectQuery(`(?s)FROM recall_candidate_view rc.*LEFT JOIN knowledge_home_items khi.*khi\.projection_version = COALESCE\(\(\s*SELECT version FROM knowledge_projection_versions\s*WHERE status = 'active'\s*ORDER BY version DESC\s*LIMIT 1\s*\), 1\).*LEFT JOIN articles art.*LEFT JOIN articles art_fallback.*WHERE rc\.user_id = \$1.*ORDER BY rc\.recall_score DESC.*LIMIT \$2`).
		WithArgs(userID, 5).
		WillReturnRows(rows)

	candidates, err := repo.GetRecallCandidates(context.Background(), userID, 5)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.NotNil(t, candidates[0].Item)
	require.Equal(t, "Enriched recall title", candidates[0].Item.Title)
	require.Equal(t, "Enriched recall summary", candidates[0].Item.SummaryExcerpt)
	require.Equal(t, []string{"AI", "Go"}, candidates[0].Item.Tags)
	require.Equal(t, domain.SummaryStateReady, candidates[0].Item.SummaryState)
	require.Equal(t, "https://example.com/article", candidates[0].Item.Link)
	require.NotNil(t, candidates[0].Item.PrimaryRefID)
	require.Equal(t, articleID, *candidates[0].Item.PrimaryRefID)
	require.Len(t, candidates[0].Item.WhyReasons, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetRecallCandidates_LeavesItemNilWhenNoHomeItemMatch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	firstEligibleAt := now.Add(-24 * time.Hour)
	nextSuggestAt := now.Add(-1 * time.Hour)
	updatedAt := now.Add(-15 * time.Minute)

	rows := pgxmock.NewRows([]string{
		"user_id", "item_key", "recall_score", "reason_json", "next_suggest_at",
		"first_eligible_at", "snoozed_until", "updated_at", "projection_version",
		"home_item_key", "tenant_id", "item_type", "primary_ref_id", "title",
		"summary_excerpt", "tags_json", "why_json", "item_score", "published_at",
		"summary_state", "link", "fb_title", "fb_url", "fb_published_at",
	}).AddRow(
		userID,
		"article:missing",
		0.42,
		[]byte(`[{"type":"opened_before_but_not_revisited","description":"Opened before"}]`),
		&nextSuggestAt,
		&firstEligibleAt,
		nil,
		updatedAt,
		3,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	mock.ExpectQuery(`(?s)FROM recall_candidate_view rc.*LEFT JOIN knowledge_home_items khi.*LEFT JOIN articles art.*LEFT JOIN articles art_fallback.*WHERE rc\.user_id = \$1.*ORDER BY rc\.recall_score DESC.*LIMIT \$2`).
		WithArgs(userID, 5).
		WillReturnRows(rows)

	candidates, err := repo.GetRecallCandidates(context.Background(), userID, 5)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Nil(t, candidates[0].Item)
	require.Equal(t, "article:missing", candidates[0].ItemKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetRecallCandidates_FallsBackToArticlesWhenHomeItemMissing(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	articleID := uuid.New()
	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	firstEligibleAt := now.Add(-24 * time.Hour)
	nextSuggestAt := now.Add(-1 * time.Hour)
	updatedAt := now.Add(-15 * time.Minute)
	publishedAt := now.Add(-7 * 24 * time.Hour)

	rows := pgxmock.NewRows([]string{
		"user_id", "item_key", "recall_score", "reason_json", "next_suggest_at",
		"first_eligible_at", "snoozed_until", "updated_at", "projection_version",
		"home_item_key", "tenant_id", "item_type", "primary_ref_id", "title",
		"summary_excerpt", "tags_json", "why_json", "item_score", "published_at",
		"summary_state", "link", "fb_title", "fb_url", "fb_published_at",
	}).AddRow(
		userID,
		"article:"+articleID.String(),
		0.73,
		[]byte(`[{"type":"opened_before_but_not_revisited","description":"Opened before"}]`),
		&nextSuggestAt,
		&firstEligibleAt,
		nil,
		updatedAt,
		3,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"Fallback article title",
		"https://example.com/fallback",
		publishedAt,
	)

	mock.ExpectQuery(`(?s)FROM recall_candidate_view rc.*LEFT JOIN knowledge_home_items khi.*LEFT JOIN articles art.*LEFT JOIN articles art_fallback.*WHERE rc\.user_id = \$1.*ORDER BY rc\.recall_score DESC.*LIMIT \$2`).
		WithArgs(userID, 5).
		WillReturnRows(rows)

	candidates, err := repo.GetRecallCandidates(context.Background(), userID, 5)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.NotNil(t, candidates[0].Item)
	require.Equal(t, "article:"+articleID.String(), candidates[0].Item.ItemKey)
	require.Equal(t, domain.ItemArticle, candidates[0].Item.ItemType)
	require.Equal(t, "Fallback article title", candidates[0].Item.Title)
	require.Equal(t, "https://example.com/fallback", candidates[0].Item.Link)
	require.Equal(t, domain.SummaryStateMissing, candidates[0].Item.SummaryState)
	require.NotNil(t, candidates[0].Item.PublishedAt)
	require.Equal(t, publishedAt, *candidates[0].Item.PublishedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_UpsertRecallCandidate_PassesJSONAsTextForPgBouncerCompat(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	updatedAt := time.Date(2026, 3, 18, 13, 0, 0, 0, time.UTC)
	nextSuggestAt := time.Date(2026, 3, 19, 9, 0, 0, 0, time.UTC)
	firstEligibleAt := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO recall_candidate_view
		(user_id, item_key, recall_score, reason_json, next_suggest_at, first_eligible_at, updated_at, projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		  recall_score = EXCLUDED.recall_score,
		  reason_json = EXCLUDED.reason_json,
		  next_suggest_at = EXCLUDED.next_suggest_at,
		  updated_at = EXCLUDED.updated_at,
		  projection_version = EXCLUDED.projection_version`)).
		WithArgs(
			userID,
			"article:1",
			0.8,
			`[{"type":"stale","description":"Needs recall"}]`,
			&nextSuggestAt,
			&firstEligibleAt,
			updatedAt,
			1,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertRecallCandidate(context.Background(), domain.RecallCandidate{
		UserID:            userID,
		ItemKey:           "article:1",
		RecallScore:       0.8,
		Reasons:           []domain.RecallReason{{Type: "stale", Description: "Needs recall"}},
		NextSuggestAt:     &nextSuggestAt,
		FirstEligibleAt:   &firstEligibleAt,
		UpdatedAt:         updatedAt,
		ProjectionVersion: 1,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
