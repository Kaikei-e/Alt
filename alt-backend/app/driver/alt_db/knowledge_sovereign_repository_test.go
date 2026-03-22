package alt_db

import (
	"alt/port/knowledge_sovereign_port"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKnowledgeSovereign_ApplyProjectionMutation_ReturnsNotImplemented(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{
		MutationType: "upsert_home_item",
		EntityID:     "article-123",
	})
	require.ErrorIs(t, err, ErrKnowledgeSovereignNotImplemented)
}

func TestKnowledgeSovereign_ApplyRecallMutation_ReturnsNotImplemented(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: "upsert_candidate",
		EntityID:     "article-123",
	})
	require.ErrorIs(t, err, ErrKnowledgeSovereignNotImplemented)
}

func TestKnowledgeSovereign_ApplyCurationMutation_ReturnsNotImplemented(t *testing.T) {
	repo := &AltDBRepository{}
	err := repo.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: "dismiss",
		EntityID:     "article-123",
	})
	require.ErrorIs(t, err, ErrKnowledgeSovereignNotImplemented)
}

func TestKnowledgeSovereign_ResolveRetentionDecision_ReturnsNotImplemented(t *testing.T) {
	repo := &AltDBRepository{}
	_, err := repo.ResolveRetentionDecision(context.Background(), "article_metadata", "article-123")
	require.ErrorIs(t, err, ErrKnowledgeSovereignNotImplemented)
}

func TestKnowledgeSovereign_ResolveExportScope_ReturnsNotImplemented(t *testing.T) {
	repo := &AltDBRepository{}
	_, err := repo.ResolveExportScope(context.Background(), "feed_subscriptions", "sub-123")
	require.ErrorIs(t, err, ErrKnowledgeSovereignNotImplemented)
}
