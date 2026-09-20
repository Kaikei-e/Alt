package sovereign_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rag-orchestrator/internal/usecase"
)

func TestLoadSovereignEventToken(t *testing.T) {
	t.Run("disabled mode returns empty token without reading file", func(t *testing.T) {
		token, err := LoadSovereignEventToken("", "disabled")
		require.NoError(t, err)
		assert.Empty(t, token)

		token, err = LoadSovereignEventToken("/non/existent/file", "DISABLED")
		require.NoError(t, err)
		assert.Empty(t, token)
	})

	t.Run("empty token file path returns error when not disabled", func(t *testing.T) {
		t.Setenv("SOVEREIGN_EVENT_TOKEN", "")
		_, err := LoadSovereignEventToken("", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SOVEREIGN_EVENT_TOKEN_FILE or SOVEREIGN_EVENT_TOKEN is required")
	})

	t.Run("unreadable file returns error", func(t *testing.T) {
		missingFile := filepath.Join(t.TempDir(), "not-found")
		_, err := LoadSovereignEventToken(missingFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read SOVEREIGN_EVENT_TOKEN_FILE")
	})

	t.Run("empty file returns error", func(t *testing.T) {
		emptyFile := filepath.Join(t.TempDir(), "empty_token")
		require.NoError(t, os.WriteFile(emptyFile, []byte(""), 0o600))
		_, err := LoadSovereignEventToken(emptyFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be at least 24 characters")
	})

	t.Run("whitespace-only file returns error", func(t *testing.T) {
		wsFile := filepath.Join(t.TempDir(), "whitespace_token")
		require.NoError(t, os.WriteFile(wsFile, []byte("   \n\t  "), 0o600))
		_, err := LoadSovereignEventToken(wsFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be at least 24 characters")
	})

	t.Run("short token file (< 24 chars) returns error", func(t *testing.T) {
		shortFile := filepath.Join(t.TempDir(), "short_token")
		require.NoError(t, os.WriteFile(shortFile, []byte("short-token"), 0o600))
		_, err := LoadSovereignEventToken(shortFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be at least 24 characters")
	})

	t.Run("valid plain env token returns token", func(t *testing.T) {
		t.Setenv("SOVEREIGN_EVENT_TOKEN", "plain-env-sovereign-token-24chars")
		token, err := LoadSovereignEventToken("", "")
		require.NoError(t, err)
		assert.Equal(t, "plain-env-sovereign-token-24chars", token)
	})

	t.Run("short plain env token returns error", func(t *testing.T) {
		t.Setenv("SOVEREIGN_EVENT_TOKEN", "short")
		_, err := LoadSovereignEventToken("", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be at least 24 characters")
	})

	t.Run("valid file returns trimmed token", func(t *testing.T) {
		validFile := filepath.Join(t.TempDir(), "valid_token")
		require.NoError(t, os.WriteFile(validFile, []byte("my-secret-sovereign-token\n"), 0o600))
		token, err := LoadSovereignEventToken(validFile, "")
		require.NoError(t, err)
		assert.Equal(t, "my-secret-sovereign-token", token)
	})
}

func TestAppendEventClient_BearerHeaderSent(t *testing.T) {
	var receivedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"eventSeq": 1}`))
	}))
	defer ts.Close()

	client := NewAppendEventClient(ts.URL, ts.Client(), WithToken("test-bearer-token"))
	err := client.EmitAugurConversationLinked(context.Background(), usecase.AugurConversationLinkedInput{
		UserID:         uuid.New(),
		TenantID:       uuid.New(),
		ConversationID: uuid.New(),
		EntryKey:       "entry:test",
		LensModeID:     "default",
		LinkedAt:       time.Now().UnixMilli(),
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-bearer-token", receivedAuth)
}
