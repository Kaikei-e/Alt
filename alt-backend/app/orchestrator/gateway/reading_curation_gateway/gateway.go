package reading_curation_gateway

import (
	"context"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/reading_curation_port"
	"alt/shared/driver/sovereign_client"
)

type sovereignCurationClient interface {
	ListReadingAnnotations(ctx context.Context, userID uuid.UUID, articleIDs []uuid.UUID, limit int32) ([]sovereign_client.ReadingAnnotationRow, error)
	ListReadingThemeCurations(ctx context.Context, userID, themeID uuid.UUID, limit int32) ([]sovereign_client.ReadingThemeCurationRow, error)
}

// Gateway maps sovereign driver rows to domain annotation and curation models.
type Gateway struct {
	client sovereignCurationClient
}

var (
	_ reading_curation_port.ListArticleAnnotationsPort = (*Gateway)(nil)
	_ reading_curation_port.ListThemeCurationsPort     = (*Gateway)(nil)
)

func NewGateway(client sovereignCurationClient) *Gateway {
	if client == nil {
		panic("reading_curation_gateway: sovereign client is required")
	}
	return &Gateway{client: client}
}

func (g *Gateway) ListArticleAnnotations(ctx context.Context, userID uuid.UUID, articleIDs []uuid.UUID, limit int32) ([]domain.ArticleAnnotation, error) {
	rows, err := g.client.ListReadingAnnotations(ctx, userID, articleIDs, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ArticleAnnotation, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.ArticleAnnotation{
			AnnotationID: r.AnnotationID,
			ArticleID:    r.ArticleID,
			Stance:       domain.AnnotationStance(r.Stance),
			Excerpt:      r.Excerpt,
			Note:         r.Note,
			CreatedAt:    r.CreatedAt,
		})
	}
	return out, nil
}

func (g *Gateway) ListThemeCurations(ctx context.Context, userID, themeID uuid.UUID, limit int32) ([]domain.ThemeCuration, error) {
	rows, err := g.client.ListReadingThemeCurations(ctx, userID, themeID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ThemeCuration, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.ThemeCuration{
			CurationID:          r.CurationID,
			ThemeID:             r.ThemeID,
			Kind:                domain.ThemeCurationKind(r.ActionKind),
			TargetHash:          r.TargetHash,
			TargetText:          r.TargetText,
			UserNote:            r.UserNote,
			AugurConversationID: r.AugurConversationID,
			CreatedAt:           r.CreatedAt,
		})
	}
	return out, nil
}
