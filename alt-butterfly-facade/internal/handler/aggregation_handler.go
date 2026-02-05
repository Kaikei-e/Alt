// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"alt-butterfly-facade/internal/middleware"
)

// MaxQueriesPerRequest is the maximum number of queries allowed in a single aggregation request.
const MaxQueriesPerRequest = 10

// AggregationRequest represents a request to aggregate multiple queries.
type AggregationRequest struct {
	Queries []string `json:"queries"`
}

// Validate validates the aggregation request.
func (r *AggregationRequest) Validate() error {
	if len(r.Queries) == 0 {
		return errors.New("queries cannot be empty")
	}
	if len(r.Queries) > MaxQueriesPerRequest {
		return errors.New("too many queries")
	}
	return nil
}

// AggregatedResult represents the result of a single query in an aggregation.
type AggregatedResult struct {
	Data       json.RawMessage `json:"data,omitempty"`
	Error      string          `json:"error,omitempty"`
	StatusCode int             `json:"status_code"`
}

// AggregationResponse represents the response from an aggregation request.
type AggregationResponse struct {
	Results map[string]*AggregatedResult `json:"results"`
}

// QueryFetcher is a function that fetches data for a query.
type QueryFetcher func(path string, token string, body []byte) (*AggregatedResult, error)

// queryEndpointMapping maps query names to backend endpoints.
var queryEndpointMapping = map[string]string{
	"feed_stats":          "/alt.feeds.v2.FeedService/GetFeedStats",
	"unread_count":        "/alt.feeds.v2.FeedService/GetUnreadCount",
	"detailed_feed_stats": "/alt.feeds.v2.FeedService/GetDetailedFeedStats",
	"trends":              "/alt.feeds.v2.FeedService/GetTrends",
}

// QueryToEndpoint converts a query name to a backend endpoint.
func QueryToEndpoint(query string) (string, bool) {
	endpoint, ok := queryEndpointMapping[query]
	return endpoint, ok
}

// AggregationHandler handles aggregation requests.
type AggregationHandler struct {
	fetcher QueryFetcher
	logger  *slog.Logger
}

// NewAggregationHandler creates a new aggregation handler.
func NewAggregationHandler(fetcher QueryFetcher, logger *slog.Logger) *AggregationHandler {
	return &AggregationHandler{
		fetcher: fetcher,
		logger:  logger,
	}
}

// ServeHTTP handles aggregation requests.
func (h *AggregationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read and parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req AggregationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract token
	token := r.Header.Get(middleware.BackendTokenHeader)

	// Fetch all queries in parallel
	results := h.fetchAll(req.Queries, token, body)

	// Build response
	resp := AggregationResponse{
		Results: results,
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// fetchAll fetches all queries in parallel.
func (h *AggregationHandler) fetchAll(queries []string, token string, body []byte) map[string]*AggregatedResult {
	results := make(map[string]*AggregatedResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, query := range queries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()

			result := h.fetchSingle(q, token, body)

			mu.Lock()
			results[q] = result
			mu.Unlock()
		}(query)
	}

	wg.Wait()
	return results
}

// fetchSingle fetches a single query.
func (h *AggregationHandler) fetchSingle(query string, token string, body []byte) *AggregatedResult {
	endpoint, ok := QueryToEndpoint(query)
	if !ok {
		return &AggregatedResult{
			Error:      "unknown query: " + query,
			StatusCode: http.StatusBadRequest,
		}
	}

	if h.fetcher == nil {
		return &AggregatedResult{
			Error:      "fetcher not configured",
			StatusCode: http.StatusInternalServerError,
		}
	}

	result, err := h.fetcher(endpoint, token, body)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("fetch failed", "query", query, "error", err)
		}
		return &AggregatedResult{
			Error:      err.Error(),
			StatusCode: http.StatusBadGateway,
		}
	}

	if result == nil {
		return &AggregatedResult{
			Error:      "no result from fetcher",
			StatusCode: http.StatusInternalServerError,
		}
	}

	return result
}
