package global_search_gateway

import (
	"alt/domain"
	"alt/orchestrator/port/feed_url_link_port"
	"alt/orchestrator/port/search_indexer_port"
	"context"
	"html"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// ArticleSearchGateway implements global_search_port.SearchArticlesPort.
type ArticleSearchGateway struct {
	searchIndexer search_indexer_port.ArticleSearchPort
	urlPort       feed_url_link_port.FeedURLLinkPort
	logger        *slog.Logger
}

// NewArticleSearchGateway creates a new ArticleSearchGateway.
func NewArticleSearchGateway(
	searchIndexer search_indexer_port.ArticleSearchPort,
	urlPort feed_url_link_port.FeedURLLinkPort,
) *ArticleSearchGateway {
	return &ArticleSearchGateway{
		searchIndexer: searchIndexer,
		urlPort:       urlPort,
		logger:        slog.Default(),
	}
}

// SearchArticlesForGlobal searches articles for the global search overview.
func (g *ArticleSearchGateway) SearchArticlesForGlobal(ctx context.Context, query string, userID string, limit int) (*domain.ArticleSearchSection, error) {
	hits, totalCount, err := g.searchIndexer.SearchArticlesWithPagination(ctx, query, userID, 0, limit)
	if err != nil {
		g.logger.ErrorContext(ctx, "failed to search articles for global search", "error", err, "query", query)
		return nil, err
	}

	if len(hits) == 0 {
		return &domain.ArticleSearchSection{
			Hits:           []domain.GlobalArticleHit{},
			EstimatedTotal: 0,
			HasMore:        false,
		}, nil
	}

	// Extract article IDs for URL enrichment
	articleIDs := extractArticleIDsFromHits(hits)

	// Get feed URLs for the articles
	feedURLs, err := g.urlPort.GetFeedURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		g.logger.WarnContext(ctx, "failed to get feed URLs, proceeding without links", "error", err)
		feedURLs = nil
	}

	urlMap := buildURLMap(feedURLs)

	// Convert to GlobalArticleHit
	articleHits := mapHitsToGlobalArticleHits(hits, urlMap, query)

	return &domain.ArticleSearchSection{
		Hits:           articleHits,
		EstimatedTotal: totalCount,
		HasMore:        totalCount > int64(limit),
	}, nil
}

// stripHTMLTags removes HTML tags, decodes HTML entities, and collapses whitespace.
func stripHTMLTags(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			if inTag {
				inTag = false
				continue
			}
			b.WriteRune(r)
		case !inTag:
			b.WriteRune(r)
		}
	}
	unescaped := html.UnescapeString(b.String())
	return strings.Join(strings.Fields(unescaped), " ")
}

func containsAnyToken(textLower string, tokens []string) bool {
	for _, token := range tokens {
		if token != "" && strings.Contains(textLower, token) {
			return true
		}
	}
	return false
}

// detectMatchedFields determines which fields matched the query.
func detectMatchedFields(hit domain.SearchIndexerArticleHit, queryLower string) []string {
	tokens := strings.Fields(queryLower)
	if len(tokens) == 0 {
		if queryLower == "" {
			return nil
		}
		tokens = []string{queryLower}
	}

	var matched []string

	if containsAnyToken(strings.ToLower(hit.Title), tokens) {
		matched = append(matched, "title")
	}
	cleanContent := stripHTMLTags(hit.Content)
	if containsAnyToken(strings.ToLower(cleanContent), tokens) {
		matched = append(matched, "content")
	}
	for _, tag := range hit.Tags {
		if containsAnyToken(strings.ToLower(tag), tokens) {
			matched = append(matched, "tags")
			break
		}
	}

	return matched
}

// sanitizeUTF8 replaces invalid UTF-8 sequences with the Unicode replacement character.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

// truncateSnippet truncates content to a maximum rune length at a word boundary.
// It also sanitizes invalid UTF-8 sequences to prevent protobuf serialization errors.
func truncateSnippet(content string, maxRunes int) string {
	// Sanitize invalid UTF-8 first
	if !utf8.ValidString(content) {
		content = strings.ToValidUTF8(content, "\uFFFD")
	}

	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}

	truncated := string(runes[:maxRunes])
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > len(truncated)/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "..."
}

func extractArticleIDsFromHits(hits []domain.SearchIndexerArticleHit) []string {
	articleIDs := make([]string, len(hits))
	for i, hit := range hits {
		articleIDs[i] = hit.ID
	}
	return articleIDs
}

func buildURLMap(feedURLs []domain.FeedAndArticle) map[string]string {
	urlMap := make(map[string]string, len(feedURLs))
	for _, feedURL := range feedURLs {
		urlMap[feedURL.ArticleID] = feedURL.URL
	}
	return urlMap
}

func mapHitsToGlobalArticleHits(hits []domain.SearchIndexerArticleHit, urlMap map[string]string, query string) []domain.GlobalArticleHit {
	queryLower := strings.ToLower(query)
	articleHits := make([]domain.GlobalArticleHit, len(hits))
	for i, hit := range hits {
		cleanContent := stripHTMLTags(hit.Content)
		matchedFields := detectMatchedFields(hit, queryLower)

		articleHits[i] = domain.GlobalArticleHit{
			ID:            hit.ID,
			Title:         sanitizeUTF8(hit.Title),
			Snippet:       truncateSnippet(cleanContent, 200),
			Link:          urlMap[hit.ID],
			Tags:          hit.Tags,
			MatchedFields: matchedFields,
		}
	}
	return articleHits
}
