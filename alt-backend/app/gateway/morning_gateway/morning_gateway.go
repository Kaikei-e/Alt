package morning_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/morning_letter_port"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type MorningGateway struct {
	pool           alt_db.PgxIface
	httpClient     *http.Client
	recapWorkerURL string
}

func NewMorningGateway(pool alt_db.PgxIface) morning_letter_port.MorningRepository {
	recapWorkerURL := os.Getenv("RECAP_WORKER_URL")
	if recapWorkerURL == "" {
		recapWorkerURL = "http://recap-worker:9005"
	}

	return &MorningGateway{
		pool: pool,
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch morning updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recap-worker returned status %d", resp.StatusCode)
	}

	var groupResps []MorningArticleGroupResponse
	if err := json.NewDecoder(resp.Body).Decode(&groupResps); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(groupResps) == 0 {
		return []*domain.MorningArticleGroup{}, nil
	}

	// 2. Collect Article IDs
	articleIDs := make([]uuid.UUID, 0, len(groupResps))
	for _, gr := range groupResps {
		articleIDs = append(articleIDs, gr.ArticleID)
	}

	// 3. Fetch Articles from DB
	query := `
		SELECT id, feed_id, tenant_id, title, content, summary, url, author, language, tags, published_at, created_at, updated_at
		FROM articles
		WHERE id = ANY($1)
	`
	rows, err := g.pool.Query(ctx, query, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch articles: %w", err)
	}
	defer rows.Close()

	articleMap := make(map[uuid.UUID]*domain.Article)
	for rows.Next() {
		var a domain.Article
		err := rows.Scan(
			&a.ID, &a.FeedID, &a.TenantID, &a.Title, &a.Content, &a.Summary, &a.URL, &a.Author, &a.Language, &a.Tags, &a.PublishedAt, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan article: %w", err)
		}
		articleMap[a.ID] = &a
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	// 4. Map to Domain
	var result []*domain.MorningArticleGroup
	for _, gr := range groupResps {
		article, ok := articleMap[gr.ArticleID]
		if !ok {
			continue // Article might have been deleted or not found
		}

		result = append(result, &domain.MorningArticleGroup{
			GroupID:   gr.GroupID,
			ArticleID: gr.ArticleID,
			IsPrimary: gr.IsPrimary,
			Article:   article,
		})
	}

	return result, nil
}
