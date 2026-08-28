package resilience

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassForEndpoint_CriticalMutation(t *testing.T) {
	for _, ep := range []string{
		"/alt.feeds.v2.FeedService/MarkAsRead",
		"/alt.feeds.v2.FeedService/MarkAsUnread",
	} {
		assert.Equal(t, ClassCriticalMutation, ClassForEndpoint(ep), ep)
	}
}

func TestClassForEndpoint_UnreadProjection(t *testing.T) {
	for _, ep := range []string{
		"/alt.feeds.v2.FeedService/GetUnreadFeeds",
		"/alt.feeds.v2.FeedService/GetUnreadCount",
		"/alt.feeds.v2.FeedService/GetAllFeeds",
	} {
		assert.Equal(t, ClassUnreadProjection, ClassForEndpoint(ep), ep)
	}
}

func TestClassForEndpoint_ExternalContent(t *testing.T) {
	assert.Equal(t, ClassExternalContent, ClassForEndpoint("/alt.articles.v2.ArticleService/FetchArticleContent"))
	assert.Equal(t, "external_content", ClassExternalContent.String())
	assert.Equal(t, 3, ExternalContentEndpointCount())
}

// TestClassForEndpoint_ArticleSiblingsStayNonCritical guards the boundary of
// the external-content class: only an RPC whose latency and status are
// dominated by a publisher belongs there. BatchPrefetchImages generates signed
// proxy URLs and reads article_heads; the image bytes are fetched on a REST
// path that never traverses the BFF. The other Article RPCs read alt-db.
func TestClassForEndpoint_ArticleSiblingsStayNonCritical(t *testing.T) {
	for _, ep := range []string{
		"/alt.articles.v2.ArticleService/BatchPrefetchImages",
		"/alt.articles.v2.ArticleService/FetchArticleSummary",
		"/alt.articles.v2.ArticleService/FetchArticlesCursor",
		"/alt.articles.v2.ArticleService/GetArticleSourceURL",
	} {
		assert.Equal(t, ClassNonCritical, ClassForEndpoint(ep), ep)
	}
}

// TestClassForEndpoint_Telemetry guards the fire-and-forget write endpoints:
// the frontend discards their result (.catch(() => {})), so a run of backend
// 5xx on one of them carries no signal about read-path health and must not
// be charged to any breaker.
func TestClassForEndpoint_Telemetry(t *testing.T) {
	for _, ep := range []string{
		"/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction",
		"/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeItemsSeen",
		"/alt.knowledge_trail.v1.KnowledgeTrailService/EmitTrailOutcome",
	} {
		assert.Equal(t, ClassTelemetry, ClassForEndpoint(ep), ep)
	}
	assert.Equal(t, "telemetry", ClassTelemetry.String())
	assert.Equal(t, 4, TelemetryEndpointCount())
}

func TestClassForEndpoint_UnclassifiedDefaultsNonCritical(t *testing.T) {
	nonCritical := []string{
		"/alt.feeds.v2.FeedService/GetFeedStats",
		"/alt.feeds.v2.FeedService/StreamSummarize",
		"/alt.augur.v2.AugurService/StreamChat",
		"/unknown/rpc",
	}
	for _, ep := range nonCritical {
		assert.Equal(t, ClassNonCritical, ClassForEndpoint(ep), ep)
	}
}

func TestIsCriticalFeedMutation(t *testing.T) {
	assert.True(t, IsCriticalFeedMutation("/alt.feeds.v2.FeedService/MarkAsRead"))
	assert.True(t, IsCriticalFeedMutation("/alt.feeds.v2.FeedService/MarkAsUnread"))
	assert.False(t, IsCriticalFeedMutation("/alt.feeds.v2.FeedService/GetUnreadFeeds"))
}

func TestUnreadProjectionEndpoints(t *testing.T) {
	eps := UnreadProjectionEndpoints()
	assert.Contains(t, eps, "/alt.feeds.v2.FeedService/GetUnreadFeeds")
	assert.Contains(t, eps, "/alt.feeds.v2.FeedService/GetUnreadCount")
	assert.Contains(t, eps, "/alt.feeds.v2.FeedService/GetAllFeeds")
	assert.Equal(t, 2, CriticalMutationEndpointCount())
	assert.Equal(t, 3, UnreadProjectionEndpointCount())
	assert.Equal(t, 5, CriticalEndpointCount())
	assert.Equal(t, 3, ExternalContentEndpointCount())
}

