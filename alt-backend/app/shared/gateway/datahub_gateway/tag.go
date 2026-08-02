package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TagGateway is every tag read alt-backend performs, plus the write-back that
// on-the-fly generation needs (catalog §2.J, and W2-13 for the upsert).
//
// The upsert is here rather than left on the alt_db driver because of what it
// closes. fetch_article_tags_gateway had two sources — the article body came
// from alt-data-hub after batch 2, the tag read and the tag write still went
// straight to the database — and a gateway with two routes to one story is a
// place where only one of them gets a timeout, a retry or a metric. With this
// gateway it has one dependency and the alt_db import goes away.
//
// What stays with the caller is the generation itself: asking mq-hub for tags,
// the retry around it, and the singleflight that keeps two concurrent readers
// of the same article from asking twice. Those are calls to another service and
// orchestration around them (ADR-000954 D4).
type TagGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewTagGateway(client datahubv1connect.DataHubServiceClient) *TagGateway {
	if client == nil {
		panic("datahub_gateway: TagGateway requires a DataHubService client — " +
			"a nil client would make every article look untagged, which the " +
			"on-the-fly path reads as 'generate some' and would turn into an " +
			"mq-hub request per view (see .claude/rules/di-wiring.md)")
	}
	return &TagGateway{client: client}
}

// FetchArticleTags returns an article's tags. An article nobody has tagged is
// an empty slice and not an error — the on-the-fly path keys on exactly that.
func (g *TagGateway) FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	resp, err := g.client.GetArticleTags(ctx, connect.NewRequest(&datahubv1.GetArticleTagsRequest{
		ArticleId: articleID,
	}))
	if err != nil {
		return nil, fmt.Errorf("get article tags %s: %w", articleID, err)
	}
	return feedTagsFromProto(resp.Msg.GetTags()), nil
}

// FetchFeedTags pages one feed's tags, newest first.
func (g *TagGateway) FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	req := &datahubv1.GetFeedTagsRequest{
		FeedId: feedID,
		Limit:  safeconv.Int32(limit),
	}
	if cursor != nil {
		req.Cursor = timestamppb.New(*cursor)
	}

	resp, err := g.client.GetFeedTags(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("get feed tags %s: %w", feedID, err)
	}
	return feedTagsFromProto(resp.Msg.GetTags()), nil
}

// UpsertArticleTags writes generated tags back. It returns how many rows the
// provider upserted, which the caller logs but does not branch on: the
// write-back is deliberately fail-open, because tags already generated are
// worth showing even if persisting them failed.
func (g *TagGateway) UpsertArticleTags(ctx context.Context, articleID, feedID string, tags []domain.TagUpsert) (int32, error) {
	items := make([]*datahubv1.TagItem, 0, len(tags))
	for _, t := range tags {
		items = append(items, &datahubv1.TagItem{Name: t.Name, Confidence: t.Confidence})
	}

	resp, err := g.client.UpsertArticleTags(ctx, connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
		ArticleId: articleID,
		FeedId:    feedID,
		Tags:      items,
	}))
	if err != nil {
		return 0, fmt.Errorf("upsert %d tags for article %s: %w", len(tags), articleID, err)
	}
	return resp.Msg.GetUpsertedCount(), nil
}

