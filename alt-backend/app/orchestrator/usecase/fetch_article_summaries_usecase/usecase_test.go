package fetch_article_summaries_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/summarize_quota_port"
	"alt/utils/batch_article_fetcher"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	byURL    map[string]*domain.ArticleContent
	byURLErr map[string]error
	savedIDs map[string]string
	saveErr  error
}

func (f *fakeRepo) FetchArticlesByURLs(_ context.Context, urls []string) (map[string]*domain.ArticleContent, error) {
	result := make(map[string]*domain.ArticleContent, len(urls))
	for _, url := range urls {
		if err, ok := f.byURLErr[url]; ok {
			return nil, err
		}
		if a, ok := f.byURL[url]; ok && a != nil {
			result[url] = a
		}
	}
	return result, nil
}

func (f *fakeRepo) SaveArticle(_ context.Context, url, title, _ string) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}
	if id, ok := f.savedIDs[url]; ok {
		return id, nil
	}
	return "generated-" + title, nil
}

type fakeBatchFetcher struct {
	results map[string]*batch_article_fetcher.FetchResult
}

func (f *fakeBatchFetcher) FetchMultiple(_ context.Context, urls []string) map[string]*batch_article_fetcher.FetchResult {
	out := make(map[string]*batch_article_fetcher.FetchResult, len(urls))
	for _, u := range urls {
		if r, ok := f.results[u]; ok {
			out[u] = r
		}
	}
	return out
}

type fakeSummarizer struct {
	summaries   map[string]string
	errs        map[string]error
	cached      map[string]string
	ensureCalls int
}

func (f *fakeSummarizer) GetCachedSummary(_ context.Context, articleID string) (string, bool) {
	s, ok := f.cached[articleID]
	return s, ok
}

type fakeQuota struct {
	budget  int
	charges int
}

func (f *fakeQuota) ChargeSummarization(_ context.Context) error {
	f.charges++
	if f.budget <= 0 {
		return summarize_quota_port.ErrBudgetExhausted
	}
	f.budget--
	return nil
}

func (f *fakeSummarizer) EnsureSummary(_ context.Context, articleID, _, _ string) (string, bool, error) {
	f.ensureCalls++
	if err, ok := f.errs[articleID]; ok {
		return "", false, err
	}
	return f.summaries[articleID], false, nil
}

func TestExecute_ExistingArticleGetsSummarized(t *testing.T) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{
		"https://example.com/a": {ID: "id-a", Title: "Article A"},
	}}
	summarizer := &fakeSummarizer{summaries: map[string]string{"id-a": "summary a"}}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer, &fakeQuota{budget: 1000})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/a"})

	require.Len(t, results, 1)
	assert.Equal(t, "https://example.com/a", results[0].FeedURL)
	assert.Equal(t, "summary a", results[0].Summary)
}

func TestExecute_MissingArticleIsFetchedAndSaved(t *testing.T) {
	repo := &fakeRepo{
		byURL:    map[string]*domain.ArticleContent{},
		savedIDs: map[string]string{"https://example.com/b": "id-b"},
	}
	fetcher := &fakeBatchFetcher{results: map[string]*batch_article_fetcher.FetchResult{
		"https://example.com/b": {Content: "body", Title: "Article B"},
	}}
	summarizer := &fakeSummarizer{summaries: map[string]string{"id-b": "summary b"}}
	uc := NewUsecase(repo, fetcher, summarizer, &fakeQuota{budget: 1000})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/b"})

	require.Len(t, results, 1)
	assert.Equal(t, "summary b", results[0].Summary)
}

func TestExecute_SkipsURLsThatFailToResolve(t *testing.T) {
	repo := &fakeRepo{byURLErr: map[string]error{"https://example.com/broken": errors.New("db down")}}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, &fakeSummarizer{}, &fakeQuota{budget: 1000})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/broken"})

	assert.Empty(t, results)
}

func TestExecute_SkipsURLsWhereFetchFails(t *testing.T) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{}}
	fetcher := &fakeBatchFetcher{results: map[string]*batch_article_fetcher.FetchResult{
		"https://example.com/fail": {Error: errors.New("fetch failed")},
	}}
	uc := NewUsecase(repo, fetcher, &fakeSummarizer{}, &fakeQuota{budget: 1000})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/fail"})

	assert.Empty(t, results)
}

func TestExecute_SkipsURLsWhereSummarizeFails(t *testing.T) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{
		"https://example.com/a": {ID: "id-a", Title: "Article A"},
	}}
	summarizer := &fakeSummarizer{errs: map[string]error{"id-a": errors.New("llm down")}}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer, &fakeQuota{budget: 1000})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/a"})

	assert.Empty(t, results)
}

func TestExecute_CleansMarkdownFromSummary(t *testing.T) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{
		"https://example.com/a": {ID: "id-a", Title: "Article A"},
	}}
	summarizer := &fakeSummarizer{summaries: map[string]string{"id-a": "```summary with code block```"}}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer, &fakeQuota{budget: 1000})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/a"})

	require.Len(t, results, 1)
	assert.NotContains(t, results[0].Summary, "```")
}

func manyArticles(n int, prefix string) (*fakeRepo, []string) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{}}
	urls := make([]string, 0, n)
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("https://example.com/%s%d", prefix, i)
		repo.byURL[u] = &domain.ArticleContent{ID: fmt.Sprintf("%s-%d", prefix, i), Title: u}
		urls = append(urls, u)
	}
	return repo, urls
}

func TestExecute_UncachedItemsBeyondBudgetAreSkippedWithoutGeneration(t *testing.T) {
	repo, urls := manyArticles(50, "u")
	summarizer := &fakeSummarizer{}
	quota := &fakeQuota{budget: 10}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer, quota)

	results := uc.Execute(context.Background(), "user-1", urls)

	assert.Len(t, results, 10)
	assert.Equal(t, 10, summarizer.ensureCalls)
	for i, r := range results {
		assert.Equal(t, urls[i], r.FeedURL, "items are served in request order")
	}
	assert.LessOrEqual(t, quota.charges, 11, "stops charging after the first refusal")
}

func TestExecute_CachedItemsAreReturnedAndNeverCharged(t *testing.T) {
	repo, urls := manyArticles(50, "c")
	cached := make(map[string]string, 50)
	for i := 0; i < 50; i++ {
		cached[fmt.Sprintf("c-%d", i)] = fmt.Sprintf("cached summary %d", i)
	}
	summarizer := &fakeSummarizer{cached: cached}
	quota := &fakeQuota{budget: 0}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer, quota)

	results := uc.Execute(context.Background(), "user-1", urls)

	assert.Len(t, results, 50)
	assert.Equal(t, 0, quota.charges)
	assert.Equal(t, 0, summarizer.ensureCalls)
}

func TestExecute_CachedItemsAfterExhaustionAreStillReturned(t *testing.T) {
	repo, urls := manyArticles(6, "m")
	summarizer := &fakeSummarizer{cached: map[string]string{"m-4": "cached four", "m-5": "cached five"}}
	quota := &fakeQuota{budget: 1}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer, quota)

	results := uc.Execute(context.Background(), "user-1", urls)

	got := make([]string, 0, len(results))
	for _, r := range results {
		got = append(got, r.ArticleID)
	}
	assert.Equal(t, []string{"m-0", "m-4", "m-5"}, got)
	assert.Equal(t, 1, summarizer.ensureCalls)
}

func TestNewUsecase_PanicsWithoutQuotaPort(t *testing.T) {
	assert.Panics(t, func() { NewUsecase(&fakeRepo{}, &fakeBatchFetcher{}, &fakeSummarizer{}, nil) })
}
