package admin_metrics_usecase

import (
	"alt/domain"
	"context"
	"errors"
	"fmt"
	"strings"
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
	return []domain.MetricCatalogEntry{
		{Key: domain.MetricAvailability, Title: "Availability", Unit: "bool", Kind: domain.SeriesKindInstant},
		{Key: domain.MetricHTTPRPS, Title: "HTTP RPS", Unit: "req/s", Kind: domain.SeriesKindRange},
	}
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
	if len(cat) != 2 {
		t.Fatalf("want 2 catalog entries, got %d", len(cat))
	}
}

func TestResolveKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    []domain.MetricKey
		expected []domain.MetricKey
	}{
		{
			name:     "empty returns default keys",
			input:    nil,
			expected: defaultKeys(),
		},
		{
			name:     "non-empty returns provided keys",
			input:    []domain.MetricKey{domain.MetricAvailability},
			expected: []domain.MetricKey{domain.MetricAvailability},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveKeys(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d keys, got %d", len(tt.expected), len(got))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("at index %d, expected %v, got %v", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestStreamSnapshots_RejectsUnknownKeyBeforeStarting(t *testing.T) {
	p := &stubPort{}
	uc := NewStreamSnapshotsUsecase(p, WithInterval(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := uc.Execute(ctx, []domain.MetricKey{domain.MetricAvailability, "unlisted_key"}, domain.RangeWindow15m, domain.Step30s)
	if err == nil {
		t.Fatalf("expected error for unlisted key, got nil")
	}
	if !errors.Is(err, domain.ErrUnknownMetricKey) {
		t.Fatalf("expected errors.Is(err, domain.ErrUnknownMetricKey), got: %v", err)
	}
	if ch != nil {
		t.Fatalf("expected nil channel on error, got %v", ch)
	}
	if got := p.calls.Load(); got != 0 {
		t.Fatalf("expected 0 Snapshot calls when key validation fails, got %d", got)
	}
}

func TestStreamSnapshots_PortError_EmitsDegradedPerRequestedKey(t *testing.T) {
	keys := []domain.MetricKey{
		domain.MetricAvailability,
		domain.MetricHTTPRPS,
	}
	wantMeta := map[domain.MetricKey]struct {
		kind domain.SeriesKind
		unit string
	}{
		domain.MetricAvailability: {kind: domain.SeriesKindInstant, unit: "bool"},
		domain.MetricHTTPRPS:      {kind: domain.SeriesKindRange, unit: "req/s"},
	}

	cases := []struct {
		name       string
		err        error
		wantReason string
	}{
		{
			name:       "upstream unavailable",
			err:        errors.New(`Get "http://prometheus:9090/api/v1/query": dial tcp 10.0.0.1:9090: connect: connection refused`),
			wantReason: "prometheus unavailable",
		},
		{
			name:       "invalid range sentinel",
			err:        fmt.Errorf("admin_metrics_gateway: %w: window/step ratio exceeds 720", domain.ErrInvalidRange),
			wantReason: "invalid window/step combination",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &stubPort{snapshotErr: tc.err}
			uc := NewStreamSnapshotsUsecase(p, WithInterval(time.Hour))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			ch, err := uc.Execute(ctx, keys, domain.RangeWindow15m, domain.Step30s)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			select {
			case snap := <-ch:
				if snap == nil {
					t.Fatalf("expected non-nil snapshot")
				}
				if len(snap.Metrics) != len(keys) {
					t.Fatalf("expected %d degraded metrics, got %d: %+v", len(keys), len(snap.Metrics), snap.Metrics)
				}
				for _, k := range keys {
					mr, ok := snap.Metrics[k]
					if !ok || mr == nil {
						t.Fatalf("missing metric result for %q", k)
					}
					if mr.Key != k {
						t.Errorf("mr.Key = %q, want %q", mr.Key, k)
					}
					if !mr.Degraded {
						t.Errorf("mr %q expected Degraded=true", k)
					}
					if mr.Reason != tc.wantReason {
						t.Errorf("mr %q Reason = %q, want %q", k, mr.Reason, tc.wantReason)
					}
					if strings.Contains(mr.Reason, "http://") || strings.Contains(mr.Reason, "10.0.0.1") {
						t.Errorf("mr %q Reason leaked internal URL/host: %q", k, mr.Reason)
					}
					meta := wantMeta[k]
					if mr.Kind != meta.kind {
						t.Errorf("mr %q Kind = %q, want %q", k, mr.Kind, meta.kind)
					}
					if mr.Unit != meta.unit {
						t.Errorf("mr %q Unit = %q, want %q", k, mr.Unit, meta.unit)
					}
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("did not receive snapshot")
			}
		})
	}
}
