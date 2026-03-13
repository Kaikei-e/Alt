package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	rng              = rand.New(rand.NewSource(time.Now().UnixNano()))
	rssDelay         = envInt("MOCK_RSS_DELAY_MS", 150)
	articleDelay     = envInt("MOCK_ARTICLE_DELAY_MS", 350)
	delayJitter      = envInt("MOCK_DELAY_JITTER_MS", 75)
	defaultImagePath = "/images/og-default.png"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/feeds/", handleFeed)
	mux.HandleFunc("/articles/", handleArticle)
	mux.HandleFunc("/images/", handleImage)

	log.Println("mock-rss-server listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func envInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return parsed
}

func applyDelay(baseMs int) {
	if baseMs <= 0 {
		return
	}
	jitter := 0
	if delayJitter > 0 {
		jitter = rng.Intn(delayJitter + 1)
	}
	time.Sleep(time.Duration(baseMs+jitter) * time.Millisecond)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

// handleFeed serves /feeds/{id}/rss.xml
// Path contains "feeds", "rss", "xml" to pass isValidRSSPath.
func handleFeed(w http.ResponseWriter, r *http.Request) {
	applyDelay(rssDelay)

	// Expected path: /feeds/001/rss.xml
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "feeds" || parts[2] != "rss.xml" {
		http.NotFound(w, r)
		return
	}
	feedID := parts[1]

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	pubDate := time.Now().UTC().Format(time.RFC1123Z)
	host := r.Host // e.g. "mock-rss-001:8080"

	var items strings.Builder
	for i := 1; i <= 10; i++ {
		items.WriteString(fmt.Sprintf(`    <item>
      <title>Feed %s - Article %d</title>
      <link>http://%s/articles/feed-%s/item-%d</link>
      <description>Test article %d for feed %s</description>
      <pubDate>%s</pubDate>
      <guid isPermaLink="true">http://%s/articles/feed-%s/item-%d</guid>
    </item>
`, feedID, i, host, feedID, i, i, feedID, pubDate, host, feedID, i))
	}

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Mock Feed %s</title>
    <link>http://%s/feeds/%s/rss.xml</link>
    <description>Load test mock feed %s</description>
    <lastBuildDate>%s</lastBuildDate>
%s  </channel>
</rss>
`, feedID, host, feedID, feedID, pubDate, items.String())
}

// handleArticle serves /articles/feed-{id}/item-{n} as simple HTML.
func handleArticle(w http.ResponseWriter, r *http.Request) {
	applyDelay(articleDelay)

	// Expected path: /articles/feed-{id}/item-{n}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "articles" {
		http.NotFound(w, r)
		return
	}
	feedPart := parts[1]
	itemPart := parts[2]

	if !strings.HasPrefix(feedPart, "feed-") || !strings.HasPrefix(itemPart, "item-") {
		http.NotFound(w, r)
		return
	}

	feedID := strings.TrimPrefix(feedPart, "feed-")
	itemNum := strings.TrimPrefix(itemPart, "item-")
	if _, err := strconv.Atoi(itemNum); err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
  <title>Feed %s - Article %s</title>
  <meta property="og:title" content="Feed %s - Article %s" />
  <meta property="og:image" content="http://%s%s" />
  <meta name="description" content="Mock article %s from feed %s for Connect-RPC load testing" />
</head>
<body>
<h1>Feed %s - Article %s</h1>
<article>
  <p>This is a mock article for Connect-RPC load testing.</p>
  <p>Feed ID: %s</p>
  <p>Article No: %s</p>
  <p>The body is intentionally stable so Alt can extract, sanitize, and persist it deterministically.</p>
</article>
</body>
</html>
`, feedID, itemNum, feedID, itemNum, r.Host, defaultImagePath, itemNum, feedID, feedID, itemNum, feedID, itemNum)
}

func handleImage(w http.ResponseWriter, r *http.Request) {
	applyDelay(50)

	path := strings.TrimPrefix(r.URL.Path, "/")
	if !strings.HasPrefix(path, "images/") {
		http.NotFound(w, r)
		return
	}

	// 1x1 transparent PNG.
	image := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x60, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x01, 0xe5, 0x27, 0xd4, 0xa2, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}
