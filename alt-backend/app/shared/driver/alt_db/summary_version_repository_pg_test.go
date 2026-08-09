//go:build integration

package alt_db

import (
	"context"
	"sync"
	"testing"
	"time"

	"alt/domain"
	"alt/test_utils/pgtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// summary_versions carries the same versioned-artifact invariant as
// tag_set_versions and the same supersede mechanics, so it gets the same
// coverage. See tag_set_version_repository_pg_test.go for why these
// properties cannot be reached through pgxmock.

func writeSummaryVersionAt(t *testing.T, repo *SummaryRepository, articleID, userID uuid.UUID, generatedAt time.Time) uuid.UUID {
	t.Helper()

	id := uuid.New()
	require.NoError(t, repo.CreateSummaryVersion(context.Background(), domain.SummaryVersion{
		SummaryVersionID: id,
		ArticleID:        articleID,
		UserID:           userID,
		GeneratedAt:      generatedAt,
		Model:            "pgtest",
		PromptVersion:    "v1",
		InputHash:        id.String(),
		SummaryText:      "summary",
	}))
	return id
}

func latestSummaries(t *testing.T, repo *SummaryRepository, articleID uuid.UUID) []uuid.UUID {
	t.Helper()

	rows, err := repo.pool.Query(context.Background(),
		`SELECT summary_version_id FROM summary_versions
		 WHERE article_id = $1 AND superseded_by IS NULL
		 ORDER BY generated_at DESC`, articleID)
	require.NoError(t, err)
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func TestMarkSummaryVersionSuperseded_SequentialKeepsExactlyOneLatest(t *testing.T) {
	repo := NewSummaryRepository(pgtest.NewDB(t))
	ctx := context.Background()

	articleID, userID := uuid.New(), uuid.New()

	v1 := writeSummaryVersionAt(t, repo, articleID, userID, epoch)
	prev, err := repo.MarkSummaryVersionSuperseded(ctx, articleID, v1)
	require.NoError(t, err)
	require.Nil(t, prev, "the first version supersedes nothing")

	v2 := writeSummaryVersionAt(t, repo, articleID, userID, epoch.Add(time.Minute))
	prev, err = repo.MarkSummaryVersionSuperseded(ctx, articleID, v2)
	require.NoError(t, err)
	require.NotNil(t, prev)
	require.Equal(t, v1, prev.SummaryVersionID)

	require.Equal(t, []uuid.UUID{v2}, latestSummaries(t, repo, articleID))
}

func TestMarkSummaryVersionSuperseded_ConcurrentKeepsExactlyOneLatest(t *testing.T) {
	repo := NewSummaryRepository(pgtest.NewDB(t))
	ctx := context.Background()

	articleID, userID := uuid.New(), uuid.New()
	writeSummaryVersionAt(t, repo, articleID, userID, epoch)

	const writers = 2

	ids := make([]uuid.UUID, writers)
	for i := range ids {
		ids[i] = writeSummaryVersionAt(t, repo, articleID, userID, epoch.Add(time.Duration(i+1)*time.Minute))
	}
	newest := ids[writers-1]

	start := make(chan struct{})
	errs := make([]error, writers)

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := range ids {
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.MarkSummaryVersionSuperseded(ctx, articleID, ids[i])
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "writer %d", i)
	}

	require.Equal(t, []uuid.UUID{newest}, latestSummaries(t, repo, articleID),
		"whichever order the two writers commit in, the newest version is the surviving one")
}

func TestMarkSummaryVersionSuperseded_LateOlderVersionLosesToTheNewer(t *testing.T) {
	repo := NewSummaryRepository(pgtest.NewDB(t))
	ctx := context.Background()

	articleID, userID := uuid.New(), uuid.New()

	original := writeSummaryVersionAt(t, repo, articleID, userID, epoch)
	_, err := repo.MarkSummaryVersionSuperseded(ctx, articleID, original)
	require.NoError(t, err)

	newer := writeSummaryVersionAt(t, repo, articleID, userID, epoch.Add(2*time.Minute))
	_, err = repo.MarkSummaryVersionSuperseded(ctx, articleID, newer)
	require.NoError(t, err)

	older := writeSummaryVersionAt(t, repo, articleID, userID, epoch.Add(time.Minute))
	prev, err := repo.MarkSummaryVersionSuperseded(ctx, articleID, older)
	require.NoError(t, err)
	require.Nil(t, prev, "a version that arrives already beaten supersedes nothing, so it must not report a predecessor it did not replace")

	require.Equal(t, []uuid.UUID{newer}, latestSummaries(t, repo, articleID))
}

func TestMarkSummaryVersionSuperseded_UnknownVersionIsAnError(t *testing.T) {
	repo := NewSummaryRepository(pgtest.NewDB(t))

	articleID, userID := uuid.New(), uuid.New()
	writeSummaryVersionAt(t, repo, articleID, userID, epoch)

	_, err := repo.MarkSummaryVersionSuperseded(context.Background(), articleID, uuid.New())
	require.Error(t, err, "naming a version that was never written is a wiring fault, not a no-op")
}
