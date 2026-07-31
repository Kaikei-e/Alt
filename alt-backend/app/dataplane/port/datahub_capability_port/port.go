// Package datahub_capability_port declares what alt-data-hub needs from
// alt_db to serve the capabilities ADR-000954 Wave 3 moved off the callers'
// direct database access (catalog §2.A / §2.D / §2.E / §2.L / §2.O).
//
// One package, one interface per capability group, mirroring the single
// anti-corruption package on the consumer side. Each interface is the exact
// set of operations one group of procedures performs — nothing wider, so a
// handler cannot reach a query it has no procedure for.
package datahub_capability_port

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

// OutboxPort is the transactional outbox's state machine (catalog §2.A).
//
// Claim is a capability rather than a read: the lock and the status change
// belong to one transaction, and the interface says so by having no separate
// "mark processing" method for a caller to forget.
type OutboxPort interface {
	// ClaimBatch selects up to limit PENDING rows FOR UPDATE SKIP LOCKED and
	// marks them PROCESSING in the same transaction.
	ClaimBatch(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	// MarkProcessed records a terminal status and stamps processed_at.
	MarkProcessed(ctx context.Context, id string, status domain.OutboxEventStatus, errorMessage string) error
	// Release returns a claimed row to PENDING.
	Release(ctx context.Context, id string) error
	// Prune deletes PROCESSED rows older than the window and reports how many.
	Prune(ctx context.Context, olderThan time.Duration) (int64, error)
}

// OgImagePort covers article_heads: the scraped <head> of an article page and
// the og:image extracted from it (catalog §2.D, plus §2.B's SaveArticleHead —
// the write lands in the same table, and the scrape that produces the markup
// stays with the caller either way).
type OgImagePort interface {
	// SaveArticleHead upserts by article id. head_html is NOT NULL, so a
	// caller with no markup sends a placeholder rather than an empty string.
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
	// GetArticleHead returns nil without error when the article has never
	// been scraped — the caller re-scrapes on that answer.
	GetArticleHead(ctx context.Context, articleID string) (*domain.ArticleHead, error)
	// BatchGetOgImageURLs omits article ids with no row and rows with no
	// image, rather than mapping them to the empty string.
	BatchGetOgImageURLs(ctx context.Context, articleIDs []string) (map[string]string, error)
	// ListFeedsMissingOgImage is the backfill work list.
	ListFeedsMissingOgImage(ctx context.Context, limit int) ([]domain.OgImageBackfillCandidate, error)
	// ListUnwarmedOgImageURLs is the cache-warmer work list.
	ListUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error)
	// PurgeExpiredArticleHeads enforces the copyright retention window.
	PurgeExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error)
}

// ImageProxyCachePort is the image proxy's cache tier (catalog §2.E).
type ImageProxyCachePort interface {
	// Get returns nil without error on a miss or an expired entry; the caller
	// re-fetches either way.
	Get(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error)
	Put(ctx context.Context, entry *domain.ImageProxyCacheEntry) error
	// EvictExpired deletes entries past their own TTL.
	EvictExpired(ctx context.Context) (int64, error)
	// PurgeOlderThan deletes by first-acquisition time regardless of TTL —
	// the copyright retention cap, which is a different question from the TTL.
	PurgeOlderThan(ctx context.Context, ttl time.Duration) (int64, error)
}

// ScrapingPolicyPort is the recorded per-publisher scraping policy
// (catalog §2.L). Fetching robots.txt is not here: that is an outbound HTTP
// call and stays with alt-harvester (ADR-000954 D4).
type ScrapingPolicyPort interface {
	// GetByDomain returns nil without error for a host never recorded, which
	// the caller distinguishes from a recorded permissive policy.
	GetByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error)
	// Save upserts by hostname and returns the row as persisted, including
	// the id and timestamps the database assigned.
	Save(ctx context.Context, sd *domain.ScrapingDomain) (*domain.ScrapingDomain, error)
	List(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error)
	// UpdatePolicy applies a partial update; an absent field is left alone.
	UpdatePolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error
	// SaveDeclinedDomain records a user's refusal. Idempotent.
	SaveDeclinedDomain(ctx context.Context, userID, domainName string) error
	IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error)
}

