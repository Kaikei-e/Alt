package repository

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestNewHybridSearchRepository_ImplementsInterface(t *testing.T) {
	// Verify that the constructor returns the correct interface.
	// Cannot call methods without a real DB, but we verify the type.
	var _ domain.HybridSearcher = NewHybridSearchRepository(nil, 60)
}

func TestNewHybridSearchRepository_DefaultRRFK(t *testing.T) {
	repo := NewHybridSearchRepository(nil, 0)
	// With rrfK=0, should default to 60
	assert.NotNil(t, repo)

	concrete := repo.(*hybridSearchRepository)
	assert.Equal(t, 60, concrete.rrfK)
}

func TestNewHybridSearchRepository_CustomRRFK(t *testing.T) {
	repo := NewHybridSearchRepository(nil, 30)
	concrete := repo.(*hybridSearchRepository)
	assert.Equal(t, 30, concrete.rrfK)
}
