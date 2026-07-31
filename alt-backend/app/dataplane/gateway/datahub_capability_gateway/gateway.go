// Package datahub_capability_gateway adapts the alt_db drivers to the
// capability ports alt-data-hub serves (ADR-000954 Wave 3, catalog §2.A /
// §2.D / §2.E / §2.L / §2.O).
//
// It is deliberately thin. The drivers already hold the transaction
// boundaries and the ON CONFLICT clauses these capabilities are drawn around
// — FOR UPDATE SKIP LOCKED in the outbox claim, the upserts in the scraping
// and cache writes — and moving that SQL is a separate step: Wave 3's exit
// condition is that alt_db has no callers outside cmd/datahub, not that the
// files have been relocated. Relocating them in the same change would mix a
// behaviour-preserving file move into a commit that also moves a process
// boundary, and the two failure modes would be indistinguishable in a bisect.
//
// What this package does add is the shape the port asks for: domain types
// instead of driver structs, a status enum instead of a bare string, and a
// returned row instead of an argument mutated in place.
package datahub_capability_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.A Outbox
// ---------------------------------------------------------------------------

// outboxDriver is the slice of alt_db the outbox capabilities use.
type outboxDriver interface {
	FetchAndLockPendingOutboxEvents(ctx context.Context, limit int) ([]alt_db.OutboxEvent, error)
	UpdateOutboxEventStatus(ctx context.Context, id string, status string, errorMessage *string) error
	PruneOutboxEvents(ctx context.Context, olderThan time.Duration) (int64, error)
}

// OutboxGateway implements datahub_capability_port.OutboxPort.
type OutboxGateway struct {
	db outboxDriver
}

func NewOutboxGateway(db *alt_db.AltDBRepository) *OutboxGateway {
	return &OutboxGateway{db: db}
}

func (g *OutboxGateway) ClaimBatch(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	rows, err := g.db.FetchAndLockPendingOutboxEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox batch: %w", err)
	}

	events := make([]domain.OutboxEvent, 0, len(rows))
	for _, r := range rows {
		events = append(events, domain.OutboxEvent{
			ID:        r.ID,
			EventType: r.EventType,
			Payload:   r.Payload,
			// The driver reports the pre-claim status it selected on. The
			// rows are PROCESSING by the time the transaction commits, and
			// that is what the caller must see: a response saying PENDING
			// would describe a row nobody else can take.
			Status:    domain.OutboxProcessing,
			CreatedAt: r.CreatedAt,
		})
	}
	return events, nil
}

func (g *OutboxGateway) MarkProcessed(ctx context.Context, id string, status domain.OutboxEventStatus, errorMessage string) error {
	var errPtr *string
	if errorMessage != "" {
		errPtr = &errorMessage
	}
	if err := g.db.UpdateOutboxEventStatus(ctx, id, string(status), errPtr); err != nil {
		return fmt.Errorf("mark outbox event %s as %s: %w", id, status, err)
	}
	return nil
}

func (g *OutboxGateway) Release(ctx context.Context, id string) error {
	if err := g.db.UpdateOutboxEventStatus(ctx, id, string(domain.OutboxPending), nil); err != nil {
		return fmt.Errorf("release outbox event %s: %w", id, err)
	}
	return nil
}

func (g *OutboxGateway) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	pruned, err := g.db.PruneOutboxEvents(ctx, olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune outbox events: %w", err)
	}
	return pruned, nil
}

// ---------------------------------------------------------------------------
// §2.D OG image / article_heads
// ---------------------------------------------------------------------------

type ogImageDriver interface {
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
	FetchArticleHeadByArticleID(ctx context.Context, articleID string) (*domain.ArticleHead, error)
	FetchOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error)
	FetchFeedsMissingOgImage(ctx context.Context, limit int) ([]alt_db.OgBackfillCandidate, error)
	FetchUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error)
	CleanupExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error)
}

// OgImageGateway implements datahub_capability_port.OgImagePort.
type OgImageGateway struct {
	db ogImageDriver
}

func NewOgImageGateway(db *alt_db.AltDBRepository) *OgImageGateway {
	return &OgImageGateway{db: db}
}

