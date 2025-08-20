package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	ory "github.com/ory/client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
	"os"
	"go.uber.org/mock/gomock"
	"github.com/google/uuid"

	mock_port "auth-service/app/mocks"
	"auth-service/app/domain"
)

func TestValidate_OK_ReturnsJSON(t *testing.T) {
	tests := []struct {
		name           string
		mockSetup      func() (*ory.Session, *http.Response, error)
		expectedStatus int
		expectedBody   func(*testing.T, []byte)
	}{
		{
			name: "valid session returns JSON",
			mockSetup: func() (*ory.Session, *http.Response, error) {
				sessionID := uuid.New().String()
				identityID := uuid.New().String()
				session := &ory.Session{
					Id: sessionID,
					Active: &[]bool{true}[0],
					Identity: &ory.Identity{
						Id: identityID,
					},
				}
				response := &http.Response{
					StatusCode: 200,
				}
				return session, response, nil
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				require.NotZero(t, len(body), "must not be empty body on 200")
				var v ValidateOK
				require.NoError(t, json.Unmarshal(body, &v))
				require.True(t, v.Valid)
				require.NotEmpty(t, v.SessionID)
				require.NotEmpty(t, v.IdentityID)
			},
		},
		{
			name: "invalid session returns JSON error",
			mockSetup: func() (*ory.Session, *http.Response, error) {
				return nil, nil, assert.AnError
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody: func(t *testing.T, body []byte) {
				require.NotZero(t, len(body), "must not be empty body on 401")
				var v ErrorPayload
				require.NoError(t, json.Unmarshal(body, &v))
				require.NotEmpty(t, v.Message)
			},
		},
		{
			name: "inactive session returns JSON error",
			mockSetup: func() (*ory.Session, *http.Response, error) {
				sessionID := uuid.New().String()
				identityID := uuid.New().String()
				session := &ory.Session{
					Id: sessionID,
					Active: &[]bool{false}[0],
					Identity: &ory.Identity{
						Id: identityID,
					},
				}
				response := &http.Response{
					StatusCode: 200,
				}
				return session, response, nil
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody: func(t *testing.T, body []byte) {
				require.NotZero(t, len(body), "must not be empty body on 401")
				var v ErrorPayload
				require.NoError(t, json.Unmarshal(body, &v))
				require.Equal(t, "session inactive", v.Message)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			
			mockAuthUsecase := mock_port.NewMockAuthUsecase(ctrl)
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			
			// Set required environment variable for Ory client initialization
			os.Setenv("KRATOS_PUBLIC_URL", "http://test-kratos:4433")
			defer os.Unsetenv("KRATOS_PUBLIC_URL")
			
			handler := NewAuthHandler(mockAuthUsecase, logger)

			// Setup mock expectations based on test case
			session, _, err := tt.mockSetup()
			if err != nil {
				mockAuthUsecase.EXPECT().ValidateSessionWithCookie(gomock.Any(), gomock.Any()).Return(nil, err)
			} else {
				// Convert ory.Session to domain.SessionContext
				sessionCtx := &domain.SessionContext{
					UserID:          uuid.MustParse(session.Identity.Id),
					TenantID:        uuid.New(),
					Email:           "test@example.com",
					Name:            "Test User",
					Role:            domain.UserRoleUser,
					SessionID:       session.Id,
					KratosSessionID: session.Id,
					IsActive:        session.Active != nil && *session.Active,
					ExpiresAt:       time.Now().Add(time.Hour),
					LastActivityAt:  time.Now(),
				}
				mockAuthUsecase.EXPECT().ValidateSessionWithCookie(gomock.Any(), gomock.Any()).Return(sessionCtx, nil)
			}

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/v1/auth/validate", nil)
			req.Header.Set("Cookie", "ory_kratos_session=test-session")
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			// Execute test
			handlerErr := handler.Validate(c)
			
			// Echo handlers return nil even for error HTTP responses (401, etc.)
			require.NoError(t, handlerErr)

			// Validate response
			require.Equal(t, tt.expectedStatus, rec.Code)
			tt.expectedBody(t, rec.Body.Bytes())
		})
	}
}

func TestValidate_ContentTypeHeader(t *testing.T) {
	// Test that Content-Type header is set correctly
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mockAuthUsecase := mock_port.NewMockAuthUsecase(ctrl)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Set required environment variable for Ory client initialization
	os.Setenv("KRATOS_PUBLIC_URL", "http://test-kratos:4433")
	defer os.Unsetenv("KRATOS_PUBLIC_URL")
	
	handler := NewAuthHandler(mockAuthUsecase, logger)

	// Mock successful validation
	sessionCtx := &domain.SessionContext{
		UserID:          uuid.New(),
		TenantID:        uuid.New(),
		Email:           "test@example.com",
		Name:            "Test User",
		Role:            domain.UserRoleUser,
		SessionID:       "session-123",
		KratosSessionID: "session-123",
		IsActive:        true,
		ExpiresAt:       time.Now().Add(time.Hour),
		LastActivityAt:  time.Now(),
	}
	mockAuthUsecase.EXPECT().ValidateSessionWithCookie(gomock.Any(), gomock.Any()).Return(sessionCtx, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/validate", nil)
	req.Header.Set("Cookie", "ory_kratos_session=test-session")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err := handler.Validate(c)
	require.NoError(t, err)
	
	// Expected: Content-Type should be application/json for both success and error cases
	expectedContentType := "application/json"
	assert.Equal(t, expectedContentType, rec.Header().Get("Content-Type"))
}