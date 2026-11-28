package sovereign_db

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

func TestClearKnowledgeHomeItemSummary_ClearsSummaryWithoutWallClockSQL(t *testing.T) {
	require.NotContains(t, strings.ToUpper(clearKnowledgeHomeItemSummaryQuery), "NOW()",
		"clear summary SQL must not use wall-clock NOW()")
	require.NotContains(t, strings.ToUpper(clearKnowledgeHomeItemSummaryWithTenantQuery), "NOW()",
		"tenant clear summary SQL must not use wall-clock NOW()")
	require.Contains(t, clearKnowledgeHomeItemSummaryQuery, "summary_excerpt = ''")
	require.Contains(t, clearKnowledgeHomeItemSummaryQuery, "summary_state = 'pending'")

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	tenantID := uuid.New()
	occurredAt := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)

	payload, err := json.Marshal(ClearKnowledgeHomeItemSummaryPayload{
		UserID:            userID.String(),
		TenantID:          tenantID.String(),
		ItemKey:           "article:11111111-1111-4111-8111-111111111111",
		ProjectionVersion: 5,
		OccurredAt:        occurredAt,
	})
	require.NoError(t, err)

	mock.ExpectExec(regexp.QuoteMeta(clearKnowledgeHomeItemSummaryWithTenantQuery)).
		WithArgs(
			userID,
			"article:11111111-1111-4111-8111-111111111111",
			5,
			occurredAt,
			tenantID,
		).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.ClearKnowledgeHomeItemSummary(context.Background(), payload))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearKnowledgeHomeItemSummary_RejectsInvalidPayload(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	badPayload, _ := json.Marshal(ClearKnowledgeHomeItemSummaryPayload{
		UserID:            uuid.Nil.String(),
		ItemKey:           "article:abc",
		ProjectionVersion: 1,
		OccurredAt:        time.Now().UTC(),
	})
	require.ErrorIs(t, repo.ClearKnowledgeHomeItemSummary(context.Background(), badPayload), ErrInvalidMutationPayload)
}
