package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/port"

	"github.com/google/uuid"
)

var (
	ErrBlankSeedArticleID    = errors.New("candidateSimilarityUsecase: seed_article_id is required")
	ErrInvalidRepresentation = errors.New("candidateSimilarityUsecase: invalid representation")
	ErrTooManyCandidates     = fmt.Errorf("candidateSimilarityUsecase: candidate_article_ids exceeds maximum of %d", domain.MaxCandidateSimilarityArticles)
)

// CandidateSimilarityInput is the input to CandidateSimilarityUsecase.
type CandidateSimilarityInput struct {
	UserID              string
	SeedArticleID       string
	CandidateArticleIDs []string
	Representation      string
}

// CandidateSimilarityOutput is the output of CandidateSimilarityUsecase.
type CandidateSimilarityOutput struct {
	Results                    []domain.CandidateSimilarityItem
	SeedIndexed                bool
	CandidatesWithoutEmbedding int
}

// CandidateSimilarityUsecase computes embedding similarity between one seed
// article and a caller-supplied candidate article set.
type CandidateSimilarityUsecase interface {
	Execute(ctx context.Context, input CandidateSimilarityInput) (*CandidateSimilarityOutput, error)
}

type candidateSimilarityUsecase struct {
	repo port.CandidateSimilarityRepository
}

// NewCandidateSimilarityUsecase creates a CandidateSimilarityUsecase backed by repo.
func NewCandidateSimilarityUsecase(repo port.CandidateSimilarityRepository) CandidateSimilarityUsecase {
	return &candidateSimilarityUsecase{repo: repo}
}

func (u *candidateSimilarityUsecase) Execute(ctx context.Context, input CandidateSimilarityInput) (*CandidateSimilarityOutput, error) {
	trimmedUserID := strings.TrimSpace(input.UserID)
	if trimmedUserID == "" {
		return nil, ErrEmptyUserID
	}
	parsedUserID, err := uuid.Parse(trimmedUserID)
	if err != nil || parsedUserID == uuid.Nil {
		return nil, ErrInvalidUserID
	}

	seedID := strings.TrimSpace(input.SeedArticleID)
	if seedID == "" {
		return nil, ErrBlankSeedArticleID
	}

	rep, ok := domain.ParseSimilarityRepresentation(input.Representation)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidRepresentation, input.Representation)
	}

	if len(input.CandidateArticleIDs) > domain.MaxCandidateSimilarityArticles {
		return nil, ErrTooManyCandidates
	}

	candidates, candidateSet := sanitizeCandidates(input.CandidateArticleIDs, seedID)
	if len(candidates) == 0 {
		return &CandidateSimilarityOutput{
			Results:                    []domain.CandidateSimilarityItem{},
			SeedIndexed:                false,
			CandidatesWithoutEmbedding: 0,
		}, nil
	}

	if u.repo == nil {
		panic("candidateSimilarityUsecase: repository is not wired")
	}

	res, err := u.repo.ComputeCandidateSimilarities(ctx, seedID, candidates, rep, parsedUserID)
	if err != nil {
		return nil, err
	}

	if !res.SeedIndexed {
		return &CandidateSimilarityOutput{
			Results:                    []domain.CandidateSimilarityItem{},
			SeedIndexed:                false,
			CandidatesWithoutEmbedding: res.CandidatesWithoutEmbedding,
		}, nil
	}

	// Defense-in-depth: never emit the seed or any ID outside the caller's candidate set.
	filtered := make([]domain.CandidateSimilarityItem, 0, len(res.Results))
	for _, item := range res.Results {
		if item.ArticleID == seedID {
			continue
		}
		if _, allowed := candidateSet[item.ArticleID]; !allowed {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Similarity != filtered[j].Similarity {
			return filtered[i].Similarity > filtered[j].Similarity
		}
		return filtered[i].ArticleID < filtered[j].ArticleID
	})

	return &CandidateSimilarityOutput{
		Results:                    filtered,
		SeedIndexed:                true,
		CandidatesWithoutEmbedding: res.CandidatesWithoutEmbedding,
	}, nil
}

func sanitizeCandidates(raw []string, seedID string) ([]string, map[string]struct{}) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, id := range raw {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || trimmed == seedID {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out, seen
}
