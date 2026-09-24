package recap_job_driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// JobDTO represents a recap job payload returned by recap-worker.
type JobDTO struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	LastStage *string   `json:"last_stage"`
	KickedAt  time.Time `json:"kicked_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Driver provides HTTP access to recap-worker endpoints.
type Driver struct {
	baseURL    string
	httpClient *http.Client
}

// NewDriver creates a new recap job driver.
func NewDriver(baseURL string) *Driver {
	return &Driver{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchRecapJobs fetches recap jobs from the recap worker dashboard.
func (d *Driver) FetchRecapJobs(ctx context.Context, windowSeconds int64, limit int64) ([]JobDTO, error) {
	if d.baseURL == "" {
		return nil, fmt.Errorf("recap worker URL is not configured")
	}

	url := fmt.Sprintf("%s/v1/dashboard/recap_jobs?window=%d&limit=%d", d.baseURL, windowSeconds, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request to %s: %w", url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	var jobs []JobDTO
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return jobs, nil
}
