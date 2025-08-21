package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"alt/config"
	"alt/domain"
	"alt/mocks"
)

// TDD: GREEN - Test for UserContext creation (implementing the failing test)
func TestAuthMiddleware_CreatesUserContextFromEnhancedResponse(t *testing.T) {
	tests := []struct {
		name              string
		mockAuthResponse  ValidateOKResponse
		expectedUserID    string
		expectedEmail     string
		expectedTenantID  string
		expectedRole      domain.UserRole
	}{
		{
			name: "creates UserContext from auth-service response with full user details",
			mockAuthResponse: ValidateOKResponse{
				Valid:      true,
				SessionID:  "sess-123",
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
				SessionID:  "sess-admin-456",
				IdentityID: "12345678-9abc-def0-1234-56789abcdef0",
				Email:      "admin@example.com",
				TenantID:   "87654321-fedc-ba98-7654-321098765432",
				Role:       "admin",
			},
			expectedUserID:   "12345678-9abc-def0-1234-56789abcdef0",
			expectedEmail:    "admin@example.com",
			expectedTenantID: "87654321-fedc-ba98-7654-321098765432", 
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

			// Use minimal mock (we're testing direct HTTP validation)
			mockAuth := mocks.NewMockAuthPort(ctrl)
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			
			cfg := &config.Config{
				Auth: config.AuthConfig{
					ServiceURL: mockServer.URL,
					ValidateEmpty200OK: false,
					KratosInternalURL: "http://kratos.test:4433",
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