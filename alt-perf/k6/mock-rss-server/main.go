package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/feeds/", handleFeed)
	mux.HandleFunc("/articles/", handleArticle)

	log.Println("mock-rss-server listening on :8090")
	if err := http.ListenAndServe(":8090", mux); err != nil {
		log.Fatal(err)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

// handleFeed serves /feeds/{id}/rss.xml
// Path contains "feeds", "rss", "xml" to pass isValidRSSPath.
func handleFeed(w http.ResponseWriter, r *http.Request) {
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
	host := r.Host // e.g. "mock-rss-001:8090"

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
<head><title>Feed %s - Article %s</title></head>
<body>
<h1>Feed %s - Article %s</h1>
<p>This is a mock article for load testing.</p>
</body>
</html>
`, feedID, itemNum, feedID, itemNum)
}
