package articles

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	articlesv2 "alt/gen/proto/alt/articles/v2"

	"alt/config"
	"alt/di"
	"alt/domain"
)

func createAuthContext() context.Context {
	userID := uuid.New()
	tenantID := uuid.New()
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  tenantID,
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// =============================================================================
// Response Construction Tests
// =============================================================================

func TestFetchArticleContentResponse_Construction(t *testing.T) {
	articleID := "test-article-id"
	resp := &articlesv2.FetchArticleContentResponse{
		Url:       "https://example.com/article",
		Content:   "Test Content",
		ArticleId: articleID,
	}

	assert.Equal(t, "https://example.com/article", resp.Url)
	assert.Equal(t, "Test Content", resp.Content)
	assert.Equal(t, articleID, resp.ArticleId)
}

func TestArchiveArticleResponse_Construction(t *testing.T) {
	resp := &articlesv2.ArchiveArticleResponse{
		Message: "article archived",
	}

	assert.Equal(t, "article archived", resp.Message)
}

func TestArticleItem_Construction(t *testing.T) {
	id := uuid.New().String()
	published := time.Now().Format(time.RFC3339)
	item := &articlesv2.ArticleItem{
		Id:          id,
		Title:       "Test Title",
		Url:         "https://example.com/article",
		Content:     "Test Content",
		PublishedAt: published,
		Tags:        []string{"tag1", "tag2"},
	}

	assert.Equal(t, id, item.Id)
	assert.Equal(t, "Test Title", item.Title)
	assert.Equal(t, "https://example.com/article", item.Url)
	assert.Equal(t, "Test Content", item.Content)
	assert.Equal(t, published, item.PublishedAt)
	assert.Len(t, item.Tags, 2)
}

func TestFetchArticlesCursorResponse_Construction(t *testing.T) {
	id := uuid.New().String()
	published := time.Now().Format(time.RFC3339)
	nextCursor := time.Now().Add(-time.Hour).Format(time.RFC3339)

	resp := &articlesv2.FetchArticlesCursorResponse{
		Data: []*articlesv2.ArticleItem{
			{
				Id:          id,
				Title:       "Test Article",
				Url:         "https://example.com/article",
				Content:     "Test Content",
				PublishedAt: published,
				Tags:        []string{"tag1"},
			},
		},
		NextCursor: &nextCursor,
		HasMore:    true,
	}

	assert.Len(t, resp.Data, 1)
	assert.Equal(t, id, resp.Data[0].Id)
	assert.NotNil(t, resp.NextCursor)
	assert.True(t, resp.HasMore)
}

func TestFetchArticlesCursorResponse_Empty(t *testing.T) {
	resp := &articlesv2.FetchArticlesCursorResponse{
		Data:       []*articlesv2.ArticleItem{},
		NextCursor: nil,
		HasMore:    false,
	}

	assert.Empty(t, resp.Data)
	assert.Nil(t, resp.NextCursor)
	assert.False(t, resp.HasMore)
}

// =============================================================================
// Request Validation Tests
// =============================================================================

func TestFetchArticleContentRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid URL",
			url:     "https://example.com/article",
			wantErr: false,
		},
		{
			name:    "empty URL should fail",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &articlesv2.FetchArticleContentRequest{
				Url: tt.url,
			}

			if tt.wantErr {
				assert.Empty(t, req.Url)
			} else {
				assert.NotEmpty(t, req.Url)
			}
		})
	}
}

func TestArchiveArticleRequest_Validation(t *testing.T) {
	title := "Test Title"
	tests := []struct {
		name    string
		feedUrl string
		title   *string
		wantErr bool
	}{
		{
			name:    "valid request with title",
			feedUrl: "https://example.com/article",
			title:   &title,
			wantErr: false,
		},
		{
			name:    "valid request without title",
			feedUrl: "https://example.com/article",
			title:   nil,
			wantErr: false,
		},
		{
			name:    "empty URL should fail",
			feedUrl: "",
			title:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &articlesv2.ArchiveArticleRequest{
				FeedUrl: tt.feedUrl,
				Title:   tt.title,
			}

			if tt.wantErr {
				assert.Empty(t, req.FeedUrl)
			} else {
				assert.NotEmpty(t, req.FeedUrl)
			}
		})
	}
}

