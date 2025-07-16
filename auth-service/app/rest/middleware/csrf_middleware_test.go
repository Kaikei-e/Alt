package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"auth-service/app/domain"
)

// MockAuthUsecase for testing
type MockAuthUsecase struct {
	mock.Mock
}

func (m *MockAuthUsecase) InitiateLogin(ctx context.Context) (*domain.LoginFlow, error) {
	args := m.Called(ctx)
	return args.Get(0).(*domain.LoginFlow), args.Error(1)
}

func (m *MockAuthUsecase) InitiateRegistration(ctx context.Context) (*domain.RegistrationFlow, error) {
	args := m.Called(ctx)
	return args.Get(0).(*domain.RegistrationFlow), args.Error(1)
}

func (m *MockAuthUsecase) CompleteLogin(ctx context.Context, flowID string, body interface{}) (*domain.SessionContext, error) {
	args := m.Called(ctx, flowID, body)
	return args.Get(0).(*domain.SessionContext), args.Error(1)
}

func (m *MockAuthUsecase) CompleteRegistration(ctx context.Context, flowID string, body interface{}) (*domain.SessionContext, error) {
	args := m.Called(ctx, flowID, body)
	return args.Get(0).(*domain.SessionContext), args.Error(1)
}

func (m *MockAuthUsecase) Logout(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockAuthUsecase) ValidateSession(ctx context.Context, sessionToken string) (*domain.SessionContext, error) {
	args := m.Called(ctx, sessionToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SessionContext), args.Error(1)
}

func (m *MockAuthUsecase) RefreshSession(ctx context.Context, sessionID string) (*domain.SessionContext, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).(*domain.SessionContext), args.Error(1)
}

func (m *MockAuthUsecase) GenerateCSRFToken(ctx context.Context, sessionID string) (*domain.CSRFToken, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CSRFToken), args.Error(1)
}

func (m *MockAuthUsecase) ValidateCSRFToken(ctx context.Context, token, sessionID string) error {
	args := m.Called(ctx, token, sessionID)
	return args.Error(0)
}

// MockKratosGateway for testing
type MockKratosGateway struct {
	mock.Mock
}

func (m *MockKratosGateway) CreateLoginFlow(ctx context.Context) (*domain.LoginFlow, error) {
	args := m.Called(ctx)
	return args.Get(0).(*domain.LoginFlow), args.Error(1)
}

func (m *MockKratosGateway) CreateRegistrationFlow(ctx context.Context) (*domain.RegistrationFlow, error) {
	args := m.Called(ctx)
	return args.Get(0).(*domain.RegistrationFlow), args.Error(1)
}

func (m *MockKratosGateway) SubmitLoginFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error) {
	args := m.Called(ctx, flowID, body)
	return args.Get(0).(*domain.KratosSession), args.Error(1)
}

func (m *MockKratosGateway) SubmitRegistrationFlow(ctx context.Context, flowID string, body interface{}) (*domain.KratosSession, error) {
	args := m.Called(ctx, flowID, body)
	return args.Get(0).(*domain.KratosSession), args.Error(1)
}

func (m *MockKratosGateway) GetSession(ctx context.Context, sessionToken string) (*domain.KratosSession, error) {
	args := m.Called(ctx, sessionToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.KratosSession), args.Error(1)
}

