package feature_flag_gateway

import (
	"alt/config"
	"alt/domain"
	"testing"

	"github.com/google/uuid"
)

func TestIsEnabled_AllFlagsDisabled(t *testing.T) {
	cfg := &config.KnowledgeHomeConfig{
		EnableHomePage:     false,
		EnableTracking:     false,
		EnableProjectionV2: false,
		RolloutPercentage:  0,
		AllowedUserIDs:     "",
	}
	gw := NewGateway(cfg)
	userID := uuid.New()

	if gw.IsEnabled(domain.FlagKnowledgeHomePage, userID) {
		t.Error("expected home page flag to be disabled")
	}
	if gw.IsEnabled(domain.FlagKnowledgeHomeTracking, userID) {
		t.Error("expected tracking flag to be disabled")
	}
	if gw.IsEnabled(domain.FlagKnowledgeHomeProjectionV2, userID) {
		t.Error("expected projection v2 flag to be disabled")
	}
}

func TestIsEnabled_AllFlagsEnabled(t *testing.T) {
	cfg := &config.KnowledgeHomeConfig{
		EnableHomePage:     true,
		EnableTracking:     true,
		EnableProjectionV2: true,
		RolloutPercentage:  100,
		AllowedUserIDs:     "",
	}
	gw := NewGateway(cfg)
	userID := uuid.New()

	if !gw.IsEnabled(domain.FlagKnowledgeHomePage, userID) {
		t.Error("expected home page flag to be enabled")
	}
	if !gw.IsEnabled(domain.FlagKnowledgeHomeTracking, userID) {
		t.Error("expected tracking flag to be enabled")
	}
	if !gw.IsEnabled(domain.FlagKnowledgeHomeProjectionV2, userID) {
		t.Error("expected projection v2 flag to be enabled")
	}
}

func TestIsEnabled_AllowedUserIDs(t *testing.T) {
	allowedUser := uuid.New()
	otherUser := uuid.New()

	cfg := &config.KnowledgeHomeConfig{
		EnableHomePage:     true,
		EnableTracking:     true,
		EnableProjectionV2: false,
		RolloutPercentage:  0,
		AllowedUserIDs:     allowedUser.String(),
	}
	gw := NewGateway(cfg)

	// Allowed user should have access even with 0% rollout
	if !gw.IsEnabled(domain.FlagKnowledgeHomePage, allowedUser) {
		t.Error("expected allowed user to have home page enabled")
	}
	if !gw.IsEnabled(domain.FlagKnowledgeHomeTracking, allowedUser) {
		t.Error("expected allowed user to have tracking enabled")
	}

	// Other user should not have access at 0% rollout
	if gw.IsEnabled(domain.FlagKnowledgeHomePage, otherUser) {
		t.Error("expected other user to have home page disabled")
	}

	// Projection v2 flag is off globally, even allowlist doesn't override
	if gw.IsEnabled(domain.FlagKnowledgeHomeProjectionV2, allowedUser) {
		t.Error("expected projection v2 to be disabled even for allowed user when flag is off")
	}
}

func TestIsEnabled_MultipleAllowedUsers(t *testing.T) {
	user1 := uuid.New()
	user2 := uuid.New()
	user3 := uuid.New()

	cfg := &config.KnowledgeHomeConfig{
		EnableHomePage:     true,
		EnableTracking:     true,
		EnableProjectionV2: false,
		RolloutPercentage:  0,
		AllowedUserIDs:     user1.String() + "," + user2.String(),
	}
	gw := NewGateway(cfg)

	if !gw.IsEnabled(domain.FlagKnowledgeHomePage, user1) {
		t.Error("expected user1 to be allowed")
	}
	if !gw.IsEnabled(domain.FlagKnowledgeHomePage, user2) {
		t.Error("expected user2 to be allowed")
	}
	if gw.IsEnabled(domain.FlagKnowledgeHomePage, user3) {
		t.Error("expected user3 to be disallowed")
	}
}

func TestIsEnabled_PercentageRollout(t *testing.T) {
	cfg := &config.KnowledgeHomeConfig{
		EnableHomePage:     true,
		EnableTracking:     true,
		EnableProjectionV2: false,
		RolloutPercentage:  100,
		AllowedUserIDs:     "",
	}
	gw := NewGateway(cfg)

	// With 100% rollout, all users should be enabled
	for i := 0; i < 100; i++ {
		userID := uuid.New()
		if !gw.IsEnabled(domain.FlagKnowledgeHomePage, userID) {
			t.Errorf("expected 100%% rollout to enable all users, user %s was disabled", userID)
		}
	}

	// With 0% rollout and no allowlist, no users should be enabled
	cfg.RolloutPercentage = 0
	gw2 := NewGateway(cfg)
	for i := 0; i < 100; i++ {
		userID := uuid.New()
		if gw2.IsEnabled(domain.FlagKnowledgeHomePage, userID) {
			t.Errorf("expected 0%% rollout to disable all users, user %s was enabled", userID)
		}
	}
}

func TestIsEnabled_PercentageRolloutDeterministic(t *testing.T) {
	cfg := &config.KnowledgeHomeConfig{
		EnableHomePage:     true,
		EnableTracking:     true,
		EnableProjectionV2: false,
		RolloutPercentage:  50,
		AllowedUserIDs:     "",
	}
	gw := NewGateway(cfg)

	// Same user should always get the same result
	userID := uuid.New()
	first := gw.IsEnabled(domain.FlagKnowledgeHomePage, userID)
	for i := 0; i < 10; i++ {
		if gw.IsEnabled(domain.FlagKnowledgeHomePage, userID) != first {
			t.Error("expected deterministic rollout for the same user")
		}
	}
}

func TestIsEnabled_UnknownFlag(t *testing.T) {
	cfg := &config.KnowledgeHomeConfig{
		EnableHomePage:     true,
		EnableTracking:     true,
		EnableProjectionV2: true,
		RolloutPercentage:  100,
		AllowedUserIDs:     "",
	}
	gw := NewGateway(cfg)

	if gw.IsEnabled("unknown_flag", uuid.New()) {
		t.Error("expected unknown flag to be disabled")
	}
}
