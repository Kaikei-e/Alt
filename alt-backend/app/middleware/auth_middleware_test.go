package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"alt/config"
	"alt/domain"
	"alt/mocks"
)

func TestAuthMiddleware_Middleware(t *testing.T) {
	// Setup mock auth server that returns valid JSON
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		response := ValidateOKResponse{
			Valid:      true,
			SessionID:  "sess-123",
			IdentityID: "id-456",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := mocks.NewMockAuthPort(ctrl)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	cfg := &config.Config{
		Auth: config.AuthConfig{
			ServiceURL: mockServer.URL,
			ValidateEmpty200OK: false,
		},
	}
	
	m := NewAuthMiddleware(mockAuth, logger, "http://kratos.test:4433", cfg)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Cookie", "ory_kratos_session=valid")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := m.RequireAuth()(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	err := handler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, c.Get("auth.valid").(bool))
}

func TestAuthMiddleware_Middleware_Invalid(t *testing.T) {
	// Setup mock auth server that returns 401
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		w.Write([]byte(`{"message": "invalid session"}`))
	}))
	defer mockServer.Close()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := mocks.NewMockAuthPort(ctrl)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	cfg := &config.Config{
		Auth: config.AuthConfig{
			ServiceURL: mockServer.URL,
			ValidateEmpty200OK: false,
		},
	}
	
	m := NewAuthMiddleware(mockAuth, logger, "http://kratos.test:4433", cfg)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Cookie", "ory_kratos_session=invalid")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := m.RequireAuth()(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	err := handler(c)
	assert.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestAuthMiddleware_OptionalAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := mocks.NewMockAuthPort(ctrl)
	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		SessionID: "valid",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	mockAuth.EXPECT().
		ValidateSessionWithCookie(gomock.Any(), "ory_kratos_session=valid").
		Return(userCtx, nil)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &config.Config{
		Auth: config.AuthConfig{
			ServiceURL: "http://auth-service.test:8080",
			ValidateEmpty200OK: false,
		},
	}
	m := NewAuthMiddleware(mockAuth, logger, "http://kratos.test:4433", cfg)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: "valid"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := m.OptionalAuth()(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	err := handler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// Contract consumer tests for auth-service validation
func TestAuthMiddleware_DirectValidation(t *testing.T) {
	tests := []struct {
		name              string
		mockAuthResponse  string
		mockStatusCode    int
		featureFlag       bool
		expectedValid     bool
		expectError       bool
	}{
		{
			name:           "200 with valid JSON",
			mockAuthResponse: `{"valid": true, "session_id": "sess-123", "identity_id": "id-456"}`,
			mockStatusCode:   200,
			featureFlag:      false,
			expectedValid:    true,
			expectError:      false,
		},
		{
			name:           "200 with empty body, feature flag disabled",
			mockAuthResponse: "",
			mockStatusCode:   200,
			featureFlag:      false,
			expectedValid:    false,
			expectError:      false,
		},
		{
			name:           "200 with empty body, feature flag enabled",
			mockAuthResponse: "",
			mockStatusCode:   200,
			featureFlag:      true,
			expectedValid:    true,
			expectError:      false,
		},
		{
			name:           "401 unauthorized",
			mockAuthResponse: `{"message": "invalid session"}`,
			mockStatusCode:   401,
			featureFlag:      false,
			expectedValid:    false,
			expectError:      false,
		},
		{
			name:           "500 server error",
			mockAuthResponse: `{"error": "internal error"}`,
			mockStatusCode:   500,
			featureFlag:      false,
			expectedValid:    false,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock auth server
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify X-Request-Id header propagation
				assert.NotEmpty(t, r.Header.Get("X-Request-Id"))
				
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockAuthResponse != "" {
					w.Write([]byte(tt.mockAuthResponse))
				}
			}))
			defer mockServer.Close()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAuth := mocks.NewMockAuthPort(ctrl)
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			
			cfg := &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: mockServer.URL,
					ValidateEmpty200OK: tt.featureFlag,
				},
			}
			
			m := NewAuthMiddleware(mockAuth, logger, "http://kratos.test:4433", cfg)

			// Create test request with session cookie
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Cookie", "ory_kratos_session=test-session")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Execute middleware
			handler := m.RequireAuth()(func(c echo.Context) error {
				return c.String(http.StatusOK, "success")
			})

			err := handler(c)

			if tt.expectError {
				assert.Error(t, err)
			} else if tt.expectedValid {
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.True(t, c.Get("auth.valid").(bool))
			} else {
				assert.Error(t, err)
				httpErr, ok := err.(*echo.HTTPError)
				require.True(t, ok)
				assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
			}
		})
	}
}
