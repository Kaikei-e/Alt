package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCandidateSimilarityRepo struct {
	calls          int
	lastSeed       string
	lastCandidates []string
	lastRep        domain.SimilarityRepresentation
	lastUserID     uuid.UUID
	result         *domain.CandidateSimilarityResult
	err            error
}

func (f *fakeCandidateSimilarityRepo) ComputeCandidateSimilarities(
	_ context.Context,
	seedArticleID string,
	candidateArticleIDs []string,
	rep domain.SimilarityRepresentation,
	userID uuid.UUID,
) (*domain.CandidateSimilarityResult, error) {
	f.calls++
	f.lastSeed = seedArticleID
	f.lastCandidates = append([]string(nil), candidateArticleIDs...)
	f.lastRep = rep
	f.lastUserID = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestCandidateSimilarityUsecase_Validation(t *testing.T) {
	validUser := uuid.NewString()
	repo := &fakeCandidateSimilarityRepo{}
	uc := NewCandidateSimilarityUsecase(repo)

	t.Run("missing user_id", func(t *testing.T) {
		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              "",
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: []string{"cand-1"},
			Representation:      "first_chunk",
		})
		require.ErrorIs(t, err, ErrEmptyUserID)
		assert.Equal(t, 0, repo.calls)
	})

	t.Run("invalid user_id", func(t *testing.T) {
		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              "not-a-uuid",
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: []string{"cand-1"},
			Representation:      "first_chunk",
		})
		require.ErrorIs(t, err, ErrInvalidUserID)
		assert.Equal(t, 0, repo.calls)
	})

	t.Run("nil uuid user_id", func(t *testing.T) {
		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              uuid.Nil.String(),
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: []string{"cand-1"},
			Representation:      "first_chunk",
		})
		require.ErrorIs(t, err, ErrInvalidUserID)
		assert.Equal(t, 0, repo.calls)
	})

	t.Run("blank seed_article_id", func(t *testing.T) {
		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser,
			SeedArticleID:       "  ",
			CandidateArticleIDs: []string{"cand-1"},
			Representation:      "first_chunk",
		})
		require.ErrorIs(t, err, ErrBlankSeedArticleID)
		assert.Equal(t, 0, repo.calls)
	})

	t.Run("invalid representation", func(t *testing.T) {
		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser,
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: []string{"cand-1"},
			Representation:      "summary_vector",
		})
		require.ErrorIs(t, err, ErrInvalidRepresentation)
		assert.Equal(t, 0, repo.calls)
	})

	t.Run("candidate set larger than MaxCandidateSimilarityArticles returns invalid-argument error", func(t *testing.T) {
		tooMany := make([]string, domain.MaxCandidateSimilarityArticles+1)
		for i := range tooMany {
			tooMany[i] = fmt.Sprintf("cand-%d", i)
		}
		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser,
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: tooMany,
			Representation:      "chunk_mean",
		})
		require.ErrorIs(t, err, ErrTooManyCandidates)
		assert.Equal(t, 0, repo.calls)
	})
}

func TestCandidateSimilarityUsecase_EmptyCandidatesNeverTouchesDB(t *testing.T) {
	validUser := uuid.NewString()

	t.Run("nil candidates", func(t *testing.T) {
		repo := &fakeCandidateSimilarityRepo{}
		uc := NewCandidateSimilarityUsecase(repo)

		out, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser,
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: nil,
			Representation:      "first_chunk",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, repo.calls)
		assert.Empty(t, out.Results)
		assert.Equal(t, 0, out.CandidatesWithoutEmbedding)
	})

	t.Run("candidates containing only seed and blanks", func(t *testing.T) {
		repo := &fakeCandidateSimilarityRepo{}
		uc := NewCandidateSimilarityUsecase(repo)

		out, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser,
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: []string{"seed-1", "  ", ""},
			Representation:      "chunk_mean",
		})
		require.NoError(t, err)
		assert.Equal(t, 0, repo.calls)
		assert.Empty(t, out.Results)
		assert.Equal(t, 0, out.CandidatesWithoutEmbedding)
	})
}

