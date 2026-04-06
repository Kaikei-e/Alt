package global_search_gateway

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestArticleSearchGateway_SearchArticlesForGlobal(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)

	mockSearchIndexer := mocks.NewMockSearchIndexerPort(ctrl)
	mockURLPort := mocks.NewMockFeedURLLinkPort(ctrl)

	gw := NewArticleSearchGateway(mockSearchIndexer, mockURLPort)

	mockSearchIndexer.EXPECT().SearchArticlesWithPagination(
		gomock.Any(), "AI", "user-1", 0, 5,
	).Return([]domain.SearchIndexerArticleHit{
		{ID: "a1", Title: "AI in 2026", Content: "Artificial intelligence is transforming the world", Tags: []string{"AI", "technology"}},
		{ID: "a2", Title: "Machine Learning", Content: "ML is a subset of AI", Tags: []string{"ML", "AI"}},
	}, int64(10), nil)

	mockURLPort.EXPECT().GetFeedURLsByArticleIDs(
		gomock.Any(), []string{"a1", "a2"},
	).Return([]domain.FeedAndArticle{
		{ArticleID: "a1", URL: "https://example.com/ai-2026"},
		{ArticleID: "a2", URL: "https://example.com/ml"},
	}, nil)

	result, err := gw.SearchArticlesForGlobal(context.Background(), "AI", "user-1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(result.Hits))
	}
	if result.EstimatedTotal != 10 {
		t.Errorf("expected estimated_total=10, got %d", result.EstimatedTotal)
	}
	if !result.HasMore {
		t.Error("expected has_more=true")
	}

	// Verify first hit
	hit := result.Hits[0]
	if hit.ID != "a1" {
		t.Errorf("expected ID=a1, got %s", hit.ID)
	}
	if hit.Link != "https://example.com/ai-2026" {
		t.Errorf("expected link, got %q", hit.Link)
	}
	// "AI" should match title and tags
	if !containsStr(hit.MatchedFields, "title") {
		t.Error("expected 'title' in matched_fields")
	}
	if !containsStr(hit.MatchedFields, "tags") {
		t.Error("expected 'tags' in matched_fields")
	}
}

func TestArticleSearchGateway_EmptyResults(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)

	mockSearchIndexer := mocks.NewMockSearchIndexerPort(ctrl)
	mockURLPort := mocks.NewMockFeedURLLinkPort(ctrl)

	gw := NewArticleSearchGateway(mockSearchIndexer, mockURLPort)

	mockSearchIndexer.EXPECT().SearchArticlesWithPagination(
		gomock.Any(), "nonexistent", "user-1", 0, 5,
	).Return([]domain.SearchIndexerArticleHit{}, int64(0), nil)

	result, err := gw.SearchArticlesForGlobal(context.Background(), "nonexistent", "user-1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(result.Hits))
	}
	if result.HasMore {
		t.Error("expected has_more=false")
	}
}

func TestArticleSearchGateway_SearchError(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)

	mockSearchIndexer := mocks.NewMockSearchIndexerPort(ctrl)
	mockURLPort := mocks.NewMockFeedURLLinkPort(ctrl)

	gw := NewArticleSearchGateway(mockSearchIndexer, mockURLPort)

	mockSearchIndexer.EXPECT().SearchArticlesWithPagination(
		gomock.Any(), "AI", "user-1", 0, 5,
	).Return(nil, int64(0), errors.New("search engine down"))

	_, err := gw.SearchArticlesForGlobal(context.Background(), "AI", "user-1", 5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestArticleSearchGateway_URLEnrichmentFailure(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)

	mockSearchIndexer := mocks.NewMockSearchIndexerPort(ctrl)
	mockURLPort := mocks.NewMockFeedURLLinkPort(ctrl)

	gw := NewArticleSearchGateway(mockSearchIndexer, mockURLPort)

	mockSearchIndexer.EXPECT().SearchArticlesWithPagination(
		gomock.Any(), "AI", "user-1", 0, 5,
	).Return([]domain.SearchIndexerArticleHit{
		{ID: "a1", Title: "AI Article", Content: "Content about AI", Tags: []string{"AI"}},
	}, int64(1), nil)

	// URL enrichment fails — should still return results with empty links
	mockURLPort.EXPECT().GetFeedURLsByArticleIDs(
		gomock.Any(), []string{"a1"},
	).Return(nil, errors.New("db error"))

	result, err := gw.SearchArticlesForGlobal(context.Background(), "AI", "user-1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}
	// Link should be empty since URL enrichment failed
	if result.Hits[0].Link != "" {
		t.Errorf("expected empty link, got %q", result.Hits[0].Link)
	}
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
