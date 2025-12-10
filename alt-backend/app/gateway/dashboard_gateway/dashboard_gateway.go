package dashboard_gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type DashboardGateway struct {
	httpClient     *http.Client
	recapWorkerURL string
}

func NewDashboardGateway() *DashboardGateway {
	recapWorkerURL := os.Getenv("RECAP_WORKER_URL")
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005"
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
	u, err := url.Parse(fmt.Sprintf("%s/v1/dashboard/metrics", g.recapWorkerURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics from recap-worker: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetOverview fetches recent activity from recap-worker
func (g *DashboardGateway) GetOverview(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v1/dashboard/overview", g.recapWorkerURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	if windowSeconds > 0 {
		q.Set("window", strconv.FormatInt(windowSeconds, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch overview from recap-worker: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetLogs fetches error logs from recap-worker
func (g *DashboardGateway) GetLogs(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v1/dashboard/logs", g.recapWorkerURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	if windowSeconds > 0 {
		q.Set("window", strconv.FormatInt(windowSeconds, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs from recap-worker: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetJobs fetches admin jobs from recap-worker
func (g *DashboardGateway) GetJobs(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v1/dashboard/jobs", g.recapWorkerURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	if windowSeconds > 0 {
		q.Set("window", strconv.FormatInt(windowSeconds, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs from recap-worker: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
