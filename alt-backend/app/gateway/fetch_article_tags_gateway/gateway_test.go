package fetch_article_tags_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/driver/mqhub_connect"
	"alt/utils/logger"
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	logger.InitLogger()
	os.Exit(m.Run())
}

// --- stubs ---

type stubDB struct {
	fetchTagsResult []*domain.FeedTag
	fetchTagsErr    error
	fetchArticle    *domain.ArticleContent
	fetchArticleErr error
	upsertCalled    bool
	upsertArticleID string
	upsertFeedID    string
	upsertTags      []alt_db.TagUpsertItem
	upsertResult    int32
	upsertErr       error
}

func (s *stubDB) FetchArticleTags(_ context.Context, _ string) ([]*domain.FeedTag, error) {
	return s.fetchTagsResult, s.fetchTagsErr
}

func (s *stubDB) FetchArticleByID(_ context.Context, _ string) (*domain.ArticleContent, error) {
	return s.fetchArticle, s.fetchArticleErr
}

func (s *stubDB) UpsertArticleTags(_ context.Context, articleID, feedID string, tags []alt_db.TagUpsertItem) (int32, error) {
	s.upsertCalled = true
	s.upsertArticleID = articleID
	s.upsertFeedID = feedID
	s.upsertTags = tags
	return s.upsertResult, s.upsertErr
}

type stubTagger struct {
	enabled      bool
	responses    []*mqhub_connect.GenerateTagsResponse
	errors       []error
	callCount    int
	lastRequests []mqhub_connect.GenerateTagsRequest
}

func (s *stubTagger) IsEnabled() bool {
	return s.enabled
}

func (s *stubTagger) GenerateTagsForArticle(_ context.Context, req mqhub_connect.GenerateTagsRequest) (*mqhub_connect.GenerateTagsResponse, error) {
	idx := s.callCount
	s.callCount++
	s.lastRequests = append(s.lastRequests, req)
	if idx < len(s.errors) && s.errors[idx] != nil {
		return nil, s.errors[idx]
	}
	if idx < len(s.responses) {
		return s.responses[idx], nil
	}
	return &mqhub_connect.GenerateTagsResponse{Success: false, ErrorMessage: "no response configured"}, nil
}

// --- tests ---

func TestFetchArticleTags_ExistingTags_ReturnsCached(t *testing.T) {
	now := time.Now()
	existingTags := []*domain.FeedTag{
		{ID: "tag-1", TagName: "golang", CreatedAt: now},
		{ID: "tag-2", TagName: "testing", CreatedAt: now},
	}

	db := &stubDB{fetchTagsResult: existingTags}
	tagger := &stubTagger{enabled: true}
	gw := newGateway(db, tagger, DefaultConfig())

	tags, err := gw.FetchArticleTags(context.Background(), "article-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].TagName != "golang" {
		t.Errorf("expected tag 'golang', got %q", tags[0].TagName)
	}
	// Tag generator should NOT be called
	if tagger.callCount != 0 {
		t.Errorf("expected tagger not to be called, got %d calls", tagger.callCount)
	}
}

