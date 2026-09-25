package datahub_capability_port

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

// FeedLinkPort is the subscription list — the URLs the collector polls
// (catalog §2.F).
//
// feed_links and feeds are different tables and this is a different port from
// FeedPort below for the same reason: a feed link is a URL somebody subscribed
// to, a feed is what polling it produced. Registration writes the first; the
// collector writes the second (ADR-000953).
type FeedLinkPort interface {
	// Register subscribes to a URL. Idempotent — the duplicate is reported,
	// not raised — and the bool says which happened.
	Register(ctx context.Context, url string) (alreadyExisted bool, err error)
	// BulkRegister subscribes to many URLs, one transaction each. Partial
	// success is the outcome, so the result names the failures rather than
	// discarding the successes.
	BulkRegister(ctx context.Context, urls []string) (registered, skipped int, failedURLs []string, err error)
	List(ctx context.Context) ([]*domain.FeedLink, error)
	// ListWithHealth joins in the availability row, absent for a link never
	// polled.
	ListWithHealth(ctx context.Context) ([]*domain.FeedLinkWithHealth, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// ResolveIDByURL returns nil without error for a URL nobody subscribes to.
	ResolveIDByURL(ctx context.Context, feedURL string) (*string, error)
	// ListDomains is the publisher-host work list the scraping policy job
	// seeds itself from.
	ListDomains(ctx context.Context) ([]domain.FeedLinkDomain, error)
	// ListPollable returns the links that are active or never assessed —
	// the collector's input.
	ListPollable(ctx context.Context) ([]domain.FeedLink, error)
	// ListForExport pairs each link with its newest feed title, for OPML.
	ListForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error)
}

// FeedLinkAvailabilityPort is the poll-health state machine (catalog §2.G).
//
// Two methods, not three. Increment-then-decide-then-disable was a
// read-modify-write the caller ran across a process boundary; RecordFailure
// closes it in one transaction, and the interface offers no way to take it
// apart again (catalog §4-4).
type FeedLinkAvailabilityPort interface {
	// RecordFailure increments the consecutive failure run and disables the
	// link in the same transaction once it reaches disableAfter. The bool
	// reports the transition to inactive, not the state.
	RecordFailure(ctx context.Context, feedURL, reason string, disableAfter int) (*domain.FeedLinkAvailability, bool, error)
	// ResetFailures clears the run and re-activates after a successful poll.
	ResetFailures(ctx context.Context, feedURL string) error
}

// FeedScope selects which slice of one user's feeds a cursor walk returns.
type FeedScope int

const (
	FeedScopeAll FeedScope = iota
	FeedScopeUnread
	FeedScopeRead
	FeedScopeFavorite
)

// FeedPort is the feeds table: what polling the subscriptions produced
// (catalog §2.H).
//
// Every read here returns driver rows rather than domain.FeedItem. The RSS
// rendering alt-backend serves to the browser — sanitised description, RFC3339
// published string — is a pure function of these columns and stays with the
// caller (ADR-000954 D4).
type FeedPort interface {
	// Register upserts a batch of collected items in one transaction, and
	// writes no articles row (ADR-000953). Results are in request order.
	Register(ctx context.Context, feeds []domain.FeedRegistration) ([]domain.FeedRegistrationResult, error)
	// ListCursor pages one user's feeds newest first within a scope.
	ListCursor(ctx context.Context, scope FeedScope, userID uuid.UUID, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedRow, error)
	// ListPage is the legacy offset pager. userID is required only when
	// unreadOnly is set — the two are different queries.
	ListPage(ctx context.Context, page int, unreadOnly bool, userID uuid.UUID) ([]*domain.FeedRow, error)
	// ListLimit returns the newest feeds unscoped. A non-positive limit means
	// the driver's standing ceiling, which is what the unbounded list was.
	ListLimit(ctx context.Context, limit int) ([]*domain.FeedRow, error)
	// GetSingle returns the most recently created feed, or nil when there are
	// none.
	GetSingle(ctx context.Context) (*domain.FeedRow, error)
	// ListByFeedLinkID returns the feeds one subscription produced.
	ListByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*domain.FeedRow, error)
	// GetSummary returns the generated summary for the article at a URL, or
	// nil when none has been generated. A nil userID is the unscoped lookup.
	GetSummary(ctx context.Context, feedURL string, userID *uuid.UUID) (*domain.FeedSummary, error)
	// GetSummaryByArticleID is the same lookup keyed by article id.
	GetSummaryByArticleID(ctx context.Context, articleID string, userID *uuid.UUID) (*domain.FeedSummary, error)
	// SearchByTitle is the SQL title search, scoped to one tenant.
	SearchByTitle(ctx context.Context, query, userID string) ([]*domain.FeedRow, error)
	// GetRandom returns a random feed that has at least one tag, or nil when
	// nothing is tagged yet.
	GetRandom(ctx context.Context) (*domain.Feed, error)
	// GetFeedURLsByArticleIDs resolves search hits back to their feeds.
	GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]domain.FeedAndArticle, error)
	// BatchGetTitlesByIDs omits unknown ids rather than mapping them to "".
	BatchGetTitlesByIDs(ctx context.Context, feedIDs []uuid.UUID) (map[uuid.UUID]string, error)
	// GetInoreaderSummariesByURLs returns imported Inoreader bodies.
	GetInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error)
}

