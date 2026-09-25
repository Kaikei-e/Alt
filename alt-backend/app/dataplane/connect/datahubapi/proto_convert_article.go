package datahubapi

import (
	"time"

	"alt/dataplane/port/internal_article_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// timePtrOrNil keeps an unset cursor nil rather than turning it into the zero
// time, which the first-page query branches on.
func timePtrOrNil(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil || !ts.IsValid() {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func timestampOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func articleContentToProto(a *domain.ArticleContent) *datahubv1.ArticleContent {
	if a == nil {
		return nil
	}
	return &datahubv1.ArticleContent{
		Id:      a.ID,
		Title:   a.Title,
		Content: a.Content,
		Url:     a.URL,
		FeedId:  a.FeedID,
	}
}

func userArticleToProto(a *domain.Article) *datahubv1.UserArticle {
	if a == nil {
		return nil
	}
	// The zero feed id is sent as an empty string rather than
	// "00000000-...": an article whose feed was never resolved has no feed,
	// and a caller comparing against the nil UUID text would be comparing
	// against a value that means nothing to it.
	feedID := ""
	if a.FeedID != uuid.Nil {
		feedID = a.FeedID.String()
	}
	return &datahubv1.UserArticle{
		Id:          a.ID.String(),
		FeedId:      feedID,
		Title:       a.Title,
		Content:     a.Content,
		Url:         a.URL,
		Tags:        a.Tags,
		PublishedAt: timestampOrNil(a.PublishedAt),
		CreatedAt:   timestampOrNil(a.CreatedAt),
	}
}

// ── Helpers ──

func toProtoArticles(articles []*internal_article_port.ArticleWithTags) []*datahubv1.ArticleWithTags {
	result := make([]*datahubv1.ArticleWithTags, len(articles))
	for i, a := range articles {
		result[i] = toProtoArticle(a)
	}
	return result
}

func toProtoArticle(a *internal_article_port.ArticleWithTags) *datahubv1.ArticleWithTags {
	proto := &datahubv1.ArticleWithTags{
		Id:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		Tags:      a.Tags,
		CreatedAt: timestamppb.New(a.CreatedAt),
		UserId:    a.UserID,
		Language:  a.Language,
	}
	// A NULL articles.published_at stays an unset Timestamp rather than being
	// substituted with created_at: only the consumer knows whether a fallback
	// is acceptable for its use.
	if a.PublishedAt != nil {
		proto.PublishedAt = timestamppb.New(*a.PublishedAt)
	}
	return proto
}
