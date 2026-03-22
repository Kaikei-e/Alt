package knowledge_write_service_usecase

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type mockProjectionMutator struct {
	calls []knowledge_sovereign_port.ProjectionMutation
	err   error
}

func (m *mockProjectionMutator) ApplyProjectionMutation(_ context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mutation)
	return nil
}

type mockRecallMutator struct {
	calls []knowledge_sovereign_port.RecallMutation
	err   error
}

func (m *mockRecallMutator) ApplyRecallMutation(_ context.Context, mutation knowledge_sovereign_port.RecallMutation) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mutation)
	return nil
}

type mockCurationMutator struct {
	calls []knowledge_sovereign_port.CurationMutation
	err   error
}

func (m *mockCurationMutator) ApplyCurationMutation(_ context.Context, mutation knowledge_sovereign_port.CurationMutation) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mutation)
	return nil
}

type mockRetentionResolver struct {
	policy domain.RetentionPolicy
	err    error
	called bool
}

func (m *mockRetentionResolver) ResolveRetentionDecision(_ context.Context, _, _ string) (domain.RetentionPolicy, error) {
	m.called = true
	return m.policy, m.err
}

type mockExportScopeResolver struct {
	classification domain.ExportClassification
	err            error
	called         bool
}

func (m *mockExportScopeResolver) ResolveExportScope(_ context.Context, _, _ string) (domain.ExportClassification, error) {
	m.called = true
	return m.classification, m.err
}

// --- ApplyProjectionMutation ---

func TestApplyProjectionMutation_DelegatesToPort(t *testing.T) {
	mock := &mockProjectionMutator{}
	uc := NewKnowledgeWriteServiceUsecase(mock, nil, nil, nil, nil)

	mutation := knowledge_sovereign_port.ProjectionMutation{
		MutationType: "upsert_home_item",
		EntityID:     "article-123",
	}
	err := uc.ApplyProjectionMutation(context.Background(), mutation)
	require.NoError(t, err)
	assert.Len(t, mock.calls, 1)
	assert.Equal(t, "article-123", mock.calls[0].EntityID)
}

func TestApplyProjectionMutation_PortError(t *testing.T) {
	mock := &mockProjectionMutator{err: errors.New("port failure")}
	uc := NewKnowledgeWriteServiceUsecase(mock, nil, nil, nil, nil)

	err := uc.ApplyProjectionMutation(context.Background(), knowledge_sovereign_port.ProjectionMutation{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply projection mutation")
}

// --- ApplyRecallMutation ---

func TestApplyRecallMutation_DelegatesToPort(t *testing.T) {
	mock := &mockRecallMutator{}
	uc := NewKnowledgeWriteServiceUsecase(nil, mock, nil, nil, nil)

	err := uc.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{
		MutationType: "upsert_candidate",
		EntityID:     "article-456",
	})
	require.NoError(t, err)
	assert.Len(t, mock.calls, 1)
}

func TestApplyRecallMutation_PortError(t *testing.T) {
	mock := &mockRecallMutator{err: errors.New("port failure")}
	uc := NewKnowledgeWriteServiceUsecase(nil, mock, nil, nil, nil)

	err := uc.ApplyRecallMutation(context.Background(), knowledge_sovereign_port.RecallMutation{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply recall mutation")
}

// --- ApplyCurationMutation ---

func TestApplyCurationMutation_DelegatesToPort(t *testing.T) {
	mock := &mockCurationMutator{}
	uc := NewKnowledgeWriteServiceUsecase(nil, nil, mock, nil, nil)

	err := uc.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{
		MutationType: "dismiss",
		EntityID:     "article-789",
	})
	require.NoError(t, err)
	assert.Len(t, mock.calls, 1)
}

func TestApplyCurationMutation_PortError(t *testing.T) {
	mock := &mockCurationMutator{err: errors.New("port failure")}
	uc := NewKnowledgeWriteServiceUsecase(nil, nil, mock, nil, nil)

	err := uc.ApplyCurationMutation(context.Background(), knowledge_sovereign_port.CurationMutation{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply curation mutation")
}

// --- ResolveRetentionDecision ---

func TestResolveRetentionDecision_DelegatesToPort(t *testing.T) {
	expected := domain.RetentionPolicy{EntityType: "article_metadata", Tier: domain.RetentionTierHot}
	mock := &mockRetentionResolver{policy: expected}
	uc := NewKnowledgeWriteServiceUsecase(nil, nil, nil, mock, nil)

	got, err := uc.ResolveRetentionDecision(context.Background(), "article_metadata", "article-123")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.True(t, mock.called)
}

func TestResolveRetentionDecision_PortError(t *testing.T) {
	mock := &mockRetentionResolver{err: errors.New("port failure")}
	uc := NewKnowledgeWriteServiceUsecase(nil, nil, nil, mock, nil)

	_, err := uc.ResolveRetentionDecision(context.Background(), "article_metadata", "article-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve retention decision")
}

// --- ResolveExportScope ---

func TestResolveExportScope_DelegatesToPort(t *testing.T) {
	expected := domain.ExportClassification{EntityType: "feed_subscriptions", Tier: domain.ExportTierA}
	mock := &mockExportScopeResolver{classification: expected}
	uc := NewKnowledgeWriteServiceUsecase(nil, nil, nil, nil, mock)

	got, err := uc.ResolveExportScope(context.Background(), "feed_subscriptions", "sub-123")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.True(t, mock.called)
}

func TestResolveExportScope_PortError(t *testing.T) {
	mock := &mockExportScopeResolver{err: errors.New("port failure")}
	uc := NewKnowledgeWriteServiceUsecase(nil, nil, nil, nil, mock)

	_, err := uc.ResolveExportScope(context.Background(), "feed_subscriptions", "sub-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve export scope")
}
