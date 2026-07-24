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

func TestClassForEndpoint_UnclassifiedDefaultsNonCritical(t *testing.T) {
	nonCritical := []string{
		"/alt.article.v2.ArticleService/FetchArticleContent",
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
}
