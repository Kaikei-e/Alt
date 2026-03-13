package job

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// mockTagCloudWarmer implements tagCloudExecutor for testing.
type mockTagCloudWarmer struct {
	callCount atomic.Int32
	lastLimit int
	err       error
}

func (m *mockTagCloudWarmer) Execute(ctx context.Context, limit int) (any, error) {
	m.callCount.Add(1)
	m.lastLimit = limit
	if m.err != nil {
		return nil, m.err
	}
	return nil, nil
}

func TestTagCloudCacheWarmerJob_Success(t *testing.T) {
	mock := &mockTagCloudWarmer{}

	fn := tagCloudCacheWarmerJobFn(mock)
	err := fn(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mock.callCount.Load() != 1 {
		t.Errorf("expected 1 call, got %d", mock.callCount.Load())
	}
	if mock.lastLimit != 300 {
		t.Errorf("expected limit=300, got %d", mock.lastLimit)
	}
}

func TestTagCloudCacheWarmerJob_UsecaseError(t *testing.T) {
	mock := &mockTagCloudWarmer{err: errors.New("database error")}

	fn := tagCloudCacheWarmerJobFn(mock)
	err := fn(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "tag cloud cache warm: database error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTagCloudCacheWarmerJob_NilUsecase(t *testing.T) {
	fn := TagCloudCacheWarmerJob(nil)
	err := fn(context.Background())

	// Should not panic, just skip
	if err != nil {
		t.Fatalf("expected no error for nil usecase, got %v", err)
	}
}
