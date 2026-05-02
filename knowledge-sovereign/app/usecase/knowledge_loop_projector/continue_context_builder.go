package knowledge_loop_projector

import (
	"encoding/json"
	"time"

	"knowledge-sovereign/driver/sovereign_db"
)

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

func continueSummary(action string) string {
	switch action {
	case "open", "opened", "read":
		return "Last opened before this loop pass."
	case "ask", "asked":
		return "Last question is ready to continue."
	case "save", "saved":
		return "Saved earlier; ready for the next step."
	default:
		return "Previous interaction is ready to continue."
	}
}
