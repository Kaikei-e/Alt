package test_utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// MockSearchServer provides a mock search server for testing
type MockSearchServer struct {
	server    *httptest.Server
	mu        sync.RWMutex
	indexes   map[string][]map[string]interface{}
	searchLog []string
}

func NewMockSearchServer() *MockSearchServer {
	mock := &MockSearchServer{
		indexes:   make(map[string][]map[string]interface{}),
		searchLog: make([]string, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleSearchRequest))
	mock.setupDefaultIndexes()

	return mock
}

func (m *MockSearchServer) Close() {
	m.server.Close()
}

func (m *MockSearchServer) URL() string {
	return m.server.URL
}

func (m *MockSearchServer) AddToIndex(indexName string, documents []map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.indexes[indexName] = append(m.indexes[indexName], documents...)
}

func (m *MockSearchServer) GetSearchLog() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	log := make([]string, len(m.searchLog))
	copy(log, m.searchLog)
	return log
}

func (m *MockSearchServer) handleSearchRequest(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()

	// Log the search query
	query := r.URL.Query().Get("q")
	m.searchLog = append(m.searchLog, query)

	// Get index name from path
	indexName := strings.TrimPrefix(r.URL.Path, "/indexes/")
	indexName = strings.TrimSuffix(indexName, "/search")

	// Get documents from index
	documents, exists := m.indexes[indexName]
	if !exists {
		documents = m.indexes["default"]
	}

	m.mu.Unlock()

	// Simple search simulation - return documents that contain query term
	var results []map[string]interface{}

	if query == "" {
		results = documents
	} else {
		for _, doc := range documents {
			if title, ok := doc["title"].(string); ok {
				if strings.Contains(strings.ToLower(title), strings.ToLower(query)) {
					results = append(results, doc)
				}
			}
		}
	}

	// Return search results
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := fmt.Sprintf(`{
		"hits": %s,
		"query": "%s",
		"processingTimeMs": 1,
		"hitsPerPage": 20,
		"page": 0,
		"totalPages": 1,
		"totalHits": %d
	}`, formatDocumentsAsJSON(results), query, len(results))

	_, _ = w.Write([]byte(response)) //#nosec G104,G705 -- test mock response writer; query is echoed through a fixed template, not rendered to a browser
}

func (m *MockSearchServer) setupDefaultIndexes() {
	m.indexes["default"] = []map[string]interface{}{
		{
			"id":          "1",
			"title":       "Technology News",
			"description": "Latest technology updates and news",
			"link":        "http://tech.example.com/1",
		},
		{
			"id":          "2",
			"title":       "Science Updates",
			"description": "Recent scientific discoveries and research",
			"link":        "http://science.example.com/1",
		},
		{
			"id":          "3",
			"title":       "Business Today",
			"description": "Current business and economic news",
			"link":        "http://business.example.com/1",
		},
	}

	m.indexes["feeds"] = m.indexes["default"]
}

func formatDocumentsAsJSON(docs []map[string]interface{}) string {
	if len(docs) == 0 {
		return "[]"
	}

	var parts []string
	for _, doc := range docs {
		parts = append(parts, fmt.Sprintf(`{
			"id": "%v",
			"title": "%v", 
			"description": "%v",
			"link": "%v"
		}`, doc["id"], doc["title"], doc["description"], doc["link"]))
	}

	return "[" + strings.Join(parts, ",") + "]"
}