// AutoFulltextPort is the groundwork for automatic full-text fetch
// (catalog §2.O).
type AutoFulltextPort interface {
	ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error)
	CheckArticleExistsByURLForUser(ctx context.Context, url, userID string) (bool, string, error)
}

// ArticleWritePort is the article archive (catalog §2.B).
//
// One method, because there is one write: the articles upsert and the outbox
// insert are a single transaction and the interface offers no way to do half
// of it. A port with separate "upsert" and "enqueue" methods would compile,
// and would let a caller commit an article nobody is ever told about.
type ArticleWritePort interface {
	// Archive upserts by (url, user_id) and appends the ARTICLE_UPSERT outbox
	// row in the same transaction. The bool reports whether the row was newly
	// inserted.
	Archive(ctx context.Context, url, title, content string, userID uuid.UUID) (articleID string, created bool, err error)
}

// ArticleReadPort is what alt-backend's article-serving surfaces read
// (catalog §2.C).
//
// userID is an argument on every method that has one rather than something
// read from the context: alt-data-hub serves these over Connect, where the
// peer certificate identifies alt-backend and not the person whose articles
// are being listed.
type ArticleReadPort interface {
	// GetByURL returns nil without error when the URL has not been archived.
	// A nil userID means the unscoped lookup, which is a different query and
	// not a default.
	GetByURL(ctx context.Context, url string, userID *uuid.UUID) (*domain.ArticleContent, error)
	// BatchGetByURLs omits URLs with no archived article.
	BatchGetByURLs(ctx context.Context, urls []string, userID *uuid.UUID) (map[string]*domain.ArticleContent, error)
	// GetContentByID returns nil without error for an unknown id.
	GetContentByID(ctx context.Context, articleID string) (*domain.ArticleContent, error)
	// ListCursor pages one user's articles newest first, with tags.
	ListCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*domain.Article, error)
	// ListIDsCursor is the same walk without the tag join or the bodies.
	ListIDsCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]uuid.UUID, error)
	// BatchGetByIDs preserves the requested order and omits unknown ids.
	BatchGetByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error)
	// GetLatestByFeedID returns nil without error for a feed with no articles.
	GetLatestByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error)
	// LookupURL returns "" for an article outside the user's tenant, which is
	// deliberately the same answer as for one that does not exist.
	LookupURL(ctx context.Context, articleID string, userID uuid.UUID) (string, error)
}

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

// KnowledgeBackfillPort is the alt_db half of the knowledge backfill jobs
// (catalog §2.N).
//
// Only the reads. The sovereign append, the progress bookkeeping and the
// reprojection stay with alt-backend, because talking to another service is
// the caller's business (ADR-000954 D4).
type KnowledgeBackfillPort interface {
	CountArticles(ctx context.Context) (int, error)
	// ListArticles walks (created_at, article_id) ascending. Both cursor
	// parts are nil on the first page and both are set afterwards.
	ListArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error)
	CountSummaryTitles(ctx context.Context) (int, error)
	// ListSummaryTitles walks (generated_at, summary_version_id) ascending.
	ListSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error)
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
	AllReadFeedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
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

// TagReadPort is the read half of the tag tables (catalog §2.J).
//
// Reads only. UpsertArticleTags — the write that on-the-fly generation
// performs after mq-hub answers — is already a procedure of its own from
// Wave 2, and the mq-hub call between the two stays with the caller
// (ADR-000954 D4).
type TagReadPort interface {
	// ArticleTags returns an article's tags. An untagged article is an empty
	// slice, not an error: the caller reads emptiness as "generate some".
	ArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error)
	// FeedTags pages one feed's tags newest first. A nil cursor is the first
	// page.
	FeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error)
	// Cooccurrences returns tag pairs sharing articles — the edge set the tag
	// cloud's layout consumes. The layout itself is a pure function and stays
	// with the caller.
	Cooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error)
	// SearchByPrefix is the global search box's tag section.
	SearchByPrefix(ctx context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error)
	// ArticleCounts counts one user's tagged articles since a timestamp.
	// Counts, not trending tags: which of them is a surge is arithmetic the
	// caller owns.
	ArticleCounts(ctx context.Context, userID uuid.UUID, since time.Time) ([]domain.TagArticleCount, error)
}
