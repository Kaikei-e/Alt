package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeContextBudget_NormalCase(t *testing.T) {
	budget := ComputeContextBudget(500, 100, 6000)
	// Available = 6000 - 500 - 100 - 50 = 5350
	// MaxChunks = min(7, 5350/400) = 7 (capped)
	assert.Equal(t, 5350, budget.MaxTokens)
	assert.Equal(t, 7, budget.MaxChunks)
	assert.InDelta(t, 0.25, budget.MinScore, 0.01)
}

func TestComputeContextBudget_TightBudget(t *testing.T) {
	budget := ComputeContextBudget(1500, 200, 2000)
	// Available = 2000 - 1500 - 200 - 50 = 250
	// MaxChunks = 250/400 = 0, but clamped to 1 since available > 0
	assert.Equal(t, 250, budget.MaxTokens)
	assert.Equal(t, 1, budget.MaxChunks)
}

func TestComputeContextBudget_NoBudget(t *testing.T) {
	budget := ComputeContextBudget(5000, 1000, 5000)
	// Available = 5000 - 5000 - 1000 - 50 = negative → 0
	assert.Equal(t, 0, budget.MaxTokens)
	assert.Equal(t, 0, budget.MaxChunks)
}

func TestComputeContextBudget_MediumBudget(t *testing.T) {
	budget := ComputeContextBudget(700, 100, 3000)
	// Available = 3000 - 700 - 100 - 50 = 2150
	// MaxChunks = min(7, 2150/400) = min(7, 5) = 5
	assert.Equal(t, 2150, budget.MaxTokens)
	assert.Equal(t, 5, budget.MaxChunks)
}

func TestComputeContextBudget_IntegrationWithTemplateRegistry(t *testing.T) {
	reg := NewTemplateRegistry()

	// Causal template should have reasonable token estimate
	causalTokens := reg.EstimateSystemTokens(PromptInput{IntentType: IntentCausalExplanation})
	assert.Greater(t, causalTokens, 100)
	assert.Less(t, causalTokens, 900) // must be smaller than old 1500

	// Budget should leave room for context
	budget := ComputeContextBudget(causalTokens, 50, 6000)
	assert.Greater(t, budget.MaxChunks, 3)
	assert.Greater(t, budget.MaxTokens, 3000)
}

func TestEstimateSystemPromptTokens_AlphaV2UsesRegistry(t *testing.T) {
	reg := NewTemplateRegistry()

	// All intent types should produce estimates greater than 0 and less than old hardcoded 500
	// (since templates are 60% smaller than the monolithic builder)
	intents := []IntentType{
		IntentGeneral,
		IntentCausalExplanation,
		IntentSynthesis,
		IntentComparison,
		IntentTemporal,
		IntentFactCheck,
		IntentTopicDeepDive,
	}
	for _, intent := range intents {
		tokens := reg.EstimateSystemTokens(PromptInput{IntentType: intent})
		assert.Greater(t, tokens, 0, "intent %s should have positive token estimate", intent)
		assert.Less(t, tokens, 500, "intent %s should be smaller than legacy 500 estimate", intent)
	}
}

func TestEstimateSystemPromptTokens_LegacyFallback(t *testing.T) {
	// When promptVersion is not "alpha-v2", estimateSystemPromptTokens should
	// return the legacy hardcoded value of 500.
	// This is tested via the exported EstimateSystemPromptTokens helper.
	tokens := EstimateSystemPromptTokens("legacy", IntentGeneral, nil)
	assert.Equal(t, 500, tokens)
}

func TestEstimateSystemPromptTokens_AlphaV2WithRegistry(t *testing.T) {
	reg := NewTemplateRegistry()
	tokens := EstimateSystemPromptTokens("alpha-v2", IntentCausalExplanation, reg)
	assert.Greater(t, tokens, 0)
	assert.Less(t, tokens, 500)
}
