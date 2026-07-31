// Package article_repository_gateway satisfies
// fetch_article_usecase.ArticleRepository from more than one source while
// ADR-000954 Wave 3 migrates alt_db capability by capability.
//
// That interface bundles seven methods across four capability groups in the
// catalog: article reads (§2.C), the archive write (§2.B), article_heads
// (§2.D) and declined domains (§2.L). Batch 1 moves the last two. Without
// something in this shape the only options would be to migrate all four groups
// in one commit, or to leave §2.D and §2.L on the direct driver until the
// others were ready — a bigger change, or a slower one, for the same result.
//
// This is a seam, not a destination. Each later batch moves one more method to
// the data hub and shrinks localArticleStore; when the last one goes, this
// package collapses into the data-hub gateway and is deleted. The tests name
// which methods are expected on which side, so a batch that forgets to move
// one fails rather than silently keeping a database call alive.
package article_repository_gateway

import (
	"context"

	"alt/domain"
)

// localArticleStore is the part still served by the direct alt_db driver:
// catalog §2.B (SaveArticle → ArchiveArticle) and §2.C (article reads), plus
// §2.B's SaveArticleHead. Shrinks as later batches land.
type localArticleStore interface {
	FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error)
	SaveArticle(ctx context.Context, url, title, content string) (string, error)
	SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error
}

// ogImageReader is catalog §2.D, served by alt-data-hub.
type ogImageReader interface {
	FetchArticleHeadByArticleID(ctx context.Context, articleID string) (*domain.ArticleHead, error)
	FetchOgImageURLByArticleID(ctx context.Context, articleID string) (string, error)
}

// declinedDomainStore is catalog §2.L (W3-L6 / W3-L7), served by
// alt-data-hub.
type declinedDomainStore interface {
	SaveDeclinedDomain(ctx context.Context, userID, domainName string) error
	IsDomainDeclined(ctx context.Context, userID, domainName string) (bool, error)
}

// Gateway implements fetch_article_usecase.ArticleRepository.
type Gateway struct {
	local    localArticleStore
	ogImage  ogImageReader
	declined declinedDomainStore
}

// New panics on a nil collaborator.
//
// The usecase holds one interface value, so a nil source would not be visible
// until whichever of the seven methods it backs was called — on a user request
// for an article, not at boot (CLAUDE.md rule 8).
func New(local localArticleStore, ogImage ogImageReader, declined declinedDomainStore) *Gateway {
	switch {
	case local == nil:
		panic("article_repository_gateway: local article store is required")
	case ogImage == nil:
		panic("article_repository_gateway: og image reader is required — article_heads moved to alt-data-hub in ADR-000954 Wave 3")
	case declined == nil:
		panic("article_repository_gateway: declined domain store is required — declined_domains moved to alt-data-hub in ADR-000954 Wave 3")
	}
	return &Gateway{local: local, ogImage: ogImage, declined: declined}
}

func (g *Gateway) FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error) {
	return g.local.FetchArticleByURL(ctx, articleURL)
}

func (g *Gateway) SaveArticle(ctx context.Context, url, title, content string) (string, error) {
	return g.local.SaveArticle(ctx, url, title, content)
}

func (g *Gateway) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	return g.local.SaveArticleHead(ctx, articleID, headHTML, ogImageURL)
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
