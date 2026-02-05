package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAggregationHandler(t *testing.T) {
	handler := NewAggregationHandler(nil, nil)
	assert.NotNil(t, handler)
}

func TestAggregationRequest_Parse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		queries []string
	}{
		{
			name:    "valid request",
			body:    `{"queries": ["feed_stats", "unread_count"]}`,
			wantErr: false,
			queries: []string{"feed_stats", "unread_count"},
		},
		{
			name:    "empty queries",
			body:    `{"queries": []}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			body:    `{invalid}`,
			wantErr: true,
		},
		{
			name:    "missing queries field",
			body:    `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req AggregationRequest
			err := json.Unmarshal([]byte(tt.body), &req)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}

			err = req.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.queries, req.Queries)
			}
		})
	}
}

func TestAggregationHandler_ServeHTTP_Success(t *testing.T) {
	// Create mock backend
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/alt.feeds.v2.FeedService/GetFeedStats":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"total_feeds": 10}`))
		case "/alt.feeds.v2.FeedService/GetUnreadCount":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"unread_count": 5}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockBackend.Close()

	handler := NewAggregationHandler(
		func(path string, token string, body []byte) (*AggregatedResult, error) {
			// Mock fetcher
			switch path {
			case "/alt.feeds.v2.FeedService/GetFeedStats":
				return &AggregatedResult{
					Data:       json.RawMessage(`{"total_feeds": 10}`),
					StatusCode: http.StatusOK,
				}, nil
			case "/alt.feeds.v2.FeedService/GetUnreadCount":
				return &AggregatedResult{
					Data:       json.RawMessage(`{"unread_count": 5}`),
					StatusCode: http.StatusOK,
				}, nil
			}
			return nil, nil
		},
		nil,
	)

	reqBody := `{"queries": ["feed_stats", "unread_count"]}`
	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("X-Alt-Backend-Token", "valid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AggregationResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Contains(t, resp.Results, "feed_stats")
	assert.Contains(t, resp.Results, "unread_count")
}

