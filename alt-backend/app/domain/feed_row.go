package domain

import "time"

// FeedRow is one row of the feeds table as the data plane moves it.
//
// It exists because ADR-000954 put a process boundary in the middle of the
// feed list path. The drivers scan into orchestrator/driver/models.Feed, which
// is a driver type; the browser is served domain.FeedItem, which is the
// RSS-spec rendering with a sanitised description and a formatted timestamp.
// Neither can be the currency of a port: the first would make the port depend
// on the driver layer, and the second would force the sanitising and
// formatting across the RPC boundary into alt-data-hub, where a presentation
// decision has no business living (ADR-000954 D4).
//
// So this is the columns and nothing else — the same fields models.Feed
// carries, expressed where a port is allowed to see them.
type FeedRow struct {
	ID          string
	Title       string
	Description string
	// WebsiteURL is feeds.website_url — the RSS <channel><link> value, not the
	// subscription URL. See the Feed doc comment and ADR-000868.
	WebsiteURL string
	PubDate    time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// ArticleID is the newest undeleted article of this feed, nil when the
	// feed has none yet.
	ArticleID *string
	IsRead    bool
	// FeedLinkID is nil for feeds collected before the link was recorded.
	FeedLinkID *string
	// OgImageURL is nil both when no image was found and when the feed has
	// aged out of the 7-day copyright retention window. The read path does not
	// distinguish them and neither does the placeholder the frontend renders.
	OgImageURL *string
}

// FeedRegistration is one collected RSS item to upsert into feeds.
//
// CreatedAt and UpdatedAt are the caller's, not the database's: the collector
// stamps one time for a whole poll so that a run's feeds sort together, and a
// server-side now() per row would interleave a slow batch with a fast one.
type FeedRegistration struct {
	Title       string
	Description string
	WebsiteURL  string
	PubDate     time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	FeedLinkID  *string
	OgImageURL  *string
}

// FeedRegistrationResult reports the outcome of one upsert.
//
// Created distinguishes an insert from an update — the `(xmax = 0)` the driver
// has always returned. The registration transaction writes no articles row, so
// there is no article id to report here (ADR-000953).
type FeedRegistrationResult struct {
	FeedID  string
	Created bool
}