// SaveArticleHead upserts the scraped head (catalog §2.B W3-B2). It lives on
// the OG image gateway because it writes the table the §2.D reads read.
func (g *OgImageGateway) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	if err := g.db.SaveArticleHead(ctx, articleID, headHTML, ogImageURL); err != nil {
		return fmt.Errorf("save article head %s: %w", articleID, err)
	}
	return nil
}

func (g *OgImageGateway) GetArticleHead(ctx context.Context, articleID string) (*domain.ArticleHead, error) {
	head, err := g.db.FetchArticleHeadByArticleID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("get article head %s: %w", articleID, err)
	}
	return head, nil
}

func (g *OgImageGateway) BatchGetOgImageURLs(ctx context.Context, articleIDs []string) (map[string]string, error) {
	urls, err := g.db.FetchOgImageURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get og image urls: %w", err)
	}
	return urls, nil
}

func (g *OgImageGateway) ListFeedsMissingOgImage(ctx context.Context, limit int) ([]domain.OgImageBackfillCandidate, error) {
	rows, err := g.db.FetchFeedsMissingOgImage(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list feeds missing og image: %w", err)
	}

	candidates := make([]domain.OgImageBackfillCandidate, 0, len(rows))
	for _, r := range rows {
		candidates = append(candidates, domain.OgImageBackfillCandidate{ArticleID: r.ArticleID, URL: r.URL})
	}
	return candidates, nil
}

func (g *OgImageGateway) ListUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error) {
	urls, err := g.db.FetchUnwarmedOgImageURLs(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list unwarmed og image urls: %w", err)
	}
	return urls, nil
}

func (g *OgImageGateway) PurgeExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error) {
	purged, err := g.db.CleanupExpiredArticleHeads(ctx, ttl)
	if err != nil {
		return 0, fmt.Errorf("purge expired article heads: %w", err)
	}
	return purged, nil
}

// ---------------------------------------------------------------------------
// §2.E Image proxy cache
// ---------------------------------------------------------------------------

type imageProxyCacheDriver interface {
	GetImageProxyCache(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error)
	SaveImageProxyCache(ctx context.Context, entry *domain.ImageProxyCacheEntry) error
	CleanupExpiredImageProxyCache(ctx context.Context) (int64, error)
	CleanupImageProxyCacheOlderThan(ctx context.Context, ttl time.Duration) (int64, error)
}

// ImageProxyCacheGateway implements datahub_capability_port.ImageProxyCachePort.
type ImageProxyCacheGateway struct {
	db imageProxyCacheDriver
}

func NewImageProxyCacheGateway(db *alt_db.AltDBRepository) *ImageProxyCacheGateway {
	return &ImageProxyCacheGateway{db: db}
}

func (g *ImageProxyCacheGateway) Get(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error) {
	entry, err := g.db.GetImageProxyCache(ctx, urlHash)
	if err != nil {
		return nil, fmt.Errorf("get image proxy cache %s: %w", urlHash, err)
	}
	return entry, nil
}

func (g *ImageProxyCacheGateway) Put(ctx context.Context, entry *domain.ImageProxyCacheEntry) error {
	if err := g.db.SaveImageProxyCache(ctx, entry); err != nil {
		return fmt.Errorf("put image proxy cache: %w", err)
	}
	return nil
}

func (g *ImageProxyCacheGateway) EvictExpired(ctx context.Context) (int64, error) {
	evicted, err := g.db.CleanupExpiredImageProxyCache(ctx)
	if err != nil {
		return 0, fmt.Errorf("evict expired image proxy cache: %w", err)
	}
	return evicted, nil
}

func (g *ImageProxyCacheGateway) PurgeOlderThan(ctx context.Context, ttl time.Duration) (int64, error) {
	purged, err := g.db.CleanupImageProxyCacheOlderThan(ctx, ttl)
	if err != nil {
		return 0, fmt.Errorf("purge image proxy cache older than %s: %w", ttl, err)
	}
	return purged, nil
}

// ---------------------------------------------------------------------------
// §2.L Scraping policy
// ---------------------------------------------------------------------------

