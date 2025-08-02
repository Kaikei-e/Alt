package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBasicMetricsCollector_InitialState(t *testing.T) {
	collector := NewBasicMetricsCollector()

	assert.NotNil(t, collector, "Collector should not be nil")
	assert.Equal(t, int64(0), collector.GetTotalRequests(), "Initial total requests should be 0")
	assert.Equal(t, int64(0), collector.GetSuccessfulRequests(), "Initial successful requests should be 0")
	assert.Equal(t, int64(0), collector.GetFailedRequests(), "Initial failed requests should be 0")
}

func TestBasicMetricsCollector_RequestCounting(t *testing.T) {
	tests := []struct {
		name              string
		operations        []string // "success" or "failure"
		expectedTotal     int64
		expectedSuccesses int64
		expectedFailures  int64
	}{
		{
			name:              "single success",
			operations:        []string{"success"},
			expectedTotal:     1,
			expectedSuccesses: 1,
			expectedFailures:  0,
		},
		{
			name:              "single failure",
			operations:        []string{"failure"},
			expectedTotal:     1,
			expectedSuccesses: 0,
			expectedFailures:  1,
		},
		{
			name:              "mixed operations",
			operations:        []string{"success", "failure", "success", "failure", "success"},
			expectedTotal:     5,
			expectedSuccesses: 3,
			expectedFailures:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewBasicMetricsCollector()

			for _, op := range tt.operations {
				switch op {
				case "success":
					collector.RecordSuccess()
				case "failure":
					collector.RecordFailure()
				}
			}

			assert.Equal(t, tt.expectedTotal, collector.GetTotalRequests(), "Total requests should match")
			assert.Equal(t, tt.expectedSuccesses, collector.GetSuccessfulRequests(), "Successful requests should match")
			assert.Equal(t, tt.expectedFailures, collector.GetFailedRequests(), "Failed requests should match")
		})
	}
}

func TestBasicMetricsCollector_ResponseTimeTracking(t *testing.T) {
	collector := NewBasicMetricsCollector()

	// Record some response times
	collector.RecordResponseTime(100 * time.Millisecond)
	collector.RecordResponseTime(200 * time.Millisecond)
	collector.RecordResponseTime(300 * time.Millisecond)

	avgResponseTime := collector.GetAverageResponseTime()
	assert.Greater(t, avgResponseTime, time.Duration(0), "Average response time should be greater than 0")
	assert.Equal(t, 200*time.Millisecond, avgResponseTime, "Average should be 200ms")
}

func TestBasicMetricsCollector_ConcurrentAccess(t *testing.T) {
	collector := NewBasicMetricsCollector()
	
	// Test concurrent access to ensure thread safety
	done := make(chan bool, 10)

	// Start 5 goroutines recording successes
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				collector.RecordSuccess()
			}
			done <- true
		}()
	}

	// Start 5 goroutines recording failures
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				collector.RecordFailure()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, int64(100), collector.GetTotalRequests(), "Total should be 100")
	assert.Equal(t, int64(50), collector.GetSuccessfulRequests(), "Successes should be 50")
	assert.Equal(t, int64(50), collector.GetFailedRequests(), "Failures should be 50")
}

func TestBasicMetricsCollector_GetSuccessRate(t *testing.T) {
	tests := []struct {
		name            string
		successes       int
		failures        int
		expectedRate    float64
	}{
		{
			name:         "100% success rate",
			successes:    10,
			failures:     0,
			expectedRate: 1.0,
		},
		{
			name:         "0% success rate",
			successes:    0,
			failures:     10,
			expectedRate: 0.0,
		},
		{
			name:         "50% success rate",
			successes:    5,
			failures:     5,
			expectedRate: 0.5,
		},
		{
			name:         "no requests",
			successes:    0,
			failures:     0,
			expectedRate: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := NewBasicMetricsCollector()

			for i := 0; i < tt.successes; i++ {
				collector.RecordSuccess()
			}
			for i := 0; i < tt.failures; i++ {
				collector.RecordFailure()
			}

			rate := collector.GetSuccessRate()
			assert.Equal(t, tt.expectedRate, rate, "Success rate should match expected")
		})
	}
}

func TestBasicMetricsCollector_Reset(t *testing.T) {
	collector := NewBasicMetricsCollector()

	// Record some metrics
	collector.RecordSuccess()
	collector.RecordFailure()
	collector.RecordResponseTime(100 * time.Millisecond)

	// Verify metrics are recorded
	assert.Equal(t, int64(2), collector.GetTotalRequests(), "Should have 2 total requests")
	assert.Greater(t, collector.GetAverageResponseTime(), time.Duration(0), "Should have response time")

	// Reset metrics
	collector.Reset()

	// Verify all metrics are reset
	assert.Equal(t, int64(0), collector.GetTotalRequests(), "Total requests should be reset to 0")
	assert.Equal(t, int64(0), collector.GetSuccessfulRequests(), "Successful requests should be reset to 0")
	assert.Equal(t, int64(0), collector.GetFailedRequests(), "Failed requests should be reset to 0")
	assert.Equal(t, time.Duration(0), collector.GetAverageResponseTime(), "Average response time should be reset")
}

func TestBasicMetricsCollector_GetSnapshot(t *testing.T) {
	collector := NewBasicMetricsCollector()

	// Record some metrics
	collector.RecordSuccess()
	collector.RecordSuccess()
	collector.RecordFailure()
	collector.RecordResponseTime(150 * time.Millisecond)
	collector.RecordResponseTime(250 * time.Millisecond)

	snapshot := collector.GetSnapshot()

	assert.Equal(t, int64(3), snapshot.TotalRequests, "Snapshot should capture total requests")
	assert.Equal(t, int64(2), snapshot.SuccessfulRequests, "Snapshot should capture successful requests")
	assert.Equal(t, int64(1), snapshot.FailedRequests, "Snapshot should capture failed requests")
	assert.Equal(t, 2.0/3.0, snapshot.SuccessRate, "Snapshot should capture success rate")
	assert.Equal(t, 200*time.Millisecond, snapshot.AverageResponseTime, "Snapshot should capture average response time")
}