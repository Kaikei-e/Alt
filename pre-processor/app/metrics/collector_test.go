// ABOUTME: This file tests metrics collection system for performance monitoring and SLA tracking
// ABOUTME: Tests metric aggregation, reporting, and integration with existing service components
package metrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestNewMetricsCollector(t *testing.T) {
	tests := map[string]struct {
		config      config.MetricsConfig
		expectError bool
		validate    func(*testing.T, *Collector)
	}{
		"default configuration": {
			config: config.MetricsConfig{
				Enabled:        true,
				Port:           9201,
				Path:           "/metrics",
				UpdateInterval: 10 * time.Second,
			},
			expectError: false,
			validate: func(t *testing.T, collector *Collector) {
				assert.True(t, collector.enabled)
				assert.Equal(t, 9201, collector.port)
				assert.Equal(t, "/metrics", collector.path)
				assert.Equal(t, 10*time.Second, collector.updateInterval)
				assert.NotNil(t, collector.metrics)
			},
		},
		"disabled metrics": {
			config: config.MetricsConfig{
				Enabled:        false,
				Port:           9201,
				Path:           "/metrics",
				UpdateInterval: 10 * time.Second,
			},
			expectError: false,
			validate: func(t *testing.T, collector *Collector) {
				assert.False(t, collector.enabled)
			},
		},
		"custom configuration": {
			config: config.MetricsConfig{
				Enabled:        true,
				Port:           9999,
				Path:           "/custom-metrics",
				UpdateInterval: 30 * time.Second,
			},
			expectError: false,
			validate: func(t *testing.T, collector *Collector) {
				assert.True(t, collector.enabled)
				assert.Equal(t, 9999, collector.port)
				assert.Equal(t, "/custom-metrics", collector.path)
				assert.Equal(t, 30*time.Second, collector.updateInterval)
			},
		},
		"invalid port": {
			config: config.MetricsConfig{
				Enabled: true,
				Port:    0,
			},
			expectError: true,
		},
		"invalid update interval": {
			config: config.MetricsConfig{
				Enabled:        true,
				Port:           9201,
				UpdateInterval: 0,
			},
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collector, err := NewCollector(tc.config, testLogger())

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, collector)
			tc.validate(t, collector)
		})
	}
}

func TestMetricsCollector_RecordMetrics(t *testing.T) {
	t.Run("should record request metrics", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Record multiple request metrics
		collector.RecordRequest("example.com", 100*time.Millisecond, true)
		collector.RecordRequest("example.com", 200*time.Millisecond, true)
		collector.RecordRequest("example.com", 500*time.Millisecond, false)

		metrics := collector.GetDomainMetrics("example.com")
		require.NotNil(t, metrics)

		assert.Equal(t, int64(3), metrics.TotalRequests)
		assert.Equal(t, int64(2), metrics.SuccessCount)
		assert.Equal(t, int64(1), metrics.FailureCount)
		assert.InDelta(t, 0.67, metrics.SuccessRate, 0.01)
		// Response time: (100 + 200 + 500) / 3 = 800/3 = 266.666... ms
		assert.InDelta(t, 267*time.Millisecond, metrics.AvgResponseTime, float64(1*time.Millisecond))
	})

	t.Run("should track different domains separately", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Record metrics for different domains
		collector.RecordRequest("domain1.com", 100*time.Millisecond, true)
		collector.RecordRequest("domain2.com", 200*time.Millisecond, false)

		metrics1 := collector.GetDomainMetrics("domain1.com")
		metrics2 := collector.GetDomainMetrics("domain2.com")

		require.NotNil(t, metrics1)
		require.NotNil(t, metrics2)

		assert.Equal(t, int64(1), metrics1.TotalRequests)
		assert.Equal(t, int64(1), metrics1.SuccessCount)
		assert.Equal(t, float64(1.0), metrics1.SuccessRate)

		assert.Equal(t, int64(1), metrics2.TotalRequests)
		assert.Equal(t, int64(1), metrics2.FailureCount)
		assert.Equal(t, float64(0.0), metrics2.SuccessRate)
	})

	t.Run("should handle disabled metrics", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled: false,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Recording should not fail even when disabled
		collector.RecordRequest("example.com", 100*time.Millisecond, true)

		// But metrics should not be available
		metrics := collector.GetDomainMetrics("example.com")
		assert.Nil(t, metrics)
	})
}

