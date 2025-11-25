package domain

import (
	"time"

	"github.com/google/uuid"
)

// ScrapingDomain represents a domain-level scraping policy and robots.txt cache
type ScrapingDomain struct {
	ID                  uuid.UUID  `json:"id"`
	Domain              string     `json:"domain"`
	Scheme              string     `json:"scheme"`
	AllowFetchBody      bool       `json:"allow_fetch_body"`
	AllowMLTraining     bool       `json:"allow_ml_training"`
	AllowCacheDays      int        `json:"allow_cache_days"`
	ForceRespectRobots  bool       `json:"force_respect_robots"`
	RobotsTxtURL        *string    `json:"robots_txt_url,omitempty"`
	RobotsTxtContent    *string    `json:"robots_txt_content,omitempty"`
	RobotsTxtFetchedAt  *time.Time `json:"robots_txt_fetched_at,omitempty"`
	RobotsTxtLastStatus *int       `json:"robots_txt_last_status,omitempty"`
	RobotsCrawlDelaySec *int       `json:"robots_crawl_delay_sec,omitempty"`
	RobotsDisallowPaths []string   `json:"robots_disallow_paths"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// RobotsTxt represents parsed robots.txt information
type RobotsTxt struct {
	URL           string    `json:"url"`
	Content       string    `json:"content"`
	FetchedAt     time.Time `json:"fetched_at"`
	StatusCode    int       `json:"status_code"`
	CrawlDelay    int       `json:"crawl_delay"` // in seconds
	DisallowPaths []string  `json:"disallow_paths"`
}

// ScrapingPolicyUpdate represents a partial update to a scraping domain policy
type ScrapingPolicyUpdate struct {
	AllowFetchBody     *bool `json:"allow_fetch_body,omitempty"`
	AllowMLTraining    *bool `json:"allow_ml_training,omitempty"`
	AllowCacheDays     *int  `json:"allow_cache_days,omitempty"`
	ForceRespectRobots *bool `json:"force_respect_robots,omitempty"`
}
