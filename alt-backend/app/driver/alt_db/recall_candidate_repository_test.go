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
