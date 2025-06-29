package test_utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// MockRSSServer provides a configurable mock RSS server for testing
type MockRSSServer struct {
	server       *httptest.Server
	mu           sync.RWMutex
	responses    map[string]MockResponse
	requestLog   []MockRequest
	delay        time.Duration
	failureRate  float64 // 0.0 to 1.0
	requestCount int
}

type MockResponse struct {
	StatusCode int
	Body       string
	Headers    map[string]string
	Delay      time.Duration
}

type MockRequest struct {
	Method    string
	URL       string
	Headers   map[string]string
	Timestamp time.Time
}

// NewMockRSSServer creates a new mock RSS server
func NewMockRSSServer() *MockRSSServer {
	mock := &MockRSSServer{
		responses:  make(map[string]MockResponse),
		requestLog: make([]MockRequest, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleRequest))

	// Set up default responses
	mock.setupDefaultResponses()

	return mock
}

// Close shuts down the mock server
func (m *MockRSSServer) Close() {
	m.server.Close()
}

// URL returns the base URL of the mock server
func (m *MockRSSServer) URL() string {
	return m.server.URL
}

// SetResponse configures a custom response for a specific path
func (m *MockRSSServer) SetResponse(path string, response MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[path] = response
}

// SetDelay sets a global delay for all responses
func (m *MockRSSServer) SetDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
}

// SetFailureRate sets the rate at which requests should fail (0.0 to 1.0)
func (m *MockRSSServer) SetFailureRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = rate
}

// GetRequestLog returns all recorded requests
func (m *MockRSSServer) GetRequestLog() []MockRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	log := make([]MockRequest, len(m.requestLog))
	copy(log, m.requestLog)
	return log
}

// GetRequestCount returns the total number of requests received
func (m *MockRSSServer) GetRequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestCount
}

// ClearRequestLog clears the request log
func (m *MockRSSServer) ClearRequestLog() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestLog = make([]MockRequest, 0)
	m.requestCount = 0
}

func (m *MockRSSServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()

	// Log the request
	m.requestCount++
	m.requestLog = append(m.requestLog, MockRequest{
		Method:    r.Method,
		URL:       r.URL.String(),
		Headers:   extractHeaders(r),
		Timestamp: time.Now(),
	})

	// Check if request should fail
	shouldFail := float64(m.requestCount%100)/100.0 < m.failureRate

	// Get response configuration
	response, exists := m.responses[r.URL.Path]
	if !exists {
		response = m.responses["/default"]
	}

	// Apply global delay
	delay := m.delay
	if response.Delay > 0 {
		delay = response.Delay
	}

	m.mu.Unlock()

	// Apply delay
	if delay > 0 {
		time.Sleep(delay)
	}

	// Handle failure simulation
	if shouldFail {
		http.Error(w, "Simulated server error", http.StatusInternalServerError)
		return
	}

	// Set response headers
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}

	// Set status code
	w.WriteHeader(response.StatusCode)

	// Write response body
	w.Write([]byte(response.Body))
}

func (m *MockRSSServer) setupDefaultResponses() {
	// Default valid RSS feed
	defaultRSS := GenerateMockRSSFeedXML(5)
	m.responses["/default"] = MockResponse{
		StatusCode: http.StatusOK,
		Body:       defaultRSS,
		Headers: map[string]string{
			"Content-Type": "application/rss+xml",
		},
	}

	// Valid RSS feed
	m.responses["/feed.xml"] = MockResponse{
		StatusCode: http.StatusOK,
		Body:       defaultRSS,
		Headers: map[string]string{
			"Content-Type": "application/rss+xml",
		},
	}

	// Large RSS feed
	largeRSS := GenerateMockRSSFeedXML(1000)
	m.responses["/large-feed.xml"] = MockResponse{
		StatusCode: http.StatusOK,
		Body:       largeRSS,
		Headers: map[string]string{
			"Content-Type": "application/rss+xml",
		},
	}

	// Invalid RSS feed
	m.responses["/invalid.xml"] = MockResponse{
		StatusCode: http.StatusOK,
		Body:       "This is not valid RSS XML",
		Headers: map[string]string{
			"Content-Type": "application/rss+xml",
		},
	}

	// Slow response
	m.responses["/slow-feed.xml"] = MockResponse{
		StatusCode: http.StatusOK,
		Body:       defaultRSS,
		Headers: map[string]string{
			"Content-Type": "application/rss+xml",
		},
		Delay: 5 * time.Second,
	}

	// Not found
	m.responses["/notfound.xml"] = MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       "Not Found",
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
	}

	// Server error
	m.responses["/error.xml"] = MockResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       "Internal Server Error",
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
	}

	// Rate limited
	m.responses["/rate-limited.xml"] = MockResponse{
		StatusCode: http.StatusTooManyRequests,
		Body:       "Rate Limited",
		Headers: map[string]string{
			"Content-Type":      "text/plain",
			"Retry-After":       "60",
			"X-RateLimit-Limit": "100",
		},
	}
}

