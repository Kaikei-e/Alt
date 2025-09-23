package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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
			SessionID:  "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			IdentityID: "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
			Email:      "user@example.com",
			TenantID:   "c1eebc99-9c0b-4ef8-bb6d-6bb9bd380a13",
			Role:       "user",
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
			ServiceURL:         mockServer.URL,
			ValidateEmpty200OK: false,
			KratosInternalURL:  "http://kratos.test:4433",
		},
	}

	m := NewAuthMiddleware(mockAuth, logger, cfg)

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
			ServiceURL:         mockServer.URL,
			ValidateEmpty200OK: false,
			KratosInternalURL:  "http://kratos.test:4433",
		},
	}

	m := NewAuthMiddleware(mockAuth, logger, cfg)

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

func TestAuthMiddleware_Middleware_FallbackToKratos(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kratosServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/sessions/whoami", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		session := map[string]any{
			"id":         "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			"active":     true,
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			"identity": map[string]any{
				"id": "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
				"traits": map[string]any{
					"email": "fallback@example.com",
				},
				"metadata_public": map[string]any{
					"tenant_id":   "c1eebc99-9c0b-4ef8-bb6d-6bb9bd380a13",
					"role":        "user",
					"permissions": []string{"read"},
				},
			},
		}
		json.NewEncoder(w).Encode(session)
	}))
	defer kratosServer.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAuth := mocks.NewMockAuthPort(ctrl)
	cfg := &config.Config{
		Auth: config.AuthConfig{
			ServiceURL:        "http://127.0.0.1:1", // force direct validation to fail
			KratosInternalURL: kratosServer.URL,
		},
	}

	m := NewAuthMiddleware(mockAuth, logger, cfg)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Cookie", "ory_kratos_session=fallback")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := m.RequireAuth()(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	err := handler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	userCtx, ctxErr := domain.GetUserFromContext(c.Request().Context())
	assert.NoError(t, ctxErr)
	assert.Equal(t, "fallback@example.com", userCtx.Email)
	assert.Equal(t, domain.UserRoleUser, userCtx.Role)
}

func TestAuthMiddleware_OptionalAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mock HTTP server for direct validation
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		response := ValidateOKResponse{
			Valid:      true,
			SessionID:  "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			IdentityID: "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
			Email:      "test@example.com",
			TenantID:   "c1eebc99-9c0b-4ef8-bb6d-6bb9bd380a13",
			Role:       "user",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	mockAuth := mocks.NewMockAuthPort(ctrl)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := &config.Config{
		Auth: config.AuthConfig{
			ServiceURL:         mockServer.URL,
			ValidateEmpty200OK: false,
			KratosInternalURL:  mockServer.URL,
		},
	}

	// Create middleware with mock HTTP client
	m := &AuthMiddleware{
		authGateway:       mockAuth,
		logger:            logger,
		config:            cfg,
		httpClient:        &http.Client{Timeout: 5 * time.Second},
		kratosInternalURL: mockServer.URL,
	}

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

// TDD: RED - Test for UserContext creation (this should fail initially)
func TestAuthMiddleware_SetsUserContextFromValidation(t *testing.T) {
	tests := []struct {
		name             string
		mockAuthResponse ValidateOKResponse
		expectedUserID   string
		expectedEmail    string
		expectedTenantID string
		expectedRole     domain.UserRole
	}{
		{
			name: "creates UserContext from auth-service response with full user details",
			mockAuthResponse: ValidateOKResponse{
				Valid:      true,
				SessionID:  "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				IdentityID: "01234567-89ab-cdef-0123-456789abcdef",
				Email:      "test@example.com",
				TenantID:   "87654321-fedc-ba98-7654-321098765432",
				Role:       "user",
			},
			expectedUserID:   "01234567-89ab-cdef-0123-456789abcdef",
			expectedEmail:    "test@example.com",
			expectedTenantID: "87654321-fedc-ba98-7654-321098765432",
			expectedRole:     domain.UserRoleUser,
		},
		{
			name: "creates UserContext for admin user",
			mockAuthResponse: ValidateOKResponse{
				Valid:      true,
				SessionID:  "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13",
				IdentityID: "d1eebc99-9c0b-4ef8-bb6d-6bb9bd380a14",
				Email:      "admin@example.com",
				TenantID:   "e2eebc99-9c0b-4ef8-bb6d-6bb9bd380a15",
				Role:       "admin",
			},
			expectedUserID:   "d1eebc99-9c0b-4ef8-bb6d-6bb9bd380a14",
			expectedEmail:    "admin@example.com",
			expectedTenantID: "e2eebc99-9c0b-4ef8-bb6d-6bb9bd380a15",
			expectedRole:     domain.UserRoleAdmin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock auth server with enhanced response
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				json.NewEncoder(w).Encode(tt.mockAuthResponse)
			}))
			defer mockServer.Close()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAuth := mocks.NewMockAuthPort(ctrl)
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			cfg := &config.Config{
				Auth: config.AuthConfig{
					ServiceURL:         mockServer.URL,
					ValidateEmpty200OK: false,
				},
			}

			m := NewAuthMiddleware(mockAuth, logger, cfg)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Cookie", "ory_kratos_session=valid")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := m.RequireAuth()(func(c echo.Context) error {
				// TDD: This is what we want to achieve - UserContext should be available
				user, err := domain.GetUserFromContext(c.Request().Context())
				require.NoError(t, err, "UserContext should be available in request context")

				// Verify UserContext contains correct information
				assert.Equal(t, tt.expectedUserID, user.UserID.String())
				assert.Equal(t, tt.expectedEmail, user.Email)
				assert.Equal(t, tt.expectedTenantID, user.TenantID.String())
				assert.Equal(t, tt.expectedRole, user.Role)
				assert.Equal(t, tt.mockAuthResponse.SessionID, user.SessionID)
				assert.True(t, user.IsValid())

				return c.String(http.StatusOK, "success")
			})

			err := handler(c)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.True(t, c.Get("auth.valid").(bool))
		})
	}
}

// Contract consumer tests for auth-service validation
func TestAuthMiddleware_DirectValidation(t *testing.T) {
	tests := []struct {
		name             string
		mockAuthResponse string
		mockStatusCode   int
		featureFlag      bool
		expectedValid    bool
		expectError      bool
	}{
		{
			name:             "200 with valid JSON",
			mockAuthResponse: `{"valid": true, "session_id": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "identity_id": "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a12"}`,
			mockStatusCode:   200,
			featureFlag:      false,
			expectedValid:    true,
			expectError:      false,
		},
		{
			name:             "200 with empty body, feature flag disabled",
			mockAuthResponse: "",
			mockStatusCode:   200,
			featureFlag:      false,
			expectedValid:    false,
			expectError:      false,
		},
		{
			name:             "200 with empty body, feature flag enabled",
			mockAuthResponse: "",
			mockStatusCode:   200,
			featureFlag:      true,
			expectedValid:    true,
			expectError:      false,
		},
		{
			name:             "401 unauthorized",
			mockAuthResponse: `{"message": "invalid session"}`,
			mockStatusCode:   401,
			featureFlag:      false,
			expectedValid:    false,
			expectError:      false,
		},
		{
			name:             "500 server error",
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
					ServiceURL:         mockServer.URL,
					ValidateEmpty200OK: tt.featureFlag,
				},
			}

			m := NewAuthMiddleware(mockAuth, logger, cfg)

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
