package usecase

// ContextBudget defines the token allocation for context chunks.
type ContextBudget struct {
	MaxTokens int
	MaxChunks int
	MinScore  float32
}

// ComputeContextBudget calculates how many tokens/chunks are available for context
// given the system prompt size and other overhead.
func ComputeContextBudget(systemTokens, queryTokens, maxPromptTokens int) ContextBudget {
	overhead := 50 // framing tokens
	available := maxPromptTokens - systemTokens - queryTokens - overhead
	if available < 0 {
		available = 0
	}

	// ~400 tokens per chunk average (Japanese + English bilingual chunks)
	tokensPerChunk := 400
	maxChunks := available / tokensPerChunk
	if maxChunks > 7 {
		maxChunks = 7
	}
	if maxChunks < 1 && available > 0 {
		maxChunks = 1
	}

	return ContextBudget{
		MaxTokens: available,
		MaxChunks: maxChunks,
		MinScore:  0.25,
	}
}

// EstimateSystemPromptTokens returns the estimated system prompt token count.
// For "alpha-v2" prompt version, it uses the TemplateRegistry for accurate
// per-intent estimation. For legacy versions, it returns the hardcoded 500.
func EstimateSystemPromptTokens(promptVersion string, intentType IntentType, registry *TemplateRegistry) int {
	if promptVersion == "alpha-v2" && registry != nil {
		return registry.EstimateSystemTokens(PromptInput{IntentType: intentType})
	}
	return 500
}
