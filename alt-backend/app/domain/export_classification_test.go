package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportTierConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"tier A - non-regenerable", ExportTierA, "A"},
		{"tier B - regenerable but costly", ExportTierB, "B"},
		{"tier C - easily regenerable", ExportTierC, "C"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

func TestExportClassification_Fields(t *testing.T) {
	c := ExportClassification{
		EntityType: "feed_subscriptions",
		Tier:       ExportTierA,
		Reason:     "user configuration, non-regenerable",
	}
	assert.Equal(t, "feed_subscriptions", c.EntityType)
	assert.Equal(t, ExportTierA, c.Tier)
	assert.Equal(t, "user configuration, non-regenerable", c.Reason)
}

func TestDefaultExportClassification_ContainsAllEntityTypes(t *testing.T) {
	required := []string{
		"feed_subscriptions", "user_curation_state",
		"summary_latest", "tag_latest",
		"recall_candidates", "projection_checkpoints",
	}
	for _, et := range required {
		t.Run(et, func(t *testing.T) {
			cls, ok := DefaultExportClassification[et]
			assert.True(t, ok, "missing entity type: %s", et)
			if ok {
				assert.Equal(t, et, cls.EntityType)
				assert.NotEmpty(t, cls.Tier)
				assert.NotEmpty(t, cls.Reason)
			}
		})
	}
}

func TestDefaultExportClassification_TierValues(t *testing.T) {
	assert.Equal(t, ExportTierA, DefaultExportClassification["feed_subscriptions"].Tier)
	assert.Equal(t, ExportTierB, DefaultExportClassification["summary_latest"].Tier)
	assert.Equal(t, ExportTierC, DefaultExportClassification["recall_candidates"].Tier)
}
