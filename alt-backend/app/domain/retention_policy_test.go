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
