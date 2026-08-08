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
	// ClassTelemetry is fire-and-forget analytics writes whose result the
	// caller discards, so a failure carries no signal about read-path health.
	ClassTelemetry
)

func (c DependencyClass) String() string {
	switch c {
	case ClassCriticalMutation:
		return "critical_mutation"
	case ClassUnreadProjection:
		return "unread_projection"
	case ClassExternalContent:
		return "external_content"
	case ClassTelemetry:
		return "telemetry"
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

// telemetryEndpoints are fire-and-forget analytics writes: the frontend
// issues them and discards the result (.catch(() => {})), so their failures
// carry no signal about backend health and must never be charged to — or
// gated by — a shared breaker (postmortem: a burst of TrackHomeAction 500s
// tripped the non-critical breaker and blacked out unrelated reads).
var telemetryEndpoints = map[string]struct{}{
	"/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction":    {},
	"/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeItemsSeen": {},
	"/alt.knowledge_trail.v1.KnowledgeTrailService/EmitTrailOutcome": {},
}

// ClassForEndpoint returns the dependency class for a Connect-RPC path.
// Unclassified endpoints default to ClassNonCritical.
//
// Unlike the other classes, ClassNonCritical is not a semantic grouping of
// endpoints that are meant to share a failure budget -- it is everything
// nobody has classified yet, an open-ended and ever-growing set. Enumerating
// each new read endpoint into its own class does not scale and reliably
// leaves gaps (adversarial review: only the three telemetry endpoints were
// carved out here, leaving KnowledgeHome / Search / FetchArticlesByTag and
// everything else still sharing one budget). The caller (BFFHandler) is
// therefore required to give every ClassNonCritical endpoint its own,
// independent circuit breaker instance rather than one shared per class, so
// this label is a log/metrics grouping only, never a shared-budget one.
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
	if _, ok := telemetryEndpoints[endpoint]; ok {
		return ClassTelemetry
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

// TelemetryEndpointCount returns how many fire-and-forget endpoints are
// exempt from breaker gating.
func TelemetryEndpointCount() int {
	return len(telemetryEndpoints)
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
