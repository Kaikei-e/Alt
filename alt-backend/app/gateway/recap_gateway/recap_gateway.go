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
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
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

// GetEveningPulse fetches Evening Pulse data from recap-worker
func (g *RecapGateway) GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error) {
	url := fmt.Sprintf("%s/v1/pulse/latest", g.recapWorkerURL)
	if date != "" {
		url = fmt.Sprintf("%s?date=%s", url, date)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch evening pulse from recap-worker: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrEveningPulseNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(body))
	}

	var pulseResponse eveningPulseResponse
	if err := json.NewDecoder(resp.Body).Decode(&pulseResponse); err != nil {
		return nil, fmt.Errorf("failed to decode evening pulse response: %w", err)
	}

	return pulseResponse.toDomain()
}

// eveningPulseResponse represents the JSON response from recap-worker
type eveningPulseResponse struct {
	JobID       string               `json:"job_id"`
	Date        string               `json:"date"`
	GeneratedAt string               `json:"generated_at"`
	Status      string               `json:"status"`
	Topics      []pulseTopicResponse `json:"topics"`
	QuietDay    *quietDayResponse    `json:"quiet_day,omitempty"`
}

type pulseTopicResponse struct {
	ClusterID       int64            `json:"cluster_id"`
	Role            string           `json:"role"`
	Title           string           `json:"title"`
	Rationale       rationaleResponse `json:"rationale"`
	ArticleCount    int              `json:"article_count"`
	SourceCount     int              `json:"source_count"`
	Tier1Count      *int             `json:"tier1_count,omitempty"`
	TimeAgo         string           `json:"time_ago"`
	TrendMultiplier *float64         `json:"trend_multiplier,omitempty"`
	Genre           *string          `json:"genre,omitempty"`
	ArticleIDs      []string         `json:"article_ids"`
}

type rationaleResponse struct {
	Text       string `json:"text"`
	Confidence string `json:"confidence"`
}

type quietDayResponse struct {
	Message          string                    `json:"message"`
	WeeklyHighlights []weeklyHighlightResponse `json:"weekly_highlights"`
}

type weeklyHighlightResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Date  string `json:"date"`
	Role  string `json:"role"`
}

func (r *eveningPulseResponse) toDomain() (*domain.EveningPulse, error) {
	generatedAt, err := time.Parse(time.RFC3339, r.GeneratedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated_at: %w", err)
	}

	topics := make([]domain.PulseTopic, len(r.Topics))
	for i, t := range r.Topics {
		topics[i] = domain.PulseTopic{
			ClusterID:       t.ClusterID,
			Role:            parseTopicRole(t.Role),
			Title:           t.Title,
			Rationale:       domain.PulseRationale{
				Text:       t.Rationale.Text,
				Confidence: parseConfidence(t.Rationale.Confidence),
			},
			ArticleCount:    t.ArticleCount,
			SourceCount:     t.SourceCount,
			Tier1Count:      t.Tier1Count,
			TimeAgo:         t.TimeAgo,
			TrendMultiplier: t.TrendMultiplier,
			Genre:           t.Genre,
			ArticleIDs:      t.ArticleIDs,
		}
	}

	var quietDay *domain.QuietDayInfo
	if r.QuietDay != nil {
		highlights := make([]domain.WeeklyHighlight, len(r.QuietDay.WeeklyHighlights))
		for i, h := range r.QuietDay.WeeklyHighlights {
			highlights[i] = domain.WeeklyHighlight{
				ID:    h.ID,
				Title: h.Title,
				Date:  h.Date,
				Role:  h.Role,
			}
		}
		quietDay = &domain.QuietDayInfo{
			Message:          r.QuietDay.Message,
			WeeklyHighlights: highlights,
		}
	}

	return &domain.EveningPulse{
		JobID:       r.JobID,
		Date:        r.Date,
		GeneratedAt: generatedAt,
		Status:      parsePulseStatus(r.Status),
		Topics:      topics,
		QuietDay:    quietDay,
	}, nil
}

func parsePulseStatus(s string) domain.PulseStatus {
	switch s {
	case "normal":
		return domain.PulseStatusNormal
	case "partial":
		return domain.PulseStatusPartial
	case "quiet_day":
		return domain.PulseStatusQuietDay
	case "error":
		return domain.PulseStatusError
	default:
		return domain.PulseStatusError
	}
}

func parseTopicRole(s string) domain.TopicRole {
	switch s {
	case "need_to_know":
		return domain.TopicRoleNeedToKnow
	case "trend":
		return domain.TopicRoleTrend
	case "serendipity":
		return domain.TopicRoleSerendipity
	default:
		return domain.TopicRoleNeedToKnow
	}
}

func parseConfidence(s string) domain.Confidence {
	switch s {
	case "high":
		return domain.ConfidenceHigh
	case "medium":
		return domain.ConfidenceMedium
	case "low":
		return domain.ConfidenceLow
	default:
		return domain.ConfidenceMedium
	}
}
