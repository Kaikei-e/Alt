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

	mockSearchIndexer := mocks.NewMockArticleSearchPort(ctrl)
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

	mockSearchIndexer := mocks.NewMockArticleSearchPort(ctrl)
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

	mockSearchIndexer := mocks.NewMockArticleSearchPort(ctrl)
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

	mockSearchIndexer := mocks.NewMockArticleSearchPort(ctrl)
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

func TestArticleSearchGatewayPureHelpers(t *testing.T) {
	hits := []domain.SearchIndexerArticleHit{
		{ID: "art-1", Title: "Title 1", Content: "Content 1", Tags: []string{"tag1"}},
		{ID: "art-2", Title: "Title 2", Content: "Content 2", Tags: []string{"tag2"}},
	}

	ids := extractArticleIDsFromHits(hits)
	if len(ids) != 2 || ids[0] != "art-1" || ids[1] != "art-2" {
		t.Errorf("unexpected extracted IDs: %+v", ids)
	}

	feedURLs := []domain.FeedAndArticle{
		{ArticleID: "art-1", URL: "https://example.com/art1"},
	}
	urlMap := buildURLMap(feedURLs)
	if urlMap["art-1"] != "https://example.com/art1" {
		t.Errorf("unexpected urlMap: %+v", urlMap)
	}

	globalHits := mapHitsToGlobalArticleHits(hits, urlMap, "Title 1")
	if len(globalHits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(globalHits))
	}
	if globalHits[0].Link != "https://example.com/art1" || globalHits[0].Title != "Title 1" {
		t.Errorf("unexpected global hit 0: %+v", globalHits[0])
	}
	if !containsStr(globalHits[0].MatchedFields, "title") {
		t.Errorf("expected title to match")
	}
}

func TestArticleSearchGateway_StripsHTMLAndUnescapesSnippet(t *testing.T) {
	hits := []domain.SearchIndexerArticleHit{
		{
			ID:      "art-html",
			Title:   "イラン情勢の最新動向",
			Content: "<p><img src=\"https://example.com/iran.jpg\" alt=\"banner\"></p>\n<p>イラン情勢が緊迫化する中、 &amp; 中東地域の外交交渉が続いています。</p>",
			Tags:    []string{"中東", "外交"},
		},
	}
	urlMap := map[string]string{"art-html": "https://example.com/art-html"}

	globalHits := mapHitsToGlobalArticleHits(hits, urlMap, "イラン情勢 中東")
	if len(globalHits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(globalHits))
	}

	gotSnippet := globalHits[0].Snippet
	wantSnippet := "イラン情勢が緊迫化する中、 & 中東地域の外交交渉が続いています。"
	if gotSnippet != wantSnippet {
		t.Errorf("Snippet = %q, want %q", gotSnippet, wantSnippet)
	}
}

func TestDetectMatchedFields_MultiTermAndStrippedHTML(t *testing.T) {
	t.Run("multi-term query matches individual tokens across title, content, and tags", func(t *testing.T) {
		hit := domain.SearchIndexerArticleHit{
			ID:      "art-1",
			Title:   "イラン情勢の解説",
			Content: "<p>この記事では中東におけるエネルギー安全保障を扱います。</p>",
			Tags:    []string{"中東情勢"},
		}

		matched := detectMatchedFields(hit, "イラン情勢 中東")
		if !containsStr(matched, "title") {
			t.Errorf("expected 'title' in matched fields, got %v", matched)
		}
		if !containsStr(matched, "content") {
			t.Errorf("expected 'content' in matched fields, got %v", matched)
		}
		if !containsStr(matched, "tags") {
			t.Errorf("expected 'tags' in matched fields, got %v", matched)
		}
	})

	t.Run("does not match query terms inside HTML tag attributes", func(t *testing.T) {
		hit := domain.SearchIndexerArticleHit{
			ID:      "art-2",
			Title:   "別のトピック",
			Content: `<p><img src="https://example.com/zelenskyy.jpg"></p><p>本文には該当語なし</p>`,
			Tags:    []string{"general"},
		}

		matched := detectMatchedFields(hit, "zelenskyy")
		if containsStr(matched, "content") {
			t.Errorf("expected 'content' NOT to match HTML attribute URL, got %v", matched)
		}
	})
}
