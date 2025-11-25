package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StringArray is a custom type for PostgreSQL text[] arrays
type StringArray []string

// Value implements the driver.Valuer interface
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}

// Scan implements the sql.Scanner interface
func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("cannot scan non-string value into StringArray")
	}

	return json.Unmarshal(bytes, a)
}

// GetScrapingDomainByDomain fetches a scraping domain by domain name
func (r *AltDBRepository) GetScrapingDomainByDomain(ctx context.Context, domainName string) (*domain.ScrapingDomain, error) {
	query := `
		SELECT id, domain, scheme, allow_fetch_body, allow_ml_training,
		       allow_cache_days, force_respect_robots, robots_txt_url, robots_txt_content,
		       robots_txt_fetched_at, robots_txt_last_status, robots_crawl_delay_sec,
		       robots_disallow_paths, created_at, updated_at
		FROM scraping_domains
		WHERE domain = $1
		LIMIT 1
	`

	var sd domain.ScrapingDomain
	var robotsTxtURL, robotsTxtContent sql.NullString
	var robotsTxtFetchedAt sql.NullTime
	var robotsTxtLastStatus sql.NullInt32
	var robotsCrawlDelaySec sql.NullInt32
	var robotsDisallowPaths StringArray

	err := r.pool.QueryRow(ctx, query, domainName).Scan(
		&sd.ID, &sd.Domain, &sd.Scheme,
		&sd.AllowFetchBody, &sd.AllowMLTraining, &sd.AllowCacheDays,
		&sd.ForceRespectRobots, &robotsTxtURL, &robotsTxtContent,
		&robotsTxtFetchedAt, &robotsTxtLastStatus, &robotsCrawlDelaySec,
		&robotsDisallowPaths, &sd.CreatedAt, &sd.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.SafeError("Error fetching scraping domain", "error", err, "domain", domainName)
		return nil, errors.New("error fetching scraping domain")
	}

	// Convert nullable fields
	if robotsTxtURL.Valid {
		sd.RobotsTxtURL = &robotsTxtURL.String
	}
	if robotsTxtContent.Valid {
		sd.RobotsTxtContent = &robotsTxtContent.String
	}
	if robotsTxtFetchedAt.Valid {
		sd.RobotsTxtFetchedAt = &robotsTxtFetchedAt.Time
	}
	if robotsTxtLastStatus.Valid {
		status := int(robotsTxtLastStatus.Int32)
		sd.RobotsTxtLastStatus = &status
	}
	if robotsCrawlDelaySec.Valid {
		delay := int(robotsCrawlDelaySec.Int32)
		sd.RobotsCrawlDelaySec = &delay
	}
	if robotsDisallowPaths != nil {
		sd.RobotsDisallowPaths = []string(robotsDisallowPaths)
	} else {
		sd.RobotsDisallowPaths = []string{}
	}

	return &sd, nil
}

