package domain

// OgImageBackfillCandidate is an article whose feed carried no og:image in its
// RSS payload and whose page has not been scraped yet — one item of the
// og-image-backfill work list.
//
// It lives in domain rather than beside the SQL that produces it because the
// job that consumes it runs in a different process from the query since
// ADR-000954 Wave 3: alt-harvester asks alt-data-hub for the list, fetches the
// pages itself, and hands the result back. A type owned by the driver package
// would put the database driver in the harvester's import graph for the sake
// of two strings.
type OgImageBackfillCandidate struct {
	ArticleID string
	URL       string
}