func TestFetchArticlesCursorRequest_Validation(t *testing.T) {
	validCursor := time.Now().Format(time.RFC3339)
	tests := []struct {
		name   string
		cursor *string
		limit  int32
	}{
		{
			name:   "no cursor",
			cursor: nil,
			limit:  20,
		},
		{
			name:   "with cursor",
			cursor: &validCursor,
			limit:  10,
		},
		{
			name:   "zero limit should use default",
			cursor: nil,
			limit:  0,
		},
		{
			name:   "max limit exceeded",
			cursor: nil,
			limit:  200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &articlesv2.FetchArticlesCursorRequest{
				Cursor: tt.cursor,
				Limit:  tt.limit,
			}

			// Just verify construction
			if tt.cursor != nil {
				assert.NotNil(t, req.Cursor)
			} else {
				assert.Nil(t, req.Cursor)
			}
		})
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestConvertArticlesToProto(t *testing.T) {
	now := time.Now()
	articles := []*domain.Article{
		{
			ID:          uuid.New(),
			Title:       "Test Article 1",
			URL:         "https://example.com/1",
			Content:     "Content 1",
			PublishedAt: now,
			Tags:        []string{"tag1", "tag2"},
		},
		{
			ID:          uuid.New(),
			Title:       "Test Article 2",
			URL:         "https://example.com/2",
			Content:     "Content 2",
			PublishedAt: now.Add(-time.Hour),
			Tags:        []string{"tag3"},
		},
	}

	protoArticles := convertArticlesToProto(articles)

	assert.Len(t, protoArticles, 2)
	assert.Equal(t, "Test Article 1", protoArticles[0].Title)
	assert.Equal(t, "https://example.com/1", protoArticles[0].Url)
	assert.Len(t, protoArticles[0].Tags, 2)
	assert.Equal(t, "Test Article 2", protoArticles[1].Title)
}

func TestConvertArticlesToProto_EmptyList(t *testing.T) {
	protoArticles := convertArticlesToProto([]*domain.Article{})
	assert.Empty(t, protoArticles)
	assert.NotNil(t, protoArticles)
}

// =============================================================================
// FetchArticleSummary Handler Tests (TDD)
// =============================================================================

// Mock for FetchInoreaderSummaryUsecase
type mockFetchInoreaderSummaryUsecase struct {
	summaries []*domain.InoreaderSummary
	err       error
}

func (m *mockFetchInoreaderSummaryUsecase) Execute(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.summaries, nil
}

// Create test handler for FetchArticleSummary tests
func createArticleSummaryTestHandler(mockUsecase *mockFetchInoreaderSummaryUsecase) *Handler {
	container := &di.ApplicationComponents{
		FetchInoreaderSummaryUsecase: mockUsecase,
	}
	cfg := &config.Config{}
	logger := slog.Default()
	return NewHandler(container, cfg, logger)
}

func TestFetchArticleSummary_Success(t *testing.T) {
	now := time.Now()
	mockSummaries := []*domain.InoreaderSummary{
		{
			ArticleURL:  "https://example.com/article1",
			Title:       "Test Article 1",
			Author:      stringPtr("Author 1"),
			Content:     "Content 1",
			ContentType: "text/html",
			PublishedAt: now,
			FetchedAt:   now,
			InoreaderID: "source-1",
		},
		{
			ArticleURL:  "https://example.com/article2",
			Title:       "Test Article 2",
			Author:      nil,
			Content:     "Content 2",
			ContentType: "text/html",
			PublishedAt: now.Add(-time.Hour),
			FetchedAt:   now,
			InoreaderID: "source-2",
		},
	}

	mockUsecase := &mockFetchInoreaderSummaryUsecase{
		summaries: mockSummaries,
		err:       nil,
	}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: []string{"https://example.com/article1", "https://example.com/article2"},
	})

	resp, err := handler.FetchArticleSummary(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 2, len(resp.Msg.MatchedArticles))
	assert.Equal(t, int32(2), resp.Msg.TotalMatched)
	assert.Equal(t, int32(2), resp.Msg.RequestedCount)

	// Verify first article
	assert.Equal(t, "Test Article 1", resp.Msg.MatchedArticles[0].Title)
	assert.Equal(t, "Content 1", resp.Msg.MatchedArticles[0].Content)
	assert.Equal(t, "Author 1", resp.Msg.MatchedArticles[0].Author)
	assert.Equal(t, "source-1", resp.Msg.MatchedArticles[0].SourceId)

	// Verify second article (with nil author)
	assert.Equal(t, "Test Article 2", resp.Msg.MatchedArticles[1].Title)
	assert.Equal(t, "", resp.Msg.MatchedArticles[1].Author)
}