func TestAggregationHandler_ServeHTTP_InvalidMethod(t *testing.T) {
	handler := NewAggregationHandler(nil, nil)

	req := httptest.NewRequest("GET", "/v1/aggregate", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestAggregationHandler_ServeHTTP_InvalidBody(t *testing.T) {
	handler := NewAggregationHandler(nil, nil)

	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader([]byte(`{invalid`)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAggregationHandler_ServeHTTP_UnknownQuery(t *testing.T) {
	handler := NewAggregationHandler(
		func(path string, token string, body []byte) (*AggregatedResult, error) {
			return nil, nil
		},
		nil,
	)

	reqBody := `{"queries": ["unknown_query"]}`
	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader([]byte(reqBody)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AggregationResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Unknown queries should have error in result
	result, ok := resp.Results["unknown_query"]
	assert.True(t, ok)
	assert.NotNil(t, result.Error)
}

func TestAggregationHandler_ParallelFetch(t *testing.T) {
	var callCount int32
	var maxConcurrent int32
	var currentConcurrent int32

	handler := NewAggregationHandler(
		func(path string, token string, body []byte) (*AggregatedResult, error) {
			current := atomic.AddInt32(&currentConcurrent, 1)
			atomic.AddInt32(&callCount, 1)

			// Track max concurrency
			for {
				max := atomic.LoadInt32(&maxConcurrent)
				if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
					break
				}
			}

			time.Sleep(50 * time.Millisecond) // Simulate work
			atomic.AddInt32(&currentConcurrent, -1)

			return &AggregatedResult{
				Data:       json.RawMessage(`{}`),
				StatusCode: http.StatusOK,
			}, nil
		},
		nil,
	)

	reqBody := `{"queries": ["feed_stats", "unread_count", "trends"]}`
	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader([]byte(reqBody)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// All queries should have been fetched
	assert.Equal(t, int32(3), callCount)
	// Requests should have been parallel (max concurrent > 1)
	assert.GreaterOrEqual(t, maxConcurrent, int32(2))
}

func TestQueryToEndpoint(t *testing.T) {
	tests := []struct {
		query    string
		expected string
		ok       bool
	}{
		{"feed_stats", "/alt.feeds.v2.FeedService/GetFeedStats", true},
		{"unread_count", "/alt.feeds.v2.FeedService/GetUnreadCount", true},
		{"detailed_feed_stats", "/alt.feeds.v2.FeedService/GetDetailedFeedStats", true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			endpoint, ok := QueryToEndpoint(tt.query)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, endpoint)
			}
		})
	}
}

func TestAggregationResponse_JSON(t *testing.T) {
	resp := AggregationResponse{
		Results: map[string]*AggregatedResult{
			"feed_stats": {
				Data:       json.RawMessage(`{"total": 10}`),
				StatusCode: http.StatusOK,
			},
			"unread_count": {
				Data:       json.RawMessage(`{"count": 5}`),
				StatusCode: http.StatusOK,
			},
		},
	}

	jsonBytes, err := json.Marshal(resp)
	require.NoError(t, err)

	assert.Contains(t, string(jsonBytes), `"feed_stats"`)
	assert.Contains(t, string(jsonBytes), `"unread_count"`)
	assert.Contains(t, string(jsonBytes), `"total":10`)
	assert.Contains(t, string(jsonBytes), `"count":5`)
}

func TestAggregatedResult_WithError(t *testing.T) {
	result := &AggregatedResult{
		Error:      "backend unavailable",
		StatusCode: http.StatusServiceUnavailable,
	}

	jsonBytes, err := json.Marshal(result)
	require.NoError(t, err)

	assert.Contains(t, string(jsonBytes), `"error":"backend unavailable"`)
	assert.Contains(t, string(jsonBytes), `"status_code":503`)
}

func TestAggregationHandler_PartialFailure(t *testing.T) {
	handler := NewAggregationHandler(
		func(path string, token string, body []byte) (*AggregatedResult, error) {
			switch path {
			case "/alt.feeds.v2.FeedService/GetFeedStats":
				return &AggregatedResult{
					Data:       json.RawMessage(`{"total": 10}`),
					StatusCode: http.StatusOK,
				}, nil
			case "/alt.feeds.v2.FeedService/GetUnreadCount":
				return &AggregatedResult{
					Error:      "service unavailable",
					StatusCode: http.StatusServiceUnavailable,
				}, nil
			}
			return nil, nil
		},
		nil,
	)

	reqBody := `{"queries": ["feed_stats", "unread_count"]}`
	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader([]byte(reqBody)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should still return 200, with partial results
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AggregationResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Successful query
	feedStats := resp.Results["feed_stats"]
	assert.Equal(t, http.StatusOK, feedStats.StatusCode)

	// Failed query
	unreadCount := resp.Results["unread_count"]
	assert.Equal(t, http.StatusServiceUnavailable, unreadCount.StatusCode)
	assert.Equal(t, "service unavailable", unreadCount.Error)
}

func TestAggregationHandler_ReadBody(t *testing.T) {
	var receivedBody []byte

	handler := NewAggregationHandler(
		func(path string, token string, body []byte) (*AggregatedResult, error) {
			receivedBody = body
			return &AggregatedResult{
				Data:       json.RawMessage(`{}`),
				StatusCode: http.StatusOK,
			}, nil
		},
		nil,
	)

	reqBody := `{"queries": ["feed_stats"]}`
	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader([]byte(reqBody)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// The handler should have read the body
	assert.NotNil(t, receivedBody)
}

func TestAggregationHandler_TokenForwarding(t *testing.T) {
	var receivedToken string

	handler := NewAggregationHandler(
		func(path string, token string, body []byte) (*AggregatedResult, error) {
			receivedToken = token
			return &AggregatedResult{
				Data:       json.RawMessage(`{}`),
				StatusCode: http.StatusOK,
			}, nil
		},
		nil,
	)

	reqBody := `{"queries": ["feed_stats"]}`
	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("X-Alt-Backend-Token", "test-token-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, "test-token-123", receivedToken)
}

func TestMaxQueries(t *testing.T) {
	handler := NewAggregationHandler(
		func(path string, token string, body []byte) (*AggregatedResult, error) {
			return &AggregatedResult{
				Data:       json.RawMessage(`{}`),
				StatusCode: http.StatusOK,
			}, nil
		},
		nil,
	)

	// Request with too many queries
	queries := make([]string, MaxQueriesPerRequest+1)
	for i := range queries {
		queries[i] = "feed_stats"
	}

	reqBody, _ := json.Marshal(AggregationRequest{Queries: queries})
	req := httptest.NewRequest("POST", "/v1/aggregate", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	body, _ := io.ReadAll(rec.Body)
	assert.Contains(t, string(body), "too many queries")
}
