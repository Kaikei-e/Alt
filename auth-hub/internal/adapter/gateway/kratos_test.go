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

func TestKratosGateway_ValidateSession_EmptyCookie(t *testing.T) {
	gw := NewKratosGateway("http://unused", "", 5*time.Second)
	identity, err := gw.ValidateSession(context.Background(), "")

	assert.Nil(t, identity)
	assert.True(t, errors.Is(err, domain.ErrSessionNotFound))
}
