package usecase

// Named LLM max-token budgets for short generation calls.
// Keep these centralized so prompt/tool call sites stay consistent.
const (
	// LLMTokensToolChat caps a single agentic tool-calling turn.
	LLMTokensToolChat = 256
	// LLMTokensToolPlan caps JSON tool-plan generation.
	LLMTokensToolPlan = 500
)
