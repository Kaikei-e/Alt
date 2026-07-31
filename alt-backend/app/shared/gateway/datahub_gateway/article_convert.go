package datahub_gateway

import (
	"context"
	"fmt"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"

	"github.com/google/uuid"
)

// userIDFromContext renders the signed-in user for a request field that treats
// the empty string as "no scope".
//
// It returns "" rather than an error because the reads that use it have a
// genuinely unscoped variant — the anonymous article lookup that has always
// worked. Callers whose write or list has no unscoped meaning check the
// context themselves and fail; they do not call this.
func userIDFromContext(ctx context.Context) string {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return ""
	}
	return user.UserID.String()
}

// articleContentFromProto maps the narrow projection back to the domain type.
// A nil message is a nil article: the procedures that return it leave the
// field unset to mean "no such article", and every caller checks for nil.
func articleContentFromProto(msg *datahubv1.ArticleContent) *domain.ArticleContent {
	if msg == nil {
		return nil
	}
	return &domain.ArticleContent{
		ID:      msg.GetId(),
		Title:   msg.GetTitle(),
		Content: msg.GetContent(),
		URL:     msg.GetUrl(),
		FeedID:  msg.GetFeedId(),
	}
}

// userArticleFromProto maps the wide projection back to domain.Article.
//
// The ids are errors rather than zero values when they do not parse. A
// domain.Article with the nil UUID would flow into the read-through cache
// keyed by article id, where every unparseable row would collide on the same
// key and serve each other's bodies.
func userArticleFromProto(msg *datahubv1.UserArticle) (*domain.Article, error) {
	if msg == nil {
		return nil, nil
	}

	id, err := uuid.Parse(msg.GetId())
	if err != nil {
		return nil, fmt.Errorf("article id %q is not a uuid: %w", msg.GetId(), err)
	}

	// feed_id is empty for an article whose feed was never resolved, which is
	// a real state rather than a malformed one — parseUUID maps it to the nil
	// UUID, the same value the driver read out of a NULL column.
	feedID, err := parseUUID(msg.GetFeedId())
	if err != nil {
		return nil, fmt.Errorf("article %s: feed id: %w", id, err)
	}

	createdAt := timeFromProto(msg.GetCreatedAt())
	publishedAt := timeFromProto(msg.GetPublishedAt())

	return &domain.Article{
		ID:          id,
		FeedID:      feedID,
		Title:       msg.GetTitle(),
		Content:     msg.GetContent(),
		URL:         msg.GetUrl(),
		Tags:        msg.GetTags(),
		PublishedAt: publishedAt,
		CreatedAt:   createdAt,
		// The driver set these from created_at rather than reading columns
		// that do not exist on articles. Keeping that here means the cache and
		// the renderers see the same values they always have.
		UpdatedAt: createdAt,
	}, nil
}

// backfillArticleFromProto maps one historic article for the knowledge
// backfill (catalog §2.N).
func backfillArticleFromProto(msg *datahubv1.BackfillArticle) (domain.KnowledgeBackfillArticle, error) {
	articleID, err := uuid.Parse(msg.GetArticleId())
	if err != nil {
		return domain.KnowledgeBackfillArticle{}, fmt.Errorf("backfill article id %q is not a uuid: %w", msg.GetArticleId(), err)
	}
	userID, err := uuid.Parse(msg.GetUserId())
	if err != nil {
		return domain.KnowledgeBackfillArticle{}, fmt.Errorf("backfill article %s: user id %q is not a uuid: %w", articleID, msg.GetUserId(), err)
	}

	return domain.KnowledgeBackfillArticle{
		ArticleID:   articleID,
		UserID:      userID,
		CreatedAt:   timeFromProto(msg.GetCreatedAt()),
		PublishedAt: timeFromProto(msg.GetPublishedAt()),
		Title:       msg.GetTitle(),
		URL:         msg.GetUrl(),
	}, nil
}

// backfillSummaryTitleFromProto maps one (summary_version, article) pair.
func backfillSummaryTitleFromProto(msg *datahubv1.BackfillSummaryTitle) (domain.KnowledgeBackfillSummaryTitle, error) {
	versionID, err := uuid.Parse(msg.GetSummaryVersionId())
	if err != nil {
		return domain.KnowledgeBackfillSummaryTitle{}, fmt.Errorf("summary version id %q is not a uuid: %w", msg.GetSummaryVersionId(), err)
	}
	articleID, err := uuid.Parse(msg.GetArticleId())
	if err != nil {
		return domain.KnowledgeBackfillSummaryTitle{}, fmt.Errorf("summary version %s: article id %q is not a uuid: %w", versionID, msg.GetArticleId(), err)
	}
	userID, err := uuid.Parse(msg.GetUserId())
	if err != nil {
		return domain.KnowledgeBackfillSummaryTitle{}, fmt.Errorf("summary version %s: user id %q is not a uuid: %w", versionID, msg.GetUserId(), err)
	}
	// tenant_id is the user id in every row the query produces today, but it
	// is a separate column and a separate field, so it is parsed separately
	// rather than aliased.
	tenantID, err := uuid.Parse(msg.GetTenantId())
	if err != nil {
		return domain.KnowledgeBackfillSummaryTitle{}, fmt.Errorf("summary version %s: tenant id %q is not a uuid: %w", versionID, msg.GetTenantId(), err)
	}

	return domain.KnowledgeBackfillSummaryTitle{
		SummaryVersionID: versionID,
		ArticleID:        articleID,
		UserID:           userID,
		TenantID:         tenantID,
		Title:            msg.GetTitle(),
		GeneratedAt:      timeFromProto(msg.GetGeneratedAt()),
	}, nil
}
