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
	Wear            string
	// ContactCount is how many acts of this verb on this item are collapsed
	// into this footprint (>= 1). OccurredAt is the latest contact,
	// FirstOccurredAt the earliest.
	ContactCount    int
	FirstOccurredAt time.Time
}

// TrailEvidenceRef is one piece of evidence backing a branch.
type TrailEvidenceRef struct {
	RefID string
	Label string
	Kind  string
}

// TrailBranch is the domain view of a system-proposed branch. It always carries
// the four-tuple (relation kind / why / evidence / confidence).
type TrailBranch struct {
	BranchKey     string
	AnchorItemKey string
	RelationKind  string
	Why           string
	EvidenceRefs  []TrailEvidenceRef
	Confidence    string
	TargetItemKey string
	TargetTitle   string
}
