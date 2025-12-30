package search_engine

import (
	"fmt"
	"regexp"
	"search-indexer/logger"
	"strings"
	"unicode"

	"github.com/meilisearch/meilisearch-go"
)

func NewMeilisearchClient(host string, apiKey string) meilisearch.ServiceManager {
	return meilisearch.New(host, meilisearch.WithAPIKey(apiKey))
}

func SearchArticles(idxArticles meilisearch.IndexManager, query string) (*meilisearch.SearchResponse, error) {

	const LIMIT = 20

	results, err := idxArticles.Search(query, &meilisearch.SearchRequest{
		Limit: LIMIT,
	})
	if err != nil {
		logger.Logger.Error("Failed to search articles", "error", err)
		return nil, err
	}

	return results, nil
}

// SearchArticlesWithFilter searches articles with a filter (e.g., user_id filter)
func SearchArticlesWithFilter(idxArticles meilisearch.IndexManager, query string, filter string) (*meilisearch.SearchResponse, error) {
	const LIMIT = 20

	results, err := idxArticles.Search(query, &meilisearch.SearchRequest{
		Limit:  LIMIT,
		Filter: filter,
	})
	if err != nil {
		logger.Logger.Error("Failed to search articles with filter", "error", err, "filter", filter)
		return nil, err
	}

	return results, nil
}

// MakeSecureSearchFilter creates a secure Meilisearch filter from tags
func MakeSecureSearchFilter(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	var filters []string
	for _, tag := range tags {
		escapedTag := EscapeMeilisearchValue(tag)
		filters = append(filters, fmt.Sprintf("tags = \"%s\"", escapedTag))
	}

	return strings.Join(filters, " AND ")
}

// EscapeMeilisearchValue escapes special characters in Meilisearch values
func EscapeMeilisearchValue(value string) string {
	// First escape backslashes, then escape quotes
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

// ValidateFilterTags validates input tags for security
func ValidateFilterTags(tags []string) error {
	if len(tags) > 10 {
		return fmt.Errorf("too many filter tags: maximum 10 allowed, got %d", len(tags))
	}

	// Regex for allowed characters: alphanumeric, spaces, hyphens, underscores, and Unicode letters
	validTagRegex := regexp.MustCompile(`^[\p{L}\p{N}\s\-_]+$`)

	for _, tag := range tags {
		// Check for empty or whitespace-only tags
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("empty or whitespace-only tag not allowed")
		}

		// Check tag length
		if len(tag) > 100 {
			return fmt.Errorf("tag too long: maximum 100 characters, got %d", len(tag))
		}

		// Check for invalid characters using regex
		if !validTagRegex.MatchString(tag) {
			return fmt.Errorf("invalid characters in tag: %s", tag)
		}

		// Check for control characters
		for _, r := range tag {
			if unicode.IsControl(r) {
				return fmt.Errorf("control characters not allowed in tag: %s", tag)
			}
		}
	}

	return nil
}
