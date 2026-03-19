package auto_fulltext_fetch_usecase

import (
	"alt/domain"
	"alt/port/internal_article_port"
	"alt/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubFetcher struct {
	html *string
	err  error
}

func (s *stubFetcher) FetchArticleContents(ctx context.Context, articleURL string) (*string, error) {
	return s.html, s.err
}

type stubPolicy struct {
	allowed bool
	err     error
}

func (s *stubPolicy) CanFetchArticle(ctx context.Context, articleURL string) (bool, error) {
	return s.allowed, s.err
}

type stubArticleCreator struct {
	mu        sync.Mutex
	params    []internal_article_port.CreateArticleParams
	articleID string
	err       error
}

func (s *stubArticleCreator) CreateArticle(ctx context.Context, params internal_article_port.CreateArticleParams) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.params = append(s.params, params)
	if s.err != nil {
		return "", s.err
	}
	if s.articleID == "" {
		return "article-1", nil
	}
	return s.articleID, nil
}

type stubRepo struct {
	userIDs               []string
	exists                bool
	existingID            string
	isDeclined            bool
	listErr               error
	existsErr             error
	isDeclinedErr         error
	saveDeclinedCount     int
	saveArticleHeadCount  int
	savedArticleHeadID    string
	savedArticleHeadOGURL string
}

func (s *stubRepo) ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error) {
	return s.userIDs, s.listErr
}

func (s *stubRepo) CheckArticleExistsByURLForUser(ctx context.Context, url string, userID string) (bool, string, error) {
	return s.exists, s.existingID, s.existsErr
}

func (s *stubRepo) IsDomainDeclined(ctx context.Context, userID, domain string) (bool, error) {
	return s.isDeclined, s.isDeclinedErr
}

func (s *stubRepo) SaveDeclinedDomain(ctx context.Context, userID, domain string) error {
	s.saveDeclinedCount++
	return nil
}

func (s *stubRepo) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	s.saveArticleHeadCount++
	s.savedArticleHeadID = articleID
	s.savedArticleHeadOGURL = ogImageURL
	return nil
}

type stubRAG struct{}

func (s *stubRAG) RetrieveContext(ctx context.Context, query string, candidateIDs []string) ([]rag_integration_port.RagContext, error) {
	return nil, nil
}

func (s *stubRAG) UpsertArticle(ctx context.Context, input rag_integration_port.UpsertArticleInput) error {
	return nil
}

func (s *stubRAG) Answer(ctx context.Context, input rag_integration_port.AnswerInput) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func init() {
	logger.InitLogger()
}

func TestAutoFulltextFetchUsecase_Process_Success(t *testing.T) {
	rawHTML := `<html><head><meta property="og:image" content="https://cdn.example.com/og.jpg"></head><body><article><h1>Fetched Title</h1><p>` +
		`This is a long enough article body to satisfy extraction and persistence expectations in the test suite.` +
		`</p></article></body></html>`
	repo := &stubRepo{userIDs: []string{"user-1"}}
	creator := &stubArticleCreator{articleID: "article-123"}
	usecase := NewAutoFulltextFetchUsecase(
		&stubFetcher{html: &rawHTML},
		&stubPolicy{allowed: true},
		creator,
		repo,
		&stubRAG{},
	)

	feedLinkID := "feed-link-1"
	err := usecase.Process(context.Background(), []*domain.FeedItem{
		{
			Title:           "RSS Title",
			Link:            "https://example.com/articles/123?utm_source=rss",
			PublishedParsed: time.Date(2026, 3, 20, 8, 0, 0, 0, time.UTC),
			FeedLinkID:      &feedLinkID,
		},
	}, []string{"feed-1"})

	require.NoError(t, err)
	require.Len(t, creator.params, 1)
	require.Equal(t, "Fetched Title", creator.params[0].Title)
	require.Equal(t, "feed-1", creator.params[0].FeedID)
	require.Equal(t, "user-1", creator.params[0].UserID)
	require.Equal(t, "https://example.com/articles/123", creator.params[0].URL)
	require.Contains(t, creator.params[0].Content, "long enough article body")
	require.Equal(t, 1, repo.saveArticleHeadCount)
	require.Equal(t, "article-123", repo.savedArticleHeadID)
	require.Equal(t, "https://cdn.example.com/og.jpg", repo.savedArticleHeadOGURL)
}

func TestAutoFulltextFetchUsecase_Process_BlockedByPolicy(t *testing.T) {
	rawHTML := "<html><body><article><p>content</p></article></body></html>"
	repo := &stubRepo{userIDs: []string{"user-1"}}
	creator := &stubArticleCreator{}
	usecase := NewAutoFulltextFetchUsecase(
		&stubFetcher{html: &rawHTML},
		&stubPolicy{allowed: false},
		creator,
		repo,
		&stubRAG{},
	)

	feedLinkID := "feed-link-1"
	err := usecase.Process(context.Background(), []*domain.FeedItem{
		{
			Link:       "https://example.com/blocked",
			FeedLinkID: &feedLinkID,
		},
	}, []string{"feed-1"})

	require.NoError(t, err)
	require.Len(t, creator.params, 0)
	require.Equal(t, 1, repo.saveDeclinedCount)
	require.Equal(t, 0, repo.saveArticleHeadCount)
}

func TestAutoFulltextFetchUsecase_Process_FetchFailureDoesNotCreateArticle(t *testing.T) {
	repo := &stubRepo{userIDs: []string{"user-1"}}
	creator := &stubArticleCreator{}
	usecase := NewAutoFulltextFetchUsecase(
		&stubFetcher{err: errors.New("boom")},
		&stubPolicy{allowed: true},
		creator,
		repo,
		&stubRAG{},
	)

	feedLinkID := "feed-link-1"
	err := usecase.Process(context.Background(), []*domain.FeedItem{
		{
			Link:       "https://example.com/failure",
			FeedLinkID: &feedLinkID,
		},
	}, []string{"feed-1"})

	require.NoError(t, err)
	require.Len(t, creator.params, 0)
	require.Equal(t, 0, repo.saveDeclinedCount)
	require.Equal(t, 0, repo.saveArticleHeadCount)
}
