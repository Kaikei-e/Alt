package csrf_token_driver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreToken_CapsMapAtMaxCSRFTokensAndSweepsExpiredFirst(t *testing.T) {
	ctx := context.Background()
	d := &InMemoryCSRFTokenDriver{
		tokens: make(map[string]time.Time),
	}

	past := time.Now().Add(-10 * time.Minute)
	future := time.Now().Add(10 * time.Minute)

	// Fill half with expired and half with unexpired up to maxCSRFTokens
	for i := 0; i < maxCSRFTokens/2; i++ {
		require.NoError(t, d.StoreToken(ctx, fmt.Sprintf("expired-%d", i), past))
	}
	for i := 0; i < maxCSRFTokens/2; i++ {
		require.NoError(t, d.StoreToken(ctx, fmt.Sprintf("valid-%d", i), future))
	}
	require.Equal(t, maxCSRFTokens, len(d.tokens))

	// Storing one more token should sweep the expired ones first
	require.NoError(t, d.StoreToken(ctx, "new-token", future))
	assert.Equal(t, maxCSRFTokens/2+1, len(d.tokens))
	_, err := d.GetToken(ctx, "new-token")
	require.NoError(t, err)
	_, err = d.GetToken(ctx, "valid-0")
	require.NoError(t, err)
}

func TestStoreToken_EvictsWhenAtCapacityWithUnexpiredTokens(t *testing.T) {
	ctx := context.Background()
	d := &InMemoryCSRFTokenDriver{
		tokens: make(map[string]time.Time),
	}

	base := time.Now().Add(5 * time.Minute)
	for i := 0; i < maxCSRFTokens; i++ {
		require.NoError(t, d.StoreToken(ctx, fmt.Sprintf("tok-%d", i), base.Add(time.Duration(i)*time.Millisecond)))
	}
	require.Equal(t, maxCSRFTokens, len(d.tokens))

	// Storing extra tokens must never grow the map beyond maxCSRFTokens
	for i := 0; i < 100; i++ {
		require.NoError(t, d.StoreToken(ctx, fmt.Sprintf("overflow-%d", i), base.Add(time.Hour)))
		assert.LessOrEqual(t, len(d.tokens), maxCSRFTokens)
	}
}