// MockDatabaseServer provides a mock database server for testing
type MockDatabaseServer struct {
	server      *httptest.Server
	mu          sync.RWMutex
	data        map[string]interface{}
	queryLog    []string
	queryCount  int
	delay       time.Duration
	failureRate float64
}

func NewMockDatabaseServer() *MockDatabaseServer {
	mock := &MockDatabaseServer{
		data:     make(map[string]interface{}),
		queryLog: make([]string, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleDatabaseRequest))
	mock.setupDefaultData()

	return mock
}

func (m *MockDatabaseServer) Close() {
	m.server.Close()
}

func (m *MockDatabaseServer) URL() string {
	return m.server.URL
}

func (m *MockDatabaseServer) SetData(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

func (m *MockDatabaseServer) GetQueryLog() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	log := make([]string, len(m.queryLog))
	copy(log, m.queryLog)
	return log
}

func (m *MockDatabaseServer) SetDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
}

func (m *MockDatabaseServer) SetFailureRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = rate
}

func (m *MockDatabaseServer) handleDatabaseRequest(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()

	// Log the query
	m.queryCount++
	query := r.URL.Query().Get("q")
	m.queryLog = append(m.queryLog, query)

	// Check if request should fail
	shouldFail := float64(m.queryCount%100)/100.0 < m.failureRate

	delay := m.delay
	m.mu.Unlock()

	// Apply delay
	if delay > 0 {
		time.Sleep(delay)
	}

	// Handle failure simulation
	if shouldFail {
		http.Error(w, "Database connection failed", http.StatusInternalServerError)
		return
	}

	// Handle different query types
	w.Header().Set("Content-Type", "application/json")

	switch {
	case strings.Contains(query, "SELECT"):
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"rows": [{"id": 1, "title": "Test Feed", "link": "http://example.com"}]}`))

	case strings.Contains(query, "INSERT"):
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 123, "status": "created"}`))

	case strings.Contains(query, "UPDATE"):
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"affected_rows": 1, "status": "updated"}`))

	case strings.Contains(query, "DELETE"):
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"affected_rows": 1, "status": "deleted"}`))

	default:
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}
}

func (m *MockDatabaseServer) setupDefaultData() {
	m.data["feeds"] = []map[string]interface{}{
		{
			"id":          1,
			"title":       "Test Feed 1",
			"description": "Description 1",
			"link":        "http://example1.com",
			"published":   time.Now().Add(-1 * time.Hour),
		},
		{
			"id":          2,
			"title":       "Test Feed 2",
			"description": "Description 2",
			"link":        "http://example2.com",
			"published":   time.Now().Add(-2 * time.Hour),
		},
	}
}

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

	w.Write([]byte(response))
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

// Helper functions
func extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
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

// Multi-server test environment
type TestEnvironment struct {
	RSSServer      *MockRSSServer
	DatabaseServer *MockDatabaseServer
	SearchServer   *MockSearchServer
}

func NewTestEnvironment() *TestEnvironment {
	return &TestEnvironment{
		RSSServer:      NewMockRSSServer(),
		DatabaseServer: NewMockDatabaseServer(),
		SearchServer:   NewMockSearchServer(),
	}
}

func (te *TestEnvironment) Close() {
	if te.RSSServer != nil {
		te.RSSServer.Close()
	}
	if te.DatabaseServer != nil {
		te.DatabaseServer.Close()
	}
	if te.SearchServer != nil {
		te.SearchServer.Close()
	}
}
