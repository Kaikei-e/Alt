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

	"alt/config"
	"alt/domain"
)

// Simple test without mocks to check UserContext creation
func TestAuthMiddleware_UserContextCreation_Simple(t *testing.T) {
	// Setup mock auth server with enhanced response
	mockAuthResponse := ValidateOKResponse{
		Valid:      true,
		SessionID:  "sess-123",
		IdentityID: "01234567-89ab-cdef-0123-456789abcdef",
		Email:      "test@example.com",
		TenantID:   "87654321-fedc-ba98-7654-321098765432",
		Role:       "user",
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(mockAuthResponse)
	}))
	defer mockServer.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	cfg := &config.Config{
		Auth: config.AuthConfig{
			ServiceURL: mockServer.URL,
			ValidateEmpty200OK: false,
			KratosInternalURL: "http://kratos.test:4433",
		},
	}
	
	// Create middleware without mock AuthPort (we'll use nil since we're testing direct validation)
	m := &AuthMiddleware{
		authGateway:       nil, // Not used in direct validation path
		logger:            logger,
		kratosInternalURL: cfg.Auth.KratosInternalURL,
		config:            cfg,
		httpClient: &http.Client{
			Timeout: 5 * 1000000000, // 5 seconds in nanoseconds
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Cookie", "ory_kratos_session=valid")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := m.RequireAuth()(func(c echo.Context) error {
		// TDD GREEN: Check that UserContext is now available
		user, err := domain.GetUserFromContext(c.Request().Context())
		require.NoError(t, err, "UserContext should be available in request context")
		
		// Verify UserContext contains correct information
		assert.Equal(t, "01234567-89ab-cdef-0123-456789abcdef", user.UserID.String())
		assert.Equal(t, "test@example.com", user.Email)
		assert.Equal(t, "87654321-fedc-ba98-7654-321098765432", user.TenantID.String()) 
		assert.Equal(t, domain.UserRoleUser, user.Role)
		assert.Equal(t, "sess-123", user.SessionID)
		assert.True(t, user.IsValid())
		
		return c.String(http.StatusOK, "success")
	})

	err := handler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, c.Get("auth.valid").(bool))
}