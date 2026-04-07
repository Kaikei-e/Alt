package morning_gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/morning_letter_port"
	"alt/utils/logger"

	"github.com/google/uuid"
)

// MorningLetterGateway implements MorningLetterRepository by calling recap-worker REST.
type MorningLetterGateway struct {
	altDBRepository *alt_db.AltDBRepository
	httpClient      *http.Client
	recapWorkerURL  string
}

func NewMorningLetterGateway(pool alt_db.PgxIface) morning_letter_port.MorningLetterRepository {
	recapWorkerURL := os.Getenv("RECAP_WORKER_URL")
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005"
	}
	return &MorningLetterGateway{
		altDBRepository: alt_db.NewAltDBRepository(pool),
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		recapWorkerURL:  recapWorkerURL,
	}
}

// --- REST response DTOs ---

type MorningLetterAPIResponse struct {
	ID                 string                `json:"id"`
	TargetDate         string                `json:"target_date"`
	EditionTimezone    string                `json:"edition_timezone"`
	IsDegraded         bool                  `json:"is_degraded"`
	SchemaVersion      int                   `json:"schema_version"`
	GenerationRevision int                   `json:"generation_revision"`
	Model              *string               `json:"model"`
	CreatedAt          string                `json:"created_at"`
	Etag               string                `json:"etag"`
	Body               MorningLetterBodyAPI  `json:"body"`
}

type MorningLetterBodyAPI struct {
	Lead                  string                      `json:"lead"`
	Sections              []MorningLetterSectionAPI   `json:"sections"`
	GeneratedAt           string                      `json:"generated_at"`
	SourceRecapWindowDays *int                        `json:"source_recap_window_days,omitempty"`
}

type MorningLetterSectionAPI struct {
	Key     string   `json:"key"`
	Title   string   `json:"title"`
	Bullets []string `json:"bullets"`
	Genre   *string  `json:"genre,omitempty"`
}

type MorningLetterSourceAPI struct {
	LetterID   string `json:"letter_id"`
	SectionKey string `json:"section_key"`
	ArticleID  string `json:"article_id"`
	SourceType string `json:"source_type"`
	Position   int    `json:"position"`
}

func (g *MorningLetterGateway) GetLatestLetter(ctx context.Context) (*domain.MorningLetterDocument, error) {
	return g.fetchLetter(ctx, fmt.Sprintf("%s/v1/morning/letters/latest", g.recapWorkerURL))
}

func (g *MorningLetterGateway) GetLetterByDate(ctx context.Context, targetDate string) (*domain.MorningLetterDocument, error) {
	return g.fetchLetter(ctx, fmt.Sprintf("%s/v1/morning/letters/%s", g.recapWorkerURL, targetDate))
}

func (g *MorningLetterGateway) fetchLetter(ctx context.Context, url string) (*domain.MorningLetterDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch morning letter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp MorningLetterAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode morning letter response: %w", err)
	}

	return mapAPIToDomain(&apiResp), nil
}

func (g *MorningLetterGateway) GetLetterSources(ctx context.Context, letterID string) ([]*domain.MorningLetterSourceEntry, error) {
	url := fmt.Sprintf("%s/v1/morning/letters/%s/sources", g.recapWorkerURL, letterID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch letter sources: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiSources []MorningLetterSourceAPI
	if err := json.NewDecoder(resp.Body).Decode(&apiSources); err != nil {
		return nil, fmt.Errorf("failed to decode letter sources: %w", err)
	}

	if len(apiSources) == 0 {
		return []*domain.MorningLetterSourceEntry{}, nil
	}

	// Collect article IDs to fetch feed_id from DB
	articleIDs := make([]uuid.UUID, 0, len(apiSources))
	for _, s := range apiSources {
		if id, err := uuid.Parse(s.ArticleID); err == nil {
			articleIDs = append(articleIDs, id)
		}
	}

	// Fetch articles to get feed_id mapping
	feedIDMap := make(map[uuid.UUID]uuid.UUID)
	if g.altDBRepository != nil && len(articleIDs) > 0 {
		articles, err := g.altDBRepository.FetchArticlesByIDs(ctx, articleIDs)
		if err != nil {
			logger.Logger.WarnContext(ctx, "Failed to fetch articles for feed_id mapping", "error", err)
		} else {
			for _, a := range articles {
				feedIDMap[a.ID] = a.FeedID
			}
		}
	}

	// Map to domain, dropping sources with unknown articles
	result := make([]*domain.MorningLetterSourceEntry, 0, len(apiSources))
	for _, s := range apiSources {
		articleID, err := uuid.Parse(s.ArticleID)
		if err != nil {
			continue
		}
		feedID, ok := feedIDMap[articleID]
		if !ok {
			logger.Logger.WarnContext(ctx, "Article not found for morning letter source, dropping",
				"article_id", s.ArticleID, "letter_id", s.LetterID, "section_key", s.SectionKey)
			continue
		}
		result = append(result, &domain.MorningLetterSourceEntry{
			LetterID:   s.LetterID,
			SectionKey: s.SectionKey,
			ArticleID:  articleID,
			SourceType: s.SourceType,
			Position:   s.Position,
			FeedID:     feedID,
		})
	}

	return result, nil
}

func mapAPIToDomain(api *MorningLetterAPIResponse) *domain.MorningLetterDocument {
	sections := make([]domain.MorningLetterSection, len(api.Body.Sections))
	for i, s := range api.Body.Sections {
		genre := ""
		if s.Genre != nil {
			genre = *s.Genre
		}
		sections[i] = domain.MorningLetterSection{
			Key:     s.Key,
			Title:   s.Title,
			Bullets: s.Bullets,
			Genre:   genre,
		}
	}

	model := ""
	if api.Model != nil {
		model = *api.Model
	}

	generatedAt, _ := time.Parse(time.RFC3339, api.Body.GeneratedAt)
	createdAt, _ := time.Parse(time.RFC3339, api.CreatedAt)

	return &domain.MorningLetterDocument{
		ID:                 api.ID,
		TargetDate:         api.TargetDate,
		EditionTimezone:    api.EditionTimezone,
		IsDegraded:         api.IsDegraded,
		SchemaVersion:      api.SchemaVersion,
		GenerationRevision: api.GenerationRevision,
		Model:              model,
		CreatedAt:          createdAt,
		Etag:               api.Etag,
		Body: domain.MorningLetterBody{
			Lead:                  api.Body.Lead,
			Sections:              sections,
			GeneratedAt:           generatedAt,
			SourceRecapWindowDays: api.Body.SourceRecapWindowDays,
		},
	}
}
