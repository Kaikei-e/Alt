//go:build contract

package contract

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"alt/domain"
	"alt/orchestrator/connect/v2/articles"
	"alt/orchestrator/usecase/fetch_article_usecase"
	"alt/shared/usecase/fetch_tag_cloud_usecase"

	"github.com/google/uuid"
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

type pactTagCloudPort struct{}

func (pactTagCloudPort) FetchTagCloud(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
	const clusters = 8
	const clusterSize = 32
	items := make([]*domain.TagCloudItem, 0, clusters*clusterSize)
	for c := range clusters {
		for m := range clusterSize {
			items = append(items, &domain.TagCloudItem{
				TagName:      fmt.Sprintf("cluster-%02d-tag-%02d", c, m),
				ArticleCount: 1000,
			})
		}
	}
	return items, nil
}

func (pactTagCloudPort) FetchTagCloudForUser(_ context.Context, _ uuid.UUID, _ int) ([]*domain.TagCloudItem, error) {
	return nil, nil
}

func (pactTagCloudPort) FetchTagCooccurrences(_ context.Context, _ []string) ([]*domain.TagCooccurrence, error) {
	const clusters = 8
	const clusterSize = 32
	coocs := make([]*domain.TagCooccurrence, 0, 720)
	for c := range clusters {
		for i := 0; i < clusterSize; i++ {
			for step := 1; step <= 3 && i+step < clusterSize; step++ {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    fmt.Sprintf("cluster-%02d-tag-%02d", c, i),
					TagNameB:    fmt.Sprintf("cluster-%02d-tag-%02d", c, i+step),
					SharedCount: 900 - 10*i - step,
				})
			}
		}
	}
	return coocs, nil
}

// newPactArticleDeps builds the dependency set the prefetch and tag-cloud
// procedures need.
func newPactArticleDeps() articles.ArticleHandlerDeps {
	return articles.ArticleHandlerDeps{
		Article:           pactUnwiredArticleUsecase{},
		FetchTagCloud:     fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(pactTagCloudPort{}, 30*time.Minute),
		PrefetchArticle:   pactPrefetchUsecase{},
		PrefetchHostSlots: pactHostSlots{},
		PrefetchProbe:     pactStoredArticles{},
		PrefetchWiring: articles.ArticlePrefetchWiring{
			Enabled:  true,
			SlotWait: 250 * time.Millisecond,
		},
	}
}
