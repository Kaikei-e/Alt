// Package metrics provides Prometheus metrics for mq-hub.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// PublishTotal counts total publish operations.
	PublishTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "mqhub",
			Name:      "publish_total",
			Help:      "Total number of publish operations",
		},
		[]string{"stream", "status"},
	)

	// PublishDuration measures publish operation duration.
	PublishDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "mqhub",
			Name:      "publish_duration_seconds",
			Help:      "Duration of publish operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"stream"},
	)

	// BatchSize observes batch sizes.
	BatchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "mqhub",
			Name:      "batch_size",
			Help:      "Distribution of batch sizes",
			Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"stream"},
	)

	// ErrorsTotal counts errors by type.
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "mqhub",
			Name:      "errors_total",
			Help:      "Total number of errors",
		},
		[]string{"operation", "error_type"},
	)

	// RedisConnectionStatus tracks Redis connection status.
	RedisConnectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "mqhub",
			Name:      "redis_connection_status",
			Help:      "Redis connection status (1 = connected, 0 = disconnected)",
		},
	)
)

// RecordPublish records a publish operation.
func RecordPublish(stream, status string, duration float64) {
	PublishTotal.WithLabelValues(stream, status).Inc()
	PublishDuration.WithLabelValues(stream).Observe(duration)
}

// RecordBatchPublish records a batch publish operation.
func RecordBatchPublish(stream, status string, batchSize int, duration float64) {
	PublishTotal.WithLabelValues(stream, status).Inc()
	PublishDuration.WithLabelValues(stream).Observe(duration)
	BatchSize.WithLabelValues(stream).Observe(float64(batchSize))
}

// RecordError records an error.
func RecordError(operation, errorType string) {
	ErrorsTotal.WithLabelValues(operation, errorType).Inc()
}

// SetRedisConnected sets Redis connection status to connected.
func SetRedisConnected() {
	RedisConnectionStatus.Set(1)
}

// SetRedisDisconnected sets Redis connection status to disconnected.
func SetRedisDisconnected() {
	RedisConnectionStatus.Set(0)
}
