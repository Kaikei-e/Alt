package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCSRFTokenUsecase is a mock implementation of CSRF token usecase
type MockCSRFTokenUsecase struct {
	mock.Mock
}

func (m *MockCSRFTokenUsecase) GenerateToken(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockCSRFTokenUsecase) ValidateToken(ctx context.Context, token string) (bool, error) {
	args := m.Called(ctx, token)
	return args.Bool(0), args.Error(1)
}

func TestCSRFMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		csrfToken      string
		headerToken    string
		expectedStatus int
		setupMock      func(*MockCSRFTokenUsecase)
	}{
		{
			name:           "GET request should pass without CSRF validation",
			method:         "GET",
			path:           "/v1/feeds/fetch/list",
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockCSRFTokenUsecase) {
				// No mock setup needed for GET requests
			},
		},
		{
			name:           "POST request without CSRF token should fail",
			method:         "POST",
			path:           "/v1/feeds/read",
			expectedStatus: http.StatusForbidden,
			setupMock: func(m *MockCSRFTokenUsecase) {
				// No mock setup needed as it should fail before validation
			},
		},
		{
			name:           "POST request with invalid CSRF token should fail",
			method:         "POST",
			path:           "/v1/feeds/read",
			headerToken:    "invalid-token",
			expectedStatus: http.StatusForbidden,
			setupMock: func(m *MockCSRFTokenUsecase) {
				m.On("ValidateToken", mock.Anything, "invalid-token").Return(false, nil)
			},
		},
		{
			name:           "POST request with valid CSRF token should pass",
			method:         "POST",
			path:           "/v1/feeds/read",
			headerToken:    "valid-token",
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockCSRFTokenUsecase) {
				m.On("ValidateToken", mock.Anything, "valid-token").Return(true, nil)
			},
		},
		{
			name:           "POST request to rss-feed-link/register with valid token should pass",
			method:         "POST",
			path:           "/v1/rss-feed-link/register",
			headerToken:    "valid-token",
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockCSRFTokenUsecase) {
				m.On("ValidateToken", mock.Anything, "valid-token").Return(true, nil)
			},
		},
		{
			name:           "POST request to feeds/register/favorite with valid token should pass",
			method:         "POST",
			path:           "/v1/feeds/register/favorite",
			headerToken:    "valid-token",
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockCSRFTokenUsecase) {
				m.On("ValidateToken", mock.Anything, "valid-token").Return(true, nil)
			},
		},
		{
			name:           "Health check endpoint should be exempt from CSRF protection",
			method:         "GET",
			path:           "/v1/health",
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockCSRFTokenUsecase) {
				// No mock setup needed for health check
			},
		},
		{
			name:           "CSP report endpoint should be exempt from CSRF protection",
			method:         "POST",
			path:           "/security/csp-report",
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockCSRFTokenUsecase) {
				// No mock setup needed for CSP report endpoint
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Echo instance
			e := echo.New()

			// Setup mock usecase
			mockUsecase := &MockCSRFTokenUsecase{}
			tt.setupMock(mockUsecase)

			// Create CSRF middleware
			middleware := CSRFMiddleware(mockUsecase)

			// Create test handler
			handler := func(c echo.Context) error {
				return c.JSON(http.StatusOK, map[string]string{"message": "success"})
			}

			// Create request
			var req *http.Request
			if tt.method == "POST" {
				body := `{"url": "https://example.com/feed.xml"}`
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}

			// Add CSRF token to header if provided
			if tt.headerToken != "" {
				req.Header.Set("X-CSRF-Token", tt.headerToken)
			}

			// Create response recorder
			rec := httptest.NewRecorder()

			// Create Echo context
			c := e.NewContext(req, rec)
			c.SetPath(tt.path)

			// Apply middleware and handler
			middlewareHandler := middleware(handler)
			err := middlewareHandler(c)

			// Check response
			if tt.expectedStatus == http.StatusOK {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)
			} else {
				// For error cases, Echo middleware should return an error
				assert.Error(t, err)
				if httpErr, ok := err.(*echo.HTTPError); ok {
					assert.Equal(t, tt.expectedStatus, httpErr.Code)
				}
			}

			// Verify mock expectations
			mockUsecase.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenGeneration(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		setupMock      func(*MockCSRFTokenUsecase)
	}{
		{
			name:           "GET /v1/csrf-token should generate and return token",
			method:         "GET",
			path:           "/v1/csrf-token",
			expectedStatus: http.StatusOK,
			setupMock: func(m *MockCSRFTokenUsecase) {
				m.On("GenerateToken", mock.Anything).Return("generated-token", nil)
			},
		},
		{
			name:           "GET /v1/csrf-token should handle generation error",
			method:         "GET",
			path:           "/v1/csrf-token",
			expectedStatus: http.StatusInternalServerError,
			setupMock: func(m *MockCSRFTokenUsecase) {
				m.On("GenerateToken", mock.Anything).Return("", assert.AnError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Echo instance
			e := echo.New()

			// Setup mock usecase
			mockUsecase := &MockCSRFTokenUsecase{}
			tt.setupMock(mockUsecase)

			// Create CSRF token handler
			handler := CSRFTokenHandler(mockUsecase)

			// Create request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			// Create Echo context
			c := e.NewContext(req, rec)
			c.SetPath(tt.path)

			// Apply handler
			err := handler(c)

			// Check response
			if tt.expectedStatus == http.StatusOK {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)

				// Verify response contains token
				assert.Contains(t, rec.Body.String(), "csrf_token")
			} else {
				// For error cases, check the error
				assert.Error(t, err)
				if httpErr, ok := err.(*echo.HTTPError); ok {
					assert.Equal(t, tt.expectedStatus, httpErr.Code)
				}
			}

			// Verify mock expectations
			mockUsecase.AssertExpectations(t)
		})
	}
}

func TestIsCSRFProtectedEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{
			name:   "GET request should not be protected",
			method: "GET",
			path:   "/v1/feeds/fetch/list",
			want:   false,
		},
		{
			name:   "POST /v1/feeds/read should be protected",
			method: "POST",
			path:   "/v1/feeds/read",
			want:   true,
		},
		{
			name:   "POST /v1/feeds/search should be protected",
			method: "POST",
			path:   "/v1/feeds/search",
			want:   true,
		},
		{
			name:   "POST /v1/feeds/fetch/details should be protected",
			method: "POST",
			path:   "/v1/feeds/fetch/details",
			want:   true,
		},
		{
			name:   "POST /v1/feeds/tags should be protected",
			method: "POST",
			path:   "/v1/feeds/tags",
			want:   true,
		},
		{
			name:   "POST /v1/rss-feed-link/register should be protected",
			method: "POST",
			path:   "/v1/rss-feed-link/register",
			want:   true,
		},
		{
			name:   "POST /v1/feeds/register/favorite should be protected",
			method: "POST",
			path:   "/v1/feeds/register/favorite",
			want:   true,
		},
		{
			name:   "GET /v1/health should not be protected",
			method: "GET",
			path:   "/v1/health",
			want:   false,
		},
		{
			name:   "GET /v1/csrf-token should not be protected",
			method: "GET",
			path:   "/v1/csrf-token",
			want:   false,
		},
		{
			name:   "POST /security/csp-report should not be protected",
			method: "POST",
			path:   "/security/csp-report",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCSRFProtectedEndpoint(tt.method, tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}
