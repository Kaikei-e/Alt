package summarization

import (
	"encoding/json"
	"strings"
)

func parseSSESummary(sseData string) string {
	if !strings.Contains(sseData, "data:") {
		return sseData
	}

	var result strings.Builder
	lines := strings.Split(sseData, "\n")
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

	if result.Len() == 0 && len(sseData) > 0 {
		return result.String()
	}

	return result.String()
}
