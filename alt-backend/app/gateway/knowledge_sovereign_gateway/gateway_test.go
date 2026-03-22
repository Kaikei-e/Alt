package knowledge_sovereign_gateway

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ApplyProjectionMutation ---

func TestApplyProjectionMutation_DelegatesToRepo(t *testing.T) {
	stub := &stubRepo{}
	gw := NewGateway(stub)

	mutation := knowledge_sovereign_port.ProjectionMutation{
		MutationType: "upsert_home_item",
		EntityID:     "article-123",
	}
	err := gw.ApplyProjectionMutation(context.Background(), mutation)
	require.NoError(t, err)
	assert.True(t, stub.projectionCalled)
}

func TestApplyProjectionMutation_NilRepo(t *testing.T) {
	gw := NewGateway(nil)
	err := gw.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{})
	require.Error(t, err)
}

func TestApplyProjectionMutation_RepoError(t *testing.T) {
	repoErr := errors.New("db failure")
	stub := &stubRepo{err: repoErr}
	gw := NewGateway(stub)

	err := gw.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{})
	require.ErrorIs(t, err, repoErr)
}

// --- ApplyRecallMutation ---

func TestApplyRecallMutation_DelegatesToRepo(t *testing.T) {
	stub := &stubRepo{}
	gw := NewGateway(stub)

	err := gw.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: "upsert_candidate",
		EntityID:     "article-123",
	})
	require.NoError(t, err)
	assert.True(t, stub.recallCalled)
}

func TestApplyRecallMutation_NilRepo(t *testing.T) {
	gw := NewGateway(nil)
	err := gw.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{})
	require.Error(t, err)
}

// --- ApplyCurationMutation ---

func TestApplyCurationMutation_DelegatesToRepo(t *testing.T) {
	stub := &stubRepo{}
	gw := NewGateway(stub)

	err := gw.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: "dismiss",
		EntityID:     "article-123",
	})
	require.NoError(t, err)
	assert.True(t, stub.curationCalled)
}

func TestApplyCurationMutation_NilRepo(t *testing.T) {
	gw := NewGateway(nil)
	err := gw.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{})
	require.Error(t, err)
}

// --- ResolveRetentionDecision ---

func TestResolveRetentionDecision_DelegatesToRepo(t *testing.T) {
	expected := domain.RetentionPolicy{EntityType: "article_metadata", Tier: domain.RetentionTierHot}
	stub := &stubRepo{retentionPolicy: expected}
	gw := NewGateway(stub)

	got, err := gw.ResolveRetentionDecision(context.Background(), "article_metadata", "article-123")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.True(t, stub.retentionCalled)
}

func TestResolveRetentionDecision_NilRepo(t *testing.T) {
	gw := NewGateway(nil)
	_, err := gw.ResolveRetentionDecision(context.Background(), "article_metadata", "article-123")
	require.Error(t, err)
}

// --- ResolveExportScope ---

func TestResolveExportScope_DelegatesToRepo(t *testing.T) {
	expected := domain.ExportClassification{EntityType: "feed_subscriptions", Tier: domain.ExportTierA}
	stub := &stubRepo{exportClassification: expected}
	gw := NewGateway(stub)

	got, err := gw.ResolveExportScope(context.Background(), "feed_subscriptions", "sub-123")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.True(t, stub.exportCalled)
}

func TestResolveExportScope_NilRepo(t *testing.T) {
	gw := NewGateway(nil)
	_, err := gw.ResolveExportScope(context.Background(), "feed_subscriptions", "sub-123")
	require.Error(t, err)
}

// --- stub ---

type stubRepo struct {
	projectionCalled     bool
	recallCalled         bool
	curationCalled       bool
	retentionCalled      bool
	exportCalled         bool
	retentionPolicy      domain.RetentionPolicy
	exportClassification domain.ExportClassification
	err                  error
}

func (s *stubRepo) ApplyProjectionMutation(_ context.Context, _ knowledge_sovereign_port.ProjectionMutation) error {
	s.projectionCalled = true
	return s.err
}

func (s *stubRepo) ApplyRecallMutation(_ context.Context, _ knowledge_sovereign_port.RecallMutation) error {
	s.recallCalled = true
	return s.err
}

func (s *stubRepo) ApplyCurationMutation(_ context.Context, _ knowledge_sovereign_port.CurationMutation) error {
	s.curationCalled = true
	return s.err
}

func (s *stubRepo) ResolveRetentionDecision(_ context.Context, _ string, _ string) (domain.RetentionPolicy, error) {
	s.retentionCalled = true
	return s.retentionPolicy, s.err
}

func (s *stubRepo) ResolveExportScope(_ context.Context, _ string, _ string) (domain.ExportClassification, error) {
	s.exportCalled = true
	return s.exportClassification, s.err
}
