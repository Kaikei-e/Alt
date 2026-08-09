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

// The pgxmock twin of this file asserts that MarkTagSetVersionSuperseded
// issues BEGIN / pg_advisory_xact_lock / SELECT / UPDATE / COMMIT in order.
// That is all a mock can reach: it replays a recorded script, so the lock is
// only ever a string it matched. Whether the lock actually serializes two
// callers, and whether the versioned-artifact invariant survives them, is a
// property of PostgreSQL rather than of the SQL text — it needs a real server.
//
// The invariant under test: an article that has any tag set version has
// exactly one that is not superseded, and that one is its newest. An article
// where every version points at a successor has no current tags at all, and
// the read path resolves the current set by `superseded_by IS NULL`.

// epoch anchors generated_at so ordering between versions is stated by the
// test rather than inherited from how fast it runs.
var epoch = time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

func writeTagSetVersionAt(t *testing.T, repo *TagRepository, articleID, userID uuid.UUID, generatedAt time.Time) uuid.UUID {
	t.Helper()

	id := uuid.New()
	require.NoError(t, repo.CreateTagSetVersion(context.Background(), domain.TagSetVersion{
		TagSetVersionID: id,
		ArticleID:       articleID,
		UserID:          userID,
		GeneratedAt:     generatedAt,
		Generator:       "pgtest",
		InputHash:       id.String(),
		TagsJSON:        []byte(`["go","postgres"]`),
	}))
	return id
}

// latest returns the ids of every version the read path would consider
// current. More than one means the supersede chain forked; none means it ate
// its own head.
func latest(t *testing.T, repo *TagRepository, articleID uuid.UUID) []uuid.UUID {
	t.Helper()

	rows, err := repo.pool.Query(context.Background(),
		`SELECT tag_set_version_id FROM tag_set_versions
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

func TestMarkTagSetVersionSuperseded_SequentialKeepsExactlyOneLatest(t *testing.T) {
	repo := NewTagRepository(pgtest.NewDB(t))
	ctx := context.Background()

	articleID, userID := uuid.New(), uuid.New()

	v1 := writeTagSetVersionAt(t, repo, articleID, userID, epoch)
	prev, err := repo.MarkTagSetVersionSuperseded(ctx, articleID, v1)
	require.NoError(t, err)
	require.Nil(t, prev, "the first version supersedes nothing")

	v2 := writeTagSetVersionAt(t, repo, articleID, userID, epoch.Add(time.Minute))
	prev, err = repo.MarkTagSetVersionSuperseded(ctx, articleID, v2)
	require.NoError(t, err)
	require.NotNil(t, prev)
	require.Equal(t, v1, prev.TagSetVersionID)

	require.Equal(t, []uuid.UUID{v2}, latest(t, repo, articleID))
}

func TestMarkTagSetVersionSuperseded_ConcurrentKeepsExactlyOneLatest(t *testing.T) {
	repo := NewTagRepository(pgtest.NewDB(t))
	ctx := context.Background()

	articleID, userID := uuid.New(), uuid.New()
	writeTagSetVersionAt(t, repo, articleID, userID, epoch)

	// Two regenerations of one article landing together — ordinary once
	// on-the-fly tagging and the batch job can both be in flight. Both rows
	// are inserted before either supersede runs because the usecase appends a
	// knowledge event between its own insert and this call, which is more than
	// enough of a window.
	const writers = 2

	ids := make([]uuid.UUID, writers)
	for i := range ids {
		ids[i] = writeTagSetVersionAt(t, repo, articleID, userID, epoch.Add(time.Duration(i+1)*time.Minute))
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
			_, errs[i] = repo.MarkTagSetVersionSuperseded(ctx, articleID, ids[i])
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "writer %d", i)
	}

	require.Equal(t, []uuid.UUID{newest}, latest(t, repo, articleID),
		"whichever order the two writers commit in, the newest version is the surviving one")
}

// A version can reach this call after a newer one already won: the two are
// generated independently and the batch job's rows are older than what the
// on-the-fly path produces while it runs. Superseding by arrival order would
// let the loser overwrite the winner.
func TestMarkTagSetVersionSuperseded_LateOlderVersionLosesToTheNewer(t *testing.T) {
	repo := NewTagRepository(pgtest.NewDB(t))
	ctx := context.Background()

	articleID, userID := uuid.New(), uuid.New()

	original := writeTagSetVersionAt(t, repo, articleID, userID, epoch)
	_, err := repo.MarkTagSetVersionSuperseded(ctx, articleID, original)
	require.NoError(t, err)

	newer := writeTagSetVersionAt(t, repo, articleID, userID, epoch.Add(2*time.Minute))
	_, err = repo.MarkTagSetVersionSuperseded(ctx, articleID, newer)
	require.NoError(t, err)

	older := writeTagSetVersionAt(t, repo, articleID, userID, epoch.Add(time.Minute))
	prev, err := repo.MarkTagSetVersionSuperseded(ctx, articleID, older)
	require.NoError(t, err)
	require.Nil(t, prev, "a version that arrives already beaten supersedes nothing, so it must not report a predecessor it did not replace")

	require.Equal(t, []uuid.UUID{newer}, latest(t, repo, articleID))
}

func TestMarkTagSetVersionSuperseded_ScopesToItsOwnArticle(t *testing.T) {
	repo := NewTagRepository(pgtest.NewDB(t))
	ctx := context.Background()

	userID := uuid.New()
	mine, theirs := uuid.New(), uuid.New()

	writeTagSetVersionAt(t, repo, mine, userID, epoch)
	neighbour := writeTagSetVersionAt(t, repo, theirs, userID, epoch)

	v2 := writeTagSetVersionAt(t, repo, mine, userID, epoch.Add(time.Minute))
	_, err := repo.MarkTagSetVersionSuperseded(ctx, mine, v2)
	require.NoError(t, err)

	require.Equal(t, []uuid.UUID{v2}, latest(t, repo, mine))
	require.Equal(t, []uuid.UUID{neighbour}, latest(t, repo, theirs), "a neighbouring article must be untouched")
}
