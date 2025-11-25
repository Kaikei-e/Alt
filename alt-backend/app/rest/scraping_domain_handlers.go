package rest

import (
	"alt/di"
	"alt/domain"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ScrapingDomainResponse represents the response structure for scraping domain operations
type ScrapingDomainResponse struct {
	ID                  string   `json:"id"`
	Domain              string   `json:"domain"`
	Scheme              string   `json:"scheme"`
	AllowFetchBody      bool     `json:"allow_fetch_body"`
	AllowMLTraining     bool     `json:"allow_ml_training"`
	AllowCacheDays      int      `json:"allow_cache_days"`
	ForceRespectRobots  bool     `json:"force_respect_robots"`
	RobotsTxtURL        *string  `json:"robots_txt_url,omitempty"`
	RobotsTxtContent    *string  `json:"robots_txt_content,omitempty"`
	RobotsTxtFetchedAt  *string  `json:"robots_txt_fetched_at,omitempty"`
	RobotsTxtLastStatus *int     `json:"robots_txt_last_status,omitempty"`
	RobotsCrawlDelaySec *int     `json:"robots_crawl_delay_sec,omitempty"`
	RobotsDisallowPaths []string `json:"robots_disallow_paths"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

// UpdateScrapingDomainRequest represents the request structure for updating scraping domain policy
type UpdateScrapingDomainRequest struct {
	AllowFetchBody     *bool `json:"allow_fetch_body,omitempty"`
	AllowMLTraining    *bool `json:"allow_ml_training,omitempty"`
	AllowCacheDays     *int  `json:"allow_cache_days,omitempty"`
	ForceRespectRobots *bool `json:"force_respect_robots,omitempty"`
}

// registerScrapingDomainRoutes registers the scraping domain management routes
func registerScrapingDomainRoutes(v1 *echo.Group, container *di.ApplicationComponents) {
	// Admin endpoints (authentication required)
	admin := v1.Group("/admin")
	scrapingDomains := admin.Group("/scraping-domains")

	scrapingDomains.GET("", handleListScrapingDomains(container))
	scrapingDomains.GET("/:id", handleGetScrapingDomain(container))
	scrapingDomains.PATCH("/:id", handleUpdateScrapingDomainPolicy(container))
	scrapingDomains.POST("/:id/refresh-robots", handleRefreshRobotsTxt(container))
}

// handleListScrapingDomains handles GET /v1/admin/scraping-domains
func handleListScrapingDomains(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		// Parse query parameters
		offset, _ := strconv.Atoi(c.QueryParam("offset"))
		limit, _ := strconv.Atoi(c.QueryParam("limit"))
		if limit <= 0 || limit > 100 {
			limit = 20 // Default limit
		}

		domains, err := container.ScrapingDomainUsecase.ListScrapingDomains(ctx, offset, limit)
		if err != nil {
			return handleError(c, fmt.Errorf("failed to list scraping domains: %w", err), "list_scraping_domains")
		}

		// Convert to response format
		responses := make([]ScrapingDomainResponse, len(domains))
		for i, d := range domains {
			responses[i] = toScrapingDomainResponse(d)
		}

		return c.JSON(http.StatusOK, responses)
	}
}

// handleGetScrapingDomain handles GET /v1/admin/scraping-domains/:id
func handleGetScrapingDomain(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		idStr := c.Param("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			return handleValidationError(c, "Invalid domain ID", "id", idStr)
		}

		domain, err := container.ScrapingDomainUsecase.GetScrapingDomain(ctx, id)
		if err != nil {
			return handleError(c, fmt.Errorf("failed to get scraping domain: %w", err), "get_scraping_domain")
		}

		if domain == nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "scraping domain not found"})
		}

		return c.JSON(http.StatusOK, toScrapingDomainResponse(domain))
	}
}

// handleUpdateScrapingDomainPolicy handles PATCH /v1/admin/scraping-domains/:id
func handleUpdateScrapingDomainPolicy(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		idStr := c.Param("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			return handleValidationError(c, "Invalid domain ID", "id", idStr)
		}

		var req UpdateScrapingDomainRequest
		if err := c.Bind(&req); err != nil {
			return handleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		update := &domain.ScrapingPolicyUpdate{
			AllowFetchBody:     req.AllowFetchBody,
			AllowMLTraining:    req.AllowMLTraining,
			AllowCacheDays:     req.AllowCacheDays,
			ForceRespectRobots: req.ForceRespectRobots,
		}

		if err := container.ScrapingDomainUsecase.UpdateScrapingDomainPolicy(ctx, id, update); err != nil {
			return handleError(c, fmt.Errorf("failed to update scraping domain policy: %w", err), "update_scraping_domain_policy")
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "scraping domain policy updated"})
	}
}

// handleRefreshRobotsTxt handles POST /v1/admin/scraping-domains/:id/refresh-robots
func handleRefreshRobotsTxt(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		idStr := c.Param("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			return handleValidationError(c, "Invalid domain ID", "id", idStr)
		}

		if err := container.ScrapingDomainUsecase.RefreshRobotsTxt(ctx, id); err != nil {
			return handleError(c, fmt.Errorf("failed to refresh robots.txt: %w", err), "refresh_robots_txt")
		}

		return c.JSON(http.StatusOK, map[string]string{"message": "robots.txt refreshed"})
	}
}

// toScrapingDomainResponse converts domain.ScrapingDomain to ScrapingDomainResponse
func toScrapingDomainResponse(d *domain.ScrapingDomain) ScrapingDomainResponse {
	resp := ScrapingDomainResponse{
		ID:                  d.ID.String(),
		Domain:              d.Domain,
		Scheme:              d.Scheme,
		AllowFetchBody:      d.AllowFetchBody,
		AllowMLTraining:     d.AllowMLTraining,
		AllowCacheDays:      d.AllowCacheDays,
		ForceRespectRobots:  d.ForceRespectRobots,
		RobotsDisallowPaths: d.RobotsDisallowPaths,
		CreatedAt:           d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if d.RobotsTxtURL != nil {
		resp.RobotsTxtURL = d.RobotsTxtURL
	}
	if d.RobotsTxtContent != nil {
		resp.RobotsTxtContent = d.RobotsTxtContent
	}
	if d.RobotsTxtFetchedAt != nil {
		fetchedAt := d.RobotsTxtFetchedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.RobotsTxtFetchedAt = &fetchedAt
	}
	if d.RobotsTxtLastStatus != nil {
		resp.RobotsTxtLastStatus = d.RobotsTxtLastStatus
	}
	if d.RobotsCrawlDelaySec != nil {
		resp.RobotsCrawlDelaySec = d.RobotsCrawlDelaySec
	}

	return resp
}
