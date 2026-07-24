package job

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockOutboxPruneRepository implements outboxPruneRepository for testing.
type mockOutboxPruneRepository struct {
	prunedCount  int64
	err          error
	gotOlderThan time.Duration
	callCount    int
}

func (m *mockOutboxPruneRepository) PruneOutboxEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	m.callCount++
	m.gotOlderThan = olderThan
	return m.prunedCount, m.err
}

// Finding [13]: PruneOutboxEvents was implemented in
// save_outbox_event_driver.go but never called from any registered job, so
// outbox_events grew unbounded (PROCESSED rows never deleted). OutboxPruneJob
// wires it up.
func TestOutboxPruneJob_PrunesWithRetentionWindow(t *testing.T) {
	repo := &mockOutboxPruneRepository{prunedCount: 42}

	fn := OutboxPruneJob(repo)
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.callCount != 1 {
		t.Fatalf("expected PruneOutboxEvents to be called once, got %d", repo.callCount)
	}
	if repo.gotOlderThan != outboxPruneRetention {
		t.Errorf("expected retention window %v, got %v", outboxPruneRetention, repo.gotOlderThan)
	}
}

func TestOutboxPruneJob_PropagatesError(t *testing.T) {
	repo := &mockOutboxPruneRepository{err: errors.New("db down")}

	fn := OutboxPruneJob(repo)
	if err := fn(context.Background()); err == nil {
		t.Fatal("expected error to propagate")
	}
}

// Nil repo can only be a DI wiring bug (unconditionally constructed, no
// feature flag) — must panic at construction time, not no-op forever.
func TestOutboxPruneJob_NilRepo_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected OutboxPruneJob(nil) to panic")
		}
	}()
	OutboxPruneJob(nil)
}
