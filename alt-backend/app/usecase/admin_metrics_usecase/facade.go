package admin_metrics_usecase

import (
	"alt/domain"
	"alt/port/admin_metrics_port"
	"context"
	"time"
)

// Facade bundles the three admin-metrics usecases so callers (e.g. the Connect
// handler) can depend on a single aggregate without importing each one.
type Facade struct {
	get    *GetSnapshotUsecase
	stream *StreamSnapshotsUsecase
	cat    *CatalogUsecase
}

func NewFacade(p admin_metrics_port.AdminMetricsPort, interval time.Duration) *Facade {
	return &Facade{
		get:    NewGetSnapshotUsecase(p),
		stream: NewStreamSnapshotsUsecase(p, WithInterval(interval)),
		cat:    NewCatalogUsecase(p),
	}
}

func (f *Facade) GetCatalog() []domain.MetricCatalogEntry { return f.cat.Execute() }

func (f *Facade) GetSnapshot(ctx context.Context, keys []domain.MetricKey, w domain.RangeWindow, s domain.Step) (*domain.MetricsSnapshot, error) {
	return f.get.Execute(ctx, keys, w, s)
}

func (f *Facade) StreamSnapshots(ctx context.Context, keys []domain.MetricKey, w domain.RangeWindow, s domain.Step) (<-chan *domain.MetricsSnapshot, error) {
	return f.stream.Execute(ctx, keys, w, s)
}