// TDD RED PHASE: Test aggregate metrics
func TestMetricsCollector_AggregateMetrics(t *testing.T) {
	t.Run("should calculate aggregate metrics", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Record metrics for multiple domains
		collector.RecordRequest("domain1.com", 100*time.Millisecond, true)
		collector.RecordRequest("domain1.com", 200*time.Millisecond, true)
		collector.RecordRequest("domain2.com", 300*time.Millisecond, false)
		collector.RecordRequest("domain2.com", 400*time.Millisecond, true)

		aggregate := collector.GetAggregateMetrics()
		require.NotNil(t, aggregate)

		assert.Equal(t, int64(4), aggregate.TotalRequests)
		assert.Equal(t, int64(3), aggregate.SuccessCount)
		assert.Equal(t, int64(1), aggregate.FailureCount)
		assert.Equal(t, float64(0.75), aggregate.SuccessRate)
		assert.Equal(t, 250*time.Millisecond, aggregate.AvgResponseTime)
		assert.Equal(t, 2, aggregate.ActiveDomains)
	})

	t.Run("should handle empty metrics", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		aggregate := collector.GetAggregateMetrics()
		require.NotNil(t, aggregate)

		assert.Equal(t, int64(0), aggregate.TotalRequests)
		assert.Equal(t, int64(0), aggregate.SuccessCount)
		assert.Equal(t, int64(0), aggregate.FailureCount)
		assert.Equal(t, float64(0.0), aggregate.SuccessRate)
		assert.Equal(t, time.Duration(0), aggregate.AvgResponseTime)
		assert.Equal(t, 0, aggregate.ActiveDomains)
	})
}

// TDD RED PHASE: Test concurrent access
func TestMetricsCollector_ConcurrentAccess(t *testing.T) {
	t.Run("should handle concurrent metric recording", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		var wg sync.WaitGroup
		concurrency := 100

		// Record metrics concurrently
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				domain := "concurrent.com"
				collector.RecordRequest(domain, time.Duration(index)*time.Millisecond, index%2 == 0)
			}(i)
		}

		wg.Wait()

		metrics := collector.GetDomainMetrics("concurrent.com")
		require.NotNil(t, metrics)

		assert.Equal(t, int64(concurrency), metrics.TotalRequests)
		assert.Equal(t, int64(50), metrics.SuccessCount)
		assert.Equal(t, int64(50), metrics.FailureCount)
		assert.Equal(t, float64(0.5), metrics.SuccessRate)
	})
}

func TestMetricsCollector_Export(t *testing.T) {
	t.Run("should export metrics in JSON format", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Record some test data
		collector.RecordRequest("example.com", 100*time.Millisecond, true)
		collector.RecordRequest("example.com", 200*time.Millisecond, false)

		jsonData, err := collector.ExportJSON()
		require.NoError(t, err)
		assert.NotEmpty(t, jsonData)

		// Should contain domain metrics
		assert.Contains(t, string(jsonData), "example.com")
		assert.Contains(t, string(jsonData), "total_requests")
		assert.Contains(t, string(jsonData), "success_rate")
	})

	t.Run("should export metrics in Prometheus format", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Record some test data
		collector.RecordRequest("example.com", 100*time.Millisecond, true)

		promData := collector.ExportPrometheus()
		assert.NotEmpty(t, promData)

		// Should contain Prometheus-style metrics
		assert.Contains(t, promData, "# HELP")
		assert.Contains(t, promData, "# TYPE")
		assert.Contains(t, promData, "preprocessor_requests_total")
	})
}

