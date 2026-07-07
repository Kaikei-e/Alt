// ABOUTME: Tests for AdminAPIHandler.RequireAdmin, the middleware used to gate
// ABOUTME: ad-hoc admin routes (e.g. manual trigger endpoints) behind auth/rate-limit checks

package handler

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"pre-processor-sidecar/security"

	"github.com/stretchr/testify/assert"
)

type denyingAuthenticator struct{}

func (denyingAuthenticator) ValidateKubernetesServiceAccountToken(token string) (*security.ServiceAccountInfo, error) {
	return nil, assert.AnError
}

func (denyingAuthenticator) HasAdminPermissions(info *security.ServiceAccountInfo) bool {
	return false
}

type noAdminPermissionAuthenticator struct{}

func (noAdminPermissionAuthenticator) ValidateKubernetesServiceAccountToken(token string) (*security.ServiceAccountInfo, error) {
	return &security.ServiceAccountInfo{Subject: "system:serviceaccount:test:non-admin"}, nil
}

func (noAdminPermissionAuthenticator) HasAdminPermissions(info *security.ServiceAccountInfo) bool {
	return false
}

type denyingRateLimiter struct{}

func (denyingRateLimiter) IsAllowed(clientIP, endpoint string) bool { return false }
func (denyingRateLimiter) RecordRequest(clientIP, endpoint string)  {}

func newTestAdminAPIHandler(auth AdminAuthenticator, rateLimiter RateLimiter) *AdminAPIHandler {
	return NewAdminAPIHandler(
		&MockTokenManager{},
		auth,
		rateLimiter,
		&MockInputValidator{},
		slog.Default(),
		&MockAdminAPIMetricsCollector{},
	)
}

func TestRequireAdmin_MissingAuthorization_Returns401(t *testing.T) {
	h := newTestAdminAPIHandler(&MockAdminAuthenticator{}, &MockRateLimiter{})

	called := false
	wrapped := h.RequireAdmin("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/trigger/article-fetch", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	wrapped(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.False(t, called, "downstream handler must not run without authorization")
}

func TestRequireAdmin_InvalidToken_Returns401(t *testing.T) {
	h := newTestAdminAPIHandler(denyingAuthenticator{}, &MockRateLimiter{})

	called := false
	wrapped := h.RequireAdmin("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/trigger/article-fetch", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Authorization", "Bearer bad-token")
	recorder := httptest.NewRecorder()

	wrapped(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.False(t, called)
}

func TestRequireAdmin_InsufficientPermissions_Returns403(t *testing.T) {
	h := newTestAdminAPIHandler(noAdminPermissionAuthenticator{}, &MockRateLimiter{})

	called := false
	wrapped := h.RequireAdmin("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/trigger/article-fetch", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Authorization", "Bearer some-token")
	recorder := httptest.NewRecorder()

	wrapped(recorder, req)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.False(t, called)
}

func TestRequireAdmin_RateLimited_Returns429(t *testing.T) {
	h := newTestAdminAPIHandler(&MockAdminAuthenticator{}, denyingRateLimiter{})

	called := false
	wrapped := h.RequireAdmin("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/trigger/article-fetch", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Authorization", "Bearer some-token")
	recorder := httptest.NewRecorder()

	wrapped(recorder, req)

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.False(t, called)
}

func TestRequireAdmin_ValidRequest_CallsNext(t *testing.T) {
	h := newTestAdminAPIHandler(&MockAdminAuthenticator{}, &MockRateLimiter{})

	called := false
	wrapped := h.RequireAdmin("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/trigger/article-fetch", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Authorization", "Bearer good-token")
	recorder := httptest.NewRecorder()

	wrapped(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, called, "downstream handler must run once authorized")
}

func TestRequireAdmin_HTTPRequired_Returns400(t *testing.T) {
	h := newTestAdminAPIHandler(&MockAdminAuthenticator{}, &MockRateLimiter{})

	called := false
	wrapped := h.RequireAdmin("/admin/trigger/article-fetch", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/trigger/article-fetch", nil)
	recorder := httptest.NewRecorder()

	wrapped(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.False(t, called)
}
