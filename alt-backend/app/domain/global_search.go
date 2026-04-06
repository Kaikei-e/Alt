package domain

import "time"

// GlobalSearchResult is the aggregated result from all search verticals.
type GlobalSearchResult struct {
	Query            string
	Articles         *ArticleSearchSection
	Recaps           *RecapSearchSection
	Tags             *TagSearchSection
	DegradedSections []string
	SearchedAt       time.Time
}

// ArticleSearchSection contains article search results for the overview.
type ArticleSearchSection struct {
	Hits           []GlobalArticleHit
	EstimatedTotal int64
	HasMore        bool
}

// GlobalArticleHit represents a single article in the federated search results.
type GlobalArticleHit struct {
	ID            string
	Title         string
	Snippet       string
	Link          string
	Tags          []string
	MatchedFields []string
}

// RecapSearchSection contains recap search results for the overview.
type RecapSearchSection struct {
	Hits           []GlobalRecapHit
	EstimatedTotal int64
	HasMore        bool
}

// GlobalRecapHit represents a single recap genre in the federated search results.
type GlobalRecapHit struct {
	ID         string
	JobID      string
	Genre      string
	Summary    string
	TopTerms   []string
	Tags       []string
	WindowDays int
	ExecutedAt string
}

// TagSearchSection contains tag search results for the overview.
type TagSearchSection struct {
	Hits  []GlobalTagHit
	Total int64
}

// GlobalTagHit represents a single tag in the federated search results.
type GlobalTagHit struct {
	TagName      string
	ArticleCount int
}
