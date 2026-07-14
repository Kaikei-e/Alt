package feature_flag_port

import "github.com/google/uuid"

// FeatureFlagPort checks whether a feature flag is enabled for a given user.
type FeatureFlagPort interface {
	IsEnabled(flagName string, userID uuid.UUID) bool
}
