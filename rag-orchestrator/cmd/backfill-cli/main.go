package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Article struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Cursor struct {
	LastCreatedAt time.Time `json:"last_created_at"`
	LastID        string    `json:"last_id"`
}

const (
	cursorFile     = "cursor.json"
	maxRetries     = 3
	requestTimeout = 5 * time.Minute
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Println("DATABASE_URL is required")
		os.Exit(1)
	}

	orchestratorURL := os.Getenv("ORCHESTRATOR_URL")
	if orchestratorURL == "" {
		orchestratorURL = "http://localhost:9010"
	}

	// Best Practice: Custom Transport for connection pooling and timeouts
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: requestTimeout,
		DisableKeepAlives:     true, // Ensure fresh connection to avoid stuck states
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   requestTimeout + 10*time.Second, // Global timeout slightly larger than context
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		fmt.Printf("Failed to connect to DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Load cursor
	cursor := loadCursor()
	var rows *sql.Rows

	query := `
		SELECT id, title, content, url, user_id, created_at
		FROM articles
		WHERE content IS NOT NULL AND content != ''
		  AND deleted_at IS NULL
	`
	args := []interface{}{}

	if !cursor.LastCreatedAt.IsZero() {
		fmt.Printf("Resuming from %s (ID: %s)\n", cursor.LastCreatedAt.Format(time.RFC3339), cursor.LastID)
		query += ` AND (created_at, id) < ($1, $2)`
		args = append(args, cursor.LastCreatedAt, cursor.LastID)
	}

	query += ` ORDER BY created_at DESC, id DESC`

	rows, err = db.Query(query, args...)
	if err != nil {
		fmt.Printf("Failed to query articles: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	count := 0
	success := 0
	failed := 0

	for rows.Next() {
		time.Sleep(100 * time.Millisecond) // Slight throttle
		var a Article
		if err := rows.Scan(&a.ID, &a.Title, &a.Body, &a.URL, &a.UserID, &a.CreatedAt); err != nil {
			fmt.Printf("Failed to scan article: %v\n", err)
			continue
		}

		if err := sendWithRetry(client, orchestratorURL, a); err != nil {
			fmt.Printf("Failed to send article %s: %v (Skipping)\n", a.ID, err)
			failed++
		} else {
			success++
		}

		// Always update cursor to prevent getting stuck on a failing/timeout item
		saveCursor(Cursor{LastCreatedAt: a.CreatedAt, LastID: a.ID})

		count++
		if count%50 == 0 {
			fmt.Printf("Processed %d... (Success: %d, Failed: %d)\n", count, success, failed)
		}
	}

	fmt.Printf("Backfill complete. Total: %d, Success: %d, Failed: %d\n", count, success, failed)
}

func sendWithRetry(client *http.Client, baseURL string, a Article) error {
	payload := map[string]interface{}{
		"article_id":   a.ID,
		"user_id":      a.UserID,
		"title":        a.Title,
		"body":         a.Body,
		"url":          a.URL,
		"published_at": a.CreatedAt.Format(time.RFC3339),
	}
	data, _ := json.Marshal(payload)

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// Immediate retry with minimal delay as requested (avoid long hangs)
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("Retrying article %s (attempt %d)...\n", a.ID, i+1)
		}

		// Use Context with Timeout for per-request deadline
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/internal/rag/index/upsert", bytes.NewReader(data))
		if err != nil {
			cancel()
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		cancel() // Cancel context immediately after handling response/error

		if err != nil {
			lastErr = err
			// Check for context deadline exceeded (Timeout)
			if os.IsTimeout(err) || err == context.DeadlineExceeded {
				fmt.Printf("Timeout for article %s: %v. Skipping retries.\n", a.ID, err)
				return err // Do not retry on timeout, it takes too long
			}
			// Retry on network errors
			continue
		}

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
			// Drain body
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return nil
		}

		// Read response body for error details
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		errorMsg := string(bodyBytes)

		// Handle Duplicate Key error (Race Condition) gracefully
		// Since we want the doc to be indexed, and duplicate key means it IS indexed (by someone else),
		// we can consider this a success or at least safe to skip.
		if resp.StatusCode == http.StatusInternalServerError &&
			(bytes.Contains(bodyBytes, []byte("duplicate key")) || bytes.Contains(bodyBytes, []byte("Unique constraint"))) {
			fmt.Printf("Race condition detected for article %s (Duplicate Key). Treating as success.\n", a.ID)
			return nil
		}

		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, errorMsg)
		// Don't retry client errors (4xx) except 429
		if resp.StatusCode < 500 && resp.StatusCode != 429 {
			return lastErr
		}
	}

	return lastErr
}

func loadCursor() Cursor {
	file, err := os.Open(cursorFile)
	if err != nil {
		return Cursor{}
	}
	defer file.Close()

	var c Cursor
	if err := json.NewDecoder(file).Decode(&c); err != nil {
		return Cursor{}
	}
	return c
}

func saveCursor(c Cursor) {
	file, err := os.Create(cursorFile)
	if err != nil {
		fmt.Printf("Warning: failed to save cursor: %v\n", err)
		return
	}
	defer file.Close()

	json.NewEncoder(file).Encode(c)
}
