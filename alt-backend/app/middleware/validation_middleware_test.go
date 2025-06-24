package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestValidationMiddleware_FeedRegistration(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedStatus int
		shouldCallNext bool
	}{
		{
			name:           "valid feed registration",
			method:         "POST",
			path:           "/v1/rss-feed-link/register",
			body:           `{"url": "https://example.com/feed.xml"}`,
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "invalid feed registration - missing URL",
			method:         "POST",
			path:           "/v1/rss-feed-link/register",
			body:           `{"other_field": "value"}`,
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "invalid feed registration - empty URL",
			method:         "POST",
			path:           "/v1/rss-feed-link/register",
			body:           `{"url": ""}`,
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "invalid feed registration - malformed JSON",
			method:         "POST",
			path:           "/v1/rss-feed-link/register",
			body:           `{"url": "https://example.com/feed.xml"`,
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "valid feed registration - different path structure",
			method:         "POST",
			path:           "/api/v1/rss-feed-link/register",
			body:           `{"url": "https://example.com/feed.xml"}`,
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "no validation for GET request",
			method:         "GET",
			path:           "/v1/rss-feed-link/register",
			body:           "",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "no validation for different endpoint",
			method:         "POST",
			path:           "/v1/other/endpoint",
			body:           `{"invalid": "data"}`,
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			
			nextCalled := false
			next := func(c echo.Context) error {
				nextCalled = true
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			}

			middleware := ValidationMiddleware()
			handler := middleware(next)

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath(tt.path)

			err := handler(c)

			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					if he.Code != tt.expectedStatus {
						t.Errorf("Expected status %d, got %d", tt.expectedStatus, he.Code)
					}
				} else {
					t.Errorf("Unexpected error: %v", err)
				}
			} else {
				if rec.Code != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
				}
			}

			if nextCalled != tt.shouldCallNext {
				t.Errorf("Expected next called: %v, got: %v", tt.shouldCallNext, nextCalled)
			}
		})
	}
}

func TestValidationMiddleware_FeedSearch(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedStatus int
		shouldCallNext bool
	}{
		{
			name:           "valid feed search",
			method:         "POST",
			path:           "/v1/feeds/search",
			body:           `{"query": "golang programming"}`,
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "invalid feed search - missing query",
			method:         "POST",
			path:           "/v1/feeds/search",
			body:           `{"other_field": "value"}`,
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "invalid feed search - empty query",
			method:         "POST",
			path:           "/v1/feeds/search",
			body:           `{"query": ""}`,
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "invalid feed search - query too long",
			method:         "POST",
			path:           "/v1/feeds/search",
			body:           `{"query": "` + strings.Repeat("a", 1001) + `"}`,
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			
			nextCalled := false
			next := func(c echo.Context) error {
				nextCalled = true
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			}

			middleware := ValidationMiddleware()
			handler := middleware(next)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath(tt.path)

			err := handler(c)

			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					if he.Code != tt.expectedStatus {
						t.Errorf("Expected status %d, got %d", tt.expectedStatus, he.Code)
					}
				} else {
					t.Errorf("Unexpected error: %v", err)
				}
			} else {
				if rec.Code != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
				}
			}

			if nextCalled != tt.shouldCallNext {
				t.Errorf("Expected next called: %v, got: %v", tt.shouldCallNext, nextCalled)
			}
		})
	}
}

func TestValidationMiddleware_ArticleSearch(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		query          string
		expectedStatus int
		shouldCallNext bool
	}{
		{
			name:           "valid article search",
			method:         "GET",
			path:           "/v1/articles/search",
			query:          "q=golang",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "invalid article search - missing q parameter",
			method:         "GET",
			path:           "/v1/articles/search",
			query:          "other=value",
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "invalid article search - empty q parameter",
			method:         "GET",
			path:           "/v1/articles/search",
			query:          "q=",
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "invalid article search - q parameter too long",
			method:         "GET",
			path:           "/v1/articles/search",
			query:          "q=" + strings.Repeat("a", 501),
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			
			nextCalled := false
			next := func(c echo.Context) error {
				nextCalled = true
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			}

			middleware := ValidationMiddleware()
			handler := middleware(next)

			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}

			req := httptest.NewRequest(tt.method, url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath(tt.path)

			err := handler(c)

			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					if he.Code != tt.expectedStatus {
						t.Errorf("Expected status %d, got %d", tt.expectedStatus, he.Code)
					}
				} else {
					t.Errorf("Unexpected error: %v", err)
				}
			} else {
				if rec.Code != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
				}
			}

			if nextCalled != tt.shouldCallNext {
				t.Errorf("Expected next called: %v, got: %v", tt.shouldCallNext, nextCalled)
			}
		})
	}
}

func TestValidationMiddleware_Pagination(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		query          string
		expectedStatus int
		shouldCallNext bool
	}{
		{
			name:           "valid pagination with limit",
			method:         "GET",
			path:           "/v1/feeds/fetch/cursor",
			query:          "limit=20",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "valid pagination with cursor",
			method:         "GET",
			path:           "/v1/feeds/fetch/cursor",
			query:          "cursor=2023-01-01T00:00:00Z",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "invalid pagination - negative limit",
			method:         "GET",
			path:           "/v1/feeds/fetch/cursor",
			query:          "limit=-1",
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "invalid pagination - invalid cursor",
			method:         "GET",
			path:           "/v1/feeds/fetch/cursor",
			query:          "cursor=invalid-timestamp",
			expectedStatus: http.StatusBadRequest,
			shouldCallNext: false,
		},
		{
			name:           "no validation for non-paginated endpoint",
			method:         "GET",
			path:           "/v1/feeds/fetch/single",
			query:          "limit=-1",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			
			nextCalled := false
			next := func(c echo.Context) error {
				nextCalled = true
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			}

			middleware := ValidationMiddleware()
			handler := middleware(next)

			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}

			req := httptest.NewRequest(tt.method, url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath(tt.path)

			err := handler(c)

			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					if he.Code != tt.expectedStatus {
						t.Errorf("Expected status %d, got %d", tt.expectedStatus, he.Code)
					}
				} else {
					t.Errorf("Unexpected error: %v", err)
				}
			} else {
				if rec.Code != tt.expectedStatus {
					t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
				}
			}

			if nextCalled != tt.shouldCallNext {
				t.Errorf("Expected next called: %v, got: %v", tt.shouldCallNext, nextCalled)
			}
		})
	}
}