// GetScrapingDomainByID fetches a scraping domain by ID
func (r *AltDBRepository) GetScrapingDomainByID(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	query := `
		SELECT id, domain, scheme, allow_fetch_body, allow_ml_training,
		       allow_cache_days, force_respect_robots, robots_txt_url, robots_txt_content,
		       robots_txt_fetched_at, robots_txt_last_status, robots_crawl_delay_sec,
		       robots_disallow_paths, created_at, updated_at
		FROM scraping_domains
		WHERE id = $1
		LIMIT 1
	`

	var sd domain.ScrapingDomain
	var robotsTxtURL, robotsTxtContent sql.NullString
	var robotsTxtFetchedAt sql.NullTime
	var robotsTxtLastStatus sql.NullInt32
	var robotsCrawlDelaySec sql.NullInt32
	var robotsDisallowPaths StringArray

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&sd.ID, &sd.Domain, &sd.Scheme,
		&sd.AllowFetchBody, &sd.AllowMLTraining, &sd.AllowCacheDays,
		&sd.ForceRespectRobots, &robotsTxtURL, &robotsTxtContent,
		&robotsTxtFetchedAt, &robotsTxtLastStatus, &robotsCrawlDelaySec,
		&robotsDisallowPaths, &sd.CreatedAt, &sd.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.SafeError("Error fetching scraping domain by ID", "error", err, "id", id)
		return nil, errors.New("error fetching scraping domain")
	}

	// Convert nullable fields
	if robotsTxtURL.Valid {
		sd.RobotsTxtURL = &robotsTxtURL.String
	}
	if robotsTxtContent.Valid {
		sd.RobotsTxtContent = &robotsTxtContent.String
	}
	if robotsTxtFetchedAt.Valid {
		sd.RobotsTxtFetchedAt = &robotsTxtFetchedAt.Time
	}
	if robotsTxtLastStatus.Valid {
		status := int(robotsTxtLastStatus.Int32)
		sd.RobotsTxtLastStatus = &status
	}
	if robotsCrawlDelaySec.Valid {
		delay := int(robotsCrawlDelaySec.Int32)
		sd.RobotsCrawlDelaySec = &delay
	}
	if robotsDisallowPaths != nil {
		sd.RobotsDisallowPaths = []string(robotsDisallowPaths)
	} else {
		sd.RobotsDisallowPaths = []string{}
	}

	return &sd, nil
}

