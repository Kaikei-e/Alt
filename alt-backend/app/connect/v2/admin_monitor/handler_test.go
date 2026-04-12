package admin_monitor

import (
	"alt/domain"
	adminmonitorv1 "alt/gen/proto/alt/admin_monitor/v1"
	"alt/gen/proto/alt/admin_monitor/v1/adminmonitorv1connect"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
)

type fakeUC struct {
	catalog  []domain.MetricCatalogEntry
	snapshot *domain.MetricsSnapshot
	err      error
}

func (f *fakeUC) GetCatalog() []domain.MetricCatalogEntry { return f.catalog }
func (f *fakeUC) GetSnapshot(ctx context.Context, keys []domain.MetricKey, w domain.RangeWindow, s domain.Step) (*domain.MetricsSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.snapshot, nil
}
func (f *fakeUC) StreamSnapshots(ctx context.Context, keys []domain.MetricKey, w domain.RangeWindow, s domain.Step) (<-chan *domain.MetricsSnapshot, error) {
	ch := make(chan *domain.MetricsSnapshot, 2)
	ch <- f.snapshot
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func newSnapshot() *domain.MetricsSnapshot {
	return &domain.MetricsSnapshot{
		Time: time.Unix(1700000000, 0).UTC(),
		Metrics: map[domain.MetricKey]*domain.MetricResult{
			domain.MetricAvailability: {
				Key:  domain.MetricAvailability,
				Kind: domain.SeriesKindInstant,
				Unit: "bool",
				Series: []domain.MetricSeries{{
					Labels: map[string]string{"job": "alt-backend"},
					Points: []domain.MetricPoint{{Time: time.Unix(1700000000, 0).UTC(), Value: 1}},
				}},
			},
		},
	}
}

func newServer(t *testing.T, uc Usecase) (*httptest.Server, adminmonitorv1connect.AdminMonitorServiceClient) {
	t.Helper()
	h := NewHandler(uc, slog.Default())
	path, handler := adminmonitorv1connect.NewAdminMonitorServiceHandler(h)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := adminmonitorv1connect.NewAdminMonitorServiceClient(srv.Client(), srv.URL)
	return srv, client
}

func TestCatalog_ReturnsEntries(t *testing.T) {
	uc := &fakeUC{catalog: []domain.MetricCatalogEntry{{Key: domain.MetricAvailability, Title: "Availability", Unit: "bool", Kind: domain.SeriesKindInstant}}}
	_, client := newServer(t, uc)

	res, err := client.Catalog(context.Background(), connect.NewRequest(&adminmonitorv1.CatalogRequest{}))
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(res.Msg.Entries) != 1 || res.Msg.Entries[0].Key != string(domain.MetricAvailability) {
		t.Fatalf("unexpected entries: %+v", res.Msg.Entries)
	}
}

func TestSnapshot_ReturnsMetrics(t *testing.T) {
	uc := &fakeUC{snapshot: newSnapshot()}
	_, client := newServer(t, uc)

	res, err := client.Snapshot(context.Background(), connect.NewRequest(&adminmonitorv1.SnapshotRequest{
		Keys:   []string{string(domain.MetricAvailability)},
		Window: adminmonitorv1.RangeWindow_RANGE_WINDOW_UNSPECIFIED,
		Step:   adminmonitorv1.Step_STEP_UNSPECIFIED,
	}))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(res.Msg.Metrics) != 1 || res.Msg.Metrics[0].Key != string(domain.MetricAvailability) {
		t.Fatalf("unexpected metrics: %+v", res.Msg.Metrics)
	}
	if len(res.Msg.Metrics[0].Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(res.Msg.Metrics[0].Series))
	}
	if res.Msg.Metrics[0].Series[0].Labels["job"] != "alt-backend" {
		t.Fatalf("unexpected labels: %+v", res.Msg.Metrics[0].Series[0].Labels)
	}
}

func TestSnapshot_PropagatesError(t *testing.T) {
	uc := &fakeUC{err: errors.New("unknown metric key \"bogus\"")}
	_, client := newServer(t, uc)
	_, err := client.Snapshot(context.Background(), connect.NewRequest(&adminmonitorv1.SnapshotRequest{Keys: []string{"bogus"}}))
	if err == nil {
		t.Fatalf("expected error")
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("want *connect.Error, got %T: %v", err, err)
	}
	if ce.Code() != connect.CodeInvalidArgument {
		t.Fatalf("code = %v want InvalidArgument", ce.Code())
	}
}

func TestWatch_StreamsAndStopsOnCancel(t *testing.T) {
	uc := &fakeUC{snapshot: newSnapshot()}
	_, client := newServer(t, uc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.Watch(ctx, connect.NewRequest(&adminmonitorv1.WatchRequest{Keys: []string{string(domain.MetricAvailability)}}))
	if err != nil {
		t.Fatalf("Watch open: %v", err)
	}
	if !stream.Receive() {
		t.Fatalf("expected first frame: %v", stream.Err())
	}
	msg := stream.Msg()
	if len(msg.Metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(msg.Metrics))
	}
	cancel()
	// Expect EOF or canceled soon.
	for stream.Receive() {
		// drain remaining buffered frames if any
	}
	if err := stream.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		var ce *connect.Error
		if errors.As(err, &ce) && ce.Code() == connect.CodeCanceled {
			return
		}
		t.Fatalf("unexpected stream err: %v", err)
	}
}