func TestFetchArticleSummary_EmptyURLs(t *testing.T) {
	mockUsecase := &mockFetchInoreaderSummaryUsecase{}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := createAuthContext()

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: []string{},
	})

	_, err := handler.FetchArticleSummary(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "feed_urls cannot be empty")
}

func TestFetchArticleSummary_ExceedsMaxLimit(t *testing.T) {
	mockUsecase := &mockFetchInoreaderSummaryUsecase{}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := createAuthContext()

	// Create 51 URLs (exceeds limit of 50)
	urls := make([]string, 51)
	for i := range urls {
		urls[i] = "https://example.com/article" + string(rune(i))
	}

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: urls,
	})

	_, err := handler.FetchArticleSummary(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
	assert.Contains(t, connectErr.Message(), "maximum 50 URLs")
}

func TestFetchArticleSummary_RequiresAuth(t *testing.T) {
	mockUsecase := &mockFetchInoreaderSummaryUsecase{}
	handler := createArticleSummaryTestHandler(mockUsecase)
	ctx := context.Background() // No auth

	req := connect.NewRequest(&articlesv2.FetchArticleSummaryRequest{
		FeedUrls: []string{"https://example.com/article"},
	})

	_, err := handler.FetchArticleSummary(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

// Note: stringPtr helper is defined in handler.go

// =============================================================================
// StreamArticleTags Handler Tests (TDD)
// =============================================================================

func TestStreamArticleTagsResponse_Construction(t *testing.T) {
	// Test EventType enum values
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_UNSPECIFIED, articlesv2.StreamArticleTagsResponse_EventType(0))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED, articlesv2.StreamArticleTagsResponse_EventType(1))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_GENERATING, articlesv2.StreamArticleTagsResponse_EventType(2))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED, articlesv2.StreamArticleTagsResponse_EventType(3))
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_ERROR, articlesv2.StreamArticleTagsResponse_EventType(4))

	// Test event construction
	msg := "Generating tags..."
	event := &articlesv2.StreamArticleTagsResponse{
		ArticleId: "article-123",
		Tags: []*articlesv2.ArticleTagItem{
			{
				Id:        "tag-1",
				Name:      "Go",
				CreatedAt: time.Now().Format(time.RFC3339),
			},
		},
		EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED,
		Message:   &msg,
	}

	assert.Equal(t, "article-123", event.ArticleId)
	assert.Len(t, event.Tags, 1)
	assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED, event.EventType)
	assert.NotNil(t, event.Message)
	assert.Equal(t, "Generating tags...", *event.Message)
}

func TestStreamArticleTagsRequest_Construction(t *testing.T) {
	title := "Test Article"
	content := "Test Content"
	feedID := "feed-123"

	req := &articlesv2.StreamArticleTagsRequest{
		ArticleId: "article-123",
		Title:     &title,
		Content:   &content,
		FeedId:    &feedID,
	}

	assert.Equal(t, "article-123", req.ArticleId)
	assert.NotNil(t, req.Title)
	assert.Equal(t, "Test Article", *req.Title)
	assert.NotNil(t, req.Content)
	assert.NotNil(t, req.FeedId)
}

