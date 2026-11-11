//go:build integration

package alt_db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/domain"
	"alt/test_utils/pgtest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// FetchTagCooccurrences cannot be verified with pgxmock: a mock matches a
// regex against the SQL text and returns canned rows in whatever order the
// test handed it, so an invalid FROM-clause reference (SQLSTATE 42P01),
// self-pairs across per-feed feed_tags rows, split pair orientations, or
// unmerged COUNT(DISTINCT article_id) counts all stay invisible without a real
// PostgreSQL planner and executor.

func seedCooccurrenceFeed(t *testing.T, pool *pgxpool.Pool, label string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	feedLinkID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO feed_links (id, url) VALUES ($1, $2)`,
		feedLinkID,
		fmt.Sprintf("https://feeds.example/%s/%s/rss.xml", label, feedLinkID),
	)
	require.NoError(t, err)

	feedID := uuid.New()
	pubDate := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx,
		`INSERT INTO feeds (id, title, description, website_url, pub_date, feed_link_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		feedID,
		"Feed "+label,
		"Description "+label,
		fmt.Sprintf("https://feeds.example/%s/%s", label, feedID),
		pubDate,
		feedLinkID,
	)
	require.NoError(t, err)
	return feedID
}

func seedCooccurrenceFeedTag(t *testing.T, pool *pgxpool.Pool, id, feedID uuid.UUID, tagName string) uuid.UUID {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO feed_tags (id, feed_id, tag_name) VALUES ($1, $2, $3)`,
		id, feedID, tagName,
	)
	require.NoError(t, err)
	return id
}

func seedTaggedArticle(t *testing.T, pool *pgxpool.Pool, feedID, userID uuid.UUID, slug string, feedTagIDs ...uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	articleID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO articles (id, title, content, url, feed_id, user_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		articleID,
		"Title "+slug,
		"Content "+slug,
		fmt.Sprintf("https://articles.example/%s/%s", slug, articleID),
		feedID,
		userID,
	)
	require.NoError(t, err)

	for _, ftID := range feedTagIDs {
		_, err := pool.Exec(ctx,
			`INSERT INTO article_tags (article_id, feed_tag_id) VALUES ($1, $2)`,
			articleID, ftID,
		)
		require.NoError(t, err)
	}
	return articleID
}

func TestFetchTagCooccurrences_MergesDuplicateFeedTagsAndOrdersByOchiai(t *testing.T) {
	pool := pgtest.NewDB(t)
	repo := NewTagRepository(pool)
	ctx := context.Background()

	userID := uuid.New()
	feed1 := seedCooccurrenceFeed(t, pool, "feed-one")
	feed2 := seedCooccurrenceFeed(t, pool, "feed-two")

	// UUIDs are chosen deliberately against lexical tag_name order:
	//   ftRust1 (01) < ftTokio (50) < ftSerde (55) < ftRust2 (99)
	//   ftGamma (10) < ftBeta  (20) < ftAlpha (30)
	//   ftHubB  (40) < ftHubA  (41)
	// A query that oriented or grouped by feed_tag_id instead of tag_name would
	// emit reversed (tag_a > tag_b) pairs, split ("rust", "tokio") and
	// ("rust", "serde") across both orientations, and pair "rust" with itself.
	ftRust1 := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000001"), feed1, "rust")
	ftRust2 := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000099"), feed2, "rust")
	ftGamma := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000010"), feed1, "gamma")
	ftBeta := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000020"), feed1, "beta")
	ftAlpha := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000030"), feed1, "alpha")
	ftHubB := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000040"), feed1, "hubb")
	ftHubA := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000041"), feed1, "huba")
	ftTokio := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000050"), feed1, "tokio")
	ftSerde := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000055"), feed1, "serde")
	ftSingleton := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000060"), feed1, "singleton")
	ftUnlisted := seedCooccurrenceFeedTag(t, pool, uuid.MustParse("00000000-0000-0000-0000-000000000070"), feed1, "unlisted")

	// Duplicated "rust" cluster (4 distinct articles):
	// - r1 uses only feed1's "rust"; r2 uses only feed2's "rust".
	//   ("rust", "serde") therefore has 1 article on each feed_tags row and
	//   only reaches HAVING >= 2 (shared_count = 2) when merged by tag_name.
	// - r3 and r4 attach BOTH ftRust1 and ftRust2 to the same article, so:
	//   (1) without tt1.tag_name < tt2.tag_name, ("rust", "rust") would reach
	//       shared_count = 2 and leak into the result;
	//   (2) without COUNT(DISTINCT), ("rust", "tokio") and tag_counts("rust")
	//       would be double-counted as 6 instead of 4.
	// - ftSingleton shares only r1 (shared_count = 1 -> excluded by HAVING >= 2).
	// - ftUnlisted shares r1 and r2 (shared_count = 2) -> excluded by $1 filter.
	seedTaggedArticle(t, pool, feed1, userID, "r1", ftRust1, ftTokio, ftSerde, ftSingleton, ftUnlisted)
	seedTaggedArticle(t, pool, feed2, userID, "r2", ftRust2, ftTokio, ftSerde, ftUnlisted)
	seedTaggedArticle(t, pool, feed1, userID, "r3", ftRust1, ftRust2, ftTokio)
	seedTaggedArticle(t, pool, feed2, userID, "r4", ftRust1, ftRust2, ftTokio)

	// Tiebreak clique: 3 tags on the exact same 2 articles -> all 3 pairs have
	// Ochiai = 2 / sqrt(2 * 2) = 1.0 and shared_count = 2, exercising both
	// tag_a ASC and tag_b ASC tiebreaks.
	seedTaggedArticle(t, pool, feed1, userID, "c1", ftAlpha, ftBeta, ftGamma)
	seedTaggedArticle(t, pool, feed1, userID, "c2", ftAlpha, ftBeta, ftGamma)

	// Hub pair: highest raw shared_count (5) in the dataset, diluted by solo
	// articles so article_count("huba") = 10, article_count("hubb") = 10, and
	// Ochiai = 5 / sqrt(10 * 10) = 0.5 (lowest among surviving pairs).
	for i := 1; i <= 5; i++ {
		seedTaggedArticle(t, pool, feed1, userID, fmt.Sprintf("h-shared-%d", i), ftHubA, ftHubB)
		seedTaggedArticle(t, pool, feed1, userID, fmt.Sprintf("h-solo-a-%d", i), ftHubA)
		seedTaggedArticle(t, pool, feed1, userID, fmt.Sprintf("h-solo-b-%d", i), ftHubB)
	}

	tagNames := []string{"tokio", "rust", "serde", "alpha", "beta", "gamma", "huba", "hubb", "singleton"}
	got, err := repo.FetchTagCooccurrences(ctx, tagNames)
	require.NoError(t, err)

	seenUnordered := make(map[[2]string]struct{}, len(got))
	for _, row := range got {
		require.NotEqual(t, row.TagNameA, row.TagNameB, "tag must never pair with itself: %+v", row)
		require.Less(t, row.TagNameA, row.TagNameB, "every row must have tag_a < tag_b: %+v", row)
		require.GreaterOrEqual(t, row.SharedCount, 2, "HAVING >= 2 must filter single-article pairs: %+v", row)
		require.NotEqual(t, "singleton", row.TagNameA)
		require.NotEqual(t, "singleton", row.TagNameB)
		require.NotEqual(t, "unlisted", row.TagNameA)
		require.NotEqual(t, "unlisted", row.TagNameB)

		key := [2]string{row.TagNameA, row.TagNameB}
		_, dup := seenUnordered[key]
		require.False(t, dup, "unordered pair returned more than once: %+v", row)
		seenUnordered[key] = struct{}{}
	}

	want := []*domain.TagCooccurrence{
		// Ochiai = 4 / sqrt(4 * 4) = 1.0; ties clique on Ochiai and wins via shared_count DESC (4 > 2).
		{TagNameA: "rust", TagNameB: "tokio", SharedCount: 4},
		// Ochiai = 2 / sqrt(2 * 2) = 1.0, shared_count = 2; ordered by tag_a ASC, tag_b ASC.
		{TagNameA: "alpha", TagNameB: "beta", SharedCount: 2},
		{TagNameA: "alpha", TagNameB: "gamma", SharedCount: 2},
		{TagNameA: "beta", TagNameB: "gamma", SharedCount: 2},
		// Ochiai = 2 / sqrt(4 * 2) = 0.7071..., shared_count = 2; ordered by tag_a ASC ("rust" < "serde").
		{TagNameA: "rust", TagNameB: "serde", SharedCount: 2},
		{TagNameA: "serde", TagNameB: "tokio", SharedCount: 2},
		// Highest raw shared_count (5), ranked last because Ochiai = 5 / sqrt(10 * 10) = 0.5.
		{TagNameA: "huba", TagNameB: "hubb", SharedCount: 5},
	}
	require.Equal(t, want, got)
}
