package dashboard_gateway

import (
	"alt/orchestrator/port/dashboard_port"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Verify interface compliance at compile time.
var _ dashboard_port.DashboardMetricsPort = (*DashboardGateway)(nil)

type DashboardGateway struct {
	httpClient     *http.Client
	recapWorkerURL string
}

func NewDashboardGateway() *DashboardGateway {
	recapWorkerURL := os.Getenv("RECAP_WORKER_URL")
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005" //#nosec G101 -- service-discovery default, not a credential
	}

	return &DashboardGateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		recapWorkerURL: recapWorkerURL,
	}
}

// GetMetrics fetches system metrics from recap-worker
func (g *DashboardGateway) GetMetrics(ctx context.Context, metricType string, windowSeconds, limit int64) ([]byte, error) {
	reqURL, err := buildDashboardQueryURL(g.recapWorkerURL, "/v1/dashboard/metrics", metricType, windowSeconds, limit)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics from recap-worker: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := checkDashboardStatus(resp.StatusCode, body); err != nil {
		return nil, err
	}

	return body, nil
}

// GetOverview fetches recent activity from recap-worker
func (g *DashboardGateway) GetOverview(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	reqURL, err := buildDashboardQueryURL(g.recapWorkerURL, "/v1/dashboard/overview", "", windowSeconds, limit)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch overview from recap-worker: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := checkDashboardStatus(resp.StatusCode, body); err != nil {
		return nil, err
	}

	return body, nil
}

// GetLogs fetches error logs from recap-worker
func (g *DashboardGateway) GetLogs(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	reqURL, err := buildDashboardQueryURL(g.recapWorkerURL, "/v1/dashboard/logs", "", windowSeconds, limit)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs from recap-worker: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := checkDashboardStatus(resp.StatusCode, body); err != nil {
		return nil, err
	}

	return body, nil
}

// GetJobs fetches admin jobs from recap-worker
func (g *DashboardGateway) GetJobs(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	reqURL, err := buildDashboardQueryURL(g.recapWorkerURL, "/v1/dashboard/jobs", "", windowSeconds, limit)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs from recap-worker: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := checkDashboardStatus(resp.StatusCode, body); err != nil {
		return nil, err
	}

	return body, nil
}

func buildDashboardQueryURL(baseURL, path, metricType string, windowSeconds, limit int64) (string, error) {
	u, err := url.Parse(fmt.Sprintf("%s%s", baseURL, path))
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	if metricType != "" {
		q.Set("type", metricType)
	}
	if windowSeconds > 0 {
		q.Set("window", strconv.FormatInt(windowSeconds, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func checkDashboardStatus(statusCode int, body []byte) error {
	if statusCode != http.StatusOK {
		return fmt.Errorf("recap-worker returned status %d: %s", statusCode, string(body))
	}
	return nil
}
