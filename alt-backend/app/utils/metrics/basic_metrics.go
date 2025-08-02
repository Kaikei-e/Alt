package metrics

import (
	"sync"
	"time"
)

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	TotalRequests        int64         `json:"total_requests"`
	SuccessfulRequests   int64         `json:"successful_requests"`
	FailedRequests       int64         `json:"failed_requests"`
	SuccessRate          float64       `json:"success_rate"`
	AverageResponseTime  time.Duration `json:"average_response_time"`
}

// BasicMetricsCollector provides thread-safe basic metrics collection
// for RSS feed operations. It tracks request counts, success rates,
// and response time statistics.
type BasicMetricsCollector struct {
	totalRequests       int64
	successfulRequests  int64
	failedRequests      int64
	totalResponseTime   time.Duration
	responseTimeCount   int64
	mutex              sync.RWMutex
}

// NewBasicMetricsCollector creates a new BasicMetricsCollector instance
func NewBasicMetricsCollector() *BasicMetricsCollector {
	return &BasicMetricsCollector{}
}

// RecordSuccess increments the successful request counter
func (c *BasicMetricsCollector) RecordSuccess() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.totalRequests++
	c.successfulRequests++
}

// RecordFailure increments the failed request counter
func (c *BasicMetricsCollector) RecordFailure() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.totalRequests++
	c.failedRequests++
}

// RecordResponseTime records a response time measurement
func (c *BasicMetricsCollector) RecordResponseTime(duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.totalResponseTime += duration
	c.responseTimeCount++
}

// GetTotalRequests returns the total number of requests processed
func (c *BasicMetricsCollector) GetTotalRequests() int64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	return c.totalRequests
}

// GetSuccessfulRequests returns the number of successful requests
func (c *BasicMetricsCollector) GetSuccessfulRequests() int64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	return c.successfulRequests
}

// GetFailedRequests returns the number of failed requests
func (c *BasicMetricsCollector) GetFailedRequests() int64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	return c.failedRequests
}

// GetSuccessRate returns the success rate as a float between 0.0 and 1.0
func (c *BasicMetricsCollector) GetSuccessRate() float64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	if c.totalRequests == 0 {
		return 0.0
	}
	
	return float64(c.successfulRequests) / float64(c.totalRequests)
}

// GetAverageResponseTime returns the average response time
func (c *BasicMetricsCollector) GetAverageResponseTime() time.Duration {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	if c.responseTimeCount == 0 {
		return time.Duration(0)
	}
	
	return c.totalResponseTime / time.Duration(c.responseTimeCount)
}

// Reset clears all metrics back to initial state
func (c *BasicMetricsCollector) Reset() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	c.totalRequests = 0
	c.successfulRequests = 0
	c.failedRequests = 0
	c.totalResponseTime = 0
	c.responseTimeCount = 0
}

// GetSnapshot returns a point-in-time snapshot of all metrics
func (c *BasicMetricsCollector) GetSnapshot() MetricsSnapshot {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	// Calculate success rate directly
	var successRate float64
	if c.totalRequests == 0 {
		successRate = 0.0
	} else {
		successRate = float64(c.successfulRequests) / float64(c.totalRequests)
	}
	
	// Calculate average response time directly
	var avgResponseTime time.Duration
	if c.responseTimeCount == 0 {
		avgResponseTime = time.Duration(0)
	} else {
		avgResponseTime = c.totalResponseTime / time.Duration(c.responseTimeCount)
	}
	
	return MetricsSnapshot{
		TotalRequests:       c.totalRequests,
		SuccessfulRequests:  c.successfulRequests,
		FailedRequests:      c.failedRequests,
		SuccessRate:         successRate,
		AverageResponseTime: avgResponseTime,
	}
}