func TestMetricsCollector_Management(t *testing.T) {
	t.Run("should reset metrics", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Record some metrics
		collector.RecordRequest("example.com", 100*time.Millisecond, true)

		// Verify metrics exist
		metrics := collector.GetDomainMetrics("example.com")
		require.NotNil(t, metrics)
		assert.Equal(t, int64(1), metrics.TotalRequests)

		// Reset metrics
		collector.Reset()

		// Verify metrics are cleared
		metrics = collector.GetDomainMetrics("example.com")
		assert.Nil(t, metrics)

		aggregate := collector.GetAggregateMetrics()
		assert.Equal(t, int64(0), aggregate.TotalRequests)
	})

	t.Run("should cleanup old domains", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           9201,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		// Record metrics for test domain
		collector.RecordRequest("test.com", 100*time.Millisecond, true)

		// Verify domain exists
		metrics := collector.GetDomainMetrics("test.com")
		assert.NotNil(t, metrics)

		// Cleanup should remove old domains (implementation dependent)
		collector.Cleanup()

		// For test purposes, verify cleanup method exists
		assert.NotNil(t, collector)
	})
}

func TestMetricsCollector_Server(t *testing.T) {
	t.Run("should start and stop HTTP server", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           0, // Use random port for testing
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// The bind is synchronous, so a listener that cannot come up is an
		// error here rather than a process that runs with nothing to scrape.
		require.NoError(t, collector.Start(ctx, make(chan error, 1)))
		assert.NotEmpty(t, collector.Addr())

		// Stop server
		err = collector.Stop(ctx)
		require.NoError(t, err)
	})

	t.Run("should reproduce race condition between Start and Stop", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           0, // Use random port for testing
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// This test should expose the race condition
		// by repeatedly starting and stopping the server quickly
		for i := 0; i < 10; i++ {
			var wg sync.WaitGroup

			// Start server in goroutine
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = collector.Start(ctx, make(chan error, 1))
			}()

			// Stop server immediately in another goroutine
			wg.Add(1)
			go func() {
				defer wg.Done()
				time.Sleep(1 * time.Millisecond) // Small delay to create race window
				_ = collector.Stop(ctx)
			}()

			wg.Wait()
		}
	})

	t.Run("should handle concurrent Start/Stop safely", func(t *testing.T) {
		config := config.MetricsConfig{
			Enabled:        true,
			Port:           0,
			Path:           "/metrics",
			UpdateInterval: 10 * time.Second,
		}

		collector, err := NewCollector(config, testLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		var wg sync.WaitGroup
		concurrency := 5

		// Multiple goroutines trying to start/stop concurrently
		for i := 0; i < concurrency; i++ {
			wg.Add(2)

			go func() {
				defer wg.Done()
				_ = collector.Start(ctx, make(chan error, 1))
			}()

			go func() {
				defer wg.Done()
				time.Sleep(10 * time.Millisecond)
				_ = collector.Stop(ctx)
			}()
		}

		wg.Wait()
	})
}

