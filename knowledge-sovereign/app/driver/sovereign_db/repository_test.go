package sovereign_db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRepository(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)
	require.NotNil(t, repo)
	assert.Equal(t, mock, repo.pool)
}
