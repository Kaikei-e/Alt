package test_utils

import (
	"net/http"
	"net/http/httptest"
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
	_, _ = w.Write([]byte(response.Body)) //#nosec G104 -- test mock response writer
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
