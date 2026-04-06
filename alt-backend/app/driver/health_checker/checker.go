package health_checker

import (
	"alt/domain"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ServiceEndpoint defines a service to health-check.
type ServiceEndpoint struct {
	Name     string
	Endpoint string
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
func (c *Checker) CheckHealth(ctx context.Context) ([]domain.ServiceHealthStatus, error) {
	results := make([]domain.ServiceHealthStatus, len(c.endpoints))
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

func (c *Checker) checkOne(ctx context.Context, ep ServiceEndpoint) domain.ServiceHealthStatus {
	start := time.Now()
	result := domain.ServiceHealthStatus{
		ServiceName: ep.Name,
		Endpoint:    ep.Endpoint,
		CheckedAt:   start,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.Endpoint, nil)
	if err != nil {
		result.Status = domain.ServiceUnknown
		result.ErrorMessage = fmt.Sprintf("bad request: %v", err)
		return result
	}

	resp, err := c.client.Do(req)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Status = domain.ServiceUnhealthy
		result.ErrorMessage = err.Error()
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = domain.ServiceHealthy
	} else {
		result.Status = domain.ServiceUnhealthy
		result.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return result
}
