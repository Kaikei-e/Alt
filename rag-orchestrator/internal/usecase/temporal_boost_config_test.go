package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultTemporalBoostConfig(t *testing.T) {
	cfg := DefaultTemporalBoostConfig()

	// Current defaults - these values are application-specific
	// and should be validated through A/B testing in production
	assert.Equal(t, float32(1.3), cfg.Boost6h, "boost for 0-6 hours")
	assert.Equal(t, float32(1.15), cfg.Boost12h, "boost for 6-12 hours")
	assert.Equal(t, float32(1.05), cfg.Boost18h, "boost for 12-18 hours")
}

func TestTemporalBoostConfig_GetBoostFactor(t *testing.T) {
	cfg := DefaultTemporalBoostConfig()

	tests := []struct {
		name       string
		hoursSince float64
		expected   float32
	}{
		{
			name:       "within 6 hours - highest boost",
			hoursSince: 3.0,
			expected:   1.3,
		},
		{
			name:       "exactly 6 hours - boundary",
			hoursSince: 6.0,
			expected:   1.3,
		},
		{
			name:       "between 6-12 hours",
			hoursSince: 9.0,
			expected:   1.15,
		},
		{
			name:       "exactly 12 hours - boundary",
			hoursSince: 12.0,
			expected:   1.15,
		},
		{
			name:       "between 12-18 hours",
			hoursSince: 15.0,
			expected:   1.05,
		},
		{
			name:       "exactly 18 hours - boundary",
			hoursSince: 18.0,
			expected:   1.05,
		},
		{
			name:       "beyond 18 hours - no boost",
			hoursSince: 20.0,
			expected:   1.0,
		},
		{
			name:       "24 hours - no boost",
			hoursSince: 24.0,
			expected:   1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.GetBoostFactor(tt.hoursSince)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemporalBoostConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    TemporalBoostConfig
		wantError bool
	}{
		{
			name:      "valid default config",
			config:    DefaultTemporalBoostConfig(),
			wantError: false,
		},
		{
			name: "valid custom config",
			config: TemporalBoostConfig{
				Boost6h:  1.5,
				Boost12h: 1.25,
				Boost18h: 1.1,
			},
			wantError: false,
		},
		{
			name: "invalid - boost below 1.0",
			config: TemporalBoostConfig{
				Boost6h:  0.9, // invalid - would penalize recent articles
				Boost12h: 1.15,
				Boost18h: 1.05,
			},
			wantError: true,
		},
		{
			name: "valid - no boost (all 1.0)",
			config: TemporalBoostConfig{
				Boost6h:  1.0,
				Boost12h: 1.0,
				Boost18h: 1.0,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
