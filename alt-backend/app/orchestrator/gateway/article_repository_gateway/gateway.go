// Package article_repository_gateway satisfies
// fetch_article_usecase.ArticleRepository from the three alt-data-hub gateways
// that between them cover its seven methods.
//
// That interface bundles four capability groups in the catalog: article reads
// (§2.C), the archive write (§2.B), article_heads (§2.D) and declined domains
// (§2.L). Batch 1 moved the last two and this package existed to let the
// usecase keep one interface while half of it was still a direct database
// call. Batch 2 moved the rest, so the seam is closed: every method now
// crosses the RPC boundary and there is no local source left.
//
// What remains is a composite, not a migration scaffold. The usecase asks one
// question — "what do I need to know about an article?" — and the answer comes
// from three capability gateways that are grouped by table rather than by
// caller. Collapsing them into one gateway would put the og:image reads, the
// declined-domain writes and the article archive behind a single type whose
// only organising principle is that one usecase happens to want all of them.
//
// The tests name which gateway answers which method, so a later batch that
// re-points one cannot do it silently.
package article_repository_gateway

import (
	"context"

	"alt/domain"
)

// articleStore is catalog §2.B (SaveArticle → ArchiveArticle) and §2.C
// (article reads), served by alt-data-hub since Wave 3 batch 2.
type articleStore interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
}

// ogImageStore is catalog §2.D, plus §2.B's SaveArticleHead — the write lands
// in article_heads, the table the reads read.
type ogImageStore interface {
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
	FetchArticleHeadByArticleID(ctx context.Context, articleID string) (*domain.ArticleHead, error)
	FetchOgImageURLByArticleID(ctx context.Context, articleID string) (string, error)
}

// declinedDomainStore is catalog §2.L (W3-L6 / W3-L7).
type declinedDomainStore interface {
	SaveDeclinedDomain(ctx context.Context, userID, domainName string) error
	IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error)
}

// Gateway implements fetch_article_usecase.ArticleRepository.
type Gateway struct {
	article  articleStore
	ogImage  ogImageStore
	declined declinedDomainStore
}

// New panics on a nil collaborator.
//
// The usecase holds one interface value, so a nil source would not be visible
// until whichever of the seven methods it backs was called — on a user request
// for an article, not at boot (CLAUDE.md rule 8).
func New(article articleStore, ogImage ogImageStore, declined declinedDomainStore) *Gateway {
	switch {
	case article == nil:
		panic("article_repository_gateway: article store is required — articles moved to alt-data-hub in ADR-000954 Wave 3")
	case ogImage == nil:
		panic("article_repository_gateway: og image store is required — article_heads moved to alt-data-hub in ADR-000954 Wave 3")
	case declined == nil:
		panic("article_repository_gateway: declined domain store is required — declined_domains moved to alt-data-hub in ADR-000954 Wave 3")
	}
	return &Gateway{article: article, ogImage: ogImage, declined: declined}
}

func (g *Gateway) FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error) {
	return g.article.FetchArticleByURL(ctx, articleURL)
}

func (g *Gateway) SaveArticle(ctx context.Context, url, title, content string) (string, error) {
	return g.article.SaveArticle(ctx, url, title, content)
}

func (g *Gateway) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	return g.ogImage.SaveArticleHead(ctx, articleID, headHTML, ogImageURL)
}

func (g *Gateway) FetchArticleHeadByArticleID(ctx context.Context, articleID string) (*domain.ArticleHead, error) {
	return g.ogImage.FetchArticleHeadByArticleID(ctx, articleID)
}

func (g *Gateway) FetchOgImageURLByArticleID(ctx context.Context, articleID string) (string, error) {
	return g.ogImage.FetchOgImageURLByArticleID(ctx, articleID)
}

func (g *Gateway) SaveDeclinedDomain(ctx context.Context, userID, domainName string) error {
	return g.declined.SaveDeclinedDomain(ctx, userID, domainName)
}

func (g *Gateway) IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error) {
	return g.declined.IsDomainDeclined(ctx, userID, domainName)
}
