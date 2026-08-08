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
	assert.Equal(t, 3, TelemetryEndpointCount())
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
