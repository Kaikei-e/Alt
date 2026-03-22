package alt_db

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ApplyProjectionMutation dispatch ---

func TestApplyProjectionMutation_UnknownType(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown projection mutation type")
}

func TestApplyProjectionMutation_InvalidPayload(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationUpsertHomeItem,
		Payload:      []byte(`{invalid`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

// --- ApplyRecallMutation dispatch ---

func TestApplyRecallMutation_UnknownType(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown recall mutation type")
}

// --- ApplyCurationMutation dispatch ---

func TestApplyCurationMutation_UnknownType(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown curation mutation type")
}

// --- ResolveRetentionDecision ---

func TestResolveRetentionDecision_KnownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	policy, err := repo.ResolveRetentionDecision(context.Background(), "article_metadata", "article-123")
	require.NoError(t, err)
	assert.Equal(t, domain.RetentionTierHot, policy.Tier)
	assert.Equal(t, "article_metadata", policy.EntityType)
}

func TestResolveRetentionDecision_UnknownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	_, err := repo.ResolveRetentionDecision(context.Background(), "unknown_type", "id-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown entity type")
}

// --- ResolveExportScope ---

func TestResolveExportScope_KnownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	cls, err := repo.ResolveExportScope(context.Background(), "feed_subscriptions", "sub-123")
	require.NoError(t, err)
	assert.Equal(t, domain.ExportTierA, cls.Tier)
	assert.Equal(t, "feed_subscriptions", cls.EntityType)
}

func TestResolveExportScope_UnknownEntityType(t *testing.T) {
	repo := &AltDBRepository{}
	_, err := repo.ResolveExportScope(context.Background(), "unknown_type", "id-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown entity type")
}
