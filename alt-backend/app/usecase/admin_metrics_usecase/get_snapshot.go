package admin_metrics_usecase

import (
	"alt/domain"
	"alt/port/admin_metrics_port"
	"context"
)

// GetSnapshotUsecase fetches a one-shot metrics snapshot.
type GetSnapshotUsecase struct {
	port admin_metrics_port.AdminMetricsPort
}

func NewGetSnapshotUsecase(p admin_metrics_port.AdminMetricsPort) *GetSnapshotUsecase {
	return &GetSnapshotUsecase{port: p}
}

func (u *GetSnapshotUsecase) Execute(ctx context.Context, keys []domain.MetricKey, window domain.RangeWindow, step domain.Step) (*domain.MetricsSnapshot, error) {
	return u.port.Snapshot(ctx, keys, window, step)
}

// CatalogUsecase returns the allowlist descriptors.
type CatalogUsecase struct {
	port admin_metrics_port.AdminMetricsPort
}

func NewCatalogUsecase(p admin_metrics_port.AdminMetricsPort) *CatalogUsecase {
	return &CatalogUsecase{port: p}
}

func (u *CatalogUsecase) Execute() []domain.MetricCatalogEntry { return u.port.Catalog() }