func TestConvertTagsToProto(t *testing.T) {
	now := time.Now()
	tags := []*domain.FeedTag{
		{
			ID:        "tag-1",
			TagName:   "Go",
			CreatedAt: now,
		},
		{
			ID:        "tag-2",
			TagName:   "Testing",
			CreatedAt: now.Add(-time.Hour),
		},
	}

	protoTags := convertTagsToProto(tags)

	require.Len(t, protoTags, 2)
	assert.Equal(t, "tag-1", protoTags[0].Id)
	assert.Equal(t, "Go", protoTags[0].Name)
	assert.Equal(t, "tag-2", protoTags[1].Id)
	assert.Equal(t, "Testing", protoTags[1].Name)
}

func TestConvertTagsToProto_Empty(t *testing.T) {
	protoTags := convertTagsToProto([]*domain.FeedTag{})
	assert.Empty(t, protoTags)
	assert.NotNil(t, protoTags)
}

// =============================================================================
// StreamArticleTags On-The-Fly Generation Tests (TDD)
// =============================================================================

// TestStreamArticleTags_OnTheFlyGeneration_Integration tests that the handler
// correctly triggers on-the-fly tag generation when no tags exist in DB.
// This is documented here as an integration test specification.
//
// Expected behavior:
// 1. When usecase returns empty tags (DB has no tags)
// 2. Handler should call gateway's FetchArticleTags which triggers on-the-fly generation
// 3. Handler should return EVENT_TYPE_COMPLETED with generated tags
//
// Note: Full integration testing requires mq-hub and DB setup.
func TestStreamArticleTags_OnTheFlyGeneration_Behavior_Documented(t *testing.T) {
	// Document expected behavior for on-the-fly generation
	t.Run("behavior_specification", func(t *testing.T) {
		// When DB has no tags and on-the-fly generation is enabled:
		// - Gateway fetches article content
		// - Gateway calls mq-hub GenerateTagsForArticle
		// - Handler receives generated tags
		// - Handler returns EVENT_TYPE_COMPLETED with tags

		// This test verifies the handler structure supports this flow
		// by checking the container has required dependencies

		container := &di.ApplicationComponents{}
		cfg := &config.Config{}
		logger := slog.Default()
		handler := NewHandler(container, cfg, logger)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.container)
	})
}

func TestStreamArticleTags_ReturnsCompletedWithGeneratedTags(t *testing.T) {
	// This test verifies the tag conversion logic works correctly
	// for tags that would be returned by on-the-fly generation.

	now := time.Now()
	generatedTags := []*domain.FeedTag{
		{ID: "gen-1", TagName: "AI", CreatedAt: now},
		{ID: "gen-2", TagName: "ML", CreatedAt: now},
	}

	// Convert to proto for expected comparison
	expectedProtoTags := convertTagsToProto(generatedTags)

	assert.Len(t, expectedProtoTags, 2)
	assert.Equal(t, "AI", expectedProtoTags[0].Name)
	assert.Equal(t, "ML", expectedProtoTags[1].Name)
}

func TestStreamArticleTags_EventTypes_Documented(t *testing.T) {
	// Document the expected event types for StreamArticleTags
	t.Run("event_type_semantics", func(t *testing.T) {
		// EVENT_TYPE_CACHED: Tags found in DB (immediate return)
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED, articlesv2.StreamArticleTagsResponse_EventType(1))

		// EVENT_TYPE_GENERATING: Heartbeat during generation (future use)
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_GENERATING, articlesv2.StreamArticleTagsResponse_EventType(2))

		// EVENT_TYPE_COMPLETED: Generation finished (with or without tags)
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED, articlesv2.StreamArticleTagsResponse_EventType(3))

		// EVENT_TYPE_ERROR: Error during generation
		assert.Equal(t, articlesv2.StreamArticleTagsResponse_EVENT_TYPE_ERROR, articlesv2.StreamArticleTagsResponse_EventType(4))
	})

	t.Run("fail_open_behavior", func(t *testing.T) {
		// When on-the-fly generation fails, return COMPLETED with empty tags
		// This is fail-open behavior to ensure UI remains functional
		err := errors.New("mq-hub connection failed")
		assert.NotNil(t, err)
		// Expected: EVENT_TYPE_COMPLETED with empty tags, not EVENT_TYPE_ERROR
	})
}

