package scheduler

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
	// Check if we need to mock services.
	// Since we are testing logic, we can mock repository.
)

// MockSyncStateRepository
type MockSyncStateRepository struct {
	repository.SyncStateRepository
	GetOldestOneFunc func(ctx context.Context) (*models.SyncState, error)
	UpdateFunc       func(ctx context.Context, syncState *models.SyncState) error
}

func (m *MockSyncStateRepository) GetOldestOne(ctx context.Context) (*models.SyncState, error) {
	if m.GetOldestOneFunc != nil {
		return m.GetOldestOneFunc(ctx)
	}
	return nil, nil
}

func (m *MockSyncStateRepository) Update(ctx context.Context, syncState *models.SyncState) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, syncState)
	}
	return nil
}

// Since SubscriptionSyncService and InoreaderService are concrete structs in the Scheduler,
// ensuring we can test without full dependency injection is tricky unless we refactor Scheduler to use interfaces.
// However, for now, we can test Config and basic Start/Stop without mocking heavy services if we verify timing logic logic separately or ensure nil services don't crash Start().
// But runLoop calls methods that use services.
// To properly unit test runLoop, we'd need to mock the services.
// Given the struct Scheduler uses *service.SubscriptionSyncService (concrete), we cannot mock it easily without an interface.
// For this iteration, I will test Config and Start/Stop robustness.

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.FetchInterval != 16*time.Minute {
		t.Errorf("Expected FetchInterval to be 16m, got %v", cfg.FetchInterval)
	}
	if cfg.RefreshInterval != 24*time.Hour {
		t.Errorf("Expected RefreshInterval to be 24h, got %v", cfg.RefreshInterval)
	}
}

func TestScheduler_StartStop(t *testing.T) {
	logger := slog.Default()
	// We pass nil for services as we won't let the tickers fire in this short test
	s := NewScheduler(nil, nil, nil, logger)

	cfg := Config{
		FetchInterval:   time.Hour,
		RefreshInterval: time.Hour,
	}

	s.Start(cfg)
	if !s.isRunning {
		t.Error("Scheduler should be running")
	}

	s.Stop()
	if s.isRunning {
		t.Error("Scheduler should be stopped")
	}
}
