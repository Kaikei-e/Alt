package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auth-hub/cache"
	"auth-hub/client"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockKratosClient is a mock implementation of the Kratos client
type MockKratosClient struct {
	mock.Mock
}

func (m *MockKratosClient) Whoami(ctx context.Context, cookie string) (*client.Identity, error) {
	args := m.Called(ctx, cookie)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*client.Identity), args.Error(1)
}

func TestNewValidateHandler(t *testing.T) {
	t.Run("creates handler with dependencies", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		handler := NewValidateHandler(mockClient, sessionCache)

		assert.NotNil(t, handler)
	})
}

func TestValidateHandler_Handle(t *testing.T) {
	t.Run("successful validation with cache hit", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Pre-populate cache
		sessionCache.Set("session-123", "user-456", "tenant-789", "user@example.com")

		handler := NewValidateHandler(mockClient, sessionCache)

		// Create request with session cookie
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ory_kratos_session",
			Value: "session-123",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute handler
		err := handler.Handle(c)

		// Assertions
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user-456", rec.Header().Get("X-Alt-User-Id"))
		assert.Equal(t, "tenant-789", rec.Header().Get("X-Alt-Tenant-Id"))
		assert.Equal(t, "user@example.com", rec.Header().Get("X-Alt-User-Email"))

		// Kratos client should not be called on cache hit
		mockClient.AssertNotCalled(t, "Whoami")
	})

	t.Run("successful validation with cache miss", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Mock Kratos client response
		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=valid-session").
			Return(&client.Identity{
				ID:    "user-123",
				Email: "test@example.com",
			}, nil)

		handler := NewValidateHandler(mockClient, sessionCache)

		// Create request
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ory_kratos_session",
			Value: "valid-session",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute handler
		err := handler.Handle(c)

		// Assertions
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user-123", rec.Header().Get("X-Alt-User-Id"))
		assert.Equal(t, "user-123", rec.Header().Get("X-Alt-Tenant-Id"))
		assert.Equal(t, "test@example.com", rec.Header().Get("X-Alt-User-Email"))

		// Kratos client should be called
		mockClient.AssertCalled(t, "Whoami", mock.Anything, "ory_kratos_session=valid-session")

		// Cache should be populated
		entry, found := sessionCache.Get("valid-session")
		assert.True(t, found)
		assert.Equal(t, "user-123", entry.UserID)
	})

	t.Run("missing session cookie returns 401", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)
		handler := NewValidateHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		// No cookie added
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
		assert.Contains(t, httpErr.Message, "session cookie not found")
	})

	t.Run("invalid session returns 401", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Mock Kratos client to return authentication error
		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=invalid-session").
			Return(nil, errors.New("authentication failed: session invalid or expired"))

		handler := NewValidateHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ory_kratos_session",
			Value: "invalid-session",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	})

	t.Run("kratos service error returns 500", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Mock Kratos client to return service error
		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=valid-session").
			Return(nil, errors.New("kratos returned status 500"))

		handler := NewValidateHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ory_kratos_session",
			Value: "valid-session",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
	})

	t.Run("expired cache entry triggers kratos call", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(100 * time.Millisecond)

		// Pre-populate cache with short TTL
		sessionCache.Set("session-expired", "user-old", "tenant-old", "old@example.com")

		// Wait for expiration
		time.Sleep(150 * time.Millisecond)

		// Mock Kratos client for fresh validation
		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=session-expired").
			Return(&client.Identity{
				ID:    "user-new",
				Email: "new@example.com",
			}, nil)

		handler := NewValidateHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ory_kratos_session",
			Value: "session-expired",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user-new", rec.Header().Get("X-Alt-User-Id"))

		// Kratos should be called due to expired cache
		mockClient.AssertCalled(t, "Whoami", mock.Anything, "ory_kratos_session=session-expired")
	})

	t.Run("handles multiple cookies correctly", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=valid-session").
			Return(&client.Identity{
				ID:    "user-789",
				Email: "multi@example.com",
			}, nil)

		handler := NewValidateHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.AddCookie(&http.Cookie{Name: "other_cookie", Value: "other-value"})
		req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: "valid-session"})
		req.AddCookie(&http.Cookie{Name: "another_cookie", Value: "another-value"})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user-789", rec.Header().Get("X-Alt-User-Id"))
	})
}
