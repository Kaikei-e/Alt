// normalize_feed_urls.go - One-shot script to normalize existing feed URLs in the database
//
// Usage:
//   go run scripts/normalize_feed_urls.go
//
// This script:
// 1. Reads all feeds from the database
// 2. Normalizes each URL (removes UTM parameters, trailing slashes, etc.)
// 3. Updates the feeds table with normalized URLs
// 4. Handles duplicates by keeping the oldest entry
//
// Run this script after deploying the feeds_gateway.go changes.

package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// trackingParams contains query parameters to remove during normalization
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"utm_id":       true,
	"fbclid":       true,
	"gclid":        true,
	"mc_eid":       true,
	"msclkid":      true,
}

func normalizeURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Remove fragment
	parsed.Fragment = ""

	// Filter out tracking parameters
	query := parsed.Query()
	for param := range query {
		if trackingParams[strings.ToLower(param)] {
			query.Del(param)
		}
	}

	// Sort remaining parameters for consistency
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Rebuild query string
	if len(query) > 0 {
		var params []string
		for _, k := range keys {
			for _, v := range query[k] {
				params = append(params, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
			}
		}
		parsed.RawQuery = strings.Join(params, "&")
	} else {
		parsed.RawQuery = ""
	}

	// Normalize percent-encoding to uppercase
	result := parsed.String()
	result = normalizePercentEncoding(result)

	// Remove trailing slash (except for root path)
	if len(result) > 1 && strings.HasSuffix(result, "/") && !strings.HasSuffix(result, "://") {
		pathEnd := strings.LastIndex(result, "/")
		if pathEnd > 0 && result[pathEnd-1] != '/' {
			result = result[:pathEnd]
		}
	}

	return result, nil
}

func normalizePercentEncoding(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			// Convert to uppercase
			result = append(result, '%')
			result = append(result, strings.ToUpper(string(s[i+1:i+3]))...)
			i += 2
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}

type Feed struct {
	ID        string
	Link      string
	CreatedAt string
}

func main() {
	// Get database connection string from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Fallback to individual environment variables
		host := getEnvOrDefault("DB_HOST", "localhost")
		port := getEnvOrDefault("DB_PORT", "5432")
		user := getEnvOrDefault("DB_USER", "alt_appuser")
		password := os.Getenv("DB_PASSWORD")
		dbName := getEnvOrDefault("DB_NAME", "alt")

		if password == "" {
			log.Fatal("DB_PASSWORD environment variable is required")
		}

		dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			user, password, host, port, dbName)
	}

	ctx := context.Background()

	// Connect to database
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to database")

	// Get all feeds
	rows, err := pool.Query(ctx, `
		SELECT id, link, created_at::text
		FROM feeds
		ORDER BY created_at ASC
	`)
	if err != nil {
		log.Fatalf("Failed to query feeds: %v", err)
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var f Feed
		if err := rows.Scan(&f.ID, &f.Link, &f.CreatedAt); err != nil {
			log.Fatalf("Failed to scan feed: %v", err)
		}
		feeds = append(feeds, f)
	}

	log.Printf("Found %d feeds to process", len(feeds))

	// Group feeds by normalized URL
	normalizedMap := make(map[string][]Feed)
	normalizeErrors := 0

	for _, f := range feeds {
		normalized, err := normalizeURL(f.Link)
		if err != nil {
			log.Printf("Warning: Failed to normalize URL %s: %v", f.Link, err)
			normalizeErrors++
			continue
		}
		normalizedMap[normalized] = append(normalizedMap[normalized], f)
	}

	log.Printf("Grouped into %d unique normalized URLs (%d normalization errors)",
		len(normalizedMap), normalizeErrors)

	// Process each group
	updatedCount := 0
	deletedCount := 0
	skippedCount := 0

	for normalized, group := range normalizedMap {
		if len(group) == 1 {
			// Single feed - just update if different
			f := group[0]
			if f.Link != normalized {
				_, err := pool.Exec(ctx,
					`UPDATE feeds SET link = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
					normalized, f.ID)
				if err != nil {
					log.Printf("Error updating feed %s: %v", f.ID, err)
					continue
				}
				updatedCount++
				log.Printf("Updated: %s -> %s", f.Link, normalized)
			} else {
				skippedCount++
			}
		} else {
			// Multiple feeds with same normalized URL - keep oldest, delete others
			// (group is already sorted by created_at ASC)
			keeper := group[0]

			// Update the keeper's link if needed
			if keeper.Link != normalized {
				_, err := pool.Exec(ctx,
					`UPDATE feeds SET link = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
					normalized, keeper.ID)
				if err != nil {
					log.Printf("Error updating keeper feed %s: %v", keeper.ID, err)
					continue
				}
				updatedCount++
				log.Printf("Updated (keeper): %s -> %s", keeper.Link, normalized)
			}

			// Delete duplicates
			for _, dup := range group[1:] {
				// First, update any read_status entries to point to the keeper
				_, err := pool.Exec(ctx, `
					UPDATE read_status
					SET feed_id = $1
					WHERE feed_id = $2
					AND NOT EXISTS (
						SELECT 1 FROM read_status
						WHERE feed_id = $1 AND user_id = read_status.user_id
					)
				`, keeper.ID, dup.ID)
				if err != nil {
					log.Printf("Warning: Failed to migrate read_status for %s: %v", dup.ID, err)
				}

				// Delete the duplicate feed (cascade will handle remaining read_status)
				_, err = pool.Exec(ctx, `DELETE FROM feeds WHERE id = $1`, dup.ID)
				if err != nil {
					log.Printf("Error deleting duplicate feed %s: %v", dup.ID, err)
					continue
				}
				deletedCount++
				log.Printf("Deleted duplicate: %s (kept: %s)", dup.Link, keeper.Link)
			}
		}
	}

	log.Printf("\n=== Summary ===")
	log.Printf("Total feeds processed: %d", len(feeds))
	log.Printf("Updated: %d", updatedCount)
	log.Printf("Deleted (duplicates): %d", deletedCount)
	log.Printf("Skipped (already normalized): %d", skippedCount)
	log.Printf("Normalization errors: %d", normalizeErrors)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
