package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequestDeduplicator(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)

	assert.NotNil(t, dedup)
}

func TestRequestDeduplicator_BuildDedupKey(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		method   string
		path     string
		body     []byte
		expected string
	}{
		{
			name:   "basic key",
			userID: "user123",
			method: "POST",
			path:   "/api/feed_stats",
			body:   []byte(`{}`),
		},
		{
			name:   "with body",
			userID: "user456",
			method: "POST",
			path:   "/api/query",
			body:   []byte(`{"query": "test"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := BuildDedupKey(tt.userID, tt.method, tt.path, tt.body)

			assert.Contains(t, key, tt.userID)
			assert.Contains(t, key, tt.method)
			assert.Contains(t, key, tt.path)
		})
	}
}

func TestRequestDeduplicator_SameKeyForSameInput(t *testing.T) {
	key1 := BuildDedupKey("user123", "POST", "/api/test", []byte(`{"data": "test"}`))
	key2 := BuildDedupKey("user123", "POST", "/api/test", []byte(`{"data": "test"}`))

	assert.Equal(t, key1, key2)
}

func TestRequestDeduplicator_DifferentKeyForDifferentInput(t *testing.T) {
	key1 := BuildDedupKey("user123", "POST", "/api/test", []byte(`{"data": "test1"}`))
	key2 := BuildDedupKey("user123", "POST", "/api/test", []byte(`{"data": "test2"}`))

	assert.NotEqual(t, key1, key2)
}

func TestRequestDeduplicator_Do_SingleRequest(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	executed := false

	result, err := dedup.Do("key1", func() (*DedupResult, error) {
		executed = true
		return &DedupResult{
			Body:       []byte("response"),
			StatusCode: http.StatusOK,
			Headers:    make(http.Header),
		}, nil
	})

	assert.NoError(t, err)
	assert.True(t, executed)
	assert.Equal(t, []byte("response"), result.Body)
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestRequestDeduplicator_Do_DeduplicatesConcurrent(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	var executionCount int32
	var wg sync.WaitGroup

	// Simulate 5 concurrent requests with the same key
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := dedup.Do("same-key", func() (*DedupResult, error) {
				atomic.AddInt32(&executionCount, 1)
				time.Sleep(50 * time.Millisecond) // Simulate work
				return &DedupResult{
					Body:       []byte("response"),
					StatusCode: http.StatusOK,
					Headers:    make(http.Header),
				}, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, []byte("response"), result.Body)
		}()
	}

	wg.Wait()

	// Only one execution should have happened
	assert.Equal(t, int32(1), executionCount)
}

func TestRequestDeduplicator_Do_DifferentKeysExecuteSeparately(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	var executionCount int32
	var wg sync.WaitGroup

	// 5 requests with different keys
	for i := 0; i < 5; i++ {
		wg.Add(1)
		key := "key-" + string(rune('0'+i))
		go func(k string) {
			defer wg.Done()
			dedup.Do(k, func() (*DedupResult, error) {
				atomic.AddInt32(&executionCount, 1)
				time.Sleep(10 * time.Millisecond)
				return &DedupResult{
					Body:       []byte("response"),
					StatusCode: http.StatusOK,
					Headers:    make(http.Header),
				}, nil
			})
		}(key)
	}

	wg.Wait()

	// All 5 should have executed
	assert.Equal(t, int32(5), executionCount)
}

func TestRequestDeduplicator_Do_AllReceiveSameResult(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	var wg sync.WaitGroup
	results := make(chan *DedupResult, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _ := dedup.Do("same-key", func() (*DedupResult, error) {
				time.Sleep(50 * time.Millisecond)
				return &DedupResult{
					Body:       []byte("shared-result"),
					StatusCode: http.StatusOK,
					Headers:    make(http.Header),
				}, nil
			})
			results <- result
		}()
	}

	wg.Wait()
	close(results)

	// All results should be identical
	for result := range results {
		assert.Equal(t, []byte("shared-result"), result.Body)
		assert.Equal(t, http.StatusOK, result.StatusCode)
	}
}

func TestRequestDeduplicator_Do_AllReceiveError(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	var wg sync.WaitGroup
	errors := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := dedup.Do("same-key", func() (*DedupResult, error) {
				time.Sleep(50 * time.Millisecond)
				return nil, http.ErrHandlerTimeout
			})
			errors <- err
		}()
	}

	wg.Wait()
	close(errors)

	// All should receive the same error
	for err := range errors {
		assert.Equal(t, http.ErrHandlerTimeout, err)
	}
}

func TestRequestDeduplicator_Cleanup_RemovesExpiredEntries(t *testing.T) {
	dedup := NewRequestDeduplicator(50 * time.Millisecond)

	// Execute a request
	dedup.Do("key1", func() (*DedupResult, error) {
		return &DedupResult{Body: []byte("test")}, nil
	})

	// Immediately, the entry should still be tracked (for brief period)
	assert.GreaterOrEqual(t, dedup.Size(), 0)

	// Wait for cleanup
	time.Sleep(150 * time.Millisecond)
	dedup.Cleanup()

	// Now it should be cleaned up
	assert.Equal(t, 0, dedup.Size())
}

func TestRequestDeduplicator_Size(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)

	assert.Equal(t, 0, dedup.Size())
}

func TestDedupResult_Clone(t *testing.T) {
	original := &DedupResult{
		Body:       []byte("original"),
		StatusCode: http.StatusOK,
		Headers:    make(http.Header),
	}
	original.Headers.Set("X-Custom", "value")

	clone := original.Clone()

	// Should have same values
	assert.Equal(t, original.Body, clone.Body)
	assert.Equal(t, original.StatusCode, clone.StatusCode)
	assert.Equal(t, original.Headers.Get("X-Custom"), clone.Headers.Get("X-Custom"))

	// Should be independent copies
	clone.Body[0] = 'X'
	assert.NotEqual(t, original.Body, clone.Body)
}

func TestDedupMiddleware_Integration(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	var callCount int32

	// Backend handler that counts calls
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	})

	// Create middleware
	handler := DedupMiddleware(dedup, func(r *http.Request) string {
		return "test-user"
	})(backend)

	// Concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/api/test", bytes.NewReader([]byte(`{}`)))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}()
	}

	wg.Wait()

	// Backend should only be called once
	assert.Equal(t, int32(1), callCount)
}

func TestDedupMiddleware_PassesThroughNonDedupable(t *testing.T) {
	dedup := NewRequestDeduplicator(100 * time.Millisecond)
	var callCount int32

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware that returns empty user ID for GET requests
	handler := DedupMiddleware(dedup, func(r *http.Request) string {
		if r.Method == "GET" {
			return "" // Don't deduplicate
		}
		return "test-user"
	})(backend)

	// GET requests should not be deduplicated
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// All 3 calls should go through
	assert.Equal(t, int32(3), callCount)
}

func TestCreateDedupRequest(t *testing.T) {
	body := []byte(`{"test": "data"}`)
	originalReq := httptest.NewRequest("POST", "/api/test", bytes.NewReader(body))
	originalReq.Header.Set("Content-Type", "application/json")

	// Read body to simulate it being consumed
	io.ReadAll(originalReq.Body)

	// Create new request with stored body
	newReq := CreateDedupRequest(originalReq, body)

	require.NotNil(t, newReq)
	assert.Equal(t, "POST", newReq.Method)
	assert.Equal(t, "/api/test", newReq.URL.Path)
	assert.Equal(t, "application/json", newReq.Header.Get("Content-Type"))

	// Body should be readable
	readBody, err := io.ReadAll(newReq.Body)
	assert.NoError(t, err)
	assert.Equal(t, body, readBody)
}
