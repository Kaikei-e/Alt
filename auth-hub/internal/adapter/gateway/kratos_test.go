package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestKratosGateway_GetFirstIdentityID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/admin/identities", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page_size"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]adminIdentity{{ID: "user-abc-123"}})
	}))
	defer server.Close()

	gw := NewKratosGateway("http://unused", server.URL, 5*time.Second)
	userID, err := gw.GetFirstIdentityID(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, "user-abc-123", userID)
}

func TestKratosGateway_GetFirstIdentityID_AdminNotConfigured(t *testing.T) {
	gw := NewKratosGateway("http://unused", "", 5*time.Second)
	userID, err := gw.GetFirstIdentityID(context.Background())

	assert.Empty(t, userID)
	assert.True(t, errors.Is(err, domain.ErrAdminNotConfigured))
}

func TestKratosGateway_GetFirstIdentityID_NoIdentities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]adminIdentity{})
	}))
	defer server.Close()

	gw := NewKratosGateway("http://unused", server.URL, 5*time.Second)
	userID, err := gw.GetFirstIdentityID(context.Background())

	assert.Empty(t, userID)
	assert.True(t, errors.Is(err, domain.ErrNoIdentitiesFound))
}

func TestKratosGateway_GetFirstIdentityID_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	gw := NewKratosGateway("http://unused", server.URL, 5*time.Second)
	userID, err := gw.GetFirstIdentityID(context.Background())

	assert.Empty(t, userID)
	assert.True(t, errors.Is(err, domain.ErrKratosUnavailable))
}

func TestKratosGateway_ValidateSession_429_ReturnsRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	gw := NewKratosGateway(server.URL, "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=test-cookie")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrRateLimited))
}

func TestKratosGateway_ValidateSession_EmptyCookie(t *testing.T) {
	gw := NewKratosGateway("http://unused", "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrSessionNotFound))
}

func TestKratosGateway_ValidateSession_401_ReturnsAuthFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	gw := NewKratosGateway(server.URL, "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=expired-cookie")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrAuthFailed))
}

func TestKratosGateway_ValidateSession_500_ReturnsKratosUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	gw := NewKratosGateway(server.URL, "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=test-cookie")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrKratosUnavailable))
}

func TestKratosGateway_ValidateSession_TransportError_ReturnsKratosUnavailable(t *testing.T) {
	// No listener at all: the request never gets an HTTP response, so the
	// gateway must fall back to the transport-error branch (resp == nil).
	gw := NewKratosGateway("http://127.0.0.1:1", "", 200*time.Millisecond)
	identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=test-cookie")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrKratosUnavailable))
}

func TestKratosGateway_ValidateSession_InactiveSession_ReturnsSessionInactive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sess-1","active":false,"identity":{"id":"user-1","schema_id":"default","schema_url":"http://x/schema","traits":{"email":"a@example.com"}}}`))
	}))
	defer server.Close()

	gw := NewKratosGateway(server.URL, "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=inactive-cookie")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrSessionInactive))
}

func TestKratosGateway_ValidateSession_MissingIdentity_ReturnsMissingIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sess-1","active":true}`))
	}))
	defer server.Close()

	gw := NewKratosGateway(server.URL, "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=no-identity-cookie")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrMissingIdentity))
}

// TestKratosGateway_ValidateSession_RoleParsing is the auth-critical
// table-driven suite: it locks in exactly which trait shapes are allowed to
// grant the "admin" role, and confirms every other shape safely defaults to
// "user" instead of silently propagating an attacker-controlled role string.
func TestKratosGateway_ValidateSession_RoleParsing(t *testing.T) {
	tests := []struct {
		name       string
		traitsJSON string // raw JSON fragment for the "traits" field
		wantRole   string
		wantEmail  string
	}{
		{
			name:       "admin role grants admin",
			traitsJSON: `{"email":"admin@example.com","role":"admin"}`,
			wantRole:   "admin",
			wantEmail:  "admin@example.com",
		},
		{
			name:       "explicit user role stays user",
			traitsJSON: `{"email":"user@example.com","role":"user"}`,
			wantRole:   "user",
			wantEmail:  "user@example.com",
		},
		{
			name:       "missing role trait defaults to user",
			traitsJSON: `{"email":"norole@example.com"}`,
			wantRole:   "user",
			wantEmail:  "norole@example.com",
		},
		{
			name:       "unexpected role string defaults to user",
			traitsJSON: `{"email":"weird@example.com","role":"superadmin"}`,
			wantRole:   "user",
			wantEmail:  "weird@example.com",
		},
		{
			name:       "non-string role defaults to user",
			traitsJSON: `{"email":"num@example.com","role":123}`,
			wantRole:   "user",
			wantEmail:  "num@example.com",
		},
		{
			name:       "non-map traits defaults to user and empty email",
			traitsJSON: `"not-a-map"`,
			wantRole:   "user",
			wantEmail:  "",
		},
		{
			name:       "missing email trait defaults to empty string",
			traitsJSON: `{"role":"admin"}`,
			wantRole:   "admin",
			wantEmail:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"id":"sess-1","active":true,"identity":{"id":"user-1","schema_id":"default","schema_url":"http://x/schema","traits":` + tc.traitsJSON + `}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(body))
			}))
			defer server.Close()

			gw := NewKratosGateway(server.URL, "", 5*time.Second)
			identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=cookie")

			assert.NoError(t, err)
			if assert.NotNil(t, identity) {
				assert.Equal(t, tc.wantRole, identity.Role)
				assert.Equal(t, tc.wantEmail, identity.Email)
				assert.Equal(t, "user-1", identity.UserID)
				assert.Equal(t, "sess-1", identity.SessionID)
			}
		})
	}
}

func TestKratosGateway_ValidateSession_Success_PopulatesCreatedAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sess-1","active":true,"identity":{"id":"user-1","schema_id":"default","schema_url":"http://x/schema","traits":{"email":"a@example.com"},"created_at":"2024-06-15T12:00:00Z"}}`))
	}))
	defer server.Close()

	gw := NewKratosGateway(server.URL, "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=cookie")

	assert.NoError(t, err)
	if assert.NotNil(t, identity) {
		assert.Equal(t, time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC), identity.CreatedAt)
	}
}
