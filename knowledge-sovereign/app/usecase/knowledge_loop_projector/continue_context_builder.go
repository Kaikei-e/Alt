package knowledge_loop_projector

import (
	"encoding/json"
	"time"

	"knowledge-sovereign/driver/sovereign_db"
)

// recentActionLabelsBound is the maximum number of semantic action labels
// the projector keeps on continue_context.recent_action_labels. Bounded
// list-building is reproject-safe because the caller passes the bounded
// slice of acted events that ended at-or-before the current event seq.
const recentActionLabelsBound = 5

func buildContinueContextJSON(ev *sovereign_db.KnowledgeEvent) []byte {
	if ev == nil {
		return nil
	}
	action := extractStringField(ev.Payload, "action_type", "action")
	if action == "" {
		action = "open"
	}
	interactedAt := eventObservedAt(ev)
	type out struct {
		Summary            string   `json:"summary"`
		RecentActionLabels []string `json:"recent_action_labels"`
		LastInteractedAt   string   `json:"last_interacted_at,omitempty"`
	}
	body := out{
		Summary:            continueSummary(action),
		RecentActionLabels: []string{action},
	}
	if !interactedAt.IsZero() {
		body.LastInteractedAt = interactedAt.UTC().Format(time.RFC3339)
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	return b
}

// buildContinueContextFromActed builds the continue_context JSON for the
// Phase 2 semantic feedback loop. `recent` MUST be ordered most-recent-first
// (matching ListKnowledgeLoopActedEventsForEntry). The function is pure: it
// reads only event payload + occurred_at and never the current projection
// row. Reproject-safe.
func buildContinueContextFromActed(recent []sovereign_db.KnowledgeEvent) []byte {
	if len(recent) == 0 {
		return nil
	}
	labels := make([]string, 0, recentActionLabelsBound)
	seen := make(map[string]struct{}, recentActionLabelsBound)
	var lastInteractedAt time.Time
	for _, ev := range recent {
		intent := extractStringField(ev.Payload, "acted_intent")
		label := semanticActionLabel(intent)
		if label == "" {
			continue
		}
		if _, dup := seen[label]; dup {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
		if lastInteractedAt.IsZero() {
			lastInteractedAt = ev.OccurredAt
		}
		if len(labels) >= recentActionLabelsBound {
			break
		}
	}
	if len(labels) == 0 {
		return nil
	}
	type out struct {
		Summary            string   `json:"summary"`
		RecentActionLabels []string `json:"recent_action_labels"`
		LastInteractedAt   string   `json:"last_interacted_at,omitempty"`
	}
	body := out{
		Summary:            continueSummary(labels[0]),
		RecentActionLabels: labels,
	}
	if !lastInteractedAt.IsZero() {
		body.LastInteractedAt = lastInteractedAt.UTC().Format(time.RFC3339)
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	return b
}

// semanticActionLabel maps a Phase 2 `acted_intent` enum string (proto
// long form, e.g. "DECISION_INTENT_OPEN") or short form ("open") to the
// canonical past-tense label written into recent_action_labels.
func semanticActionLabel(intent string) string {
	switch intent {
	case "DECISION_INTENT_OPEN", "open":
		return "opened"
	case "DECISION_INTENT_ASK", "ask":
		return "asked"
	case "DECISION_INTENT_SAVE", "save":
		return "saved"
	case "DECISION_INTENT_COMPARE", "compare":
		return "compared"
	case "DECISION_INTENT_REVISIT", "revisit":
		return "revisited"
	case "DECISION_INTENT_SNOOZE", "snooze":
		return "snoozed"
	default:
		return ""
	}
}

func continueSummary(action string) string {
	switch action {
	case "open", "opened", "read":
		return "Last opened before this loop pass."
	case "ask", "asked":
		return "Last question is ready to continue."
	case "save", "saved":
		return "Saved earlier; ready for the next step."
	case "compare", "compared":
		return "Compared versions; pick up the next step."
	case "revisit", "revisited":
		return "Revisited recently; ready to continue."
	case "snooze", "snoozed":
		return "Snoozed earlier; consider revisiting."
	default:
		return "Previous interaction is ready to continue."
	}
}
