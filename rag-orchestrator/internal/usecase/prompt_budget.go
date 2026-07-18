package usecase

// EstimateSystemPromptTokens returns the estimated system prompt token count.
// For "alpha-v2" prompt version, it uses the TemplateRegistry for accurate
// per-intent estimation. For legacy versions, it returns the hardcoded 500.
func EstimateSystemPromptTokens(promptVersion string, intentType IntentType, registry *TemplateRegistry) int {
	if promptVersion == "alpha-v2" && registry != nil {
		return registry.EstimateSystemTokens(PromptInput{IntentType: intentType})
	}
	return 500
}
