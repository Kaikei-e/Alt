package sovereign_db

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

// TestPatchKnowledgeHomeItemURL_AppliesPatchOnly exercises the
// corrective-event patch path. The SQL must NOT touch title,
// summary_excerpt, tags_json, why_json, score, or any other field
// — only the `url` column (plus updated_at for monitoring).
func TestPatchKnowledgeHomeItemURL_AppliesPatchOnly(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	userID := uuid.New()
	payload, _ := json.Marshal(PatchKnowledgeHomeItemURLPayload{
		UserID:            userID.String(),
		ItemKey:           "article:11111111-1111-4111-8111-111111111111",
		ProjectionVersion: 5,
		URL:               "https://example.com/recovered-article",
	})

	mock.ExpectExec(regexp.QuoteMeta(patchKnowledgeHomeItemURLQuery)).
		WithArgs(
			"https://example.com/recovered-article",
			userID,
			"article:11111111-1111-4111-8111-111111111111",
			5,
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
