package main

import (
	"testing"
)

func TestIsAdminProtectedPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "snapshot create endpoint",
			path: "/admin/snapshots/create",
			want: true,
		},
		{
			name: "retention export endpoint",
			path: "/admin/retention/export",
			want: true,
		},
		{
			name: "storage info endpoint",
			path: "/admin/storage/info",
			want: true,
		},
		{
			name: "projections rebuild endpoint",
			path: "/admin/projections/rebuild",
			want: true,
		},
		{
			name: "deep health endpoint",
			path: "/health/deep",
			want: true,
		},
		{
			name: "basic health endpoint",
			path: "/health",
			want: false,
		},
		{
			name: "metrics endpoint",
			path: "/metrics",
			want: false,
		},
		{
			name: "admin prefix without trailing slash",
			path: "/admin",
			want: false,
		},
		{
			name: "root path",
			path: "/",
			want: false,
		},
		{
			name: "rpc endpoint",
			path: "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAdminProtectedPath(tt.path)
			if got != tt.want {
				t.Errorf("isAdminProtectedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateAdminBearerToken(t *testing.T) {
	const validToken = "super-secret-admin-token-value"

	tests := []struct {
		name          string
		authHeader    string
		expectedToken string
		want          bool
	}{
		{
			name:          "matching bearer token",
			authHeader:    "Bearer " + validToken,
			expectedToken: validToken,
			want:          true,
		},
		{
			name:          "mismatched bearer token",
			authHeader:    "Bearer wrong-token",
			expectedToken: validToken,
			want:          false,
		},
		{
			name:          "empty header",
			authHeader:    "",
			expectedToken: validToken,
			want:          false,
		},
		{
			name:          "empty expected token with empty header",
			authHeader:    "",
			expectedToken: "",
			want:          false,
		},
		{
			name:          "empty expected token with bearer prefix",
			authHeader:    "Bearer ",
			expectedToken: "",
			want:          false,
		},
		{
			name:          "bearer prefix without token",
			authHeader:    "Bearer ",
			expectedToken: validToken,
			want:          false,
		},
		{
			name:          "missing bearer prefix",
			authHeader:    validToken,
			expectedToken: validToken,
			want:          false,
		},
		{
			name:          "basic auth prefix",
			authHeader:    "Basic dXNlcjpwYXNz",
			expectedToken: validToken,
			want:          false,
		},
		{
			name:          "lowercase bearer prefix",
			authHeader:    "bearer " + validToken,
			expectedToken: validToken,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAdminBearerToken(tt.authHeader, tt.expectedToken)
			if got != tt.want {
				t.Errorf("validateAdminBearerToken(%q, %q) = %v, want %v", tt.authHeader, tt.expectedToken, got, tt.want)
			}
		})
	}
}
