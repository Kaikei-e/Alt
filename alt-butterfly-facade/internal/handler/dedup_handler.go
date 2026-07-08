// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// DedupResult holds the result of a deduplicated request.
type DedupResult struct {
	Body       []byte
	StatusCode int
	Headers    http.Header
}

// Clone creates a deep copy of the result.
func (r *DedupResult) Clone() *DedupResult {
	if r == nil {
		return nil
	}

	bodyCopy := make([]byte, len(r.Body))
	copy(bodyCopy, r.Body)

	headersCopy := make(http.Header)
	for k, v := range r.Headers {
		headersCopy[k] = append([]string{}, v...)
	}

	return &DedupResult{
		Body:       bodyCopy,
		StatusCode: r.StatusCode,
		Headers:    headersCopy,
	}
}

// RequestDeduplicator deduplicates identical concurrent requests.
//
// Broadcasting the single in-flight result to every waiter is delegated to
// singleflight.Group, which has no cap on the number of waiters (unlike the
// previous hand-rolled buffered-channel fan-out, which silently dropped the
// result for the 11th+ concurrent waiter).
type RequestDeduplicator struct {
	mu       sync.Mutex
	group    singleflight.Group
	pending  map[string]struct{}
	window   time.Duration
	lastUsed map[string]time.Time
}

// NewRequestDeduplicator creates a new deduplicator with the given window.
// Requests within the window will be deduplicated.
func NewRequestDeduplicator(window time.Duration) *RequestDeduplicator {
	d := &RequestDeduplicator{
		pending:  make(map[string]struct{}),
		lastUsed: make(map[string]time.Time),
		window:   window,
	}
	go d.cleanupLoop()
	return d
}

// cleanupLoop periodically evicts stale lastUsed entries so the map doesn't
// grow unboundedly over the process lifetime (mirrors auth-hub's
// middleware.RateLimiter.cleanupLoop).
func (d *RequestDeduplicator) cleanupLoop() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		d.Cleanup()
	}
}

// Do executes the function, deduplicating concurrent requests with the same key.
// If another request with the same key is in progress, the caller waits for
// that request to complete and receives the same result via singleflight.
func (d *RequestDeduplicator) Do(key string, fn func() (*DedupResult, error)) (*DedupResult, error) {
	d.mu.Lock()
	d.pending[key] = struct{}{}
	d.lastUsed[key] = time.Now()
	d.mu.Unlock()

	v, err, _ := d.group.Do(key, func() (any, error) {
		return fn()
	})

	d.mu.Lock()
	delete(d.pending, key)
	d.mu.Unlock()

	if err != nil {
		return nil, err
	}

	result, _ := v.(*DedupResult)
	if result == nil {
		return nil, nil
	}
	return result.Clone(), nil
}

// Size returns the number of requests currently in flight.
func (d *RequestDeduplicator) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// Cleanup removes expired entries from the lastUsed map.
func (d *RequestDeduplicator) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for key, lastUsed := range d.lastUsed {
		if now.Sub(lastUsed) > d.window*2 {
			delete(d.lastUsed, key)
		}
	}
}

// BuildDedupKey creates a deduplication key from request attributes.
func BuildDedupKey(userID, method, path string, body []byte) string {
	hash := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(hash[:8])
	return userID + ":" + method + ":" + path + ":" + bodyHash
}

// UserIDExtractor extracts a user ID from a request.
type UserIDExtractor func(r *http.Request) string

// DedupMiddleware creates middleware that deduplicates requests.
// The userIDExtractor should return the user ID, or empty string to skip deduplication.
func DedupMiddleware(dedup *RequestDeduplicator, userIDExtractor UserIDExtractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract user ID
			userID := userIDExtractor(r)
			if userID == "" {
				// Skip deduplication
				next.ServeHTTP(w, r)
				return
			}

			// Read and store the body
			var body []byte
			if r.Body != nil {
				body, _ = io.ReadAll(r.Body)
				r.Body.Close()
			}

			// Build dedup key
			key := BuildDedupKey(userID, r.Method, r.URL.Path, body)

			// Execute with deduplication
			result, err := dedup.Do(key, func() (*DedupResult, error) {
				// Create a new request with the body
				newReq := CreateDedupRequest(r, body)

				// Capture the response
				rec := &responseRecorder{
					ResponseWriter: w,
					body:           &bytes.Buffer{},
					statusCode:     http.StatusOK,
				}

				next.ServeHTTP(rec, newReq)

				return &DedupResult{
					Body:       rec.body.Bytes(),
					StatusCode: rec.statusCode,
					Headers:    rec.Header().Clone(),
				}, nil
			})

			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if result != nil {
				// Write the cached response
				for k, v := range result.Headers {
					for _, vv := range v {
						w.Header().Add(k, vv)
					}
				}
				w.WriteHeader(result.StatusCode)
				w.Write(result.Body)
			}
		})
	}
}

// CreateDedupRequest creates a new request with the given body.
func CreateDedupRequest(original *http.Request, body []byte) *http.Request {
	newReq := original.Clone(original.Context())
	newReq.Body = io.NopCloser(bytes.NewReader(body))
	newReq.ContentLength = int64(len(body))
	return newReq
}

// responseRecorder captures the response for deduplication.
type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
	written    bool
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	if !r.written {
		r.statusCode = statusCode
		r.ResponseWriter.WriteHeader(statusCode)
		r.written = true
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	// ResponseWriter.Write passthrough on a recording middleware. The bytes
	// come from the upstream alt-backend response; escaping, if needed, is
	// the responsibility of the handler that originally produced them.
	return r.ResponseWriter.Write(b) //nolint:gosec // codeql[go/reflected-xss]
}
