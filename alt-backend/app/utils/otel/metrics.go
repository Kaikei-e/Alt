package otel

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

// InitMeterProvider creates a MeterProvider with a Prometheus exporter.
// Returns the provider and the HTTP handler for /metrics.
func InitMeterProvider() (*metric.MeterProvider, http.Handler, error) {
	exporter, err := promexporter.New()
	if err != nil {
		return nil, nil, err
	}

	mp := metric.NewMeterProvider(metric.WithReader(exporter))

	return mp, promhttp.Handler(), nil
}
