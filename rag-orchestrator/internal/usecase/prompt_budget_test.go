package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateSystemPromptTokens_LegacyVersion(t *testing.T) {
	assert.Equal(t, 500, EstimateSystemPromptTokens("alpha-v1", IntentGeneral, nil))
}

func TestEstimateSystemPromptTokens_AlphaV2UsesRegistry(t *testing.T) {
	registry := NewTemplateRegistry()
	causalTokens := EstimateSystemPromptTokens("alpha-v2", IntentCausalExplanation, registry)
	assert.Greater(t, causalTokens, 0)
	assert.NotEqual(t, 500, causalTokens)
}

// TestEstimateSystemPromptTokens_AlphaV2NilRegistry_FallsBackToLegacy covers
// the branch EstimateSystemPromptTokens takes when a caller requests alpha-v2
// but doesn't (or can't) supply the template registry — it must degrade to
// the legacy fixed estimate instead of panicking on a nil dereference.
func TestEstimateSystemPromptTokens_AlphaV2NilRegistry_FallsBackToLegacy(t *testing.T) {
	assert.Equal(t, 500, EstimateSystemPromptTokens("alpha-v2", IntentCausalExplanation, nil))
}
