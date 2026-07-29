package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// requireAdminToken used to return next unchanged when the token was empty,
// so a missing ADMIN_TOKEN opened /admin/* — including the snapshot writer
// and the retention exporter — to anyone who could reach the metrics port.
func TestRequireAdminToken_EnabledGatesAdminPaths(t *testing.T) {
	h := requireAdminToken("super-secret-admin-token-value", true, okHandler())

	cases := []struct {
		name     string
		path     string
		auth     string
		wantCode int
	}{
		{"admin without token", "/admin/snapshots/create", "", http.StatusUnauthorized},
		{"admin with wrong token", "/admin/snapshots/create", "Bearer nope", http.StatusUnauthorized},
		{"admin with right token", "/admin/snapshots/create", "Bearer super-secret-admin-token-value", http.StatusOK},
		{"non-admin path", "/metrics", "", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Errorf("expected %d, got %d", tc.wantCode, rec.Code)
			}
		})
	}
}

// An empty token with the gate still enabled must deny, never fall open.
func TestRequireAdminToken_EmptyTokenWhileEnabledDenies(t *testing.T) {
	h := requireAdminToken("", true, okHandler())

	req := httptest.NewRequest(http.MethodPost, "/admin/snapshots/create", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAdminToken_ExplicitlyDisabledPassesThrough(t *testing.T) {
	h := requireAdminToken("", false, okHandler())

	req := httptest.NewRequest(http.MethodPost, "/admin/snapshots/create", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
