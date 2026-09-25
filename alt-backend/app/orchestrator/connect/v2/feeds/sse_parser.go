package feeds

import (
	"encoding/json"
	"strings"
)

// extractSSEData extracts data content from an SSE event string, decoding JSON if possible.
func extractSSEData(eventStr string) string {
	var result strings.Builder
	lines := strings.Split(eventStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			dataContent := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var decoded string
			if err := json.Unmarshal([]byte(dataContent), &decoded); err == nil {
				result.WriteString(decoded)
			} else {
				result.WriteString(dataContent)
			}
		}
	}
	return result.String()
}
