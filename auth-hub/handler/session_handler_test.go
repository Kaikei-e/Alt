package handler

import (
	"encoding/json"
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

func TestNewSessionHandler(t *testing.T) {
	t.Run("creates handler with dependencies", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		handler := NewSessionHandler(mockClient, sessionCache)

		assert.NotNil(t, handler)
	})
}

func TestSessionHandler_Handle(t *testing.T) {
	t.Run("successful session with cache hit returns JSON", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Pre-populate cache
		sessionCache.Set("session-123", "user-456", "tenant-789", "user@example.com")

		handler := NewSessionHandler(mockClient, sessionCache)

		// Create request with session cookie
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
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
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

		// Parse JSON response
		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["ok"].(bool))

		// Check user object
		user := response["user"].(map[string]interface{})
		assert.Equal(t, "user-456", user["id"])
		assert.Equal(t, "tenant-789", user["tenantId"])
		assert.Equal(t, "user@example.com", user["email"])
		assert.Equal(t, "user", user["role"])

		// Check session object
		session := response["session"].(map[string]interface{})
		assert.Equal(t, "user-456", session["id"])

		// Kratos client should not be called on cache hit
		mockClient.AssertNotCalled(t, "Whoami")
	})

	t.Run("successful session with cache miss calls Kratos", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Mock Kratos client response
		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=valid-session").
			Return(&client.Identity{
				ID:    "user-123",
				Email: "test@example.com",
			}, nil)

		handler := NewSessionHandler(mockClient, sessionCache)

		// Create request
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
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

		// Parse JSON response
		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response["ok"].(bool))

		user := response["user"].(map[string]interface{})
		assert.Equal(t, "user-123", user["id"])
		assert.Equal(t, "user-123", user["tenantId"])
		assert.Equal(t, "test@example.com", user["email"])

		// Kratos client should be called
		mockClient.AssertCalled(t, "Whoami", mock.Anything, "ory_kratos_session=valid-session")

		// Cache should be populated
		entry, found := sessionCache.Get("valid-session")
		assert.True(t, found)
		assert.Equal(t, "user-123", entry.UserID)
	})

	t.Run("missing session cookie returns 401 JSON error", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)
		handler := NewSessionHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
		// No cookie added
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	})

	t.Run("invalid session returns 401 JSON error", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Mock Kratos client to return authentication error
		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=invalid-session").
			Return(nil, errors.New("authentication failed: session invalid or expired"))

		handler := NewSessionHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
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

	t.Run("kratos service error returns 502", func(t *testing.T) {
		mockClient := new(MockKratosClient)
		sessionCache := cache.NewSessionCache(5 * time.Minute)

		// Mock Kratos client to return service error
		mockClient.On("Whoami", mock.Anything, "ory_kratos_session=valid-session").
			Return(nil, errors.New("kratos returned status 500"))

		handler := NewSessionHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
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
		assert.Equal(t, http.StatusBadGateway, httpErr.Code)
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

		handler := NewSessionHandler(mockClient, sessionCache)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ory_kratos_session",
			Value: "session-expired",
		})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.Handle(c)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Parse response
		var response map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)

		user := response["user"].(map[string]interface{})
		assert.Equal(t, "user-new", user["id"])

		// Kratos should be called due to expired cache
		mockClient.AssertCalled(t, "Whoami", mock.Anything, "ory_kratos_session=session-expired")
	})
}
