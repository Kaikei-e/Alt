package middleware

import (
	"errors"
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

	"alt/domain"
	"alt/mocks"
	"alt/port/auth_port"
)

func TestAuthMiddleware_Middleware(t *testing.T) {
	tests := []struct {
		name           string
		setupRequest   func(*http.Request)
		setupMock      func(*mocks.MockAuthClient)
		expectedStatus int
		expectedError  string
		expectContext  bool
	}{
		{
			name: "valid session with cookie",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "valid-session-token",
				})
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				userContext := &domain.UserContext{
					UserID:    uuid.New(),
					Email:     "test@example.com",
					Role:      domain.UserRoleUser,
					TenantID:  uuid.New(),
					SessionID: "valid-session-token",
					ExpiresAt: time.Now().Add(time.Hour),
				}
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "valid-session-token", "").
					Return(&auth_port.SessionValidationResponse{
						Valid:   true,
						UserID:  userContext.UserID.String(),
						Email:   "test@example.com",
						Role:    "user",
						Context: userContext,
					}, nil)
			},
			expectedStatus: http.StatusOK,
			expectContext:  true,
		},
		{
			name: "valid session with Authorization header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer valid-session-token")
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				userContext := &domain.UserContext{
					UserID:    uuid.New(),
					Email:     "test@example.com",
					Role:      domain.UserRoleUser,
					TenantID:  uuid.New(),
					SessionID: "valid-session-token",
					ExpiresAt: time.Now().Add(time.Hour),
				}
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "valid-session-token", "").
					Return(&auth_port.SessionValidationResponse{
						Valid:   true,
						UserID:  userContext.UserID.String(),
						Email:   "test@example.com",
						Role:    "user",
						Context: userContext,
					}, nil)
			},
			expectedStatus: http.StatusOK,
			expectContext:  true,
		},
		{
			name: "missing session token",
			setupRequest: func(req *http.Request) {
				// No session token
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				// No expectations - should fail before calling auth client
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "authentication_required",
			expectContext:  false,
		},
		{
			name: "invalid session token",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "invalid-session-token",
				})
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "invalid-session-token", "").
					Return(&auth_port.SessionValidationResponse{
						Valid: false,
					}, nil)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "session_invalid",
			expectContext:  false,
		},
		{
			name: "auth service error",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "test-session-token",
				})
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "test-session-token", "").
					Return(nil, errors.New("auth service unavailable"))
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "session_invalid",
			expectContext:  false,
		},
		{
			name: "session with tenant ID",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "valid-session-token",
				})
				req.Header.Set("X-Tenant-ID", "tenant-123")
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				userContext := &domain.UserContext{
					UserID:    uuid.New(),
					Email:     "test@example.com",
					Role:      domain.UserRoleUser,
					TenantID:  uuid.New(),
					SessionID: "valid-session-token",
					ExpiresAt: time.Now().Add(time.Hour),
				}
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "valid-session-token", "tenant-123").
					Return(&auth_port.SessionValidationResponse{
						Valid:   true,
						UserID:  userContext.UserID.String(),
						Email:   "test@example.com",
						Role:    "user",
						Context: userContext,
					}, nil)
			},
			expectedStatus: http.StatusOK,
			expectContext:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockAuthClient(ctrl)
			tt.setupMock(mockClient)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			middleware := NewAuthMiddleware(mockClient, logger)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			tt.setupRequest(req)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := middleware.Middleware()(func(c echo.Context) error {
				return c.String(http.StatusOK, "success")
			})

			err := handler(c)

			if tt.expectedStatus == http.StatusOK {
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, rec.Code)

				if tt.expectContext {
					userCtx := c.Get("user_context")
					assert.NotNil(t, userCtx)

					userID := c.Get("user_id")
					assert.NotNil(t, userID)

					userEmail := c.Get("user_email")
					assert.NotNil(t, userEmail)
				}
			} else {
				assert.Error(t, err)
				httpErr, ok := err.(*echo.HTTPError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedStatus, httpErr.Code)

				if tt.expectedError != "" {
					message, ok := httpErr.Message.(map[string]interface{})
					assert.True(t, ok)
					assert.Equal(t, tt.expectedError, message["error"])
				}
			}
		})
	}
}

