package usecase

import "fmt"

// TemporalBoostConfig holds tunable parameters for time-based score adjustment.
// These values boost the relevance scores of more recent articles.
// Note: Unlike retrieval parameters, these values are application-specific
// and should be validated through A/B testing in production.
type TemporalBoostConfig struct {
	// Boost6h is the score multiplier for articles published within 0-6 hours
	Boost6h float32

	// Boost12h is the score multiplier for articles published within 6-12 hours
	Boost12h float32

	// Boost18h is the score multiplier for articles published within 12-18 hours
	Boost18h float32
}

// DefaultTemporalBoostConfig returns current defaults.
// These values are empirically derived and should be tuned based on user feedback.
func DefaultTemporalBoostConfig() TemporalBoostConfig {
	return TemporalBoostConfig{
		Boost6h:  1.3,  // 30% boost for last 6 hours
		Boost12h: 1.15, // 15% boost for 6-12 hours
		Boost18h: 1.05, // 5% boost for 12-18 hours
	}
}

// GetBoostFactor returns the appropriate boost factor based on hours since publication.
func (c TemporalBoostConfig) GetBoostFactor(hoursSince float64) float32 {
	switch {
	case hoursSince <= 6:
		return c.Boost6h
	case hoursSince <= 12:
		return c.Boost12h
	case hoursSince <= 18:
		return c.Boost18h
	default:
		return 1.0 // No boost for older articles
	}
}

// Validate checks if the configuration values are within acceptable ranges.
func (c TemporalBoostConfig) Validate() error {
	if c.Boost6h < 1.0 {
		return fmt.Errorf("boost6h must be >= 1.0 (got %f), boosting should not penalize recent articles", c.Boost6h)
	}
	if c.Boost12h < 1.0 {
		return fmt.Errorf("boost12h must be >= 1.0 (got %f), boosting should not penalize recent articles", c.Boost12h)
	}
	if c.Boost18h < 1.0 {
		return fmt.Errorf("boost18h must be >= 1.0 (got %f), boosting should not penalize recent articles", c.Boost18h)
	}
	return nil
}
