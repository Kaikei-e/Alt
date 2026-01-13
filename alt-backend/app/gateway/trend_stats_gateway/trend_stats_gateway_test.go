package trend_stats_gateway

import (
	"context"
	"testing"
	"time"

	"alt/domain"
	"alt/port/trend_stats_port"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewTrendStatsGateway_NilPool(t *testing.T) {
	gateway := NewTrendStatsGateway(nil)
	assert.NotNil(t, gateway, "gateway should not be nil even with nil pool")
}

func TestTrendStatsGateway_Execute_NilRepository(t *testing.T) {
	gateway := &TrendStatsGateway{altDBRepository: nil}

	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	ctx := domain.SetUserContext(context.Background(), userCtx)

	result, err := gateway.Execute(ctx, "24h")
	assert.Error(t, err, "should return error when repository is nil")
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database connection")
}

func TestTrendStatsGateway_Execute_ValidWindow(t *testing.T) {
	tests := []struct {
		name   string
		window string
	}{
		{"4 hours", "4h"},
		{"24 hours", "24h"},
		{"3 days", "3d"},
		{"7 days", "7d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test just validates the gateway accepts valid windows
			gateway := &TrendStatsGateway{altDBRepository: nil}

			userCtx := &domain.UserContext{
				UserID:    uuid.New(),
				Email:     "test@example.com",
				Role:      domain.UserRoleUser,
				TenantID:  uuid.New(),
				ExpiresAt: time.Now().Add(time.Hour),
			}
			ctx := domain.SetUserContext(context.Background(), userCtx)

			_, err := gateway.Execute(ctx, tt.window)
			// Should fail due to nil repository, not invalid window
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "database connection")
		})
	}
}

// TestTrendStatsPortInterface ensures the gateway implements the port interface
func TestTrendStatsPortInterface(t *testing.T) {
	var _ trend_stats_port.TrendStatsPort = (*TrendStatsGateway)(nil)
}