func TestAuthMiddleware_OptionalAuth(t *testing.T) {
	tests := []struct {
		name          string
		setupRequest  func(*http.Request)
		setupMock     func(*mocks.MockAuthClient)
		expectContext bool
	}{
		{
			name: "valid session",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "valid-session-token",
				})
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				userContext := &domain.UserContext{
					UserID:    uuid.New(),
					Email:     "test@example.com",
					Role:      domain.UserRoleUser,
					TenantID:  uuid.New(),
					SessionID: "valid-session-token",
					ExpiresAt: time.Now().Add(time.Hour),
				}
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "valid-session-token", "").
					Return(&auth_port.SessionValidationResponse{
						Valid:   true,
						UserID:  userContext.UserID.String(),
						Email:   "test@example.com",
						Role:    "user",
						Context: userContext,
					}, nil)
			},
			expectContext: true,
		},
		{
			name: "no session token",
			setupRequest: func(req *http.Request) {
				// No session token
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				// No expectations
			},
			expectContext: false,
		},
		{
			name: "invalid session token",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "invalid-session-token",
				})
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "invalid-session-token", "").
					Return(&auth_port.SessionValidationResponse{
						Valid: false,
					}, nil)
			},
			expectContext: false,
		},
		{
			name: "auth service error",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "test-session-token",
				})
			},
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "test-session-token", "").
					Return(nil, errors.New("auth service error"))
			},
			expectContext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockAuthClient(ctrl)
			tt.setupMock(mockClient)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			middleware := NewAuthMiddleware(mockClient, logger)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			tt.setupRequest(req)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := middleware.OptionalAuth()(func(c echo.Context) error {
				return c.String(http.StatusOK, "success")
			})

			err := handler(c)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			if tt.expectContext {
				userCtx := c.Get("user_context")
				assert.NotNil(t, userCtx)
			} else {
				userCtx := c.Get("user_context")
				assert.Nil(t, userCtx)
			}
		})
	}
}

func TestGetUserContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	t.Run("user context exists", func(t *testing.T) {
		userCtx := &domain.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
			Role:   domain.UserRoleUser,
		}
		c.Set("user_context", userCtx)

		result, err := GetUserContext(c)
		assert.NoError(t, err)
		assert.Equal(t, userCtx, result)
	})

	t.Run("user context not found", func(t *testing.T) {
		c.Set("user_context", nil)

		result, err := GetUserContext(c)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user context not found")
	})
}

func TestRequireAuth(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	t.Run("authenticated user", func(t *testing.T) {
		userCtx := &domain.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
			Role:   domain.UserRoleUser,
		}
		c.Set("user_context", userCtx)

		result, err := RequireAuth(c)
		assert.NoError(t, err)
		assert.Equal(t, userCtx, result)
	})

	t.Run("unauthenticated user", func(t *testing.T) {
		c.Set("user_context", nil)

		result, err := RequireAuth(c)
		assert.Error(t, err)
		assert.Nil(t, result)

		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	})
}

func TestRequireRole(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	t.Run("user has required role", func(t *testing.T) {
		userCtx := &domain.UserContext{
			UserID: uuid.New(),
			Email:  "admin@example.com",
			Role:   domain.UserRoleAdmin,
		}
		c.Set("user_context", userCtx)

		result, err := RequireRole(c, domain.UserRoleAdmin)
		assert.NoError(t, err)
		assert.Equal(t, userCtx, result)
	})

	t.Run("user does not have required role", func(t *testing.T) {
		userCtx := &domain.UserContext{
			UserID: uuid.New(),
			Email:  "user@example.com",
			Role:   domain.UserRoleUser,
		}
		c.Set("user_context", userCtx)

		result, err := RequireRole(c, domain.UserRoleAdmin)
		assert.Error(t, err)
		assert.Nil(t, result)

		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusForbidden, httpErr.Code)
	})

	t.Run("unauthenticated user", func(t *testing.T) {
		c.Set("user_context", nil)

		result, err := RequireRole(c, domain.UserRoleAdmin)
		assert.Error(t, err)
		assert.Nil(t, result)

		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	})
}

func TestRequirePermission(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	t.Run("user has required permission", func(t *testing.T) {
		userCtx := &domain.UserContext{
			UserID:      uuid.New(),
			Email:       "user@example.com",
			Role:        domain.UserRoleUser,
			Permissions: []string{"read", "write", "delete"},
		}
		c.Set("user_context", userCtx)

		result, err := RequirePermission(c, "write")
		assert.NoError(t, err)
		assert.Equal(t, userCtx, result)
	})

	t.Run("user does not have required permission", func(t *testing.T) {
		userCtx := &domain.UserContext{
			UserID:      uuid.New(),
			Email:       "user@example.com",
			Role:        domain.UserRoleUser,
			Permissions: []string{"read"},
		}
		c.Set("user_context", userCtx)

		result, err := RequirePermission(c, "write")
		assert.Error(t, err)
		assert.Nil(t, result)

		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusForbidden, httpErr.Code)
	})

	t.Run("unauthenticated user", func(t *testing.T) {
		c.Set("user_context", nil)

		result, err := RequirePermission(c, "write")
		assert.Error(t, err)
		assert.Nil(t, result)

		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	})
}
