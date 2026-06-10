package domain

import "time"

// TrailFootprint is one footprint on the Knowledge Trail spine — the domain
// view of a user cognitive act enriched with display fields. It mirrors the
// sovereign read model and carries no business logic.
type TrailFootprint struct {
	FootprintKey    string
	Verb            string
	ItemKey         string
	Title           string
	Excerpt         string
	Tags            []string
	Note            string
	SourceEventType string
	OccurredAt      time.Time
}