// ReadStatePort is the per-user state kept beside the feed list: what has been
// read, what is subscribed, what is starred (catalog §2.I).
//
// One port for three tables — read_status, user_feed_subscriptions,
// favorite_feeds — because they are the same question asked three ways ("what
// is this feed to this user?") and every method here is scoped by user. A
// split would put the tenant argument in three places and make it three
// separate things to forget.
//
// userID is an argument on every method, never a context lookup. The provider
// serves these over Connect, where the peer certificate identifies alt-backend
// rather than the person whose read state is being written; a context-scoped
// user would resolve to the service account for every call.
type ReadStatePort interface {
	// MarkFeedRead marks the feed at a website_url read. A URL with no feeds
	// row is domain.ErrFeedNotFound, decided from the zero rows the upsert
	// affected rather than from a preceding SELECT (catalog §4-5).
	MarkFeedRead(ctx context.Context, feedURL string, userID uuid.UUID) error
	// MarkArticleRead is the same write reached by an article's URL, with the
	// same missing-feed answer. Two methods rather than one because the two
	// URLs are two different lookups, not one lookup with a mode flag.
	MarkArticleRead(ctx context.Context, articleURL string, userID uuid.UUID) error
	// ReadFeedIDs returns the subset of the given feed ids the user has read.
	// Unread ids are absent rather than returned with a false flag.
	ReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) ([]uuid.UUID, error)
	// AllReadFeedIDs returns the user's whole read set, capped by the driver.
	// When since is non-nil, only feed IDs read at or after since are returned.
	AllReadFeedIDs(ctx context.Context, userID uuid.UUID, since *time.Time) ([]uuid.UUID, error)
	// SubscribedFeedLinkIDs returns the feed links the user follows. Feed
	// links, not feeds: subscriptions are held against the URL somebody
	// followed.
	SubscribedFeedLinkIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	// ListSubscriptions returns every feed link with this user's follow state,
	// which is the subscription screen rather than the followed subset.
	ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error)
	// Subscribe is idempotent.
	Subscribe(ctx context.Context, userID, feedLinkID uuid.UUID) error
	// Unsubscribe is idempotent in the other direction.
	Unsubscribe(ctx context.Context, userID, feedLinkID uuid.UUID) error
	// AddFavorite stars the feed at a URL. domain.ErrFeedNotFound for a URL
	// with no feed; starring twice succeeds.
	AddFavorite(ctx context.Context, feedURL string, userID uuid.UUID) error
	// RemoveFavorite unstars it. domain.ErrFeedNotFound covers both a URL with
	// no feed and a feed the user had not starred — a distinction the caller
	// has never made.
	RemoveFavorite(ctx context.Context, feedURL string, userID uuid.UUID) error
}
