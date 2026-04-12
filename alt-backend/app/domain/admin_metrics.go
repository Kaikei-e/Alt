package domain

import "time"

// MetricKey is the allowlisted identifier a client uses to request a metric.
// The server resolves it to a PromQL template.
type MetricKey string

const (
	MetricAvailability               MetricKey = "availability_services"
	MetricHTTPLatencyP95             MetricKey = "http_latency_p95"
	MetricHTTPRPS                    MetricKey = "http_rps"
	MetricHTTPErrorRatio             MetricKey = "http_error_ratio"
	MetricCPUSaturation              MetricKey = "cpu_saturation"
	MetricMemoryRSS                  MetricKey = "memory_rss"
	MetricMQHubPublishRate           MetricKey = "mqhub_publish_rate"
	MetricMQHubRedis                 MetricKey = "mqhub_redis"
	MetricRecapDBPoolInUse           MetricKey = "recap_db_pool_in_use"
	MetricRecapWorkerRSS             MetricKey = "recap_worker_rss"
	MetricRecapRequestP95            MetricKey = "recap_request_p95"
	MetricRecapSubworkerAdminSuccess MetricKey = "recap_subworker_admin_success"
)

// SeriesKind distinguishes instant samples from range matrices.
type SeriesKind string

const (
	SeriesKindInstant SeriesKind = "instant"
	SeriesKindRange   SeriesKind = "range"
)

// MetricPoint is a single (time, value) observation.
type MetricPoint struct {
	Time  time.Time
	Value float64
}

// MetricSeries is one labeled series (e.g. job=alt-backend) with its observations.
type MetricSeries struct {
	Labels map[string]string
	Points []MetricPoint
}

// MetricResult is the resolved result for a single MetricKey.
type MetricResult struct {
	Key        MetricKey
	Kind       SeriesKind
	Unit       string
	GrafanaURL string
	Series     []MetricSeries
	Degraded   bool
	Reason     string
	Warnings   []string
}

// MetricsSnapshot is a point-in-time collection of MetricResult pushed on each tick.
type MetricsSnapshot struct {
	Time    time.Time
	Metrics map[MetricKey]*MetricResult
}

// MetricCatalogEntry describes one allowlisted metric for clients.
type MetricCatalogEntry struct {
	Key         MetricKey
	Title       string
	Unit        string
	Description string
	GrafanaURL  string
	Kind        SeriesKind
}

// RangeWindow is the allowlisted set of range windows accepted by the server.
type RangeWindow string

const (
	RangeWindow5m  RangeWindow = "5m"
	RangeWindow15m RangeWindow = "15m"
	RangeWindow1h  RangeWindow = "1h"
	RangeWindow6h  RangeWindow = "6h"
	RangeWindow24h RangeWindow = "24h"
)

func (w RangeWindow) Duration() time.Duration {
	switch w {
	case RangeWindow5m:
		return 5 * time.Minute
	case RangeWindow15m:
		return 15 * time.Minute
	case RangeWindow1h:
		return time.Hour
	case RangeWindow6h:
		return 6 * time.Hour
	case RangeWindow24h:
		return 24 * time.Hour
	}
	return 0
}

// Step is the allowlisted set of step sizes.
type Step string

const (
	Step15s Step = "15s"
	Step30s Step = "30s"
	Step1m  Step = "1m"
	Step5m  Step = "5m"
)

func (s Step) Duration() time.Duration {
	switch s {
	case Step15s:
		return 15 * time.Second
	case Step30s:
		return 30 * time.Second
	case Step1m:
		return time.Minute
	case Step5m:
		return 5 * time.Minute
	}
	return 0
}
