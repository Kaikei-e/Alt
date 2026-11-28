package sovereign_client

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
)

type ReadingAnnotationRow struct {
	AnnotationID uuid.UUID
	ArticleID    uuid.UUID
	Stance       string
	Excerpt      string
	Note         string
	CreatedAt    time.Time
}

type ReadingThemeCurationRow struct {
	CurationID          uuid.UUID
	ThemeID             uuid.UUID
	ActionKind          string
	TargetHash          string
	TargetText          string
	UserNote            string
	AugurConversationID string
	CreatedAt           time.Time
}

func (c *Client) ListReadingAnnotations(ctx context.Context, userID uuid.UUID, articleIDs []uuid.UUID, limit int32) ([]ReadingAnnotationRow, error) {
	if !c.enabled {
		return nil, ErrSovereignDisabled
	}

	rawIDs := make([]string, 0, len(articleIDs))
	for _, id := range articleIDs {
		rawIDs = append(rawIDs, id.String())
	}

	resp, err := c.client.ListReadingAnnotations(ctx, connect.NewRequest(&sovereignv1.ListReadingAnnotationsRequest{
		UserId:     userID.String(),
		ArticleIds: rawIDs,
		Limit:      limit,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListReadingAnnotations: %w", err)
	}

	rows := make([]ReadingAnnotationRow, 0, len(resp.Msg.GetAnnotations()))
	for _, pb := range resp.Msg.GetAnnotations() {
		annID, err := uuid.Parse(pb.GetAnnotationId())
		if err != nil {
			return nil, fmt.Errorf("sovereign ListReadingAnnotations invalid annotation_id %q: %w", pb.GetAnnotationId(), err)
		}
		artID, err := uuid.Parse(pb.GetArticleId())
		if err != nil {
			return nil, fmt.Errorf("sovereign ListReadingAnnotations invalid article_id %q: %w", pb.GetArticleId(), err)
		}
		var createdAt time.Time
		if pb.GetCreatedAt() != nil {
			createdAt = pb.GetCreatedAt().AsTime()
		}
		rows = append(rows, ReadingAnnotationRow{
			AnnotationID: annID,
			ArticleID:    artID,
			Stance:       pb.GetStance(),
			Excerpt:      pb.GetExcerpt(),
			Note:         pb.GetNote(),
			CreatedAt:    createdAt,
		})
	}
	return rows, nil
}

func (c *Client) ListReadingThemeCurations(ctx context.Context, userID, themeID uuid.UUID, limit int32) ([]ReadingThemeCurationRow, error) {
	if !c.enabled {
		return nil, ErrSovereignDisabled
	}

	resp, err := c.client.ListReadingThemeCurations(ctx, connect.NewRequest(&sovereignv1.ListReadingThemeCurationsRequest{
		UserId:  userID.String(),
		ThemeId: themeID.String(),
		Limit:   limit,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListReadingThemeCurations: %w", err)
	}

	rows := make([]ReadingThemeCurationRow, 0, len(resp.Msg.GetCurations()))
	for _, pb := range resp.Msg.GetCurations() {
		curID, err := uuid.Parse(pb.GetCurationId())
		if err != nil {
			return nil, fmt.Errorf("sovereign ListReadingThemeCurations invalid curation_id %q: %w", pb.GetCurationId(), err)
		}
		tID, err := uuid.Parse(pb.GetThemeId())
		if err != nil {
			return nil, fmt.Errorf("sovereign ListReadingThemeCurations invalid theme_id %q: %w", pb.GetThemeId(), err)
		}
		var createdAt time.Time
		if pb.GetCreatedAt() != nil {
			createdAt = pb.GetCreatedAt().AsTime()
		}
		rows = append(rows, ReadingThemeCurationRow{
			CurationID:          curID,
			ThemeID:             tID,
			ActionKind:          pb.GetActionKind(),
			TargetHash:          pb.GetTargetHash(),
			TargetText:          pb.GetTargetText(),
			UserNote:            pb.GetUserNote(),
			AugurConversationID: pb.GetAugurConversationId(),
			CreatedAt:           createdAt,
		})
	}
	return rows, nil
}
