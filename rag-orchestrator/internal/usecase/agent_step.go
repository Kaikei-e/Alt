package usecase

// AgentStep records a single step in the agentic RAG pipeline.
type AgentStep struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Result     string `json:"result"` // "success", "retry", "fallback", "skipped"
	Detail     string `json:"detail"` // Additional info (e.g., intent type, tool name)
}
