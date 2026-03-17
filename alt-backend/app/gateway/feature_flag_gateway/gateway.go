package feature_flag_gateway

import (
	"alt/config"
	"alt/domain"
	"hash/crc32"
	"strings"

	"github.com/google/uuid"
)

// Gateway implements feature_flag_port.FeatureFlagPort using config-based flags
// with percentage rollout via crc32 hashing.
type Gateway struct {
	cfg            *config.KnowledgeHomeConfig
	allowedUserIDs map[uuid.UUID]struct{}
}

// NewGateway creates a new feature flag gateway.
func NewGateway(cfg *config.KnowledgeHomeConfig) *Gateway {
	allowed := make(map[uuid.UUID]struct{})
	if cfg.AllowedUserIDs != "" {
		for _, idStr := range strings.Split(cfg.AllowedUserIDs, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := uuid.Parse(idStr); err == nil {
				allowed[id] = struct{}{}
			}
		}
	}
	return &Gateway{
		cfg:            cfg,
		allowedUserIDs: allowed,
	}
}

// IsEnabled checks whether a feature flag is enabled for the given user.
func (g *Gateway) IsEnabled(flagName string, userID uuid.UUID) bool {
	// First check if the flag is globally enabled
	if !g.isFlagGloballyEnabled(flagName) {
		return false
	}

	// Check allowlist
	if _, ok := g.allowedUserIDs[userID]; ok {
		return true
	}

	// Check percentage rollout
	return g.isInRolloutPercentage(userID)
}

// isFlagGloballyEnabled checks the per-flag toggle.
func (g *Gateway) isFlagGloballyEnabled(flagName string) bool {
	switch flagName {
	case domain.FlagKnowledgeHomePage:
		return g.cfg.EnableHomePage
	case domain.FlagKnowledgeHomeTracking:
		return g.cfg.EnableTracking
	case domain.FlagKnowledgeHomeProjectionV2:
		return g.cfg.EnableProjectionV2
	default:
		return false
	}
}

// isInRolloutPercentage uses crc32 hash of the user ID to deterministically
// decide whether the user falls within the rollout percentage.
func (g *Gateway) isInRolloutPercentage(userID uuid.UUID) bool {
	if g.cfg.RolloutPercentage >= 100 {
		return true
	}
	if g.cfg.RolloutPercentage <= 0 {
		return false
	}
	hash := crc32.ChecksumIEEE(userID[:])
	bucket := hash % 100
	return int(bucket) < g.cfg.RolloutPercentage
}