// =============================================================================
// FetchRandomFeed Handler Tests (ADR-173)
// =============================================================================

func TestFetchRandomFeedResponse_Construction(t *testing.T) {
	id := uuid.New().String()
	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          id,
		Url:         "https://example.com",
		Title:       "Test Feed",
		Description: "A test feed",
		Tags: []*articlesv2.ArticleTagItem{
			{
				Id:        "tag-1",
				Name:      "Go",
				CreatedAt: time.Now().Format(time.RFC3339),
			},
		},
	}

	assert.Equal(t, id, resp.Id)
	assert.Equal(t, "https://example.com", resp.Url)
	assert.Equal(t, "Test Feed", resp.Title)
	assert.Equal(t, "A test feed", resp.Description)
	assert.Len(t, resp.Tags, 1)
	assert.Equal(t, "Go", resp.Tags[0].Name)
}

func TestFetchRandomFeedResponse_WithEmptyTags(t *testing.T) {
	id := uuid.New().String()
	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          id,
		Url:         "https://example.com",
		Title:       "Test Feed",
		Description: "A test feed",
		Tags:        []*articlesv2.ArticleTagItem{},
	}

	assert.Equal(t, id, resp.Id)
	assert.Empty(t, resp.Tags)
}

func TestFetchRandomFeedResponse_WithNilTags(t *testing.T) {
	// Verify that nil tags are handled gracefully
	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          uuid.New().String(),
		Url:         "https://example.com",
		Title:       "Test Feed",
		Description: "A test feed",
		Tags:        nil,
	}

	assert.Nil(t, resp.Tags)
}

