package repository

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testArticlesLanguageLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// TestArticlesLanguageRepo_InterfaceCompliance mirrors the existing repo tests
// (see summarize_job_repository_test.go). Business-logic coverage lives in the
// service-layer tests that drive the repository via a fake.
func TestArticlesLanguageRepo_InterfaceCompliance(t *testing.T) {
	t.Run("constructor does not panic with nil pool", func(t *testing.T) {
		repo := NewArticlesLanguageRepo(nil, testArticlesLanguageLogger())
		assert.NotNil(t, repo)
	})
}

func TestArticlesLanguageRepo_FetchUndArticles_RejectsNilPool(t *testing.T) {
	repo := NewArticlesLanguageRepo(nil, testArticlesLanguageLogger())
	_, err := repo.FetchUndArticles(context.Background(), "", 10)
	assert.Error(t, err)
}

func TestArticlesLanguageRepo_UpdateLanguageBulk_EmptyInputIsNoop(t *testing.T) {
	repo := NewArticlesLanguageRepo(nil, testArticlesLanguageLogger())
	updated, err := repo.UpdateLanguageBulk(context.Background(), nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, updated)
}

func TestArticlesLanguageRepo_UpdateLanguageBulk_RejectsNilPool(t *testing.T) {
	repo := NewArticlesLanguageRepo(nil, testArticlesLanguageLogger())
	_, err := repo.UpdateLanguageBulk(context.Background(), []LanguageUpdate{
		{ID: "a", Language: "ja"},
	})
	assert.Error(t, err)
}
