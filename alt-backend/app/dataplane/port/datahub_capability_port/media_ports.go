package datahub_capability_port

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

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
	// ListUnwarmedOgImageURLs is the cache-warmer work list.
	ListUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error)
	// PurgeExpiredArticleHeads enforces the copyright retention window.
	PurgeExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error)

	// GetFeedOgImageTargets answers, for feeds a reader has brought into view,
	// whether an origin request is warranted at all. Feed ids with no row are
	// omitted rather than returned blank: absent means "never asked", which is
	// the opposite of a row saying "asked, refused".
	GetFeedOgImageTargets(ctx context.Context, feedIDs []string) ([]domain.FeedOgImageTarget, error)
	// SaveFeedOgImage records one resolution outcome. An empty ogImageURL is a
	// refusal, and `refusal` says why; retryAfter of zero means not within this
	// retention window.
	SaveFeedOgImage(ctx context.Context, feedID, ogImageURL string, refusal domain.OgImageRefusal, retryAfter time.Duration) error
	// PurgeExpiredFeedOgImages enforces the copyright retention window on
	// on-demand resolutions.
	PurgeExpiredFeedOgImages(ctx context.Context, ttl time.Duration) (int64, error)
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