type scrapingDriver interface {
	GetScrapingDomainByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error)
	GetScrapingDomainByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error)
	SaveScrapingDomain(ctx context.Context, sd *domain.ScrapingDomain) error
	ListScrapingDomains(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error)
	UpdateScrapingDomainPolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error
	SaveDeclinedDomain(ctx context.Context, userID, domainName string) error
	IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error)
}

// ScrapingPolicyGateway implements datahub_capability_port.ScrapingPolicyPort.
type ScrapingPolicyGateway struct {
	db scrapingDriver
}

func NewScrapingPolicyGateway(db *alt_db.AltDBRepository) *ScrapingPolicyGateway {
	return &ScrapingPolicyGateway{db: db}
}

func (g *ScrapingPolicyGateway) GetByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error) {
	sd, err := g.db.GetScrapingDomainByDomain(ctx, domainName)
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %q: %w", domainName, err)
	}
	return sd, nil
}

func (g *ScrapingPolicyGateway) GetByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	sd, err := g.db.GetScrapingDomainByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get scraping domain %s: %w", id, err)
	}
	return sd, nil
}

// Save turns the driver's in-place mutation into a return value.
//
// SaveScrapingDomain assigns the id, created_at and updated_at by writing into
// the struct it was handed. That worked when the caller shared an address
// space with the query; over an RPC the caller's struct is a different object
// on a different host, so the assigned values have to travel back explicitly.
func (g *ScrapingPolicyGateway) Save(ctx context.Context, sd *domain.ScrapingDomain) (*domain.ScrapingDomain, error) {
	if sd == nil {
		return nil, fmt.Errorf("save scraping domain: nil domain")
	}
	saved := *sd
	if err := g.db.SaveScrapingDomain(ctx, &saved); err != nil {
		return nil, fmt.Errorf("save scraping domain %q: %w", sd.Domain, err)
	}
	return &saved, nil
}

func (g *ScrapingPolicyGateway) List(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error) {
	domains, err := g.db.ListScrapingDomains(ctx, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("list scraping domains: %w", err)
	}
	return domains, nil
}

func (g *ScrapingPolicyGateway) UpdatePolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	if err := g.db.UpdateScrapingDomainPolicy(ctx, id, update); err != nil {
		return fmt.Errorf("update scraping domain policy %s: %w", id, err)
	}
	return nil
}

func (g *ScrapingPolicyGateway) SaveDeclinedDomain(ctx context.Context, userID, domainName string) error {
	if err := g.db.SaveDeclinedDomain(ctx, userID, domainName); err != nil {
		return fmt.Errorf("save declined domain %q: %w", domainName, err)
	}
	return nil
}

func (g *ScrapingPolicyGateway) IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error) {
	declined, err := g.db.IsDomainDeclined(ctx, userID, domainName)
	if err != nil {
		return false, fmt.Errorf("check declined domain %q: %w", domainName, err)
	}
	return declined, nil
}

// ---------------------------------------------------------------------------
// §2.O Automatic full-text fetch groundwork
// ---------------------------------------------------------------------------

type autoFulltextDriver interface {
	ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error)
	CheckArticleExistsByURLForUser(ctx context.Context, url string, userID string) (bool, string, error)
}

// AutoFulltextGateway implements datahub_capability_port.AutoFulltextPort.
type AutoFulltextGateway struct {
	db autoFulltextDriver
}

func NewAutoFulltextGateway(db *alt_db.AltDBRepository) *AutoFulltextGateway {
	return &AutoFulltextGateway{db: db}
}

func (g *AutoFulltextGateway) ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error) {
	ids, err := g.db.ListSubscribedUserIDsByFeedLinkID(ctx, feedLinkID)
	if err != nil {
		return nil, fmt.Errorf("list subscribed user ids for feed link %s: %w", feedLinkID, err)
	}
	return ids, nil
}

func (g *AutoFulltextGateway) CheckArticleExistsByURLForUser(ctx context.Context, url, userID string) (bool, string, error) {
	exists, articleID, err := g.db.CheckArticleExistsByURLForUser(ctx, url, userID)
	if err != nil {
		return false, "", fmt.Errorf("check article exists for user: %w", err)
	}
	return exists, articleID, nil
}
