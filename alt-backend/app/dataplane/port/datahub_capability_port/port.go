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
// the og:image extracted from it (catalog §2.D).
type OgImagePort interface {
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
