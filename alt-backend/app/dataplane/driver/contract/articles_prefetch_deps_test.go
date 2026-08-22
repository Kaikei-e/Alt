//go:build contract

package contract

import (
	"context"
	"errors"
	"net/url"
	"time"

	"alt/domain"
	"alt/orchestrator/connect/v2/articles"
	"alt/orchestrator/usecase/fetch_article_usecase"
)

// pactPrefetchUsecase is the third-class (prefetch) ArticleUsecase the handler
// detaches its warms onto. It touches no network: the pact asserts the
// acceptance receipt the RPC returns, and what the warm does afterwards is by
// construction invisible to the caller.
type pactPrefetchUsecase struct{}

func (pactPrefetchUsecase) Execute(_ context.Context, _ string) (*string, error) {
	body := "warmed"
	return &body, nil
}

func (p pactPrefetchUsecase) FetchCompliantArticle(ctx context.Context, articleURL *url.URL, user domain.UserContext) (string, string, string, error) {
	return p.FetchCompliantArticleWithRefresh(ctx, articleURL, user, false)
}

func (pactPrefetchUsecase) FetchCompliantArticleWithRefresh(_ context.Context, _ *url.URL, _ domain.UserContext, _ bool) (string, string, string, error) {
	return "warmed", "art-001", "", nil
}

var _ fetch_article_usecase.ArticleUsecase = pactPrefetchUsecase{}

// pactHostSlots grants every turn. Refusing here would exercise the shed path
// rather than the contract.
type pactHostSlots struct{}

func (pactHostSlots) WaitForHost(_ context.Context, _ string) error { return nil }

// pactStoredArticles reports every article as absent, so a warm goes past the
// probe and the receipt counts it as accepted.
type pactStoredArticles struct{}

func (pactStoredArticles) FetchArticleByURL(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return nil, nil
}

// pactUnwiredArticleUsecase makes it obvious if a *different* ArticleService
// procedure is ever added to this pact: it fails loudly rather than returning
// a plausible zero value that would verify green.
type pactUnwiredArticleUsecase struct{ pactPrefetchUsecase }

func (pactUnwiredArticleUsecase) FetchCompliantArticleWithRefresh(_ context.Context, _ *url.URL, _ domain.UserContext, _ bool) (string, string, string, error) {
	return "", "", "", errors.New("the interactive article usecase is not wired into this verification")
}

// newPactArticleDeps builds the dependency set the prefetch procedure needs.
//
// Only the prefetch quartet is populated. The handler's other dependencies are
// left nil deliberately: this file is mounted for exactly one procedure, and a
// nil that panics if some future pact reaches a different one is a better
// outcome than a stub that answers it with something invented here.
func newPactArticleDeps() articles.ArticleHandlerDeps {
	return articles.ArticleHandlerDeps{
		Article:           pactUnwiredArticleUsecase{},
		PrefetchArticle:   pactPrefetchUsecase{},
		PrefetchHostSlots: pactHostSlots{},
		PrefetchProbe:     pactStoredArticles{},
		PrefetchWiring: articles.ArticlePrefetchWiring{
			Enabled:  true,
			SlotWait: 250 * time.Millisecond,
		},
	}
}