func TestCandidateSimilarityUsecase_ContractRules(t *testing.T) {
	validUser := uuid.New()

	t.Run("excludes seed, deduplicates candidates, filters rogue IDs, sorts descending", func(t *testing.T) {
		repo := &fakeCandidateSimilarityRepo{
			result: &domain.CandidateSimilarityResult{
				SeedIndexed:                true,
				CandidatesWithoutEmbedding: 1,
				Results: []domain.CandidateSimilarityItem{
					{ArticleID: "cand-low", Similarity: 0.4},
					{ArticleID: "seed-1", Similarity: 1.0},        // must never leak
					{ArticleID: "rogue-unread", Similarity: 0.99}, // must never leak
					{ArticleID: "cand-high", Similarity: 0.9},
					{ArticleID: "cand-tie-b", Similarity: 0.7},
					{ArticleID: "cand-tie-a", Similarity: 0.7},
				},
			},
		}
		uc := NewCandidateSimilarityUsecase(repo)

		out, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser.String(),
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: []string{"seed-1", "cand-high", "cand-low", "cand-high", "cand-tie-b", "cand-tie-a", "cand-missing"},
			Representation:      "nearest_chunk_pair",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, repo.calls)
		assert.Equal(t, "seed-1", repo.lastSeed)
		assert.Equal(t, []string{"cand-high", "cand-low", "cand-tie-b", "cand-tie-a", "cand-missing"}, repo.lastCandidates)
		assert.Equal(t, domain.SimilarityRepNearestChunkPair, repo.lastRep)
		assert.Equal(t, validUser, repo.lastUserID)

		assert.True(t, out.SeedIndexed)
		assert.Equal(t, 1, out.CandidatesWithoutEmbedding)
		require.Len(t, out.Results, 4)
		assert.Equal(t, "cand-high", out.Results[0].ArticleID)
		assert.Equal(t, "cand-tie-a", out.Results[1].ArticleID)
		assert.Equal(t, "cand-tie-b", out.Results[2].ArticleID)
		assert.Equal(t, "cand-low", out.Results[3].ArticleID)
	})

	t.Run("seed without embeddings returns empty results and seed_indexed=false", func(t *testing.T) {
		repo := &fakeCandidateSimilarityRepo{
			result: &domain.CandidateSimilarityResult{
				SeedIndexed:                false,
				CandidatesWithoutEmbedding: 2,
				Results:                    []domain.CandidateSimilarityItem{},
			},
		}
		uc := NewCandidateSimilarityUsecase(repo)

		out, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser.String(),
			SeedArticleID:       "seed-unindexed",
			CandidateArticleIDs: []string{"cand-1", "cand-2", "cand-3"},
			Representation:      "chunk_mean",
		})
		require.NoError(t, err)
		assert.False(t, out.SeedIndexed)
		assert.Empty(t, out.Results)
		assert.Equal(t, 2, out.CandidatesWithoutEmbedding)
	})

	t.Run("allows exactly MaxCandidateSimilarityArticles", func(t *testing.T) {
		exact := make([]string, domain.MaxCandidateSimilarityArticles)
		for i := range exact {
			exact[i] = fmt.Sprintf("cand-%d", i)
		}
		repo := &fakeCandidateSimilarityRepo{
			result: &domain.CandidateSimilarityResult{
				SeedIndexed: true,
				Results:     []domain.CandidateSimilarityItem{},
			},
		}
		uc := NewCandidateSimilarityUsecase(repo)

		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser.String(),
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: exact,
			Representation:      "first_chunk",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, repo.calls)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repoErr := errors.New("db failure")
		repo := &fakeCandidateSimilarityRepo{err: repoErr}
		uc := NewCandidateSimilarityUsecase(repo)

		_, err := uc.Execute(context.Background(), CandidateSimilarityInput{
			UserID:              validUser.String(),
			SeedArticleID:       "seed-1",
			CandidateArticleIDs: []string{"cand-1"},
			Representation:      "first_chunk",
		})
		require.ErrorIs(t, err, repoErr)
	})
}