func (m *MockKratosGateway) RevokeSession(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

// Additional methods to implement full KratosGateway interface
func (m *MockKratosGateway) CreateIdentity(ctx context.Context, email, name string) (*domain.KratosIdentity, error) {
	args := m.Called(ctx, email, name)
	return args.Get(0).(*domain.KratosIdentity), args.Error(1)
}

func (m *MockKratosGateway) GetIdentity(ctx context.Context, identityID string) (*domain.KratosIdentity, error) {
	args := m.Called(ctx, identityID)
	return args.Get(0).(*domain.KratosIdentity), args.Error(1)
}

func (m *MockKratosGateway) UpdateIdentity(ctx context.Context, identityID string, traits map[string]interface{}) (*domain.KratosIdentity, error) {
	args := m.Called(ctx, identityID, traits)
	return args.Get(0).(*domain.KratosIdentity), args.Error(1)
}

func (m *MockKratosGateway) DeleteIdentity(ctx context.Context, identityID string) error {
	args := m.Called(ctx, identityID)
	return args.Error(0)
}

func (m *MockKratosGateway) GetLoginFlow(ctx context.Context, flowID string) (*domain.LoginFlow, error) {
	args := m.Called(ctx, flowID)
	return args.Get(0).(*domain.LoginFlow), args.Error(1)
}

func (m *MockKratosGateway) GetRegistrationFlow(ctx context.Context, flowID string) (*domain.RegistrationFlow, error) {
	args := m.Called(ctx, flowID)
	return args.Get(0).(*domain.RegistrationFlow), args.Error(1)
}

func (m *MockKratosGateway) RefreshSession(ctx context.Context, sessionID string) (*domain.KratosSession, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).(*domain.KratosSession), args.Error(1)
}

func (m *MockKratosGateway) ListSessions(ctx context.Context, identityID string) ([]*domain.KratosSession, error) {
	args := m.Called(ctx, identityID)
	return args.Get(0).([]*domain.KratosSession), args.Error(1)
}

