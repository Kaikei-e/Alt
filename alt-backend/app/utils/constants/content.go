package constants

// Content length constants for text processing and validation.
const (
	// MinArticleLength is the minimum character count for a valid article.
	// Articles shorter than this are likely not meaningful content.
	MinArticleLength = 100

	// MaxSearchResultLength is the maximum length for search result snippets.
	// Used to truncate content for display in search results.
	MaxSearchResultLength = 300

	// MaxRecapArticleBytes is the maximum size in bytes for recap article content.
	// This is a safeguard to prevent memory issues with very large articles.
	// 2MB is the limit per PLAN5 requirements.
	MaxRecapArticleBytes = 2 * 1024 * 1024

	// MaxTitleLength is the maximum length for article titles.
	MaxTitleLength = 500

	// MaxDescriptionLength is the maximum length for article descriptions.
	MaxDescriptionLength = 2000
)