// TestClassForEndpoint_BatchPrefetchArticleContentIsTelemetry pins the
// classification of the article-body warm.
//
// It is deliberately NOT external_content, even though the work it triggers
// leaves the cluster. Membership in that class requires an outbound fetch *on
// the response path*, and this RPC returns an acceptance receipt before any
// publisher is contacted — its status and latency report alt-backend's health,
// never a publisher's. Charging a shared external-content budget with warms
// would let a run of dead links open the breaker in front of
// FetchArticleContent, blacking out the reads the warms exist to make faster.
//
// Telemetry is the right class for the same reason TrackHomeAction is there:
// the caller discards the result, so its failures carry no read-path signal
// and must not gate anything.
func TestClassForEndpoint_BatchPrefetchArticleContentIsTelemetry(t *testing.T) {
	assert.Equal(t, ClassTelemetry,
		ClassForEndpoint("/alt.articles.v2.ArticleService/BatchPrefetchArticleContent"))

	// The blocking read stays where it was: it is the one that waits on a
	// publisher and therefore the one the bulkhead is for.
	assert.Equal(t, ClassExternalContent,
		ClassForEndpoint("/alt.articles.v2.ArticleService/FetchArticleContent"))
	assert.Equal(t, 3, ExternalContentEndpointCount())
}

// TestClassForEndpoint_ResolveOgImagesIsExternalContent pins the on-demand
// og:image resolve to the publisher-facing bulkhead.
//
// og_image_resolve_usecase.Execute calls OgImageFetcher.FetchOgImage inline
// while the RPC is still open: for every feed that still needs one it fetches
// the publisher's robots.txt and then the page itself, through the per-host
// politeness slot. The status and latency this RPC returns therefore report a
// third party's health and Alt's own rate-limit gate, exactly the two things
// ADR-000959 removed from alt-backend's failure budget.
//
// It is not BatchPrefetchImages: that one only mints signed proxy URLs from
// rows it already holds, so nothing leaves the cluster on its response path
// and it stays non-critical (see the sibling test above).
func TestClassForEndpoint_ResolveOgImagesIsExternalContent(t *testing.T) {
	assert.Equal(t, ClassExternalContent,
		ClassForEndpoint("/alt.feeds.v2.FeedService/ResolveOgImages"))

	// The distinction that keeps this class honest: a URL-minting sibling
	// with no outbound fetch on the response path does not move with it.
	assert.Equal(t, ClassNonCritical,
		ClassForEndpoint("/alt.articles.v2.ArticleService/BatchPrefetchImages"))

	assert.Equal(t, 3, ExternalContentEndpointCount())
}

// TestClassForEndpoint_RegisterRSSFeedIsExternalContent pins feed registration
// to the publisher-facing bulkhead.
//
// RegisterFeedsUsecase.Execute opens with validateAndFetchPort.ValidateAndFetch
// and cannot get past it: before a single row is written it resolves the
// publisher's host, waits on the shared HostRateLimiter's per-host politeness
// floor, and downloads and parses the feed — all inline, with the RPC still
// open. Its own gateway says as much ("This is the external HTTP boundary for
// feed registration"). So the status and latency this RPC returns report a
// third party's health and Alt's own rate-limit gate, the two things
// ADR-000959 took out of alt-backend's failure budget.
//
// Frequency is the objection worth answering, because this endpoint is the
// opposite of ResolveOgImages: one deliberate click, not every scroll. It
// argues for the move rather than against it — see the note in
// dependency_class.go on why a consecutive-failure budget punishes the quiet
// endpoint hardest.
func TestClassForEndpoint_RegisterRSSFeedIsExternalContent(t *testing.T) {
	assert.Equal(t, ClassExternalContent,
		ClassForEndpoint("/alt.rss.v2.RSSService/RegisterRSSFeed"))

	// The near-miss that keeps the test honest, and this time it is a sibling
	// on the very same service: RegisterFavoriteFeed also takes a feed URL from
	// the user, and skips the SSRF check precisely because it "only does a DB
	// lookup by URL, it does not make external requests". Taking a third-party
	// URL is not the criterion; blocking on one before replying is. The rest of
	// RSSService reads and writes alt-db.
	for _, ep := range []string{
		"/alt.rss.v2.RSSService/RegisterFavoriteFeed",
		"/alt.rss.v2.RSSService/RemoveFavoriteFeed",
		"/alt.rss.v2.RSSService/ListRSSFeedLinks",
		"/alt.rss.v2.RSSService/DeleteRSSFeedLink",
		"/alt.rss.v2.RSSService/RandomSubscription",
	} {
		assert.Equal(t, ClassNonCritical, ClassForEndpoint(ep), ep)
	}

	assert.Equal(t, 3, ExternalContentEndpointCount())
}