// FetchTagCloud and FetchTagCooccurrences satisfy
// fetch_tag_cloud_port.FetchTagCloudPort together.
//
// FetchTagCloud is a Wave 2 procedure that alt-backend kept reaching past —
// it served the RPC to rag-orchestrator while reading the same table directly
// for its own Tag Verse. Both halves of the port now take the same route.
//
// The octree that turns these counts and edges into 3D coordinates stays in
// fetch_tag_cloud_usecase, along with its 30-minute cache: a pure function
// over the numbers, and a TTL, are both the caller's (ADR-000954 D4).
func (g *TagGateway) FetchTagCloud(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	resp, err := g.client.FetchTagCloud(ctx, connect.NewRequest(&datahubv1.FetchTagCloudRequest{
		Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("fetch tag cloud: %w", err)
	}

	items := make([]*domain.TagCloudItem, 0, len(resp.Msg.GetTags()))
	for _, it := range resp.Msg.GetTags() {
		items = append(items, &domain.TagCloudItem{
			TagName:      it.GetTagName(),
			ArticleCount: int(it.GetArticleCount()),
		})
	}
	return items, nil
}

func (g *TagGateway) FetchTagCooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error) {
	if len(tagNames) == 0 {
		return nil, nil
	}

	resp, err := g.client.GetTagCooccurrences(ctx, connect.NewRequest(&datahubv1.GetTagCooccurrencesRequest{
		TagNames: tagNames,
	}))
	if err != nil {
		return nil, fmt.Errorf("get tag cooccurrences for %d tags: %w", len(tagNames), err)
	}

	items := make([]*domain.TagCooccurrence, 0, len(resp.Msg.GetCooccurrences()))
	for _, c := range resp.Msg.GetCooccurrences() {
		items = append(items, &domain.TagCooccurrence{
			TagNameA:    c.GetTagNameA(),
			TagNameB:    c.GetTagNameB(),
			SharedCount: int(c.GetSharedCount()),
		})
	}
	return items, nil
}

// SearchTagsByPrefix is the global search box's tag section. The name and
// shape match the TagRepository method it replaces.
func (g *TagGateway) SearchTagsByPrefix(ctx context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error) {
	resp, err := g.client.SearchTagsByPrefix(ctx, connect.NewRequest(&datahubv1.SearchTagsByPrefixRequest{
		Prefix: prefix,
		Limit:  safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("search tags by prefix %q: %w", prefix, err)
	}

	hits := make([]domain.GlobalTagHit, 0, len(resp.Msg.GetHits()))
	for _, h := range resp.Msg.GetHits() {
		hits = append(hits, domain.GlobalTagHit{
			TagName:      h.GetTagName(),
			ArticleCount: int(h.GetArticleCount()),
		})
	}
	return hits, nil
}

// FetchTagArticleCounts is the input to the trending calculation, not the
// calculation. Which of these counts is a surge — the 7-day versus 30-day
// comparison, the thresholds, the ranking — stays in trending_tags_gateway,
// because it is a product decision and changing it should not mean redeploying
// the process that owns the database.
func (g *TagGateway) FetchTagArticleCounts(ctx context.Context, userID uuid.UUID, since time.Time) ([]domain.TagArticleCount, error) {
	resp, err := g.client.GetTagArticleCounts(ctx, connect.NewRequest(&datahubv1.GetTagArticleCountsRequest{
		UserId: userID.String(),
		Since:  timestamppb.New(since),
	}))
	if err != nil {
		return nil, fmt.Errorf("get tag article counts for user %s: %w", userID, err)
	}

	counts := make([]domain.TagArticleCount, 0, len(resp.Msg.GetCounts()))
	for _, c := range resp.Msg.GetCounts() {
		counts = append(counts, domain.TagArticleCount{
			TagName:      c.GetTagName(),
			ArticleCount: int(c.GetArticleCount()),
		})
	}
	return counts, nil
}

// FetchArticlesByTag and FetchArticlesByTagName satisfy
// fetch_articles_by_tag_port.FetchArticlesByTagPort together — the Tag Trail's
// two paged reads (catalog §2.J, ADR-000954 Wave 3 batch 6).
//
// They live on this gateway rather than beside the Wave 2 FetchArticlesByTag
// procedure because they are a different question. That procedure takes a tag
// *name*, has no cursor and returns four fields; it still serves
// rag-orchestrator and is untouched. These two page by an exclusive
// published_at bound and carry the feed each article came from, which is what
// the Trail renders. Reusing the older message would have compiled and would
// have dropped the paging in silence — the reason this pair is a capability
// rather than a DI edit.
//
// A nil cursor is the first page. It stays nil on the wire rather than
// becoming the zero time: protojson omits an unset timestamp, and the provider
// reads its absence as "no upper bound".
func (g *TagGateway) FetchArticlesByTag(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	req := &datahubv1.ListArticlesByTagIDRequest{
		TagId: tagID,
		Limit: safeconv.Int32(limit),
	}
	if cursor != nil {
		req.Cursor = timestamppb.New(*cursor)
	}

	resp, err := g.client.ListArticlesByTagID(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("list articles by tag id %s: %w", tagID, err)
	}
	return tagTrailArticlesFromProto(resp.Msg.GetArticles()), nil
}

func (g *TagGateway) FetchArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	req := &datahubv1.ListArticlesByTagNameRequest{
		TagName: tagName,
		Limit:   safeconv.Int32(limit),
	}
	if cursor != nil {
		req.Cursor = timestamppb.New(*cursor)
	}

	resp, err := g.client.ListArticlesByTagName(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("list articles by tag name %q: %w", tagName, err)
	}
	return tagTrailArticlesFromProto(resp.Msg.GetArticles()), nil
}

func tagTrailArticlesFromProto(articles []*datahubv1.TagTrailArticle) []*domain.TagTrailArticle {
	if len(articles) == 0 {
		return nil
	}
	out := make([]*domain.TagTrailArticle, 0, len(articles))
	for _, a := range articles {
		if a == nil {
			continue
		}
		out = append(out, &domain.TagTrailArticle{
			ID:          a.GetId(),
			Title:       a.GetTitle(),
			Link:        a.GetUrl(),
			PublishedAt: timeFromProto(a.GetPublishedAt()),
			FeedID:      a.GetFeedId(),
			FeedTitle:   a.GetFeedTitle(),
		})
	}
	return out
}

func feedTagsFromProto(tags []*datahubv1.FeedTag) []*domain.FeedTag {
	out := make([]*domain.FeedTag, 0, len(tags))
	for _, t := range tags {
		if t == nil {
			continue
		}
		out = append(out, &domain.FeedTag{
			ID:         t.GetId(),
			FeedID:     t.GetFeedId(),
			TagName:    t.GetTagName(),
			Confidence: t.GetConfidence(),
			TagType:    t.GetTagType(),
			CreatedAt:  timeFromProto(t.GetCreatedAt()),
			UpdatedAt:  timeFromProto(t.GetUpdatedAt()),
		})
	}
	return out
}
