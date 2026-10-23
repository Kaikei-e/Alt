package di

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
)

func generateTestVAPIDPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(key.Bytes())
}

func TestFingerprintVAPIDKey(t *testing.T) {
	const samplePubKey = "BPn4_test_public_key_value"
	sum := sha256.Sum256([]byte(samplePubKey))
	want := hex.EncodeToString(sum[:4])

	got := fingerprintVAPIDKey(samplePubKey)
	assert.Equal(t, want, got)
	assert.Len(t, got, 8, "fingerprint must be 4 bytes / 8 hex characters")
}

func TestDispatcherIdentity_NonEmpty(t *testing.T) {
	id := dispatcherIdentity()
	assert.NotEmpty(t, id)
}

func TestNewNotifierComponents_WiresUsecaseAndLogsFingerprint(t *testing.T) {
	setDataHubClientEnv(t)

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg := splitTestConfig()
	cfg.WebPush = config.WebPushConfig{
		PrivateKey: generateTestVAPIDPrivateKey(t),
		Subject:    "mailto:ops@alt.example",
	}

	components := NewNotifierComponents(context.Background(), cfg)
	require.NotNil(t, components)
	require.NotNil(t, components.Dispatch)

	logged := logBuf.String()
	assert.Contains(t, logged, `"msg":"web_push_dispatch_enabled"`)
	assert.Contains(t, logged, `"vapid_public_key_fingerprint"`)
	assert.Contains(t, logged, `"subject":"mailto:ops@alt.example"`)
	assert.NotContains(t, logged, cfg.WebPush.PrivateKey, "private key must never appear in startup logs")
}

func TestNewNotifierComponents_PanicsOnInvalidVAPIDKey(t *testing.T) {
	setDataHubClientEnv(t)

	cfg := splitTestConfig()
	cfg.WebPush = config.WebPushConfig{
		PrivateKey: "not-a-valid-p256-scalar",
		Subject:    "mailto:ops@alt.example",
	}

	assert.Panics(t, func() {
		NewNotifierComponents(context.Background(), cfg)
	})
}