// SaveScrapingDomain saves or updates a scraping domain
func (r *AltDBRepository) SaveScrapingDomain(ctx context.Context, sd *domain.ScrapingDomain) error {
	query := `
		INSERT INTO scraping_domains (
			id, domain, scheme, allow_fetch_body, allow_ml_training,
			allow_cache_days, force_respect_robots, robots_txt_url, robots_txt_content,
			robots_txt_fetched_at, robots_txt_last_status, robots_crawl_delay_sec,
			robots_disallow_paths, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
		ON CONFLICT (domain)
		DO UPDATE SET
			allow_fetch_body = EXCLUDED.allow_fetch_body,
			allow_ml_training = EXCLUDED.allow_ml_training,
			allow_cache_days = EXCLUDED.allow_cache_days,
			force_respect_robots = EXCLUDED.force_respect_robots,
			robots_txt_url = EXCLUDED.robots_txt_url,
			robots_txt_content = EXCLUDED.robots_txt_content,
			robots_txt_fetched_at = EXCLUDED.robots_txt_fetched_at,
			robots_txt_last_status = EXCLUDED.robots_txt_last_status,
			robots_crawl_delay_sec = EXCLUDED.robots_crawl_delay_sec,
			robots_disallow_paths = EXCLUDED.robots_disallow_paths,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	if sd.ID == uuid.Nil {
		sd.ID = uuid.New()
	}
	if sd.CreatedAt.IsZero() {
		sd.CreatedAt = now
	}
	sd.UpdatedAt = now

	// Convert to JSONB for robots_disallow_paths
	disallowPathsJSON, err := json.Marshal(sd.RobotsDisallowPaths)
	if err != nil {
		logger.SafeError("Error marshaling robots_disallow_paths", "error", err)
		return errors.New("error marshaling robots_disallow_paths")
	}

	_, err = r.pool.Exec(ctx, query,
		sd.ID, sd.Domain, sd.Scheme,
		sd.AllowFetchBody, sd.AllowMLTraining, sd.AllowCacheDays,
		sd.ForceRespectRobots, sd.RobotsTxtURL, sd.RobotsTxtContent,
		sd.RobotsTxtFetchedAt, sd.RobotsTxtLastStatus, sd.RobotsCrawlDelaySec,
		disallowPathsJSON, sd.CreatedAt, sd.UpdatedAt,
	)

	if err != nil {
		logger.SafeError("Error saving scraping domain", "error", err, "domain_name", sd.Domain)
		return errors.New("error saving scraping domain")
	}

	logger.SafeInfo("Scraping domain saved", "id", sd.ID, "domain_name", sd.Domain)
	return nil
}

// ListScrapingDomains lists scraping domains with pagination
func (r *AltDBRepository) ListScrapingDomains(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error) {
	query := `
		SELECT id, domain, scheme, allow_fetch_body, allow_ml_training,
		       allow_cache_days, force_respect_robots, robots_txt_url, robots_txt_content,
		       robots_txt_fetched_at, robots_txt_last_status, robots_crawl_delay_sec,
		       robots_disallow_paths, created_at, updated_at
		FROM scraping_domains
		ORDER BY domain ASC, created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		logger.SafeError("Error listing scraping domains", "error", err)
		return nil, errors.New("error listing scraping domains")
	}
	defer rows.Close()

	domains := make([]*domain.ScrapingDomain, 0)
	for rows.Next() {
		var sd domain.ScrapingDomain
		var robotsTxtURL, robotsTxtContent sql.NullString
		var robotsTxtFetchedAt sql.NullTime
		var robotsTxtLastStatus sql.NullInt32
		var robotsCrawlDelaySec sql.NullInt32
		var robotsDisallowPaths StringArray

		err := rows.Scan(
			&sd.ID, &sd.Domain, &sd.Scheme,
			&sd.AllowFetchBody, &sd.AllowMLTraining, &sd.AllowCacheDays,
			&sd.ForceRespectRobots, &robotsTxtURL, &robotsTxtContent,
			&robotsTxtFetchedAt, &robotsTxtLastStatus, &robotsCrawlDelaySec,
			&robotsDisallowPaths, &sd.CreatedAt, &sd.UpdatedAt,
		)
		if err != nil {
			logger.SafeError("Error scanning scraping domain", "error", err)
			return nil, errors.New("error scanning scraping domains")
		}

		// Convert nullable fields
		if robotsTxtURL.Valid {
			sd.RobotsTxtURL = &robotsTxtURL.String
		}
		if robotsTxtContent.Valid {
			sd.RobotsTxtContent = &robotsTxtContent.String
		}
		if robotsTxtFetchedAt.Valid {
			sd.RobotsTxtFetchedAt = &robotsTxtFetchedAt.Time
		}
		if robotsTxtLastStatus.Valid {
			status := int(robotsTxtLastStatus.Int32)
			sd.RobotsTxtLastStatus = &status
		}
		if robotsCrawlDelaySec.Valid {
			delay := int(robotsCrawlDelaySec.Int32)
			sd.RobotsCrawlDelaySec = &delay
		}
		if robotsDisallowPaths != nil {
			sd.RobotsDisallowPaths = []string(robotsDisallowPaths)
		} else {
			sd.RobotsDisallowPaths = []string{}
		}

		domains = append(domains, &sd)
	}

	if err := rows.Err(); err != nil {
		logger.SafeError("Row iteration error", "error", err)
		return nil, errors.New("error iterating scraping domains")
	}

	return domains, nil
}

// UpdateScrapingDomainPolicy updates only the policy fields of a scraping domain
func (r *AltDBRepository) UpdateScrapingDomainPolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	query := `
		UPDATE scraping_domains
		SET
			allow_fetch_body = COALESCE($2, allow_fetch_body),
			allow_ml_training = COALESCE($3, allow_ml_training),
			allow_cache_days = COALESCE($4, allow_cache_days),
			force_respect_robots = COALESCE($5, force_respect_robots),
			updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query,
		id,
		update.AllowFetchBody,
		update.AllowMLTraining,
		update.AllowCacheDays,
		update.ForceRespectRobots,
	)

	if err != nil {
		logger.SafeError("Error updating scraping domain policy", "error", err, "id", id)
		return errors.New("error updating scraping domain policy")
	}

	if result.RowsAffected() == 0 {
		logger.SafeWarn("Scraping domain not found for update", "id", id)
		return errors.New("scraping domain not found")
	}

	logger.SafeInfo("Scraping domain policy updated", "id", id)
	return nil
}
