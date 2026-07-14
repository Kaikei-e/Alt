package admin_metrics_usecase

import (
	"alt/domain"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type stubPort struct {
	calls       atomic.Int32
	snapshotErr error
	snapshot    *domain.MetricsSnapshot
	healthyErr  error
}

func (s *stubPort) Catalog() []domain.MetricCatalogEntry {
	return []domain.MetricCatalogEntry{{Key: domain.MetricAvailability, Title: "Availability", Unit: "bool"}}
}
func (s *stubPort) Snapshot(ctx context.Context, keys []domain.MetricKey, w domain.RangeWindow, st domain.Step) (*domain.MetricsSnapshot, error) {
	s.calls.Add(1)
	if s.snapshotErr != nil {
		return nil, s.snapshotErr
	}
	if s.snapshot != nil {
		return s.snapshot, nil
	}
	return &domain.MetricsSnapshot{Time: time.Now(), Metrics: map[domain.MetricKey]*domain.MetricResult{
		domain.MetricAvailability: {Key: domain.MetricAvailability, Kind: domain.SeriesKindInstant},
	}}, nil
}
func (s *stubPort) Healthy(ctx context.Context) error { return s.healthyErr }

func TestGetSnapshot_DelegatesToPort(t *testing.T) {
	p := &stubPort{}
	uc := NewGetSnapshotUsecase(p)
	snap, err := uc.Execute(context.Background(), []domain.MetricKey{domain.MetricAvailability}, "", "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if snap == nil || snap.Metrics[domain.MetricAvailability] == nil {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

func TestGetSnapshot_PropagatesError(t *testing.T) {
	p := &stubPort{snapshotErr: errors.New("boom")}
	uc := NewGetSnapshotUsecase(p)
	if _, err := uc.Execute(context.Background(), []domain.MetricKey{domain.MetricAvailability}, "", ""); err == nil {
		t.Fatalf("expected error")
	}
}

func TestStreamSnapshots_TicksAndStopsOnContextCancel(t *testing.T) {
	p := &stubPort{}
	uc := NewStreamSnapshotsUsecase(p, WithInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := uc.Execute(ctx, []domain.MetricKey{domain.MetricAvailability}, "", "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Consume at least 2 snapshots and then cancel.
	got := 0
	deadline := time.After(500 * time.Millisecond)
	for got < 2 {
		select {
		case snap, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed before receiving 2 snapshots")
			}
			if snap == nil {
				t.Fatalf("nil snapshot")
			}
			got++
		case <-deadline:
			t.Fatalf("timed out receiving snapshots")
		}
	}
	cancel()

	// Channel must close.
	select {
	case _, ok := <-ch:
		if ok {
			// Drain any buffered snapshot then expect close.
			select {
			case _, stillOpen := <-ch:
				if stillOpen {
					t.Fatalf("channel should close after cancel")
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("channel did not close after cancel")
			}
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("channel did not close after cancel")
	}
}

func TestStreamSnapshots_EmitsFirstTickImmediately(t *testing.T) {
	p := &stubPort{}
	uc := NewStreamSnapshotsUsecase(p, WithInterval(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := uc.Execute(ctx, []domain.MetricKey{domain.MetricAvailability}, "", "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	select {
	case snap := <-ch:
		if snap == nil {
			t.Fatalf("expected first snapshot")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("did not receive first snapshot immediately")
	}
}

func TestCatalog_Delegates(t *testing.T) {
	p := &stubPort{}
	uc := NewCatalogUsecase(p)
	cat := uc.Execute()
	if len(cat) != 1 {
		t.Fatalf("want 1 catalog entry, got %d", len(cat))
	}
}
