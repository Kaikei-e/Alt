package knowledge_sovereign_port

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock implementations ---

type mockProjectionMutator struct {
	calls []ProjectionMutation
	err   error
}

func (m *mockProjectionMutator) ApplyProjectionMutation(_ context.Context, mutation ProjectionMutation) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mutation)
	return nil
}

type mockRecallMutator struct {
	calls []RecallMutation
	err   error
}

func (m *mockRecallMutator) ApplyRecallMutation(_ context.Context, mutation RecallMutation) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mutation)
	return nil
}

type mockCurationMutator struct {
	calls []CurationMutation
	err   error
}

func (m *mockCurationMutator) ApplyCurationMutation(_ context.Context, mutation CurationMutation) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, mutation)
	return nil
}

// --- interface compile checks ---

func TestProjectionMutatorInterfaceCompiles(t *testing.T) {
	var iface ProjectionMutator = &mockProjectionMutator{}
	require.NotNil(t, iface)

	err := iface.ApplyProjectionMutation(context.Background(), ProjectionMutation{
		MutationType: "upsert_home_item",
		EntityID:     "article-123",
	})
	assert.NoError(t, err)
}

func TestRecallMutatorInterfaceCompiles(t *testing.T) {
	var iface RecallMutator = &mockRecallMutator{}
	require.NotNil(t, iface)

	err := iface.ApplyRecallMutation(context.Background(), RecallMutation{
		MutationType: "upsert_candidate",
		EntityID:     "article-123",
	})
	assert.NoError(t, err)
}

func TestCurationMutatorInterfaceCompiles(t *testing.T) {
	var iface CurationMutator = &mockCurationMutator{}
	require.NotNil(t, iface)

	err := iface.ApplyCurationMutation(context.Background(), CurationMutation{
		MutationType: "dismiss",
		EntityID:     "article-123",
	})
	assert.NoError(t, err)
}

// --- mutation type constant tests ---

func TestProjectionMutationTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"upsert home item", MutationUpsertHomeItem, "upsert_home_item"},
		{"dismiss home item", MutationDismissHomeItem, "dismiss_home_item"},
		{"clear supersede", MutationClearSupersede, "clear_supersede"},
		{"upsert today digest", MutationUpsertTodayDigest, "upsert_today_digest"},
		{"upsert recall candidate", MutationUpsertRecallCandidate, "upsert_recall_candidate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

func TestCurationMutationTypeConstants(t *testing.T) {
	assert.Equal(t, "dismiss_curation", MutationDismissCuration)
}

func TestLensMutationTypeConstants(t *testing.T) {
	assert.Equal(t, "create_lens", MutationCreateLens)
	assert.Equal(t, "create_lens_version", MutationCreateLensVersion)
	assert.Equal(t, "select_lens", MutationSelectLens)
	assert.Equal(t, "clear_lens", MutationClearLens)
	assert.Equal(t, "archive_lens", MutationArchiveLens)
}

func TestMutationIdempotencyKeyField(t *testing.T) {
	pm := ProjectionMutation{
		MutationType:   MutationUpsertHomeItem,
		EntityID:       "article:test",
		IdempotencyKey: "upsert_home_item:article:test",
	}
	assert.Equal(t, "upsert_home_item:article:test", pm.IdempotencyKey)

	rm := RecallMutation{
		MutationType:   MutationSnoozeCandidate,
		EntityID:       "article:test",
		IdempotencyKey: "snooze_candidate:article:test",
	}
	assert.Equal(t, "snooze_candidate:article:test", rm.IdempotencyKey)

	cm := CurationMutation{
		MutationType:   MutationDismissCuration,
		EntityID:       "article:test",
		IdempotencyKey: "dismiss_curation:article:test",
	}
	assert.Equal(t, "dismiss_curation:article:test", cm.IdempotencyKey)
}

func TestRecallMutationTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"upsert candidate", MutationUpsertCandidate, "upsert_candidate"},
		{"snooze candidate", MutationSnoozeCandidate, "snooze_candidate"},
		{"dismiss candidate", MutationDismissCandidate, "dismiss_candidate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}
