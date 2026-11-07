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

// TestPatchKnowledgeHomeItemURL_AppliesPatchOnly exercises the
// corrective-event patch path. The SQL must NOT touch title,
// summary_excerpt, tags_json, why_json, score, or any other field
// — only the `url` column (plus updated_at from the event time).
func TestPatchKnowledgeHomeItemURL_AppliesPatchOnly(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	userID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"item_key":           "article:11111111-1111-4111-8111-111111111111",
		"projection_version": 5,
		"url":                "https://example.com/recovered-article",
		"occurred_at":        occurredAt.Format(time.RFC3339Nano),
	})
	require.NoError(t, err)

	mock.ExpectExec(regexp.QuoteMeta(patchKnowledgeHomeItemURLQuery)).
		WithArgs(
			"https://example.com/recovered-article",
			userID,
			"article:11111111-1111-4111-8111-111111111111",
			5,
			occurredAt,
		).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.PatchKnowledgeHomeItemURL(context.Background(), payload))
	require.NoError(t, mock.ExpectationsWereMet())

	// Structural guard: the SQL string must not mention any other entry
	// field. If a future edit adds, say, `title = $5` to the SET clause,
	// the patch loses its surgical guarantee and the test fails loudly.
	for _, forbidden := range []string{
		"title", "summary_excerpt", "tags_json", "why_json", "score",
		"freshness_at", "published_at", "last_interacted_at",
		"projection_revision", "supersede_state", "superseded_at",
		"previous_ref_json", "summary_state", "dismissed_at",
	} {
		require.NotContains(t, patchKnowledgeHomeItemURLQuery, forbidden,
			"patch SQL must NOT touch %s — that field belongs to the upsert path", forbidden)
	}
}

func TestPatchKnowledgeHomeItemURL_BindsEventOccurredAtWithoutWallClockSQL(t *testing.T) {
	require.NotContains(t, strings.ToUpper(patchKnowledgeHomeItemURLQuery), "NOW()",
		"plain URL patch query must not use wall-clock NOW() for updated_at")
	require.NotContains(t, strings.ToUpper(patchKnowledgeHomeItemURLWithTenantQuery), "NOW()",
		"tenant URL patch query must not use wall-clock NOW() for updated_at")
	require.Contains(t, patchKnowledgeHomeItemURLQuery, "updated_at = $5")
	require.Contains(t, patchKnowledgeHomeItemURLWithTenantQuery, "updated_at = $5")

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	tenantID := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 10, 30, 0, 0, time.UTC)

	tenantPayload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"tenant_id":          tenantID.String(),
		"item_key":           "article:11111111-1111-4111-8111-111111111111",
		"projection_version": 5,
		"url":                "https://example.com/recovered-article",
		"occurred_at":        occurredAt.Format(time.RFC3339Nano),
	})
	require.NoError(t, err)

	mock.ExpectExec(regexp.QuoteMeta(patchKnowledgeHomeItemURLWithTenantQuery)).
		WithArgs(
			"https://example.com/recovered-article",
			userID,
			"article:11111111-1111-4111-8111-111111111111",
			5,
			occurredAt,
			tenantID,
		).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.PatchKnowledgeHomeItemURL(context.Background(), tenantPayload))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPatchKnowledgeHomeItemURL_RejectsEmptyURL surfaces the application
// boundary check. An empty URL must be rejected with an explicit error
// rather than silently issuing a no-op UPDATE — the SQL `AND $1 <> ”`
// is the second layer of defense.
func TestPatchKnowledgeHomeItemURL_RejectsEmptyURL(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	payload, _ := json.Marshal(PatchKnowledgeHomeItemURLPayload{
		UserID:            uuid.New().String(),
		ItemKey:           "article:abc",
		ProjectionVersion: 5,
		URL:               "",
	})

	err = repo.PatchKnowledgeHomeItemURL(context.Background(), payload)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty URL")
	// Mock had no Expect set — confirming we never hit the DB.
	require.NoError(t, mock.ExpectationsWereMet())
}