func TestFetchArticleTags_NoTags_GeneratesAndPersists(t *testing.T) {
	article := &domain.ArticleContent{
		ID:      "article-1",
		Title:   "Test Article",
		Content: "Some content",
		URL:     "https://example.com/1",
		FeedID:  "feed-123",
	}

	db := &stubDB{
		fetchTagsResult: []*domain.FeedTag{},
		fetchArticle:    article,
		upsertResult:    2,
	}

	tagger := &stubTagger{
		enabled: true,
		responses: []*mqhub_connect.GenerateTagsResponse{
			{
				Success:   true,
				ArticleID: "article-1",
				Tags: []mqhub_connect.GeneratedTag{
					{ID: "t1", Name: "tech", Confidence: 0.95},
					{ID: "t2", Name: "ai", Confidence: 0.85},
				},
				InferenceMs: 120.5,
			},
		},
	}

	gw := newGateway(db, tagger, DefaultConfig())

	tags, err := gw.FetchArticleTags(context.Background(), "article-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return generated tags
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].TagName != "tech" {
		t.Errorf("expected tag 'tech', got %q", tags[0].TagName)
	}
	if tags[1].TagName != "ai" {
		t.Errorf("expected tag 'ai', got %q", tags[1].TagName)
	}

	// Should persist tags
	if !db.upsertCalled {
		t.Fatal("expected UpsertArticleTags to be called")
	}
	if db.upsertArticleID != "article-1" {
		t.Errorf("expected articleID 'article-1', got %q", db.upsertArticleID)
	}
	if db.upsertFeedID != "feed-123" {
		t.Errorf("expected feedID 'feed-123', got %q", db.upsertFeedID)
	}
	if len(db.upsertTags) != 2 {
		t.Fatalf("expected 2 upsert tags, got %d", len(db.upsertTags))
	}

	// Should pass FeedID in request
	if tagger.lastRequests[0].FeedID != "feed-123" {
		t.Errorf("expected FeedID 'feed-123' in request, got %q", tagger.lastRequests[0].FeedID)
	}
}

func TestFetchArticleTags_GenerateFails_RetriesOnce(t *testing.T) {
	article := &domain.ArticleContent{
		ID:      "article-1",
		Title:   "Test",
		Content: "Content",
		FeedID:  "feed-1",
	}

	db := &stubDB{
		fetchTagsResult: []*domain.FeedTag{},
		fetchArticle:    article,
	}

	tagger := &stubTagger{
		enabled: true,
		errors:  []error{errors.New("network error"), nil},
		responses: []*mqhub_connect.GenerateTagsResponse{
			nil, // first call fails
			{
				Success: true,
				Tags:    []mqhub_connect.GeneratedTag{{ID: "t1", Name: "retry-tag", Confidence: 0.9}},
			},
		},
	}

	cfg := DefaultConfig()
	cfg.RetryBackoff = time.Millisecond // fast for testing
	gw := newGateway(db, tagger, cfg)

	tags, err := gw.FetchArticleTags(context.Background(), "article-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should succeed on retry
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].TagName != "retry-tag" {
		t.Errorf("expected tag 'retry-tag', got %q", tags[0].TagName)
	}

	// Should have made 2 attempts
	if tagger.callCount != 2 {
		t.Errorf("expected 2 tagger calls, got %d", tagger.callCount)
	}
}

func TestFetchArticleTags_UpsertFails_StillReturnsTags(t *testing.T) {
	article := &domain.ArticleContent{
		ID:     "article-1",
		Title:  "Test",
		FeedID: "feed-1",
	}

	db := &stubDB{
		fetchTagsResult: []*domain.FeedTag{},
		fetchArticle:    article,
		upsertErr:       errors.New("db write error"),
	}

	tagger := &stubTagger{
		enabled: true,
		responses: []*mqhub_connect.GenerateTagsResponse{
			{
				Success: true,
				Tags:    []mqhub_connect.GeneratedTag{{ID: "t1", Name: "persisted-fail", Confidence: 0.8}},
			},
		},
	}

	gw := newGateway(db, tagger, DefaultConfig())

	tags, err := gw.FetchArticleTags(context.Background(), "article-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still return tags despite upsert failure
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].TagName != "persisted-fail" {
		t.Errorf("expected tag 'persisted-fail', got %q", tags[0].TagName)
	}

	// Upsert should have been attempted
	if !db.upsertCalled {
		t.Fatal("expected UpsertArticleTags to be called even though it fails")
	}
}

func TestFetchArticleTags_EmptyFeedID_SkipsUpsert(t *testing.T) {
	article := &domain.ArticleContent{
		ID:     "article-1",
		Title:  "No Feed",
		FeedID: "", // empty
	}

	db := &stubDB{
		fetchTagsResult: []*domain.FeedTag{},
		fetchArticle:    article,
	}

	tagger := &stubTagger{
		enabled: true,
		responses: []*mqhub_connect.GenerateTagsResponse{
			{
				Success: true,
				Tags:    []mqhub_connect.GeneratedTag{{ID: "t1", Name: "no-persist", Confidence: 0.7}},
			},
		},
	}

	gw := newGateway(db, tagger, DefaultConfig())

	tags, err := gw.FetchArticleTags(context.Background(), "article-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return tags
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}

	// Should NOT call upsert
	if db.upsertCalled {
		t.Error("expected UpsertArticleTags NOT to be called for empty FeedID")
	}
}

