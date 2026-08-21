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
	assert.Equal(t, 1, ExternalContentEndpointCount())
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
	assert.Equal(t, 1, ExternalContentEndpointCount())
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
	assert.Equal(t, 1, ExternalContentEndpointCount())
}
