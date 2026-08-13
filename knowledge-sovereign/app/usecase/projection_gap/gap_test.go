package projection_gap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"knowledge-sovereign/driver/sovereign_db"
)

func events(seqs ...int64) []sovereign_db.KnowledgeEvent {
	out := make([]sovereign_db.KnowledgeEvent, 0, len(seqs))
	for _, seq := range seqs {
		out = append(out, sovereign_db.KnowledgeEvent{EventSeq: seq})
	}
	return out
}

func seqsOf(evts []sovereign_db.KnowledgeEvent) []int64 {
	out := make([]int64, 0, len(evts))
	for _, e := range evts {
		out = append(out, e.EventSeq)
	}
	return out
}

func TestContiguousPrefix(t *testing.T) {
	tests := []struct {
		name     string
		events   []sovereign_db.KnowledgeEvent
		afterSeq int64
		want     []int64
		wantHole Hole
	}{
		{
			name:     "an unbroken run is folded whole",
			events:   events(1, 2, 3),
			afterSeq: 0,
			want:     []int64{1, 2, 3},
		},
		{
			name:     "a hole at the head of the batch stops it before it starts",
			events:   events(2, 3),
			afterSeq: 0,
			want:     []int64{},
			wantHole: Hole{First: 1, Last: 1},
		},
		{
			name:     "a hole inside the batch cuts it there",
			events:   events(11, 12, 15, 16),
			afterSeq: 10,
			want:     []int64{11, 12},
			wantHole: Hole{First: 13, Last: 14},
		},
		{
			name:     "an empty batch has no hole to report",
			events:   nil,
			afterSeq: 7,
			want:     []int64{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, hole := ContiguousPrefix(tt.events, tt.afterSeq)
			assert.Equal(t, tt.want, seqsOf(prefix))
			assert.Equal(t, tt.wantHole, hole)
			assert.Equal(t, tt.wantHole != Hole{}, hole.Open())
		})
	}
}

// stillEmpty is a frontier whose snapshot found the run empty — the only shape
// that can end in a verdict.
func stillEmpty(ceiling, xmin int64) sovereign_db.SequenceGapFrontier {
	return sovereign_db.SequenceGapFrontier{Ceiling: ceiling, Xmin: xmin, HoleOpen: true}
}

// The first sighting of a hole can never abandon it: the writer holding the
// sequence may be committing at that very moment.
func TestTracker_NeverAbandonsAHoleOnFirstSight(t *testing.T) {
	var tracker Tracker
	assert.False(t, tracker.MayAbandon(Hole{First: 41, Last: 41}, stillEmpty(901, 900)))
}

func TestTracker_WaitsWhileAWriterFromBelowTheCeilingIsStillRunning(t *testing.T) {
	var tracker Tracker
	hole := Hole{First: 41, Last: 44}
	tracker.MayAbandon(hole, stillEmpty(950, 900))

	assert.False(t, tracker.MayAbandon(hole, stillEmpty(980, 949)),
		"xid 949 had already written when the hole was first seen and could still be holding one of its sequences")
}

// A writer that took one of these sequences wrote before the ceiling was
// handed out, so an xmin that has reached the ceiling proves it has finished.
func TestTracker_AbandonsOnceEveryWriterBelowTheCeilingHasFinished(t *testing.T) {
	var tracker Tracker
	hole := Hole{First: 41, Last: 44}
	tracker.MayAbandon(hole, stillEmpty(950, 900))

	assert.True(t, tracker.MayAbandon(hole, stillEmpty(980, 950)))
}

// The ids are only half the verdict. A frontier whose snapshot found an event
// in the run reports a hole that has closed: the writer committed after the
// batch was read, which is precisely why the ids call it finished. Folding
// resumes on the next batch read; nothing may be stepped over.
func TestTracker_NeverAbandonsARunTheFrontiersSnapshotFoundFilled(t *testing.T) {
	var tracker Tracker
	hole := Hole{First: 41, Last: 44}
	tracker.MayAbandon(hole, stillEmpty(950, 900))

	assert.False(t, tracker.MayAbandon(hole, sovereign_db.SequenceGapFrontier{Ceiling: 980, Xmin: 950, HoleOpen: false}))
}

// And the observation behind such a verdict is dropped rather than left
// pending: the run it described is gone, so a hole seen later starts over.
func TestTracker_DropsThePendingObservationOnceTheRunIsFilled(t *testing.T) {
	var tracker Tracker
	hole := Hole{First: 41, Last: 44}
	tracker.MayAbandon(hole, stillEmpty(950, 900))
	tracker.MayAbandon(hole, sovereign_db.SequenceGapFrontier{Ceiling: 980, Xmin: 950, HoleOpen: false})

	assert.False(t, tracker.MayAbandon(hole, stillEmpty(1000, 990)),
		"this is a first sighting again, whatever the ids say")
	assert.True(t, tracker.MayAbandon(hole, stillEmpty(1100, 1000)))
}

// The verdict is drawn against the ceiling of the first sighting, never
// against a later one: a writer that started after the hole was seen holds a
// higher id, and waiting for it to finish would be waiting for a transaction
// that cannot own any of these sequences.
func TestTracker_JudgesAgainstTheCeilingOfTheFirstSighting(t *testing.T) {
	var tracker Tracker
	hole := Hole{First: 41, Last: 44}
	tracker.MayAbandon(hole, stillEmpty(950, 900))

	assert.True(t, tracker.MayAbandon(hole, stillEmpty(1200, 1100)))
}

// A verdict is about one run of sequences only. When the hole moves — the
// projection made progress, an operator rebuilt the checkpoint, an event
// landed inside the run — the pending observation is replaced rather than
// inherited, so the new hole gets its own two sightings.
func TestTracker_DoesNotCarryAVerdictOverToADifferentHole(t *testing.T) {
	var tracker Tracker
	tracker.MayAbandon(Hole{First: 41, Last: 44}, stillEmpty(950, 900))

	assert.False(t, tracker.MayAbandon(Hole{First: 41, Last: 42}, stillEmpty(980, 950)),
		"seq 43 arrived after the first sighting; the run below it is a different hole")
	assert.True(t, tracker.MayAbandon(Hole{First: 41, Last: 42}, stillEmpty(1000, 980)))
}

// Abandoning clears the observation, so a hole seen again later (a rebuild
// replayed the same range) is judged from scratch.
func TestTracker_JudgesAReappearingHoleFromScratch(t *testing.T) {
	var tracker Tracker
	hole := Hole{First: 41, Last: 41}
	tracker.MayAbandon(hole, stillEmpty(950, 900))
	assert.True(t, tracker.MayAbandon(hole, stillEmpty(980, 950)))

	assert.False(t, tracker.MayAbandon(hole, stillEmpty(1000, 980)))
}
