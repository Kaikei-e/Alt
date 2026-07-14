package summarize_article_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/preprocessor_summarize_port"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeArticleRepository struct {
	byURL      *domain.ArticleContent
	byURLErr   error
	savedID    string
	saveErr    error
	summary    *domain.FeedSummary
	summaryErr error
	saveSumErr error
	savedArgs  []string // articleID, userID, title, summary
}

func (f *fakeArticleRepository) FetchArticleByURL(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return f.byURL, f.byURLErr
}

func (f *fakeArticleRepository) SaveArticle(_ context.Context, _, _, _ string) (string, error) {
	return f.savedID, f.saveErr
}

func (f *fakeArticleRepository) FetchArticleSummaryByArticleID(_ context.Context, _ string) (*domain.FeedSummary, error) {
	return f.summary, f.summaryErr
}

func (f *fakeArticleRepository) SaveArticleSummary(_ context.Context, articleID, userID, title, summary string) error {
	f.savedArgs = []string{articleID, userID, title, summary}
	return f.saveSumErr
}

type fakePreProcessor struct {
	summary      string
	summarizeErr error
	jobID        string
	queueErr     error
	status       *preprocessor_summarize_port.SummarizeStatus
	statusErr    error
}

func (f *fakePreProcessor) Summarize(_ context.Context, _, _, _ string) (string, error) {
	return f.summary, f.summarizeErr
}

func (f *fakePreProcessor) StreamSummarize(_ context.Context, _, _, _ string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (f *fakePreProcessor) QueueSummarize(_ context.Context, _, _ string) (string, error) {
	return f.jobID, f.queueErr
}

func (f *fakePreProcessor) GetSummarizeStatus(_ context.Context, _ string) (*preprocessor_summarize_port.SummarizeStatus, error) {
	return f.status, f.statusErr
}

type fakeFetcher struct {
	content *string
	err     error
}

func (f *fakeFetcher) FetchArticleContents(_ context.Context, _ string) (*string, error) {
	return f.content, f.err
}

func TestEnsureArticle_ReturnsExisting(t *testing.T) {
	repo := &fakeArticleRepository{byURL: &domain.ArticleContent{ID: "id-1", Title: "Existing"}}
	uc := NewUsecase(repo, &fakePreProcessor{}, &fakeFetcher{})

	id, title, existed, err := uc.EnsureArticle(context.Background(), "https://example.com/a")

	require.NoError(t, err)
	assert.Equal(t, "id-1", id)
	assert.Equal(t, "Existing", title)
	assert.True(t, existed)
}

func TestEnsureArticle_FetchesAndSavesWhenMissing(t *testing.T) {
	html := "<html><head><title>New</title></head><body><p>Some article text long enough</p></body></html>"
	repo := &fakeArticleRepository{savedID: "new-id"}
	fetcher := &fakeFetcher{content: &html}
	uc := NewUsecase(repo, &fakePreProcessor{}, fetcher)

	id, _, existed, err := uc.EnsureArticle(context.Background(), "https://example.com/new")

	require.NoError(t, err)
	assert.Equal(t, "new-id", id)
	assert.False(t, existed)
}

func TestEnsureArticle_NilRepository(t *testing.T) {
	uc := NewUsecase(nil, &fakePreProcessor{}, &fakeFetcher{})

	_, _, _, err := uc.EnsureArticle(context.Background(), "https://example.com/a")

	require.Error(t, err)
}

func TestEnsureArticle_PropagatesFetchByURLError(t *testing.T) {
	repo := &fakeArticleRepository{byURLErr: errors.New("db down")}
	uc := NewUsecase(repo, &fakePreProcessor{}, &fakeFetcher{})

	_, _, _, err := uc.EnsureArticle(context.Background(), "https://example.com/a")

	require.Error(t, err)
}

func TestEnsureSummary_ReturnsCached(t *testing.T) {
	repo := &fakeArticleRepository{summary: &domain.FeedSummary{Summary: "cached summary"}}
	uc := NewUsecase(repo, &fakePreProcessor{}, &fakeFetcher{})

	summary, fromCache, err := uc.EnsureSummary(context.Background(), "id-1", "user-1", "Title")

	require.NoError(t, err)
	assert.True(t, fromCache)
	assert.Equal(t, "cached summary", summary)
}

func TestEnsureSummary_GeneratesAndSavesWhenNotCached(t *testing.T) {
	repo := &fakeArticleRepository{}
	pre := &fakePreProcessor{summary: "fresh summary"}
	uc := NewUsecase(repo, pre, &fakeFetcher{})

	summary, fromCache, err := uc.EnsureSummary(context.Background(), "id-1", "user-1", "Title")

	require.NoError(t, err)
	assert.False(t, fromCache)
	assert.Equal(t, "fresh summary", summary)
	require.Len(t, repo.savedArgs, 4)
	assert.Equal(t, "id-1", repo.savedArgs[0])
	assert.Equal(t, "fresh summary", repo.savedArgs[3])
}

func TestEnsureSummary_SaveFailureDoesNotFailCall(t *testing.T) {
	repo := &fakeArticleRepository{saveSumErr: errors.New("save failed")}
	pre := &fakePreProcessor{summary: "fresh summary"}
	uc := NewUsecase(repo, pre, &fakeFetcher{})

	summary, fromCache, err := uc.EnsureSummary(context.Background(), "id-1", "user-1", "Title")

	require.NoError(t, err)
	assert.False(t, fromCache)
	assert.Equal(t, "fresh summary", summary)
}

func TestEnsureSummary_PropagatesSummarizeError(t *testing.T) {
	repo := &fakeArticleRepository{}
	pre := &fakePreProcessor{summarizeErr: errors.New("llm down")}
	uc := NewUsecase(repo, pre, &fakeFetcher{})

	_, _, err := uc.EnsureSummary(context.Background(), "id-1", "user-1", "Title")

	require.Error(t, err)
}

func TestQueueSummary_ReturnsJobID(t *testing.T) {
	pre := &fakePreProcessor{jobID: "job-1"}
	uc := NewUsecase(&fakeArticleRepository{}, pre, &fakeFetcher{})

	jobID, err := uc.QueueSummary(context.Background(), "id-1", "Title")

	require.NoError(t, err)
	assert.Equal(t, "job-1", jobID)
}

func TestSummaryStatus_ReturnsStatus(t *testing.T) {
	pre := &fakePreProcessor{status: &preprocessor_summarize_port.SummarizeStatus{JobID: "job-1", Status: "completed"}}
	uc := NewUsecase(&fakeArticleRepository{}, pre, &fakeFetcher{})

	status, err := uc.SummaryStatus(context.Background(), "job-1")

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "completed", status.Status)
}
