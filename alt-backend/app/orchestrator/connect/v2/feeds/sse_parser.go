package feeds

import (
	"encoding/json"
	"fmt"
	"strings"
)

type sseErrorPayload struct {
	Error string `json:"error"`
}

func isErrorPayloadSummary(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	var payload sseErrorPayload
	return json.Unmarshal([]byte(trimmed), &payload) == nil && payload.Error != ""
}

// extractSSEEvent extracts data content from an SSE event string, or returns
// an error if the SSE block is an upstream error event (`event: error`).
func extractSSEEvent(eventStr string) (string, error) {
	var result strings.Builder
	isErrorEvent := false
	lines := strings.Split(eventStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "event:") {
			eventType := strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			if eventType == "error" {
				isErrorEvent = true
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataContent := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if isErrorEvent {
				var errPayload sseErrorPayload
				if err := json.Unmarshal([]byte(dataContent), &errPayload); err == nil && errPayload.Error != "" {
					return "", fmt.Errorf("upstream SSE error: %s", errPayload.Error)
				}
				return "", fmt.Errorf("upstream SSE error: %s", dataContent)
			}
			var decoded string
			if err := json.Unmarshal([]byte(dataContent), &decoded); err == nil {
				result.WriteString(decoded)
			} else if isErrorPayloadSummary(dataContent) {
				var errPayload sseErrorPayload
				_ = json.Unmarshal([]byte(dataContent), &errPayload)
				return "", fmt.Errorf("upstream SSE error: %s", errPayload.Error)
			} else {
				result.WriteString(dataContent)
			}
		}
	}
	if isErrorEvent {
		return "", fmt.Errorf("upstream SSE error event")
	}
	return result.String(), nil
}

// extractSSEData extracts data content from an SSE event string, decoding JSON if possible.
func extractSSEData(eventStr string) string {
	data, err := extractSSEEvent(eventStr)
	if err != nil {
		return ""
	}
	return data
}
