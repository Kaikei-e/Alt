package knowledge_loop_session_state

import (
	"encoding/json"
	"sort"
	"time"

	"knowledge-sovereign/driver/sovereign_db"
)

// CognitiveLoadHint is the qualitative summary of the user's macro layer
// that the UI uses to bias affordances (e.g. promote a "clear backlog"
// CTA under "heavy"). An empty value means the projector has nothing to
// report — the byline is hidden entirely.
type CognitiveLoadHint string

const (
	CognitiveLoadHintUnspecified CognitiveLoadHint = ""
	CognitiveLoadHintLight       CognitiveLoadHint = "light"
	CognitiveLoadHintMedium      CognitiveLoadHint = "medium"
	CognitiveLoadHintHeavy       CognitiveLoadHint = "heavy"
)

// Event type literals match knowledge_loop_projector.constants. We do not
// import that package to avoid a cyclic dependency once the projector
// starts calling BuildMacroState.
const (
	eventTypeActed      = "knowledge_loop.acted.v1"
	eventTypeReviewed   = "knowledge_loop.reviewed.v1"
	eventTypeActOutcome = "knowledge_loop.act_outcome.v1"
)

// Review trigger literals match the proto enum string serialisation used
// in event payloads (see knowledge_loop_projector/review_lifecycle.go).
const (
	reviewTriggerRecheck      = "TRANSITION_TRIGGER_RECHECK"
	reviewTriggerArchive      = "TRANSITION_TRIGGER_ARCHIVE"
	reviewTriggerMarkReviewed = "TRANSITION_TRIGGER_MARK_REVIEWED"
)

const outcomeInternalized = "internalized"

// MacroState is the 7d aggregation result that the projector writes to
// the knowledge_loop_macro_state projection. Every field is derived from
// event payload — no wall-clock, no latest-state queries.
type MacroState struct {
	ActiveContinueThreads   uint32
	PendingReviewCount      uint32
	RecentInternalizedCount uint32
	CognitiveLoadHint       CognitiveLoadHint
	WindowStartAt           time.Time
	WindowEndAt             time.Time
	SeqHiwater              int64
	LensWeightsVersion      int32
}

// BuildMacroState reduces a window of knowledge_events into the macro
// projection row. Pure function — identical inputs always produce
// identical output regardless of wall-clock or input ordering. See the
// package doc for the full semantic.
func BuildMacroState(
	events []sovereign_db.KnowledgeEvent,
	windowEnd time.Time,
	lookback time.Duration,
	weights LensModeWeights,
	lensWeightsVersion int32,
) MacroState {
	windowEndUTC := windowEnd.UTC()
	windowStartUTC := windowEndUTC.Add(-lookback)

	state := MacroState{
		WindowStartAt:      windowStartUTC,
		WindowEndAt:        windowEndUTC,
		LensWeightsVersion: lensWeightsVersion,
	}

	// Sort by (event_seq ASC) so "later" semantic uses the canonical
	// event log ordering, not the caller's slice order. This is the
	// reproject-safety hinge: any shuffle of the input collapses to the
	// same ordering before the reducer touches it.
	sorted := make([]sovereign_db.KnowledgeEvent, 0, len(events))
	for _, ev := range events {
		occurred := ev.OccurredAt
		if occurred.Before(windowStartUTC) || occurred.After(windowEndUTC) {
			continue
		}
		sorted = append(sorted, ev)
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].EventSeq < sorted[j].EventSeq
	})

	// Per-entry latest signals. Tracking the latest acted continue_flag
	// and the latest review trigger is what lets a "supersede" later in
	// the window withdraw an earlier signal without any latest-state
	// query.
	type continueState struct {
		hasSignal    bool
		latestFlag   bool
		internalized bool
	}
	type reviewState struct {
		hasRecheck    bool
		hasResolution bool
	}
	continues := map[string]*continueState{}
	reviews := map[string]*reviewState{}
	internalized := map[string]bool{}

	for _, ev := range sorted {
		if ev.EventSeq > state.SeqHiwater {
			state.SeqHiwater = ev.EventSeq
		}
		entryKey := readEntryKey(ev)
		if entryKey == "" {
			continue
		}
		switch ev.EventType {
		case eventTypeActed:
			cs := continues[entryKey]
			if cs == nil {
				cs = &continueState{}
				continues[entryKey] = cs
			}
			cs.hasSignal = true
			cs.latestFlag = readBoolField(ev, "continue_flag")

		case eventTypeReviewed:
			rs := reviews[entryKey]
			if rs == nil {
				rs = &reviewState{}
				reviews[entryKey] = rs
			}
			switch readStringField(ev, "trigger") {
			case reviewTriggerRecheck:
				rs.hasRecheck = true
			case reviewTriggerMarkReviewed, reviewTriggerArchive:
				rs.hasResolution = true
			}

		case eventTypeActOutcome:
			if readStringField(ev, "outcome") == outcomeInternalized {
				internalized[entryKey] = true
				if cs := continues[entryKey]; cs != nil {
					cs.internalized = true
				} else {
					continues[entryKey] = &continueState{internalized: true}
				}
			}
		}
	}

	for _, cs := range continues {
		if cs.internalized {
			continue
		}
		if cs.hasSignal && cs.latestFlag {
			state.ActiveContinueThreads++
		}
	}
	for _, rs := range reviews {
		if rs.hasRecheck && !rs.hasResolution {
			state.PendingReviewCount++
		}
	}
	state.RecentInternalizedCount = uint32(len(internalized))

	state.CognitiveLoadHint = deriveCognitiveLoadHint(state, weights)
	return state
}

func deriveCognitiveLoadHint(s MacroState, w LensModeWeights) CognitiveLoadHint {
	load := s.ActiveContinueThreads + s.PendingReviewCount
	if load == 0 && s.RecentInternalizedCount == 0 {
		return CognitiveLoadHintUnspecified
	}
	if load == 0 {
		// Only graduations in window: a quiet but positive state.
		return CognitiveLoadHintLight
	}
	if load >= w.HeavyThreshold {
		return CognitiveLoadHintHeavy
	}
	if load >= w.MediumThreshold {
		return CognitiveLoadHintMedium
	}
	return CognitiveLoadHintLight
}

func readEntryKey(ev sovereign_db.KnowledgeEvent) string {
	if k := readStringField(ev, "entry_key"); k != "" {
		return k
	}
	if k := readStringField(ev, "entryKey"); k != "" {
		return k
	}
	return ev.AggregateID
}

func readStringField(ev sovereign_db.KnowledgeEvent, key string) string {
	if len(ev.Payload) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(ev.Payload, &m); err != nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func readBoolField(ev sovereign_db.KnowledgeEvent, key string) bool {
	if len(ev.Payload) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(ev.Payload, &m); err != nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