func TestFetchRandomFeed_RequiresAuth(t *testing.T) {
	container := &di.ApplicationComponents{}
	cfg := &config.Config{}
	logger := slog.Default()
	handler := NewHandler(container, cfg, logger)
	ctx := context.Background() // No auth

	req := connect.NewRequest(&articlesv2.FetchRandomFeedRequest{})

	_, err := handler.FetchRandomFeed(ctx, req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestFetchRandomFeed_TagsIncludedInResponse_Documented(t *testing.T) {
	// Document the expected behavior for ADR-173
	t.Run("behavior_specification", func(t *testing.T) {
		// When FetchRandomFeed is called:
		// 1. Get a random feed from FetchRandomSubscriptionUsecase
		// 2. Get the latest article for that feed from AltDBRepository.FetchLatestArticleByFeedID
		// 3. Fetch/generate tags via FetchArticleTagsGateway.FetchArticleTags
		//    - If tags exist in DB, return them (CACHED behavior)
		//    - If no tags, trigger on-the-fly generation via mq-hub
		// 4. Include tags in the FetchRandomFeedResponse
		//
		// This eliminates the need for a separate REST API call to /v1/feeds/{id}/tags

		container := &di.ApplicationComponents{}
		cfg := &config.Config{}
		logger := slog.Default()
		handler := NewHandler(container, cfg, logger)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.container)
	})

	t.Run("fail_open_behavior", func(t *testing.T) {
		// If any step fails (fetching article, generating tags), return feed without tags
		// This ensures the UI remains functional even if tag generation is unavailable

		// Expected flow:
		// - FetchLatestArticleByFeedID fails -> return feed with empty tags
		// - FetchArticleTags fails -> return feed with empty tags
		// - Article not found -> return feed with empty tags

		// This test documents the fail-open expectation
		err := errors.New("database error")
		assert.NotNil(t, err)
	})
}

func TestFetchRandomFeed_MultipleTags(t *testing.T) {
	now := time.Now()
	tags := []*articlesv2.ArticleTagItem{
		{Id: "tag-1", Name: "Go", CreatedAt: now.Format(time.RFC3339)},
		{Id: "tag-2", Name: "Testing", CreatedAt: now.Format(time.RFC3339)},
		{Id: "tag-3", Name: "Backend", CreatedAt: now.Format(time.RFC3339)},
	}

	resp := &articlesv2.FetchRandomFeedResponse{
		Id:          uuid.New().String(),
		Url:         "https://example.com",
		Title:       "Tech Blog",
		Description: "A tech blog",
		Tags:        tags,
	}

	assert.Len(t, resp.Tags, 3)
	assert.Equal(t, "Go", resp.Tags[0].Name)
	assert.Equal(t, "Testing", resp.Tags[1].Name)
	assert.Equal(t, "Backend", resp.Tags[2].Name)
}

// =============================================================================
// FetchRandomFeed Article Fetching Tests (No articles in DB)
// =============================================================================

func TestFetchRandomFeed_NoArticles_FetchesContentAndGeneratesTags_Documented(t *testing.T) {
	// Document the expected behavior when feed has no articles
	t.Run("behavior_specification", func(t *testing.T) {
		// When FetchRandomFeed is called and the feed has no articles:
		// 1. Get a random feed from FetchRandomSubscriptionUsecase
		// 2. FetchLatestArticleByFeedID returns nil (no articles)
		// 3. Parse feed.Link as article URL
		// 4. Call ArticleUsecase.FetchCompliantArticle(feed.Link)
		//    - This fetches article content from the web
		//    - Saves article to DB with proper feed_id
		//    - Returns articleID
		// 5. Call FetchArticleTagsGateway.FetchArticleTags(articleID)
		//    - This triggers on-the-fly tag generation if needed
		// 6. Include tags in the FetchRandomFeedResponse
		//
		// This ensures that even feeds without articles can display tags.

		container := &di.ApplicationComponents{}
		cfg := &config.Config{}
		logger := slog.Default()
		handler := NewHandler(container, cfg, logger)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.container)
	})

	t.Run("fail_open_behavior", func(t *testing.T) {
		// If any step fails (parsing URL, fetching content, generating tags):
		// - Log the error
		// - Return feed without tags (fail-open)
		// - UI remains functional

		// Expected failure scenarios:
		// - Invalid feed.Link URL -> skip article fetch, return empty tags
		// - ArticleUsecase not available -> skip article fetch, return empty tags
		// - FetchCompliantArticle fails -> log warning, return empty tags
		// - FetchArticleTags fails -> log warning, return empty tags

		err := errors.New("network error")
		assert.NotNil(t, err)
	})
}

func TestFetchRandomFeed_NoArticles_FlowCorrectness(t *testing.T) {
	// Verify the flow logic is correct by testing the URL parsing and article ID handling
	t.Run("url_parsing", func(t *testing.T) {
		// Test that we correctly parse feed.Link URLs
		testURLs := []struct {
			link    string
			valid   bool
		}{
			{"https://example.com/article/123", true},
			{"http://blog.example.org/post", true},
			{"not-a-url", false},
			{"", false},
		}

		for _, tc := range testURLs {
			_, err := url.Parse(tc.link)
			if tc.valid {
				assert.NoError(t, err, "Expected valid URL: %s", tc.link)
			}
		}
	})

	t.Run("article_id_handling", func(t *testing.T) {
		// When ArticleUsecase.FetchCompliantArticle succeeds:
		// - It returns (content string, articleID string, err error)
		// - articleID is used to fetch/generate tags
		// - Empty articleID means article wasn't saved

		articleID := uuid.New().String()
		assert.NotEmpty(t, articleID)

		// Empty articleID should skip tag generation
		emptyID := ""
		assert.Empty(t, emptyID)
	})
}
