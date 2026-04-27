package knowledge_loop_projector

import "time"

// pureStalenessBucket maps the gap between an event's occurrence time and the
// source observation time (article published_at, recap snapshot start, etc.)
// into a small integer bucket. Bigger bucket = staler source.
//
// Reproject-safe contract: this function is deterministic and reads only its
// arguments — no `time.Now()`, no environment lookups. Same inputs always
// yield the same bucket on replay. Boundaries are fixed so a future change
// requires bumping `WhyMappingVersion` and a full reproject; rolling them on
// the fly would silently shuffle past placements.
//
// Buckets:
//
//	0 — fresh: source observed within 1 hour of the event
//	1 — current: within 1 day
//	2 — recent: within 7 days
//	3 — aging: within 30 days
//	4 — stale: 30 days or older
//
// When `sourceObservedAt` is the zero value (no payload field, e.g. for events
// that don't carry a published_at), we return 0 — "treat as fresh" — rather
// than guessing a stale bucket from the event time alone. The resolver still
// has TopicOverlap / VersionDrift to work with.
func pureStalenessBucket(eventOccurredAt, sourceObservedAt time.Time) uint32 {
	if sourceObservedAt.IsZero() {
		return 0
	}
	gap := eventOccurredAt.Sub(sourceObservedAt)
	if gap < 0 {
		// Event observed before source was published — clock skew or a recap
		// snapshot whose window starts after the event. Treat as fresh.
		return 0
	}
	switch {
	case gap < time.Hour:
		return 0
	case gap < 24*time.Hour:
		return 1
	case gap < 7*24*time.Hour:
		return 2
	case gap < 30*24*time.Hour:
		return 3
	default:
		return 4
	}
}
