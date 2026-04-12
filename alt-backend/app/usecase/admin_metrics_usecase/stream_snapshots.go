package admin_metrics_usecase

import (
	"alt/domain"
	"alt/port/admin_metrics_port"
	"context"
	"time"
)

// StreamSnapshotsUsecase periodically emits MetricsSnapshot on a channel until ctx is cancelled.
type StreamSnapshotsUsecase struct {
	port     admin_metrics_port.AdminMetricsPort
	interval time.Duration
}

type StreamOption func(*StreamSnapshotsUsecase)

func WithInterval(d time.Duration) StreamOption {
	return func(u *StreamSnapshotsUsecase) {
		if d > 0 {
			u.interval = d
		}
	}
}

func NewStreamSnapshotsUsecase(p admin_metrics_port.AdminMetricsPort, opts ...StreamOption) *StreamSnapshotsUsecase {
	u := &StreamSnapshotsUsecase{port: p, interval: 5 * time.Second}
	for _, o := range opts {
		o(u)
	}
	return u
}

// Execute starts a ticker that emits one MetricsSnapshot per interval. The first
// snapshot is emitted immediately so clients render quickly. Errors from the
// port are swallowed so a transient Prometheus outage does not close the stream;
// the resulting snapshot will carry Degraded=true per metric.
func (u *StreamSnapshotsUsecase) Execute(ctx context.Context, keys []domain.MetricKey, window domain.RangeWindow, step domain.Step) (<-chan *domain.MetricsSnapshot, error) {
	if len(keys) == 0 {
		keys = defaultKeys()
	}
	out := make(chan *domain.MetricsSnapshot, 1)
	go func() {
		defer close(out)
		emit := func() {
			snap, err := u.port.Snapshot(ctx, keys, window, step)
			if err != nil || snap == nil {
				snap = &domain.MetricsSnapshot{Time: time.Now(), Metrics: map[domain.MetricKey]*domain.MetricResult{}}
			}
			select {
			case out <- snap:
			case <-ctx.Done():
			}
		}
		emit()
		ticker := time.NewTicker(u.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				emit()
			}
		}
	}()
	return out, nil
}

func defaultKeys() []domain.MetricKey {
	return []domain.MetricKey{
		domain.MetricAvailability,
		domain.MetricHTTPLatencyP95,
		domain.MetricHTTPRPS,
		domain.MetricHTTPErrorRatio,
		domain.MetricCPUSaturation,
		domain.MetricMemoryRSS,
		domain.MetricMQHubPublishRate,
		domain.MetricMQHubRedis,
		domain.MetricRecapDBPoolInUse,
		domain.MetricRecapWorkerRSS,
		domain.MetricRecapRequestP95,
		domain.MetricRecapSubworkerAdminSuccess,
	}
}
