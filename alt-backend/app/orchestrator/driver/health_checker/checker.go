package health_checker

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ServiceStatus represents the health status of a service at driver level.
type ServiceStatus string

const (
	StatusHealthy   ServiceStatus = "healthy"
	StatusUnhealthy ServiceStatus = "unhealthy"
	StatusUnknown   ServiceStatus = "unknown"
)

// ServiceEndpoint defines a service to health-check.
type ServiceEndpoint struct {
	Name     string
	Endpoint string
}

// Result holds the health check outcome for a service.
type Result struct {
	ServiceName  string
	Endpoint     string
	CheckedAt    time.Time
	LatencyMs    int64
	Status       ServiceStatus
	ErrorMessage string
}

// Checker performs concurrent health checks on downstream services.
type Checker struct {
	client    *http.Client
	endpoints []ServiceEndpoint
}

// NewChecker creates a new health checker with the given endpoints.
func NewChecker(endpoints []ServiceEndpoint) *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
		endpoints: endpoints,
	}
}

// CheckHealth calls /health on all configured endpoints concurrently.
func (c *Checker) CheckHealth(ctx context.Context) ([]Result, error) {
	results := make([]Result, len(c.endpoints))
	var wg sync.WaitGroup
	wg.Add(len(c.endpoints))

	for i, ep := range c.endpoints {
		go func(idx int, endpoint ServiceEndpoint) {
			defer wg.Done()
			results[idx] = c.checkOne(ctx, endpoint)
		}(i, ep)
	}

	wg.Wait()
	return results, nil
}

func (c *Checker) checkOne(ctx context.Context, ep ServiceEndpoint) Result {
	start := time.Now()
	result := Result{
		ServiceName: ep.Name,
		Endpoint:    ep.Endpoint,
		CheckedAt:   start,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.Endpoint, nil)
	if err != nil {
		result.Status = StatusUnknown
		result.ErrorMessage = fmt.Sprintf("bad request: %v", err)
		return result
	}

	resp, err := c.client.Do(req)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Status = StatusUnhealthy
		result.ErrorMessage = err.Error()
		return result
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = StatusHealthy
	} else {
		result.Status = StatusUnhealthy
		result.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return result
}
