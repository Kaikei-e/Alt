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

	"alt/port/article_url_lookup_port"
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

// Execute resolves the source URL for articleID, scoped to userID.
//
// Returns:
//   - URL string and nil error on a tenant-owned hit
//   - "" + ErrInvalidArgument when articleID is not a UUID
//   - "" + ErrNotFound when the article is missing or belongs to another tenant
//   - "" + wrapped driver error on infrastructure failures
func (u *GetArticleSourceURLUsecase) Execute(
	ctx context.Context,
	articleID string,
	userID uuid.UUID,
) (string, error) {
	if _, err := uuid.Parse(articleID); err != nil {
		return "", fmt.Errorf("%w: malformed article_id", ErrInvalidArgument)
	}
	url, err := u.lookupPort.LookupArticleURL(ctx, articleID, userID)
	if err != nil {
		return "", fmt.Errorf("get_article_source_url: lookup: %w", err)
	}
	if url == "" {
		return "", ErrNotFound
	}
	return url, nil
}
