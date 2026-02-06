package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds all OTel metric instruments for search-indexer.
var Metrics *SearchIndexerMetrics

// SearchIndexerMetrics contains all metric instruments.
type SearchIndexerMetrics struct {
	IndexedTotal      metric.Int64Counter
	DeletedTotal      metric.Int64Counter
	ErrorsTotal       metric.Int64Counter
	BatchDuration     metric.Float64Histogram
	SearchDuration    metric.Float64Histogram
}

// InitMetrics initializes all metric instruments.
func InitMetrics() error {
	meter := otel.Meter("search-indexer")

	indexedTotal, err := meter.Int64Counter("search_indexer_indexed_total",
		metric.WithDescription("Total number of articles indexed"),
	)
	if err != nil {
		return err
	}

	deletedTotal, err := meter.Int64Counter("search_indexer_deleted_total",
		metric.WithDescription("Total number of articles deleted from index"),
	)
	if err != nil {
		return err
	}

	errorsTotal, err := meter.Int64Counter("search_indexer_errors_total",
		metric.WithDescription("Total number of errors"),
	)
	if err != nil {
		return err
	}

	batchDuration, err := meter.Float64Histogram("search_indexer_batch_duration_seconds",
		metric.WithDescription("Batch processing duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	searchDuration, err := meter.Float64Histogram("search_indexer_search_duration_seconds",
		metric.WithDescription("Search request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	Metrics = &SearchIndexerMetrics{
		IndexedTotal:   indexedTotal,
		DeletedTotal:   deletedTotal,
		ErrorsTotal:    errorsTotal,
		BatchDuration:  batchDuration,
		SearchDuration: searchDuration,
	}

	return nil
}
