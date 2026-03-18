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

func TestAltDBRepository_UpsertTodayDigest_PassesJSONAsTextForPgBouncerCompat(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	digestDate := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 18, 12, 30, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO today_digest_view
		(user_id, digest_date, new_articles, summarized_articles,
		 unsummarized_articles, top_tags_json, pulse_refs_json, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, digest_date) DO UPDATE SET
		 new_articles = EXCLUDED.new_articles,
		 summarized_articles = EXCLUDED.summarized_articles,
		 unsummarized_articles = EXCLUDED.unsummarized_articles,
		 top_tags_json = EXCLUDED.top_tags_json,
		 pulse_refs_json = EXCLUDED.pulse_refs_json,
		 updated_at = EXCLUDED.updated_at`)).
		WithArgs(
			userID,
			"2026-03-18",
			8,
			5,
			3,
			`["go","postgres","knowledge-home"]`,
			`[]`,
			updatedAt,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertTodayDigest(context.Background(), domain.TodayDigest{
		UserID:               userID,
		DigestDate:           digestDate,
		NewArticles:          8,
		SummarizedArticles:   5,
		UnsummarizedArticles: 3,
		TopTags:              []string{"go", "postgres", "knowledge-home"},
		UpdatedAt:            updatedAt,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
