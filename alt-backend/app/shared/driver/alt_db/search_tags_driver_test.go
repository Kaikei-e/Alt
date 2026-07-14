package alt_db

import (
	"context"
	"regexp"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTagRepository_SearchTagsByPrefix_UsesFunctionalLowerLikeQuery anchors
// the case-insensitive prefix predicate to the functional-index-friendly
// form `lower(tag_name) LIKE lower($1) || '%'`. Raw ILIKE cannot use a
// B-tree index on text columns in non-C locales (PostgreSQL 17
// indexes-types restriction), so reverting to ILIKE would silently fall back
// to a parallel seq scan on the 240k-row feed_tags table (180ms in
// production EXPLAIN ANALYZE).
func TestTagRepository_SearchTagsByPrefix_UsesFunctionalLowerLikeQuery(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)

	rows := pgxmock.NewRows([]string{"tag_name", "article_count"}).
		AddRow("AI", 42).
		AddRow("Algorithms", 7)

	// Must match the rewritten predicate: lower(...) LIKE lower($1) || '%'
	// Reject any ExpectQuery that still uses ILIKE.
	mock.ExpectQuery(regexp.QuoteMeta("lower(ft.tag_name) LIKE lower($1) || '%'")).
		WithArgs("A", 10).
		WillReturnRows(rows)

	hits, err := repo.SearchTagsByPrefix(context.Background(), "A", 10)
	require.NoError(t, err)
	assert.Len(t, hits, 2)
	assert.Equal(t, "AI", hits[0].TagName)
	assert.Equal(t, 42, hits[0].ArticleCount)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestTagRepository_SearchTagsByPrefix_NoBareILIKE is a guardrail against
// regression: the rewritten driver must not reintroduce the raw ILIKE
// pattern. Together with the test above this pins down both "what we want"
// (functional lower() LIKE) and "what we must avoid" (ILIKE).
func TestTagRepository_SearchTagsByPrefix_NoBareILIKE(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)

	// QueryMatcherRegexp is pgxmock's default — using a substring that would
	// only appear in the old (ILIKE) form ensures the test fails loudly if a
	// regression reintroduces it.
	mock.ExpectQuery(`tag_name ILIKE`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"tag_name", "article_count"}))

	_, _ = repo.SearchTagsByPrefix(context.Background(), "A", 10)

	// If the driver still emits "tag_name ILIKE ...", ExpectationsWereMet
	// returns nil (the expectation was satisfied). If the driver has been
	// rewritten to lower(...) LIKE form, the ExpectQuery above will not match
	// and ExpectationsWereMet returns an "unfulfilled expectation" error,
	// which is what we want — assert that here.
	err = mock.ExpectationsWereMet()
	assert.Error(t, err, "driver must not emit raw ILIKE — expected unfulfilled ILIKE expectation")
}
