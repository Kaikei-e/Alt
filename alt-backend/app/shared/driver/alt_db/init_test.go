package alt_db

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitDBConnectionPool_RejectsUnusablePoolSizeBeforeConnect(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "max too large for int32", key: "DB_MAX_CONNS", value: "2147483648"},
		{name: "min too large for int32", key: "DB_MIN_CONNS", value: "2147483648"},
		{name: "max negative", key: "DB_MAX_CONNS", value: "-1"},
		{name: "min negative", key: "DB_MIN_CONNS", value: "-5"},
		{name: "max non-integer", key: "DB_MAX_CONNS", value: "forty"},
		{name: "min non-integer", key: "DB_MIN_CONNS", value: "10.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DB_MAX_CONNS", "20")
			t.Setenv("DB_MIN_CONNS", "5")
			t.Setenv(tt.key, tt.value)

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			pool, err := InitDBConnectionPool(ctx)
			if pool != nil {
				pool.Close()
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
			assert.Contains(t, err.Error(), tt.value)
			assert.NotErrorIs(t, err, context.DeadlineExceeded)
		})
	}
}

func TestInitDBConnectionPool_RejectsMissingAuthOrSSLModeBeforeConnect(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "missing-db-secret")

	tests := []struct {
		name         string
		password     string
		passwordFile string
		sslMode      string
		wantContains string
	}{
		{
			name:         "unset DB_SSL_MODE returns error without panic",
			password:     "testpass",
			passwordFile: "",
			sslMode:      "",
			wantContains: "DB_SSL_MODE",
		},
		{
			name:         "invalid DB_SSL_MODE returns error without panic",
			password:     "testpass",
			passwordFile: "",
			sslMode:      "prefer",
			wantContains: "DB_SSL_MODE",
		},
		{
			name:         "missing password returns error without panic",
			password:     "",
			passwordFile: "",
			sslMode:      "disable",
			wantContains: "DB_PASSWORD",
		},
		{
			name:         "unreadable password file returns error without panic",
			password:     "",
			passwordFile: missingFile,
			sslMode:      "disable",
			wantContains: "DB_PASSWORD_FILE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DB_MAX_CONNS", "20")
			t.Setenv("DB_MIN_CONNS", "5")
			t.Setenv("DB_PASSWORD", tt.password)
			t.Setenv("DB_PASSWORD_FILE", tt.passwordFile)
			t.Setenv("DB_SSL_MODE", tt.sslMode)

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			var poolErr error
			require.NotPanics(t, func() {
				pool, err := InitDBConnectionPool(ctx)
				if pool != nil {
					pool.Close()
				}
				poolErr = err
			})

			require.Error(t, poolErr)
			assert.Contains(t, poolErr.Error(), tt.wantContains)
			assert.NotErrorIs(t, poolErr, context.DeadlineExceeded)
		})
	}
}

func TestGetDBConnectionString_LogsSingleConfiguredSSLMode(t *testing.T) {
	for _, allowedMode := range []string{"disable", "require", "verify-ca", "verify-full"} {
		t.Run("mode_"+allowedMode, func(t *testing.T) {
			var logBuf bytes.Buffer
			prevLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
			t.Cleanup(func() { slog.SetDefault(prevLogger) })

			t.Setenv("DB_HOST", "testhost")
			t.Setenv("DB_PASSWORD", "testpass")
			t.Setenv("DB_PASSWORD_FILE", "")
			t.Setenv("DB_SSL_MODE", allowedMode)

			connStr, err := getDBConnectionString()
			require.NoError(t, err)
			assert.Contains(t, connStr, "sslmode="+allowedMode)

			logged := logBuf.String()
			assert.Contains(t, logged, "ssl_mode="+allowedMode)
			assert.NotContains(t, logged, "ssl_mode=disabled")
			assert.Equal(t, 1, strings.Count(logged, "ssl_mode="), "expected exactly one ssl_mode log entry, got:\n%s", logged)
		})
	}
}
