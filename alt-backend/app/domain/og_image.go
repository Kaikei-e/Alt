package domain

import "time"

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
//
// Deprecated: the batch backfill job it fed has been removed in favour of
// resolving on demand. Removed together with the proto surface once the
// breaking-change baseline moves.
type OgImageBackfillCandidate struct {
	ArticleID string
	URL       string
}

// FeedOgImageTarget is one feed a reader has brought into view, as far as
// resolution is concerned: the page that would be fetched, and whatever an
// earlier attempt already settled about it.
type FeedOgImageTarget struct {
	FeedID string
	// PageURL is feeds.website_url — the page whose og:image would be read.
	PageURL string
	// OgImageURL is an image URL already held for this feed, from RSS or from
	// an earlier resolution. Non-empty means there is nothing to fetch.
	OgImageURL string
	// Suppressed is true when an earlier attempt was refused and that refusal
	// still stands. A caller that fetches anyway turns every scroll past the
	// card into another request to an origin that has already said no.
	Suppressed bool
}

// NeedsFetch reports whether this feed still warrants an origin request.
func (t FeedOgImageTarget) NeedsFetch() bool {
	return t.OgImageURL == "" && !t.Suppressed && t.PageURL != ""
}

// OgImageRefusal names why an origin did not yield an og:image. It is stored so
// that the next reader to scroll past the same card does not cause the same
// request to be made again.
type OgImageRefusal string

const (
	// OgImageRefusedByRobots means robots.txt disallows fetching the page.
	// This is a policy rather than a fault, so it is never retried inside the
	// retention window.
	OgImageRefusedByRobots OgImageRefusal = "robots_disallow"
	// OgImageRefusedForbidden is an HTTP 403 from the origin.
	OgImageRefusedForbidden OgImageRefusal = "http_403"
	// OgImageRefusedNotFound is an HTTP 404 from the origin.
	OgImageRefusedNotFound OgImageRefusal = "http_404"
	// OgImageNoTag means the page was fetched and simply carries no og:image.
	OgImageNoTag OgImageRefusal = "no_og_tag"
	// OgImageFetchError covers transport failures and unexpected statuses.
	OgImageFetchError OgImageRefusal = "fetch_error"
)

// RetryAfter is how long to wait before this feed may be asked about again.
//
// Zero means "not within this retention window". A robots.txt disallow and a
// page with no og:image tag are both settled answers rather than transient
// faults, so re-asking would produce the same result at someone else's expense;
// the row is purged with the rest of the window and re-acquired naturally if
// the reader returns later.
func (r OgImageRefusal) RetryAfter() time.Duration {
	switch r {
	case OgImageRefusedByRobots, OgImageNoTag, OgImageRefusedNotFound:
		return 0
	case OgImageRefusedForbidden:
		// A 403 is usually hotlink or bot policy and usually permanent, but it
		// is also what a misconfigured edge returns during an incident. One
		// retry a day costs the origin nothing and recovers that case.
		return 24 * time.Hour
	default:
		return 6 * time.Hour
	}
}
