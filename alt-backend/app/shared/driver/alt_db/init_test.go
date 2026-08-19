package alt_db

import (
	"context"
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
