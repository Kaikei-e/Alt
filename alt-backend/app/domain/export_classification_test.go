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
