package alt_db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compose/core.yaml gives alt-data-hub DB_MAX_CONNS=40 / DB_MIN_CONNS=10.
// プールはその値で作られなければならない - 既定の 20/5 に落ちても起動ログは
// 実効値を出すので、黙って捨てられたことが誰にも見えない。
func TestNewDatabaseConfigFromEnv_PoolSizesComeFromEnv(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "40")
	t.Setenv("DB_MIN_CONNS", "10")

	config := NewDatabaseConfigFromEnv()

	assert.Equal(t, 40, config.MaxConns)
	assert.Equal(t, 10, config.MinConns)
}

func TestBuildConnectionString_ConnectTimeoutComesFromEnv(t *testing.T) {
	t.Setenv("DB_CONNECT_TIMEOUT_SECONDS", "5")

	config := &DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "testuser",
		Password: "testpass",
		DBName:   "testdb",
	}

	assert.Contains(t, config.BuildConnectionString(), "connect_timeout=5")
}

// 設定されているが使えない値は「既定値で続行」ではなく起動失敗（CLAUDE.md rule 9）。
func TestNewDatabaseConfigFromEnv_RejectsUnusableValue(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "non-numeric", key: "DB_MAX_CONNS", value: "forty"},
		{name: "out of int range", key: "DB_MIN_CONNS", value: "99999999999999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)

			err := recoveredError(t, func() { NewDatabaseConfigFromEnv() })

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
			assert.Contains(t, err.Error(), tt.value)
		})
	}
}

// recoveredError runs fn and returns the error it panicked with, failing the
// test when fn returns normally instead.
func recoveredError(t *testing.T, fn func()) (recovered error) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected startup to fail, but it returned normally")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected panic with error, got %T: %v", r, r)
		}
		recovered = err
	}()
	fn()
	return nil
}
