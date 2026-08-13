// Package projection_gap decides how far a projector may fold and advance its
// checkpoint when the event log has a hole in it.
//
// knowledge_events.event_seq is a BIGSERIAL: the value is taken when a
// transaction inserts, not when it commits. A transaction that took seq 100
// and commits after the one that took 101 is invisible to every read that
// lands in between, so a batch that folds whatever the query returned and
// advances to its maximum leaves 100 behind a checkpoint that only ever asks
// for higher sequences — the event is dropped from the read model for good,
// and the tip-minus-checkpoint lag metric reads zero while it is gone.
//
// So a batch folds only the contiguous run starting at its checkpoint and
// stops at the first hole. A hole is either owned by a writer still in flight
// (it fills in on a later tick) or burned by a rolled-back transaction (it
// never fills in), and the two look identical at the moment they are seen.
// Tracker tells them apart with transaction ids rather than a timeout. Every
// sequence in a hole had been handed out by the time the hole was seen — the
// visible event above it took a later one — and its writer had a transaction
// id before that, from the dedupe INSERT preceding the event INSERT in the
// same transaction. So the id the reader is given after seeing the hole stands
// above every writer that could still deliver any of those sequences, and once
// the oldest running transaction has passed that ceiling with the hole still
// open, the whole run is burned.
//
// "With the hole still open" is not a fact the batch read can supply: it was
// taken under an earlier snapshot, and a writer that commits in between is
// absent from it while already counted as finished by the ids. The verdict
// therefore reads the hole's emptiness and the ids together, in one statement
// and so under one snapshot — sovereign_db.SequenceGapFrontier, where what
// guarantees each part is written out.
package projection_gap

import "knowledge-sovereign/driver/sovereign_db"

// Hole is a run of sequences missing from a batch: First is the sequence
// blocking the fold, Last the highest sequence below the next visible event.
// The zero value means the batch had no hole.
type Hole struct {
	First int64
	Last  int64
}

// Open reports whether the batch stopped on a hole.
func (h Hole) Open() bool { return h.First != 0 }

// ContiguousPrefix returns the leading run of events whose sequences follow
// afterSeq one by one, together with the hole that ended the run. events must
// be ordered by event_seq ascending, as ListKnowledgeEventsSince returns them.
func ContiguousPrefix(events []sovereign_db.KnowledgeEvent, afterSeq int64) ([]sovereign_db.KnowledgeEvent, Hole) {
	next := afterSeq + 1
	for i, evt := range events {
		if evt.EventSeq != next {
			return events[:i], Hole{First: next, Last: evt.EventSeq - 1}
		}
		next++
	}
	return events, Hole{}
}

// Tracker holds one projector's pending observation of the hole blocking it.
// It is not safe for concurrent use: a projector drains its log from a single
// ticker goroutine.
type Tracker struct {
	hole    Hole
	ceiling int64
}

// MayAbandon records a hole under the frontier read for it, and reports whether
// the whole run can now be treated as burned. The first sighting of a hole only
// records the ceiling it was seen under — the verdict needs a second frontier,
// read once every writer below that ceiling has had the chance to finish. A
// hole with different bounds replaces the pending observation, so neither a
// checkpoint that moved nor an event that arrived inside the run can inherit an
// older hole's verdict.
//
// A frontier that found the run no longer empty ends the matter whatever the
// ids say: the events are there, the next batch read picks them up, and the
// pending observation is about a hole that no longer exists. This is the case
// the ids alone cannot see — the writer committed after the batch was read, so
// it is finished by the time the frontier is taken and its sequences look
// burned.
func (t *Tracker) MayAbandon(hole Hole, frontier sovereign_db.SequenceGapFrontier) bool {
	if !frontier.HoleOpen {
		t.hole, t.ceiling = Hole{}, 0
		return false
	}
	if t.hole != hole {
		t.hole, t.ceiling = hole, frontier.Ceiling
		return false
	}
	if frontier.Xmin < t.ceiling {
		return false
	}
	t.hole, t.ceiling = Hole{}, 0
	return true
}
