package recap_gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"alt/domain"
	"alt/port/recap_port"
)

type RecapGateway struct {
	httpClient     *http.Client
	recapWorkerURL string
}

func NewRecapGateway() recap_port.RecapPort {
	recapWorkerURL := os.Getenv("RECAP_WORKER_URL")
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005"
	}

	return &RecapGateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		recapWorkerURL: recapWorkerURL,
	}
}

func (g *RecapGateway) GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	// recap-workerのAPIエンドポイントにリクエスト
	url := fmt.Sprintf("%s/v1/recaps/7days", g.recapWorkerURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recap from recap-worker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no 7-day recap found")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	var recapSummary domain.RecapSummary
	if err := json.NewDecoder(resp.Body).Decode(&recapSummary); err != nil {
		return nil, fmt.Errorf("failed to decode recap response: %w", err)
	}

	return &recapSummary, nil
}
