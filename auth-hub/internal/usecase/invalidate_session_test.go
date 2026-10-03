package usecase

import (
	"context"
	"log/slog"
	"testing"

	"auth-hub/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestInvalidateSession_Success(t *testing.T) {
	cache := newMockCache()
	cache.Set("test-cookie-1", domain.CachedSession{UserID: "user-1"})

	_, found := cache.Get("test-cookie-1")
	assert.True(t, found)

	uc := NewInvalidateSession(cache, slog.Default())
	err := uc.Execute(context.Background(), "test-cookie-1")
	assert.NoError(t, err)

	_, found = cache.Get("test-cookie-1")
	assert.False(t, found)
}

func TestInvalidateSession_EmptyCookie(t *testing.T) {
	cache := newMockCache()
	uc := NewInvalidateSession(cache, slog.Default())

	err := uc.Execute(context.Background(), "")
	assert.ErrorIs(t, err, domain.ErrSessionNotFound)
}
