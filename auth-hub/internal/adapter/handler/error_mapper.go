package handler

import (
	"errors"
	"net/http"

	"auth-hub/internal/domain"

	"github.com/labstack/echo/v4"
)

// mapDomainError converts a domain error into an appropriate echo.HTTPError.
func mapDomainError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, domain.ErrSessionNotFound),
		errors.Is(err, domain.ErrAuthFailed),
		errors.Is(err, domain.ErrSessionExpired),
		errors.Is(err, domain.ErrSessionInactive),
		errors.Is(err, domain.ErrMissingIdentity):
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")

	case errors.Is(err, domain.ErrKratosUnavailable):
		return echo.NewHTTPError(http.StatusBadGateway, "identity provider unavailable")

	case errors.Is(err, domain.ErrAdminNotConfigured),
		errors.Is(err, domain.ErrNoIdentitiesFound):
		return echo.NewHTTPError(http.StatusInternalServerError, "internal configuration error")

	case errors.Is(err, domain.ErrTokenGeneration),
		errors.Is(err, domain.ErrCSRFSecretMissing),
		errors.Is(err, domain.ErrBackendSecretWeak):
		return echo.NewHTTPError(http.StatusInternalServerError, "token generation error")

	case errors.Is(err, domain.ErrRateLimited):
		return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")

	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
}
