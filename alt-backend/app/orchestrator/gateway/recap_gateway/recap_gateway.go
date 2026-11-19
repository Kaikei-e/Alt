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
	"alt/orchestrator/port/reading_theme_port"
	"alt/orchestrator/port/recap_port"
)

type RecapGateway struct {
	httpClient     *http.Client
	recapWorkerURL string
}

var (
	_ recap_port.RecapPort            = (*RecapGateway)(nil)
	_ reading_theme_port.GetThemePort = (*RecapGateway)(nil)
)

func NewRecapGateway() *RecapGateway {
	recapWorkerURL := os.Getenv("RECAP_WORKER_URL")
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005" //#nosec G101 -- service-discovery default, not a credential
	}

	return &RecapGateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		recapWorkerURL: recapWorkerURL,
	}
}

func (g *RecapGateway) BaseURL() string {
	return g.recapWorkerURL
}

// NewRecapGatewayWithConfig creates a RecapGateway with custom client and URL (for testing and contract tests).
func NewRecapGatewayWithConfig(httpClient *http.Client, recapWorkerURL string) *RecapGateway {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005" //#nosec G101 -- service-discovery default, not a credential
	}
	return &RecapGateway{
		httpClient:     httpClient,
		recapWorkerURL: recapWorkerURL,
	}
}

func (g *RecapGateway) GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	url := fmt.Sprintf("%s/v1/recaps/3days", g.recapWorkerURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recap from recap-worker: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrRecapNotFound
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

func (g *RecapGateway) GetTopicCards(ctx context.Context) (*domain.RecapCardsResponse, error) {
	url := fmt.Sprintf("%s/v1/topic-cards", g.recapWorkerURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch topic cards from recap-worker: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	var cardsResp domain.RecapCardsResponse
	if err := json.NewDecoder(resp.Body).Decode(&cardsResp); err != nil {
		return nil, fmt.Errorf("failed to decode topic cards response: %w", err)
	}

	return &cardsResp, nil
}
