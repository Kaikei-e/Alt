package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOwnershipDomainConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"knowledge events", OwnershipKnowledgeEvents, "knowledge_events"},
		{"home projection", OwnershipHomeProjection, "home_projection"},
		{"recall candidate", OwnershipRecallCandidate, "recall_candidate"},
		{"curation state", OwnershipCurationState, "curation_state"},
		{"retention", OwnershipRetention, "retention"},
		{"export policy", OwnershipExportPolicy, "export_policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

func TestOwnershipEntry_Fields(t *testing.T) {
	e := OwnershipEntry{
		Domain:       OwnershipKnowledgeEvents,
		CurrentOwner: "alt-backend scattered",
		FutureOwner:  "Knowledge Sovereign",
		Note:         "append/write policy consolidation",
	}
	assert.Equal(t, OwnershipKnowledgeEvents, e.Domain)
	assert.Equal(t, "alt-backend scattered", e.CurrentOwner)
	assert.Equal(t, "Knowledge Sovereign", e.FutureOwner)
	assert.Equal(t, "append/write policy consolidation", e.Note)
}
