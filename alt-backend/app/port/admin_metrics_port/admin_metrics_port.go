package admin_metrics_port

import (
	"alt/domain"
	"context"
)

// AdminMetricsPort fetches allowlisted admin observability metrics.
// Implementations must validate inputs against their allowlist and
// never accept raw PromQL from callers.
type AdminMetricsPort interface {
	// Catalog returns the allowlist descriptors shown to clients.
	Catalog() []domain.MetricCatalogEntry

	// Snapshot fetches an instant+range bundle for the supplied keys.
	// window and step are ignored for instant-only keys.
	Snapshot(ctx context.Context, keys []domain.MetricKey, window domain.RangeWindow, step domain.Step) (*domain.MetricsSnapshot, error)

	// Healthy returns nil if the upstream metrics store is reachable and ready.
	Healthy(ctx context.Context) error
}
