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
)

func (c DependencyClass) String() string {
	switch c {
	case ClassCriticalMutation:
		return "critical_mutation"
	case ClassUnreadProjection:
		return "unread_projection"
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

// ClassForEndpoint returns the dependency class for a Connect-RPC path.
// Unclassified endpoints default to ClassNonCritical.
func ClassForEndpoint(endpoint string) DependencyClass {
	if _, ok := criticalMutationEndpoints[endpoint]; ok {
		return ClassCriticalMutation
	}
	if _, ok := unreadProjectionEndpoints[endpoint]; ok {
		return ClassUnreadProjection
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

// CriticalEndpointCount is mutation + projection opt-in endpoints (for startup logs).
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
