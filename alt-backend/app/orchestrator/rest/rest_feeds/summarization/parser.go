package summarization

import (
	"encoding/json"
	"strings"
)

type sseErrorObject struct {
	Error string `json:"error"`
}

func isErrorJSON(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	var payload sseErrorObject
	return json.Unmarshal([]byte(trimmed), &payload) == nil && payload.Error != ""
}

func parseSSESummary(sseData string) string {
	if !strings.Contains(sseData, "data:") {
		if isErrorJSON(sseData) {
			return ""
		}
		return sseData
	}

	var result strings.Builder
	isErrorEvent := false
	lines := strings.Split(sseData, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			isErrorEvent = false
			continue
		}
		if strings.HasPrefix(line, "event:") {
			if strings.TrimSpace(strings.TrimPrefix(line, "event:")) == "error" {
				isErrorEvent = true
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if isErrorEvent {
				return ""
			}
			dataContent := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var decoded string
			if err := json.Unmarshal([]byte(dataContent), &decoded); err == nil {
				result.WriteString(decoded)
			} else if isErrorJSON(dataContent) {
				return ""
			} else {
				result.WriteString(dataContent)
			}
		}
	}

	return result.String()
}
