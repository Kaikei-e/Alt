package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExtractTitle extracts the article title from HTML content using the same logic as html_parser
func ExtractTitle(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return ""
	}

	// 1. Try <title> tag first
	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title != "" {
		return title
	}

	// 2. Try Open Graph title meta tag
	ogTitle, exists := doc.Find("meta[property='og:title']").First().Attr("content")
	if exists && strings.TrimSpace(ogTitle) != "" {
		return strings.TrimSpace(ogTitle)
	}

	// 3. Fall back to first <h1> tag
	h1Title := strings.TrimSpace(doc.Find("h1").First().Text())
	if h1Title != "" {
		return h1Title
	}

	return ""
}

// FetchHTMLFromURL fetches HTML content from a URL
func FetchHTMLFromURL(ctx context.Context, url string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - data has been read
			_ = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}

type Article struct {
	ID    string
	Title string
	URL   string
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Get database connection string from environment
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "alt_db_user")
	dbPassword := getEnv("DB_PASSWORD", "")
	dbName := getEnv("DB_NAME", "alt")

	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=prefer",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	ctx := context.Background()

	// Connect to database
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Error("Unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	log.Info("Connected to database successfully")

	// Find articles with URL as title
	query := `SELECT id, title, url FROM articles WHERE title LIKE 'http%' ORDER BY id`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		log.Error("Failed to query articles", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var article Article
		if err := rows.Scan(&article.ID, &article.Title, &article.URL); err != nil {
			log.Error("Failed to scan row", "error", err)
			continue
		}
		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		log.Error("Error iterating rows", "error", err)
		os.Exit(1)
	}

	log.Info("Found articles with URL as title", "count", len(articles))

	// Process each article
	successCount := 0
	failureCount := 0
	unchangedCount := 0

	for i, article := range articles {
		log.Info("Processing article", "index", i+1, "total", len(articles), "article_id", article.ID)

		// Fetch HTML from URL
		html, err := FetchHTMLFromURL(ctx, article.URL)
		if err != nil {
			log.Error("Failed to fetch HTML", "article_id", article.ID, "error", err)
			failureCount++
			// Add delay to avoid rate limiting
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				log.Error("Interrupted during backoff", "error", err)
				os.Exit(1)
			}
			continue
		}

		// Extract title from HTML
		extractedTitle := ExtractTitle(html)
		if extractedTitle == "" || strings.HasPrefix(extractedTitle, "http://") || strings.HasPrefix(extractedTitle, "https://") {
			log.Warn("Could not extract valid title, keeping URL as title", "article_id", article.ID)
			unchangedCount++
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				log.Error("Interrupted during backoff", "error", err)
				os.Exit(1)
			}
			continue
		}

		// Update database
		updateQuery := `UPDATE articles SET title = $1 WHERE id = $2`
		_, err = pool.Exec(ctx, updateQuery, extractedTitle, article.ID)
		if err != nil {
			log.Error("Failed to update database", "article_id", article.ID, "error", err)
			failureCount++
			if err := sleepWithContext(ctx, 2*time.Second); err != nil {
				log.Error("Interrupted during backoff", "error", err)
				os.Exit(1)
			}
			continue
		}

		log.Info("Updated article title", "article_id", article.ID, "title", extractedTitle)
		successCount++

		// Add delay to avoid rate limiting (5 seconds between requests)
		if err := sleepWithContext(ctx, 5*time.Second); err != nil {
			log.Error("Interrupted during backoff", "error", err)
			os.Exit(1)
		}
	}

	log.Info("Summary",
		"total", len(articles),
		"updated", successCount,
		"failed", failureCount,
		"unchanged", unchangedCount,
	)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
