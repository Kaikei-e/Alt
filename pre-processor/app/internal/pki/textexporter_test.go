package pki

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestTextGatherer_RendersEnrollmentFamily(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	obs := NewPromObserver("pre-processor", reg)
	obs.OnClassified(StateFresh, 8*time.Hour)
	obs.OnRenewed(true)

	body, err := TextGatherer{Gatherer: reg}.PrometheusWithError()
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`pki_enrollment_healthy{subject="pre-processor"} 1`,
		`pki_enrollment_cert_remaining_seconds{subject="pre-processor"} 28800`,
		`pki_enrollment_renewal_total{result="success",subject="pre-processor"} 1`,
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("missing %q\n%s", needle, body)
		}
	}
}

func TestTextGatherer_NilGathererErrors(t *testing.T) {
	defer func() {
		if recover() != nil {
			t.Fatal("nil gatherer must not panic")
		}
	}()
	_, err := TextGatherer{}.PrometheusWithError()
	if err == nil {
		t.Fatal("expected error for unwired gatherer")
	}
}

type boomGatherer struct{}

func (boomGatherer) Gather() ([]*dto.MetricFamily, error) {
	return nil, errors.New("gather boom")
}

func TestTextGatherer_GatherError(t *testing.T) {
	defer func() {
		if recover() != nil {
			t.Fatal("gather error must not panic")
		}
	}()
	_, err := TextGatherer{Gatherer: boomGatherer{}}.PrometheusWithError()
	if err == nil {
		t.Fatal("expected gather error")
	}
}
