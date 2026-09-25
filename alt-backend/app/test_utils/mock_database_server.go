package test_utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

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
		_, _ = w.Write([]byte(`{"rows": [{"id": 1, "title": "Test Feed", "link": "http://example.com"}]}`)) //#nosec G104 -- test mock response writer

	case strings.Contains(query, "INSERT"):
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 123, "status": "created"}`)) //#nosec G104 -- test mock response writer

	case strings.Contains(query, "UPDATE"):
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"affected_rows": 1, "status": "updated"}`)) //#nosec G104 -- test mock response writer

	case strings.Contains(query, "DELETE"):
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"affected_rows": 1, "status": "deleted"}`)) //#nosec G104 -- test mock response writer

	default:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`)) //#nosec G104 -- test mock response writer
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
