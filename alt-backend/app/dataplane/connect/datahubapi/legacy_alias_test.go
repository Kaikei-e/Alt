package datahubapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The alias must rewrite exactly the retired DataHubService name and nothing
// else: a broader rewrite would quietly resurrect other retired surfaces
// (services.backend.v1) or forward names that never existed.
func TestLegacyNamespaceAlias_RewritesOnlyTheRetiredDataHubName(t *testing.T) {
	var gotPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	h := LegacyNamespaceAlias(next)

	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{
			name:     "legacy rpc rewritten",
			path:     "/alt.datahub.v1.DataHubService/CreateArticle",
			wantPath: "/services.datahub.v1.DataHubService/CreateArticle",
		},
		{
			name:     "current path untouched",
			path:     "/services.datahub.v1.DataHubService/CreateArticle",
			wantPath: "/services.datahub.v1.DataHubService/CreateArticle",
		},
		{
			name:     "other service under the legacy package untouched",
			path:     "/alt.datahub.v1.OtherService/Do",
			wantPath: "/alt.datahub.v1.OtherService/Do",
		},
		{
			name:     "retired backend namespace untouched",
			path:     "/services.backend.v1.BackendInternalService/CreateArticle",
			wantPath: "/services.backend.v1.BackendInternalService/CreateArticle",
		},
		{
			name:     "health untouched",
			path:     "/health",
			wantPath: "/health",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.path, nil))
			if gotPath != tt.wantPath {
				t.Fatalf("next saw path %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}

// The rewrite must not leak back into the caller's request: the verifier and
// any middleware holding the original *http.Request must still see the path
// they sent.
func TestLegacyNamespaceAlias_DoesNotMutateTheOriginalRequest(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := LegacyNamespaceAlias(next)

	req := httptest.NewRequest(http.MethodPost, "/alt.datahub.v1.DataHubService/CreateArticle", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if req.URL.Path != "/alt.datahub.v1.DataHubService/CreateArticle" {
		t.Fatalf("original request path mutated to %q", req.URL.Path)
	}
}
