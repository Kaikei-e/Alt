package main

import (
	"context"
	"fmt"
	"io"
	"log"
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
func FetchHTMLFromURL(url string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
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
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	log.Println("Connected to database successfully")

	// Find articles with URL as title
	query := `SELECT id, title, url FROM articles WHERE title LIKE 'http%' ORDER BY id`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		log.Fatalf("Failed to query articles: %v", err)
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var article Article
		if err := rows.Scan(&article.ID, &article.Title, &article.URL); err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}
		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	log.Printf("Found %d articles with URL as title\n", len(articles))

	// Process each article
	successCount := 0
	failureCount := 0
	unchangedCount := 0

	for i, article := range articles {
		log.Printf("[%d/%d] Processing article %s...", i+1, len(articles), article.ID)

		// Fetch HTML from URL
		html, err := FetchHTMLFromURL(article.URL)
		if err != nil {
			log.Printf("  ✗ Failed to fetch HTML: %v", err)
			failureCount++
			// Add delay to avoid rate limiting
			time.Sleep(2 * time.Second)
			continue
		}

		// Extract title from HTML
		extractedTitle := ExtractTitle(html)
		if extractedTitle == "" || strings.HasPrefix(extractedTitle, "http://") || strings.HasPrefix(extractedTitle, "https://") {
			log.Printf("  ⚠ Could not extract valid title, keeping URL as title")
			unchangedCount++
			time.Sleep(2 * time.Second)
			continue
		}

		// Update database
		updateQuery := `UPDATE articles SET title = $1 WHERE id = $2`
		_, err = pool.Exec(ctx, updateQuery, extractedTitle, article.ID)
		if err != nil {
			log.Printf("  ✗ Failed to update database: %v", err)
			failureCount++
			time.Sleep(2 * time.Second)
			continue
		}

		log.Printf("  ✓ Updated: %s", extractedTitle)
		successCount++

		// Add delay to avoid rate limiting (5 seconds between requests)
		time.Sleep(5 * time.Second)
	}

	log.Println("\n=== Summary ===")
	log.Printf("Total articles processed: %d", len(articles))
	log.Printf("Successfully updated: %d", successCount)
	log.Printf("Failed to update: %d", failureCount)
	log.Printf("Unchanged (no valid title): %d", unchangedCount)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
