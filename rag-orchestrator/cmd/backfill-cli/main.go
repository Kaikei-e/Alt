package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Article struct {
	ID    string
	Title string
	Body  string
}

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

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		fmt.Printf("Failed to connect to DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, title, content FROM articles WHERE content IS NOT NULL AND content != ''")
	if err != nil {
		fmt.Printf("Failed to query articles: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	count := 0
	success := 0
	failed := 0

	client := &http.Client{Timeout: 10 * time.Second}

	for rows.Next() {
		time.Sleep(500 * time.Millisecond)
		var a Article
		if err := rows.Scan(&a.ID, &a.Title, &a.Body); err != nil {
			fmt.Printf("Failed to scan article: %v\n", err)
			continue
		}

		payload := map[string]string{
			"article_id": a.ID,
			"title":      a.Title,
			"body":       a.Body,
		}
		data, _ := json.Marshal(payload)

		resp, err := client.Post(orchestratorURL+"/internal/rag/backfill", "application/json", strings.NewReader(string(data)))
		if err != nil {
			fmt.Printf("Failed to send article %s: %v\n", a.ID, err)
			failed++
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			fmt.Printf("Failed to send article %s: status %d\n", a.ID, resp.StatusCode)
			failed++
			continue
		}

		success++
		count++
		if count%100 == 0 {
			fmt.Printf("Processed %d articles...\n", count)
		}
	}

	fmt.Printf("Backfill complete. Total: %d, Success: %d, Failed: %d\n", count, success, failed)
}
