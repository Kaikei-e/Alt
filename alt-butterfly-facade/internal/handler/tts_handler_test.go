package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTTSHandler_ReturnsHandler(t *testing.T) {
	// In the TDD RED phase, this fails because NewTTSHandler panics "not implemented".
	var h any
	assert.NotPanics(t, func() {
		h = NewTTSHandler(
			true,
			nil,
			[]byte("test-secret-at-least-32-chars-long!"),
			"auth-hub",
			"alt-backend",
			nil,
			5*time.Minute,
		)
	})
	require.NotNil(t, h)
}
