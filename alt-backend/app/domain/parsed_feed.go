package domain

// ParsedFeed represents a parsed RSS/Atom feed with its items.
// Used by the registration flow to pass fetched feed data between layers
// without requiring a second HTTP fetch.
type ParsedFeed struct {
	FeedLink string
	Items    []*FeedItem
}
