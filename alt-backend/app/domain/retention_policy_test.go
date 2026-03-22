package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetentionTierConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"hot", RetentionTierHot, "hot"},
		{"warm", RetentionTierWarm, "warm"},
		{"cold", RetentionTierCold, "cold"},
		{"archive", RetentionTierArchive, "archive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

func TestRetentionPolicy_Fields(t *testing.T) {
	p := RetentionPolicy{
		EntityType:     "article_metadata",
		Tier:           RetentionTierHot,
		ExportPriority: "high",
	}
	assert.Equal(t, "article_metadata", p.EntityType)
	assert.Equal(t, RetentionTierHot, p.Tier)
	assert.Equal(t, "high", p.ExportPriority)
}

func TestDefaultRetentionMatrix_ContainsAllEntityTypes(t *testing.T) {
	required := []string{
		"article_metadata", "article_raw_body",
		"summary_latest", "summary_old_versions",
		"tag_latest", "tag_old_versions",
		"recall_raw_signals", "recall_aggregates",
		"recap_result", "user_curation_state", "export_manifest",
	}
	for _, et := range required {
		t.Run(et, func(t *testing.T) {
			policy, ok := DefaultRetentionMatrix[et]
			assert.True(t, ok, "missing entity type: %s", et)
			if ok {
				assert.Equal(t, et, policy.EntityType)
				assert.NotEmpty(t, policy.Tier)
				assert.NotEmpty(t, policy.ExportPriority)
			}
		})
	}
}

func TestDefaultRetentionMatrix_TierValues(t *testing.T) {
	assert.Equal(t, RetentionTierHot, DefaultRetentionMatrix["article_metadata"].Tier)
	assert.Equal(t, RetentionTierWarm, DefaultRetentionMatrix["article_raw_body"].Tier)
	assert.Equal(t, RetentionTierCold, DefaultRetentionMatrix["summary_old_versions"].Tier)
}
