// Package get_article_source_url_usecase resolves an article's canonical
// HTTPS URL by id, scoped to the caller's tenant. Used by the Knowledge Loop
// ACT workspace's Open recovery affordance when the projection's
// actTargets[].source_url is empty (legacy entry, or a producer-side ADR-879
// lookup miss).
//
// This is a read-side query: it does NOT mutate state and does NOT append any
// event. The lookup is delegated to article_url_lookup_port whose driver
// enforces tenant scope (`WHERE id = $1 AND user_id = $2`).
package get_article_source_url_usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/article_url_lookup_port"
)

// ErrInvalidArgument is returned when the input fails validation (e.g.
// malformed UUID). Mapped to Connect-RPC InvalidArgument by the handler.
var ErrInvalidArgument = errors.New("invalid_argument")

// ErrNotFound is returned when no article matches the (article_id, user_id)
// pair. Mapped to Connect-RPC NotFound by the handler.
var ErrNotFound = errors.New("not_found")

// GetArticleSourceURLUsecase implements the lookup.
type GetArticleSourceURLUsecase struct {
	lookupPort article_url_lookup_port.ArticleURLLookupPort
}

// NewGetArticleSourceURLUsecase wires the usecase. lookupPort must be non-nil.
func NewGetArticleSourceURLUsecase(
	lookupPort article_url_lookup_port.ArticleURLLookupPort,
) *GetArticleSourceURLUsecase {
	return &GetArticleSourceURLUsecase{lookupPort: lookupPort}
}

// Execute resolves the source URL and stored title for articleID, scoped to
// userID.
//
// Returns:
//   - the article source and nil error on a tenant-owned hit
//   - zero + ErrInvalidArgument when articleID is not a UUID
//   - zero + ErrNotFound when the article is missing or belongs to another tenant
//   - zero + wrapped driver error on infrastructure failures
//
// A hit with an empty title is still a hit: the URL is what decides found from
// not-found, and a row whose title was never populated is a data gap for the
// caller to render around, not a lookup failure.
func (u *GetArticleSourceURLUsecase) Execute(
	ctx context.Context,
	articleID string,
	userID uuid.UUID,
) (domain.ArticleSource, error) {
	if _, err := uuid.Parse(articleID); err != nil {
		return domain.ArticleSource{}, fmt.Errorf("%w: malformed article_id", ErrInvalidArgument)
	}
	source, err := u.lookupPort.LookupArticleSource(ctx, articleID, userID)
	if err != nil {
		return domain.ArticleSource{}, fmt.Errorf("get_article_source_url: lookup: %w", err)
	}
	if source.URL == "" {
		return domain.ArticleSource{}, ErrNotFound
	}
	return source, nil
}