func TestEnhancedCSRFMiddleware_ValidateCSRFWithKratosSession(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		sessionCookie  string
		csrfToken      string
		setupMocks     func(*MockAuthUsecase, *MockKratosGateway)
		expectedStatus int
		expectedError  string
	}{
		{
			name:   "successful CSRF validation with valid session",
			method: "POST",
			path:   "/v1/user/profile",
			sessionCookie: "valid-session-token",
			csrfToken:     "valid-csrf-token",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				sessionCtx := &domain.SessionContext{
					UserID:    testUserID,
					TenantID:  testTenantID,
					Email:     "test@example.com",
					SessionID: "session-123",
					IsActive:  true,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
				auth.On("ValidateSession", mock.Anything, "valid-session-token").Return(sessionCtx, nil)
				auth.On("ValidateCSRFToken", mock.Anything, "valid-csrf-token", "session-123").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "CSRF validation fails with invalid token",
			method: "POST",
			path:   "/v1/user/profile",
			sessionCookie: "valid-session-token",
			csrfToken:     "invalid-csrf-token",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				sessionCtx := &domain.SessionContext{
					UserID:    testUserID,
					TenantID:  testTenantID,
					Email:     "test@example.com",
					SessionID: "session-123",
					IsActive:  true,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
				auth.On("ValidateSession", mock.Anything, "valid-session-token").Return(sessionCtx, nil)
				auth.On("ValidateCSRFToken", mock.Anything, "invalid-csrf-token", "session-123").Return(assert.AnError)
			},
			expectedStatus: http.StatusForbidden,
			expectedError:  "CSRF token validation failed",
		},
		{
			name:          "no session cookie provided",
			method:        "POST",
			path:          "/v1/user/profile",
			sessionCookie: "",
			csrfToken:     "valid-csrf-token",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				auth.On("ValidateSession", mock.Anything, "").Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "invalid session",
		},
		{
			name:   "no CSRF token provided",
			method: "POST",
			path:   "/v1/user/profile",
			sessionCookie: "valid-session-token",
			csrfToken:     "",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				sessionCtx := &domain.SessionContext{
					UserID:    testUserID,
					TenantID:  testTenantID,
					Email:     "test@example.com",
					SessionID: "session-123",
					IsActive:  true,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
				auth.On("ValidateSession", mock.Anything, "valid-session-token").Return(sessionCtx, nil)
			},
			expectedStatus: http.StatusForbidden,
			expectedError:  "CSRF token required",
		},
		{
			name:   "safe method bypasses CSRF",
			method: "GET",
			path:   "/v1/user/profile",
			sessionCookie: "valid-session-token",
			csrfToken:     "",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				// No mocks needed for safe methods
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "login endpoint bypasses CSRF",
			method: "POST",
			path:   "/v1/auth/login",
			sessionCookie: "",
			csrfToken:     "",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				// No mocks needed for skipped endpoints
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockAuth := &MockAuthUsecase{}
			mockKratos := &MockKratosGateway{}
			tt.setupMocks(mockAuth, mockKratos)

			config := DefaultCSRFConfig()
			middleware := NewEnhancedCSRFMiddleware(mockAuth, mockKratos, config, testLogger())

			// Create test handler
			handler := func(c echo.Context) error {
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			}

			// Setup Echo
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			// Add session cookie if provided
			if tt.sessionCookie != "" {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: tt.sessionCookie,
				})
			}

			// Add CSRF token header if provided
			if tt.csrfToken != "" {
				req.Header.Set("X-CSRF-Token", tt.csrfToken)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath(tt.path)

			// Execute
			middlewareFunc := middleware.Middleware()
			err := middlewareFunc(handler)(c)

			// Assert
			if tt.expectedStatus >= 400 {
				require.Error(t, err)
				httpErr, ok := err.(*echo.HTTPError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedStatus, httpErr.Code)
				if tt.expectedError != "" {
					assert.Contains(t, httpErr.Message, tt.expectedError)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)
			}

			// Verify mocks
			mockAuth.AssertExpectations(t)
			mockKratos.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenProvider(t *testing.T) {
	tests := []struct {
		name           string
		sessionCookie  string
		setupMocks     func(*MockAuthUsecase, *MockKratosGateway)
		expectedStatus int
		expectedError  string
	}{
		{
			name:          "successful CSRF token generation",
			sessionCookie: "valid-session-token",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				sessionCtx := &domain.SessionContext{
					UserID:    testUserID,
					TenantID:  testTenantID,
					Email:     "test@example.com",
					SessionID: "session-123",
					IsActive:  true,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
				auth.On("ValidateSession", mock.Anything, "valid-session-token").Return(sessionCtx, nil)

				csrfToken := &domain.CSRFToken{
					Token:     "generated-csrf-token",
					SessionID: "session-123",
					ExpiresAt: time.Now().Add(1 * time.Hour),
					CreatedAt: time.Now(),
				}
				auth.On("GenerateCSRFToken", mock.Anything, "session-123").Return(csrfToken, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:          "no session provided",
			sessionCookie: "",
			setupMocks: func(auth *MockAuthUsecase, kratos *MockKratosGateway) {
				auth.On("ValidateSession", mock.Anything, "").Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "session required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockAuth := &MockAuthUsecase{}
			mockKratos := &MockKratosGateway{}
			tt.setupMocks(mockAuth, mockKratos)

			config := DefaultCSRFConfig()
			middleware := NewEnhancedCSRFMiddleware(mockAuth, mockKratos, config, testLogger())

			// Setup Echo
			e := echo.New()
			req := httptest.NewRequest("POST", "/v1/auth/csrf", nil)

			// Add session cookie if provided
			if tt.sessionCookie != "" {
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: tt.sessionCookie,
				})
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Execute
			handler := middleware.CSRFTokenProvider()
			err := handler(c)

			// Assert
			if tt.expectedStatus >= 400 {
				require.Error(t, err)
				httpErr, ok := err.(*echo.HTTPError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedStatus, httpErr.Code)
				if tt.expectedError != "" {
					assert.Contains(t, httpErr.Message, tt.expectedError)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, rec.Code)

				// Check that CSRF cookie was set
				cookies := rec.Result().Cookies()
				var csrfCookie *http.Cookie
				for _, cookie := range cookies {
					if cookie.Name == "csrf_token" {
						csrfCookie = cookie
						break
					}
				}
				require.NotNil(t, csrfCookie, "CSRF cookie should be set")
				assert.NotEmpty(t, csrfCookie.Value)
			}

			// Verify mocks
			mockAuth.AssertExpectations(t)
			mockKratos.AssertExpectations(t)
		})
	}
}

// Helper functions for testing
var (
	testUserID   = mustParseUUID("550e8400-e29b-41d4-a716-446655440000")
	testTenantID = mustParseUUID("550e8400-e29b-41d4-a716-446655440001")
)

func mustParseUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}