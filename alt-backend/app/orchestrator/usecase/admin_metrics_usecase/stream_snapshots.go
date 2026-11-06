package admin_metrics_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/admin_metrics_port"
	"context"
	"errors"
	"fmt"
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
	effectiveKeys := resolveKeys(keys)
	catalog := u.port.Catalog()
	byKey := make(map[domain.MetricKey]domain.MetricCatalogEntry, len(catalog))
	for _, c := range catalog {
		byKey[c.Key] = c
	}
	for _, k := range effectiveKeys {
		if _, ok := byKey[k]; !ok {
			return nil, fmt.Errorf("%w: %q", domain.ErrUnknownMetricKey, k)
		}
	}

	out := make(chan *domain.MetricsSnapshot, 1)
	go func() {
		defer close(out)
		emit := func() {
			snap, err := u.port.Snapshot(ctx, effectiveKeys, window, step)
			if err != nil || snap == nil {
				snap = fallbackSnapshot(time.Now(), effectiveKeys, byKey, err)
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

// resolveKeys returns default metric keys if keys is empty, otherwise returns keys.
func resolveKeys(keys []domain.MetricKey) []domain.MetricKey {
	if len(keys) == 0 {
		return defaultKeys()
	}
	return keys
}

// fallbackSnapshot builds one degraded MetricResult per requested key when the port snapshot fails.
func fallbackSnapshot(now time.Time, keys []domain.MetricKey, byKey map[domain.MetricKey]domain.MetricCatalogEntry, err error) *domain.MetricsSnapshot {
	reason := snapshotErrorReason(err)
	metrics := make(map[domain.MetricKey]*domain.MetricResult, len(keys))
	for _, k := range keys {
		mr := &domain.MetricResult{
			Key:      k,
			Degraded: true,
			Reason:   reason,
		}
		if c, ok := byKey[k]; ok {
			mr.Kind = c.Kind
			mr.Unit = c.Unit
			mr.GrafanaURL = c.GrafanaURL
		}
		metrics[k] = mr
	}
	return &domain.MetricsSnapshot{
		Time:    now,
		Metrics: metrics,
	}
}

func snapshotErrorReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidRange):
		return "invalid window/step combination"
	case errors.Is(err, domain.ErrUnknownMetricKey):
		return "unknown metric key"
	case errors.Is(err, domain.ErrNoKeys):
		return "metric keys required"
	default:
		return "prometheus unavailable"
	}
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
