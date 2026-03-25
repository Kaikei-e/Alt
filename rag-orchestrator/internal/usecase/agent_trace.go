package usecase

import "time"

// AgentStep records a single step in the agentic RAG pipeline.
type AgentStep struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Result     string `json:"result"` // "success", "retry", "fallback", "skipped"
	Detail     string `json:"detail"` // Additional info (e.g., intent type, tool name)
}

// AgentTrace collects all steps executed during an agentic RAG request.
type AgentTrace struct {
	RequestID string      `json:"request_id"`
	Steps     []AgentStep `json:"steps"`
}

// NewAgentTrace creates a new trace for a request.
func NewAgentTrace(requestID string) *AgentTrace {
	return &AgentTrace{
		RequestID: requestID,
		Steps:     make([]AgentStep, 0, 8),
	}
}

// AddStep records a completed step.
func (t *AgentTrace) AddStep(name, result, detail string, duration time.Duration) {
	t.Steps = append(t.Steps, AgentStep{
		Name:       name,
		DurationMs: duration.Milliseconds(),
		Result:     result,
		Detail:     detail,
	})
}

// TotalDurationMs returns the sum of all step durations.
func (t *AgentTrace) TotalDurationMs() int64 {
	var total int64
	for _, s := range t.Steps {
		total += s.DurationMs
	}
	return total
}
