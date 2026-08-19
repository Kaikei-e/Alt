package pki

import (
	"bytes"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// TextGatherer renders a prometheus.Gatherer as Prometheus text exposition
// so pre-processor can attach enrollment series to the :9201 collector
// (metrics_path /metrics/prometheus) instead of DefaultRegisterer.
type TextGatherer struct {
	Gatherer prometheus.Gatherer
}

// PrometheusWithError gathers and encodes without panicking. Scrape errors
// belong on the HTTP 500 path, not in process crash.
func (t TextGatherer) PrometheusWithError() (string, error) {
	if t.Gatherer == nil {
		return "", errors.New("pki: TextGatherer.Gatherer is not wired")
	}
	mfs, err := t.Gatherer.Gather()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	for _, mf := range mfs {
		if _, err := expfmt.MetricFamilyToText(&buf, mf); err != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

// Prometheus implements metrics.PrometheusExporter. On gather/encode failure
// it returns empty so ExportPrometheus callers that ignore errors stay up;
// the collector HTTP path uses PrometheusWithError.
func (t TextGatherer) Prometheus() string {
	s, err := t.PrometheusWithError()
	if err != nil {
		return ""
	}
	return s
}
