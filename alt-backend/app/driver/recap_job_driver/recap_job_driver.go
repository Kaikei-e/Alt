package recap_job_driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"alt/domain"
	"alt/port/recap_job_port"
)

type RecapJobGateway struct {
	baseURL    string
	httpClient *http.Client
}

func NewRecapJobGateway(baseURL string) recap_job_port.RecapJobRepository {
	return &RecapJobGateway{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (g *RecapJobGateway) GetRecapJobs(ctx context.Context, windowSeconds int64, limit int64) ([]domain.RecapJob, error) {
	if g.baseURL == "" {
		return nil, fmt.Errorf("recap worker URL is not configured")
	}

	url := fmt.Sprintf("%s/v1/dashboard/recap_jobs?window=%d&limit=%d", g.baseURL, windowSeconds, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	var jobs []domain.RecapJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return jobs, nil
}
