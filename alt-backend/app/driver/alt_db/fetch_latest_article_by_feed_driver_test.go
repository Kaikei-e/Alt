package alt_db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAltDBRepository_FetchLatestArticleByFeedID_NilRepository(t *testing.T) {
	var r *AltDBRepository
	_, err := r.FetchLatestArticleByFeedID(context.Background(), uuid.New())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not available")
}

func TestAltDBRepository_FetchLatestArticleByFeedID_NilPool(t *testing.T) {
	r := &AltDBRepository{pool: nil}
	_, err := r.FetchLatestArticleByFeedID(context.Background(), uuid.New())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not available")
}
