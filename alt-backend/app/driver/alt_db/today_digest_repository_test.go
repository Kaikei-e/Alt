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

// digestUpsertSQL is the full UPSERT query including availability columns (10 args).
var digestUpsertSQL = regexp.QuoteMeta(`INSERT INTO today_digest_view
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
		 evening_pulse_available = EXCLUDED.evening_pulse_available OR today_digest_view.evening_pulse_available`)

func TestAltDBRepository_UpsertTodayDigest_PassesJSONAsTextForPgBouncerCompat(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	digestDate := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 18, 12, 30, 0, 0, time.UTC)

	mock.ExpectExec(digestUpsertSQL).
		WithArgs(
			userID,
			"2026-03-18",
			8,
			5,
			3,
			`["go","postgres","knowledge-home"]`,
			`[]`,
			updatedAt,
			false,
			false,
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

func TestAltDBRepository_UpsertTodayDigest_SupportsNegativeUnsummarizedDelta(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	digestDate := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 18, 13, 0, 0, 0, time.UTC)

	mock.ExpectExec(digestUpsertSQL).
		WithArgs(
			userID,
			"2026-03-18",
			0,
			1,
			-1,
			`[]`,
			`[]`,
			updatedAt,
			false,
			false,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertTodayDigest(context.Background(), domain.TodayDigest{
		UserID:               userID,
		DigestDate:           digestDate,
		SummarizedArticles:   1,
		UnsummarizedArticles: -1,
		UpdatedAt:            updatedAt,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_UpsertTodayDigest_WithAvailabilityFlags(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	userID := uuid.New()
	digestDate := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 18, 14, 0, 0, 0, time.UTC)

	mock.ExpectExec(digestUpsertSQL).
		WithArgs(
			userID,
			"2026-03-18",
			0,
			0,
			0,
			`[]`,
			`[]`,
			updatedAt,
			false,
			true,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.UpsertTodayDigest(context.Background(), domain.TodayDigest{
		UserID:                userID,
		DigestDate:            digestDate,
		EveningPulseAvailable: true,
		UpdatedAt:             updatedAt,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, true) // availability flag was properly passed
}
