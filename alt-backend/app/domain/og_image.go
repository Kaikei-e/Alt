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
	// Attempts is how many resolution attempts this feed has already cost.
	//
	// Zero for a feed with no stored row — nothing has been spent on it yet.
	// RetryAfter treats zero and one alike, so a caller may pass this straight
	// through without deciding what "no row" means to a 1-based ladder.
	Attempts int
	// RetryAfterSeconds is how much is left of the bar an earlier refusal set,
	// or 0 when no bar stands — because none was ever set, because it has
	// expired, or because the refusal is settled for this retention window.
	//
	// Suppressed says whether the bar still stands; this says how much of it is
	// left. Both are needed: from the second attempt onwards a failing feed is
	// always suppressed, so a caller holding only Suppressed can tell a reader's
	// client nothing more precise than "not now", and a card held back by a
	// five-second bar gets abandoned for the session.
	RetryAfterSeconds int64
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

// RetryAfter is how long to wait before this feed may be asked about again,
// given how many attempts it has already cost.
//
// Zero means "not within this retention window". A robots.txt disallow and a
// page with no og:image tag are both settled answers rather than transient
// faults, so re-asking would produce the same result at someone else's expense;
// the row is purged with the rest of the window and re-acquired naturally if
// the reader returns later.
//
// attempts counts the attempt whose refusal is being recorded, so it is 1 on
// the first. Anything below 1 is normalised to 1 rather than rejected: the
// stored counter is 0 for a feed with no row, and the caller should not have to
// know that a 1-based ladder starts one above where the column does.
//
// Only a transient fault escalates. The settled answers above are the origin's
// answer rather than a fault on the way to it, and hearing the same answer four
// times does not make it more likely to change; escalating them would imply it
// might. A 403 keeps its flat daily bar for the reason stated below.
func (r OgImageRefusal) RetryAfter(attempts int) time.Duration {
	switch r {
	case OgImageRefusedByRobots, OgImageNoTag, OgImageRefusedNotFound:
		return 0
	case OgImageRefusedForbidden:
		// A 403 is usually hotlink or bot policy and usually permanent, but it
		// is also what a misconfigured edge returns during an incident. One
		// retry a day costs the origin nothing and recovers that case — and
		// escalating would break exactly the case the daily retry exists for.
		// The 7-day retention window already caps this at seven asks in total.
		return 24 * time.Hour
	default:
		// A transport failure is the one refusal worth asking about again soon.
		// The bar doubles so that a feed which keeps failing is asked about ever
		// more rarely rather than on every scroll past.
		//
		// The base is 5s because of the ceiling on the other side of the wire:
		// alt-frontend-sv clamps every wait to OG_RETRY_CEILING_MS = 10s and
		// gives up after three asks (src/lib/utils/ogImageRetry.ts). A first bar
		// above that ceiling is one the browser will not wait out, so the reader
		// sees the card stay empty for the whole session and the client's
		// re-ask never runs even once. Starting at 5s gives 5s -> 10s -> 20s:
		// two asks land inside the session, the third outgrows it, and what is
		// left is a server-side row the next page load collects for free. A base
		// of 30s would clear the ceiling on the very first refusal and the
		// client's whole retry ladder would be dead code.
		const (
			base    = 5 * time.Second
			ceiling = 6 * time.Hour
		)
		if attempts < 1 {
			attempts = 1
		}
		// base << 13 is 40960s, already past the ceiling. Clamping the shift
		// here rather than relying on the comparison below keeps a feed that has
		// failed sixty times from overflowing int64 nanoseconds and wrapping
		// round to a small — or negative — bar.
		if attempts > 13 {
			return ceiling
		}
		if d := base << (attempts - 1); d < ceiling {
			return d
		}
		return ceiling
	}
}