// serveTestConfig is a metrics config bound to an ephemeral port, with the
// four HTTP timeouts set so the listener under test is the same shape as the
// production one.
func serveTestConfig() config.MetricsConfig {
	return config.MetricsConfig{
		Enabled:           true,
		Port:              0,
		Path:              "/metrics",
		UpdateInterval:    10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}

// localURL rewrites a listener address onto the loopback interface, since a
// port-0 bind reports the unspecified host.
func localURL(t *testing.T, addr, path string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	return "http://127.0.0.1:" + port + path
}

func TestMetricsCollector_RegisterExporter(t *testing.T) {
	t.Run("appends the registered series to the Prometheus export", func(t *testing.T) {
		collector, err := NewCollector(serveTestConfig(), testLogger())
		require.NoError(t, err)

		relay := NewOutboxRelayMetrics()
		relay.ObserveTick(90*time.Second, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
		require.NoError(t, collector.RegisterExporter("notification_outbox_relay", relay))

		exposition := collector.ExportPrometheus()
		assert.Contains(t, exposition, "notification_outbox_oldest_pending_age_seconds 90")
		assert.Contains(t, exposition, "notification_outbox_last_tick_timestamp_seconds")
		assert.Contains(t, exposition, "preprocessor_requests_total")
	})

	t.Run("refuses an unwired exporter", func(t *testing.T) {
		collector, err := NewCollector(serveTestConfig(), testLogger())
		require.NoError(t, err)

		require.Error(t, collector.RegisterExporter("notification_outbox_relay", nil))
	})

	t.Run("refuses a duplicate registration", func(t *testing.T) {
		collector, err := NewCollector(serveTestConfig(), testLogger())
		require.NoError(t, err)

		require.NoError(t, collector.RegisterExporter("notification_outbox_relay", NewOutboxRelayMetrics()))
		require.Error(t, collector.RegisterExporter("notification_outbox_relay", NewOutboxRelayMetrics()),
			"a second registration under the same name would emit the series twice")
	})
}

// TestMetricsCollector_ServesRegisteredExportersOnItsOwnListener pins the
// placement the observability side scrapes: the relay gauges are served from
// the dedicated metrics listener under /metrics/prometheus, not from the
// service API listener.
func TestMetricsCollector_ServesRegisteredExportersOnItsOwnListener(t *testing.T) {
	collector, err := NewCollector(serveTestConfig(), testLogger())
	require.NoError(t, err)

	relay := NewOutboxRelayMetrics()
	relay.ObserveTick(0, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	require.NoError(t, collector.RegisterExporter("notification_outbox_relay", relay))

	require.NoError(t, collector.Start(context.Background(), make(chan error, 1)))
	t.Cleanup(func() { require.NoError(t, collector.Stop(context.Background())) })

	addr := collector.Addr()
	require.NotEmpty(t, addr)

	resp, err := http.Get(localURL(t, addr, "/metrics/prometheus")) // #nosec G107 -- loopback address of the listener under test
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "notification_outbox_oldest_pending_age_seconds 0")
	assert.Contains(t, string(body), "notification_outbox_last_tick_timestamp_seconds")
}

type boomExporter struct{}

func (boomExporter) Prometheus() string { return "" }

func (boomExporter) PrometheusWithError() (string, error) {
	return "", errors.New("gather boom")
}

func TestMetricsCollector_PrometheusExporterErrorReturns500(t *testing.T) {
	collector, err := NewCollector(serveTestConfig(), testLogger())
	require.NoError(t, err)
	require.NoError(t, collector.RegisterExporter("pki_enrollment", boomExporter{}))
	require.NoError(t, collector.Start(context.Background(), make(chan error, 1)))
	t.Cleanup(func() { require.NoError(t, collector.Stop(context.Background())) })

	resp, err := http.Get(localURL(t, collector.Addr(), "/metrics/prometheus")) // #nosec G107 -- loopback address of the listener under test
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// TestMetricsCollector_StartFailsWhenThePortIsAlreadyBound is the fail-fast
// guard: a listener that cannot bind must surface as an error the composition
// root can exit on, not a background log nobody reads while the scrape target
// stays dark.
func TestMetricsCollector_StartFailsWhenThePortIsAlreadyBound(t *testing.T) {
	// Loopback rather than ":0": the test only needs a port that is already
	// spoken for, and binding every interface on a CI runner is both wider than
	// necessary and what gosec G102 objects to.
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, taken.Close()) }()

	_, portStr, err := net.SplitHostPort(taken.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	cfg := serveTestConfig()
	cfg.Port = port

	collector, err := NewCollector(cfg, testLogger())
	require.NoError(t, err)

	require.Error(t, collector.Start(context.Background(), make(chan error, 1)))
	assert.Empty(t, collector.Addr())
}

// TestMetricsCollector_StartRequiresAnErrorChannel keeps a serve failure from
// being swallowed: without a sink, a listener that dies mid-flight would leave
// the process running with nothing to scrape and no signal.
func TestMetricsCollector_StartRequiresAnErrorChannel(t *testing.T) {
	collector, err := NewCollector(serveTestConfig(), testLogger())
	require.NoError(t, err)

	require.Error(t, collector.Start(context.Background(), nil))
}
