package datahub_gateway

import (
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/orchestrator/driver/models"
)

func feedSummaryFromProto(s *datahubv1.FeedSummary) *domain.FeedSummary {
	if s == nil {
		return nil
	}
	return &domain.FeedSummary{Summary: s.GetSummary()}
}

func feedModelsFromProto(feeds []*datahubv1.Feed) []*models.Feed {
	out := make([]*models.Feed, 0, len(feeds))
	for _, f := range feeds {
		if m := feedModelFromProto(f); m != nil {
			out = append(out, m)
		}
	}
	return out
}

func feedModelFromProto(f *datahubv1.Feed) *models.Feed {
	if f == nil {
		return nil
	}
	return &models.Feed{
		ID:          f.GetId(),
		Title:       f.GetTitle(),
		Description: f.GetDescription(),
		WebsiteURL:  f.GetWebsiteUrl(),
		PubDate:     timeFromProto(f.GetPubDate()),
		CreatedAt:   timeFromProto(f.GetCreatedAt()),
		UpdatedAt:   timeFromProto(f.GetUpdatedAt()),
		ArticleID:   f.ArticleId,
		IsRead:      f.GetIsRead(),
		FeedLinkID:  f.FeedLinkId,
		OgImageURL:  f.OgImageUrl,
	}
}

func feedModelToRow(m *models.Feed) *domain.FeedRow {
	if m == nil {
		return nil
	}
	return &domain.FeedRow{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		WebsiteURL:  m.WebsiteURL,
		PubDate:     m.PubDate,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		ArticleID:   m.ArticleID,
		IsRead:      m.IsRead,
		FeedLinkID:  m.FeedLinkID,
		OgImageURL:  m.OgImageURL,
	}
}

func feedRowFromProto(f *datahubv1.Feed) *domain.FeedRow {
	return feedModelToRow(feedModelFromProto(f))
}
