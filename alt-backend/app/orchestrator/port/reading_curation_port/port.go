package reading_curation_port

import (
	"context"

	"github.com/google/uuid"

	"alt/domain"
)

// ListArticleAnnotationsPort reads active article annotations owned by a user.
type ListArticleAnnotationsPort interface {
	ListArticleAnnotations(ctx context.Context, userID uuid.UUID, articleIDs []uuid.UUID, limit int32) ([]domain.ArticleAnnotation, error)
}

// ListThemeCurationsPort reads active theme curations owned by a user.
type ListThemeCurationsPort interface {
	ListThemeCurations(ctx context.Context, userID, themeID uuid.UUID, limit int32) ([]domain.ThemeCuration, error)
}