func TestFetchArticleTags_AllRetriesFail_ReturnsEmpty(t *testing.T) {
	article := &domain.ArticleContent{
		ID:     "article-1",
		Title:  "Test",
		FeedID: "feed-1",
	}

	db := &stubDB{
		fetchTagsResult: []*domain.FeedTag{},
		fetchArticle:    article,
	}

	tagger := &stubTagger{
		enabled: true,
		errors:  []error{errors.New("fail-1"), errors.New("fail-2")},
	}

	cfg := DefaultConfig()
	cfg.RetryBackoff = time.Millisecond
	gw := newGateway(db, tagger, cfg)

	tags, err := gw.FetchArticleTags(context.Background(), "article-1")
	if err != nil {
		t.Fatalf("unexpected error (should be fail-open): %v", err)
	}

	// Should return empty (fail-open)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}

	// Should have exhausted retries
	if tagger.callCount != 2 {
		t.Errorf("expected 2 attempts, got %d", tagger.callCount)
	}
}

// --- concurrent singleflight test ---

// concurrentTagger is a thread-safe tagger stub that tracks call count atomically
// and adds a small delay to simulate real tag generation latency.
type concurrentTagger struct {
	enabled   bool
	callCount atomic.Int32
	delay     time.Duration
	response  *mqhub_connect.GenerateTagsResponse
}

func (s *concurrentTagger) IsEnabled() bool { return s.enabled }

func (s *concurrentTagger) GenerateTagsForArticle(_ context.Context, _ mqhub_connect.GenerateTagsRequest) (*mqhub_connect.GenerateTagsResponse, error) {
	s.callCount.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.response, nil
}

func TestFetchArticleTags_ConcurrentSameArticle_SingleGeneration(t *testing.T) {
	article := &domain.ArticleContent{
		ID:      "article-concurrent",
		Title:   "Concurrent Test",
		Content: "Content for concurrent test",
		FeedID:  "feed-concurrent",
	}

	db := &stubDB{
		fetchTagsResult: []*domain.FeedTag{}, // no existing tags → triggers on-the-fly
		fetchArticle:    article,
		upsertResult:    1,
	}

	tagger := &concurrentTagger{
		enabled: true,
		delay:   50 * time.Millisecond, // simulate generation latency
		response: &mqhub_connect.GenerateTagsResponse{
			Success:   true,
			ArticleID: "article-concurrent",
			Tags: []mqhub_connect.GeneratedTag{
				{ID: "t1", Name: "concurrent-tag", Confidence: 0.9},
			},
			InferenceMs: 50,
		},
	}

	cfg := DefaultConfig()
	cfg.MaxRetries = 0 // no retries for this test
	gw := newGateway(db, tagger, cfg)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	results := make([][]*domain.FeedTag, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			tags, err := gw.FetchArticleTags(context.Background(), "article-concurrent")
			results[idx] = tags
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	// All goroutines should succeed
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d returned error: %v", i, err)
		}
	}

	// All goroutines should get the same tags
	for i, tags := range results {
		if len(tags) != 1 {
			t.Fatalf("goroutine %d: expected 1 tag, got %d", i, len(tags))
		}
		if tags[0].TagName != "concurrent-tag" {
			t.Errorf("goroutine %d: expected tag 'concurrent-tag', got %q", i, tags[0].TagName)
		}
	}

	// Critical assertion: GenerateTagsForArticle should be called only ONCE
	// thanks to singleflight deduplication.
	calls := int(tagger.callCount.Load())
	if calls != 1 {
		t.Errorf("expected GenerateTagsForArticle to be called exactly 1 time, got %d", calls)
	}
}
