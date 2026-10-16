package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"auth-hub/internal/domain"
	"auth-hub/internal/infrastructure/token"
	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHandlerCSRFSecret = "this-is-a-valid-csrf-secret-that-is-at-least-32-chars"

type stubSessionValidator struct {
	lastCookie string
	identity   *domain.Identity
	err        error
}

func (s *stubSessionValidator) ValidateSession(_ context.Context, cookie string) (*domain.Identity, error) {
	s.lastCookie = cookie
	if s.err != nil {
		return nil, s.err
	}
	return s.identity, nil
}

func TestCSRFHandler_IgnoresShadowedCookieSuffix(t *testing.T) {
	e := echo.New()
	validator := &stubSessionValidator{
		identity: &domain.Identity{
			UserID:    "user-001",
			SessionID: "kratos-sess-001",
		},
	}
	csrfGen := token.NewHMACCSRFGenerator(testHandlerCSRFSecret)
	uc := usecase.NewGenerateCSRF(validator, csrfGen, slog.Default())
	h := NewCSRFHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/csrf", nil)
	// Shadowed cookie name ending with "ory_kratos_session=" must not trick extraction.
	req.Header.Set("Cookie", "evil_ory_kratos_session=attacker-sess; ory_kratos_session=legit-sess")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handle(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp csrfResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data.CSRFToken)

	// Token must validate for "legit-sess" and NOT for "attacker-sess"
	assert.NoError(t, csrfGen.Validate("legit-sess", resp.Data.CSRFToken))
	assert.Error(t, csrfGen.Validate("attacker-sess", resp.Data.CSRFToken))
	assert.Equal(t, "ory_kratos_session=legit-sess", validator.lastCookie)
}

func TestCSRFHandler_RejectsWhenKratosCookieMissing(t *testing.T) {
	e := echo.New()
	validator := &stubSessionValidator{
		identity: &domain.Identity{UserID: "user-001"},
	}
	csrfGen := token.NewHMACCSRFGenerator(testHandlerCSRFSecret)
	uc := usecase.NewGenerateCSRF(validator, csrfGen, slog.Default())
	h := NewCSRFHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/csrf", nil)
	req.Header.Set("Cookie", "unrelated_cookie=val123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handle(c)
	if err != nil {
		he, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, he.Code)
	} else {
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	assert.Empty(t, validator.lastCookie, "validator should not be called when ory_kratos_session is absent")
}
