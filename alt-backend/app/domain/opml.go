package domain

import "time"

// OPMLDocument represents an OPML 2.0 document.
type OPMLDocument struct {
	Title       string
	DateCreated time.Time
	Outlines    []OPMLOutline
}

// OPMLOutline represents a single outline element in OPML.
type OPMLOutline struct {
	Text     string        // Feed title or category name
	Type     string        // "rss" for feed entries
	XMLURL   string        // RSS feed URL
	HTMLURL  string        // Website URL
	Children []OPMLOutline // Nested outlines (categories)
}

// OPMLImportResult represents the result of an OPML import operation.
type OPMLImportResult struct {
	Total      int      `json:"total"`
	Imported   int      `json:"imported"`
	Skipped    int      `json:"skipped"`
	Failed     int      `json:"failed"`
	FailedURLs []string `json:"failed_urls,omitempty"`
}

// FeedLinkForExport bundles feed link URL with optional metadata for OPML export.
type FeedLinkForExport struct {
	URL     string
	Title   string
	HTMLURL string
}
