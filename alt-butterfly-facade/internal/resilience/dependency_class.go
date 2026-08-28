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
	// ResolveOgImages fetches publisher pages inline, while the RPC is still
	// open: for each feed that still needs an image, og_image_resolve_usecase
	// asks the origin for robots.txt and then the page itself, through the
	// per-host politeness slot, before it can answer. So the status and latency
	// it returns report a third party's health and our own rate-limit gate —
	// the two things ADR-000959 took out of alt-backend's failure budget.
	//
	// Unclassified, it was charged at the internal budget (threshold 5, open
	// 30s) rather than the external-content one (20, 5s). Those numbers are
	// calibrated for a dependency we operate: a handful of slow or unreachable
	// publishers, or a batch that simply waited too long for its own host
	// slots, would black out og:image resolution for thirty seconds and count
	// as an alt-backend outage. That is the same self-inflicted loop
	// ADR-000959 §4 found behind FetchArticleContent, and the same argument
	// puts the re-probe at five seconds: the next viewport is usually a
	// different set of publishers.
	//
	// BatchPrefetchImages is the near-miss to keep it apart from: it mints
	// signed proxy URLs from rows already held and never contacts a publisher
	// on its response path, so it stays non-critical (ADR-000959 §2 says so in
	// as many words). "Deals in images from third-party sites" is not the test;
	// "waits on one before it can reply" is.
	"/alt.feeds.v2.FeedService/ResolveOgImages": {},
	// RegisterRSSFeed cannot answer without the publisher either. Step 1 of
	// RegisterFeedsUsecase.Execute is validateAndFetchPort.ValidateAndFetch and
	// nothing is written until it returns: it resolves the publisher's host,
	// waits on the shared HostRateLimiter's per-host politeness floor (CLAUDE.md
	// rule 2), then downloads and parses the feed — inline, with the RPC still
	// open. Its gateway names itself "the external HTTP boundary for feed
	// registration". So this RPC's status and latency report a third party's
	// health and our own rate-limit gate, the two things ADR-000959 took out of
	// alt-backend's failure budget.
	//
	// It is charged today. The usecase flattens every fetch failure into a bare
	// errors.New, so HandleUpstreamError finds no Connect code to preserve and
	// falls through to CodeInternal: a 500 with no X-Alt-Failure-Scope, which
	// recordOutcome books against alt-backend for a publisher that never
	// answered.
	//
	// Frequency is the objection and it deserves an answer, because this is not
	// ResolveOgImages: a reader triggers that one on every scroll, while
	// registration is a single deliberate click, so a budget of 20 looks far too
	// slack to detect anything here. It is — but detection was never this
	// budget's job (the transition logs and metrics do that), and the status quo
	// is not sharper, only crueller. The threshold counts *consecutive*
	// failures, so a quiet endpoint alone on its own breaker is hypersensitive
	// rather than sensitive: with almost no successful traffic to reset
	// consecFailures, one person retrying a dead or unreachable feed five times
	// reaches 5 unaided and has everyone else's registrations refused for the
	// next 30 seconds. One publisher being down, booked as our outage and paid
	// for by unrelated users — the exact thing this class exists to stop.
	//
	// The cost, stated plainly: unlike ClassNonCritical, which mints a breaker
	// per endpoint, this class shares a single externalContentCB, so
	// registration now sits behind the same breaker as FetchArticleContent and
	// ResolveOgImages. It improves in both directions anyway. Registration is
	// far too infrequent to land 20 consecutive failures without one of their
	// successes resetting the count, so it cannot realistically open the breaker
	// on the reads; and when the reads do open it, registration is refused for
	// 5s instead of the 30s it currently inflicts on itself.
	//
	// The rest of RSSService stays put — including the sibling that looks
	// closest, RegisterFavoriteFeed, which takes a feed URL just the same and
	// skips the SSRF check precisely because it "only does a DB lookup by URL".
	// The test is unchanged: not "handles a third-party URL", but "waits on one
	// before it can reply".
	"/alt.rss.v2.RSSService/RegisterRSSFeed": {},
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
	// BatchPrefetchArticleContent is fire-and-forget in the same sense: it
	// returns an acceptance receipt before any publisher is contacted, and the
	// reader discards it. It is deliberately not in externalContentEndpoints
	// above — membership there requires the outbound fetch to be on the
	// response path, and here the fetch happens after the response, detached.
	// Charging warms to the external-content budget would let a run of dead
	// links open the breaker in front of FetchArticleContent, blacking out the
	// reads the warms exist to speed up.
	"/alt.articles.v2.ArticleService/BatchPrefetchArticleContent": {},
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
