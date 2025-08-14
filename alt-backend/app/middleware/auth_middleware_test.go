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
)

func TestAuthMiddleware_Middleware(t *testing.T) {
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
		ValidateSession(gomock.Any(), "valid").
		Return(userCtx, nil)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewAuthMiddleware(mockAuth, logger)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: "valid"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := m.RequireAuth()(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	err := handler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_Middleware_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuth := mocks.NewMockAuthPort(ctrl)
	mockAuth.EXPECT().
		ValidateSession(gomock.Any(), "invalid").
		Return(nil, errors.New("invalid"))

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewAuthMiddleware(mockAuth, logger)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: "invalid"})
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
		ValidateSession(gomock.Any(), "valid").
		Return(userCtx, nil)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewAuthMiddleware(mockAuth, logger)

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
