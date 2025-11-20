package morning_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/morning_letter_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type MorningGateway struct {
	altDBRepository *alt_db.AltDBRepository
	httpClient      *http.Client
	recapWorkerURL  string
}

func NewMorningGateway(pool alt_db.PgxIface) morning_letter_port.MorningRepository {
	recapWorkerURL := os.Getenv("RECAP_WORKER_URL")
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005"
	}

	return &MorningGateway{
		altDBRepository: alt_db.NewAltDBRepository(pool),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		recapWorkerURL: recapWorkerURL,
	}
}

type MorningArticleGroupResponse struct {
	GroupID   uuid.UUID `json:"group_id"`
	ArticleID uuid.UUID `json:"article_id"`
	IsPrimary bool      `json:"is_primary"`
	CreatedAt time.Time `json:"created_at"`
}

func (g *MorningGateway) GetMorningArticleGroups(ctx context.Context, since time.Time) ([]*domain.MorningArticleGroup, error) {
	// 1. Fetch groups from recap-worker
	url := fmt.Sprintf("%s/v1/morning/updates?since=%s", g.recapWorkerURL, since.Format(time.RFC3339))
	logger.Logger.Info("Fetching morning updates from recap-worker", "url", url, "since", since)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Logger.Error("Failed to create request to recap-worker", "error", err, "url", url)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		logger.Logger.Error("Failed to fetch morning updates from recap-worker", "error", err, "url", url)
		return nil, fmt.Errorf("failed to fetch morning updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.Logger.Error("recap-worker returned non-OK status",
			"status", resp.StatusCode,
			"url", url,
			"response_body", string(bodyBytes))
		return nil, fmt.Errorf("recap-worker returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var groupResps []MorningArticleGroupResponse
	if err := json.NewDecoder(resp.Body).Decode(&groupResps); err != nil {
		logger.Logger.Error("Failed to decode recap-worker response", "error", err, "url", url)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Logger.Info("Fetched morning article groups from recap-worker", "count", len(groupResps))

	if len(groupResps) == 0 {
		return []*domain.MorningArticleGroup{}, nil
	}

	// 2. Collect Article IDs
	articleIDs := make([]uuid.UUID, 0, len(groupResps))
	for _, gr := range groupResps {
		articleIDs = append(articleIDs, gr.ArticleID)
	}

	if len(articleIDs) == 0 {
		logger.Logger.Info("No article IDs to fetch from database")
		return []*domain.MorningArticleGroup{}, nil
	}

	logger.Logger.Info("Fetching articles from database", "article_count", len(articleIDs))

	// 3. Fetch Articles from DB using Driver layer
	articles, err := g.altDBRepository.FetchArticlesByIDs(ctx, articleIDs)
	if err != nil {
		logger.Logger.Error("Failed to fetch articles from database", "error", err, "article_count", len(articleIDs))
		return nil, fmt.Errorf("failed to fetch articles: %w", err)
	}

	// Create a map for quick lookup
	articleMap := make(map[uuid.UUID]*domain.Article)
	for _, article := range articles {
		articleMap[article.ID] = article
	}

	// 4. Map to Domain
	var result []*domain.MorningArticleGroup
	missingArticles := 0
	for _, gr := range groupResps {
		article, ok := articleMap[gr.ArticleID]
		if !ok {
			missingArticles++
			logger.Logger.Warn("Article not found in database", "article_id", gr.ArticleID, "group_id", gr.GroupID)
			continue // Article might have been deleted or not found
		}

		result = append(result, &domain.MorningArticleGroup{
			GroupID:   gr.GroupID,
			ArticleID: gr.ArticleID,
			IsPrimary: gr.IsPrimary,
			CreatedAt: gr.CreatedAt,
			Article:   article,
		})
	}

	if missingArticles > 0 {
		logger.Logger.Warn("Some articles were not found in database", "missing_count", missingArticles, "total_groups", len(groupResps))
	}

	logger.Logger.Info("Successfully fetched morning article groups", "result_count", len(result), "total_groups", len(groupResps))
	return result, nil
}
