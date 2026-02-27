package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"
	"strings"
)

// ListFeedLinkDomains extracts unique domains from feed_links table
// Groups by domain and scheme, extracting hostname from URLs
func (r *AltDBRepository) ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error) {
	rows, err := r.pool.Query(ctx, "SELECT DISTINCT url FROM feed_links WHERE url IS NOT NULL AND url != ''")
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching feed link URLs", "error", err)
		return nil, errors.New("error fetching feed link URLs")
	}
	defer rows.Close()

	domainMap := make(map[string]domain.FeedLinkDomain) // key: domain, value: FeedLinkDomain

	for rows.Next() {
		var feedURL string
		if err := rows.Scan(&feedURL); err != nil {
			logger.SafeErrorContext(ctx, "Error scanning feed link URL", "error", err)
			continue // Skip invalid rows
		}

		parsedURL, err := url.Parse(feedURL)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error parsing feed link URL", "url", feedURL, "error", err)
			continue // Skip invalid URLs
		}

		domainHost := parsedURL.Hostname()
		if domainHost == "" {
			logger.SafeWarnContext(ctx, "Empty hostname in feed link URL", "url", feedURL)
			continue
		}

		// Normalize domain to lowercase
		domainHost = strings.ToLower(domainHost)

		// Determine scheme (default to https if not specified)
		scheme := parsedURL.Scheme
		if scheme == "" {
			scheme = "https"
		} else {
			scheme = strings.ToLower(scheme)
		}

		// Use domain as key to ensure uniqueness (one entry per domain)
		// If same domain appears with different schemes, prefer https
		if existing, exists := domainMap[domainHost]; exists {
			if scheme == "https" && existing.Scheme != "https" {
				domainMap[domainHost] = domain.FeedLinkDomain{
					Domain: domainHost,
					Scheme: scheme,
				}
			}
		} else {
			domainMap[domainHost] = domain.FeedLinkDomain{
				Domain: domainHost,
				Scheme: scheme,
			}
		}
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "Row iteration error", "error", err)
		return nil, errors.New("error iterating feed link URLs")
	}

	// Also collect domains from article_heads.og_image_url
	// OGP image domains often differ from feed subscription domains
	ogRows, err := r.pool.Query(ctx, "SELECT DISTINCT og_image_url FROM article_heads WHERE og_image_url IS NOT NULL AND og_image_url != ''")
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching OGP image URLs", "error", err)
		// Non-fatal: continue with feed_links domains only
	} else {
		defer ogRows.Close()

		for ogRows.Next() {
			var ogURL string
			if err := ogRows.Scan(&ogURL); err != nil {
				logger.SafeErrorContext(ctx, "Error scanning OGP image URL", "error", err)
				continue
			}

			parsedURL, err := url.Parse(ogURL)
			if err != nil {
				logger.SafeErrorContext(ctx, "Error parsing OGP image URL", "url", ogURL, "error", err)
				continue
			}

			domainHost := parsedURL.Hostname()
			if domainHost == "" {
				continue
			}

			domainHost = strings.ToLower(domainHost)

			scheme := parsedURL.Scheme
			if scheme == "" {
				scheme = "https"
			} else {
				scheme = strings.ToLower(scheme)
			}

			if existing, exists := domainMap[domainHost]; exists {
				if scheme == "https" && existing.Scheme != "https" {
					domainMap[domainHost] = domain.FeedLinkDomain{
						Domain: domainHost,
						Scheme: scheme,
					}
				}
			} else {
				domainMap[domainHost] = domain.FeedLinkDomain{
					Domain: domainHost,
					Scheme: scheme,
				}
			}
		}

		if err := ogRows.Err(); err != nil {
			logger.SafeErrorContext(ctx, "OGP row iteration error", "error", err)
			// Non-fatal: continue with what we have
		}
	}

	// Convert map to slice
	domains := make([]domain.FeedLinkDomain, 0, len(domainMap))
	for _, d := range domainMap {
		domains = append(domains, d)
	}

	logger.SafeInfoContext(ctx, "Extracted unique domains from feed_links and article_heads", "count", len(domains))
	return domains, nil
}
