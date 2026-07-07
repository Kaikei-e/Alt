package fetch_article_summaries_usecase

import (
	"alt/domain"
	"alt/utils/batch_article_fetcher"
	"context"
	"errors"
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

func (f *fakeRepo) FetchArticleByURL(_ context.Context, url string) (*domain.ArticleContent, error) {
	if err, ok := f.byURLErr[url]; ok {
		return nil, err
	}
	return f.byURL[url], nil
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
	summaries map[string]string
	errs      map[string]error
}

func (f *fakeSummarizer) EnsureSummary(_ context.Context, articleID, _, _ string) (string, bool, error) {
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
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer)

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
	uc := NewUsecase(repo, fetcher, summarizer)

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/b"})

	require.Len(t, results, 1)
	assert.Equal(t, "summary b", results[0].Summary)
}

func TestExecute_SkipsURLsThatFailToResolve(t *testing.T) {
	repo := &fakeRepo{byURLErr: map[string]error{"https://example.com/broken": errors.New("db down")}}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, &fakeSummarizer{})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/broken"})

	assert.Empty(t, results)
}

func TestExecute_SkipsURLsWhereFetchFails(t *testing.T) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{}}
	fetcher := &fakeBatchFetcher{results: map[string]*batch_article_fetcher.FetchResult{
		"https://example.com/fail": {Error: errors.New("fetch failed")},
	}}
	uc := NewUsecase(repo, fetcher, &fakeSummarizer{})

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/fail"})

	assert.Empty(t, results)
}

func TestExecute_SkipsURLsWhereSummarizeFails(t *testing.T) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{
		"https://example.com/a": {ID: "id-a", Title: "Article A"},
	}}
	summarizer := &fakeSummarizer{errs: map[string]error{"id-a": errors.New("llm down")}}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer)

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/a"})

	assert.Empty(t, results)
}

func TestExecute_CleansMarkdownFromSummary(t *testing.T) {
	repo := &fakeRepo{byURL: map[string]*domain.ArticleContent{
		"https://example.com/a": {ID: "id-a", Title: "Article A"},
	}}
	summarizer := &fakeSummarizer{summaries: map[string]string{"id-a": "```summary with code block```"}}
	uc := NewUsecase(repo, &fakeBatchFetcher{}, summarizer)

	results := uc.Execute(context.Background(), "user-1", []string{"https://example.com/a"})

	require.Len(t, results, 1)
	assert.NotContains(t, results[0].Summary, "```")
}
