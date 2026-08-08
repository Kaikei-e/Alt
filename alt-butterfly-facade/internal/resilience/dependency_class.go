package resilience

// DependencyClass partitions BFF upstream RPCs so failure budgets do not cross.
type DependencyClass int

const (
	// ClassNonCritical is the default for unclassified endpoints (opt-in critical).
	ClassNonCritical DependencyClass = iota
	// ClassCriticalMutation is Critical Feed Mutations only (MarkAsRead / MarkAsUnread).
	ClassCriticalMutation
	// ClassUnreadProjection is Unread Projection Reads (list/count views).
	ClassUnreadProjection
	// ClassExternalContent is RPCs whose upstream hop leaves the cluster for a
	// third-party publisher site.
	ClassExternalContent
)

func (c DependencyClass) String() string {
	switch c {
	case ClassCriticalMutation:
		return "critical_mutation"
	case ClassUnreadProjection:
		return "unread_projection"
	case ClassExternalContent:
		return "external_content"
	default:
		return "non_critical"
	}
}

var criticalMutationEndpoints = map[string]struct{}{
	"/alt.feeds.v2.FeedService/MarkAsRead":   {},
	"/alt.feeds.v2.FeedService/MarkAsUnread": {},
}

var unreadProjectionEndpoints = map[string]struct{}{
	"/alt.feeds.v2.FeedService/GetUnreadFeeds": {},
	"/alt.feeds.v2.FeedService/GetUnreadCount": {},
	"/alt.feeds.v2.FeedService/GetAllFeeds":    {},
}

// externalContentEndpoints are RPCs whose response status and latency are
// dominated by a publisher site rather than by alt-backend: the request leaves
// the cluster, so what comes back reports the publisher's health, not ours.
// Membership requires an outbound fetch on the response path — an RPC that
// merely returns a URL pointing at a third party stays non-critical.
var externalContentEndpoints = map[string]struct{}{
	"/alt.articles.v2.ArticleService/FetchArticleContent": {},
}

// ClassForEndpoint returns the dependency class for a Connect-RPC path.
// Unclassified endpoints default to ClassNonCritical.
func ClassForEndpoint(endpoint string) DependencyClass {
	if _, ok := criticalMutationEndpoints[endpoint]; ok {
		return ClassCriticalMutation
	}
	if _, ok := unreadProjectionEndpoints[endpoint]; ok {
		return ClassUnreadProjection
	}
	if _, ok := externalContentEndpoints[endpoint]; ok {
		return ClassExternalContent
	}
	return ClassNonCritical
}

// CriticalMutationEndpointCount returns how many endpoints share the mutation breaker.
func CriticalMutationEndpointCount() int {
	return len(criticalMutationEndpoints)
}

// UnreadProjectionEndpointCount returns how many endpoints share the projection breaker.
func UnreadProjectionEndpointCount() int {
	return len(unreadProjectionEndpoints)
}

// ExternalContentEndpointCount returns how many endpoints share the
// external-content breaker.
func ExternalContentEndpointCount() int {
	return len(externalContentEndpoints)
}

// CriticalEndpointCount is mutation + projection opt-in endpoints (for startup
// logs). External content is a bulkhead, not a criticality tier, so it is
// counted separately.
func CriticalEndpointCount() int {
	return CriticalMutationEndpointCount() + UnreadProjectionEndpointCount()
}

// IsCriticalFeedMutation reports whether the endpoint is a Critical Feed Mutation.
func IsCriticalFeedMutation(endpoint string) bool {
	_, ok := criticalMutationEndpoints[endpoint]
	return ok
}

// UnreadProjectionEndpoints are the cache keys invalidated after a Critical Feed Mutation.
func UnreadProjectionEndpoints() []string {
	return []string{
		"/alt.feeds.v2.FeedService/GetUnreadFeeds",
		"/alt.feeds.v2.FeedService/GetUnreadCount",
		"/alt.feeds.v2.FeedService/GetAllFeeds",
	}
}
