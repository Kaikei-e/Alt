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
)

// ArticleStoreGateway is the article archive and the narrow article reads
// (catalog §2.B W3-B1, §2.C W3-C1 / C2 / C3).
//
// It is the anti-corruption layer's other job on this path: turning the
// signed-in user the request context carries into an explicit `user_id` field.
// In-process the driver could read the context itself; across a Connect call
// it cannot, because the peer certificate says "alt-backend" and nothing about
// whose article this is.
type ArticleStoreGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewArticleStoreGateway(client datahubv1connect.DataHubServiceClient) *ArticleStoreGateway {
	if client == nil {
		panic("datahub_gateway: ArticleStoreGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &ArticleStoreGateway{client: client}
}

// SaveArticle archives the article for the user in the request context.
//
// An absent user is an error rather than an anonymous write: articles is keyed
// by (url, user_id), so a write with no owner would land on a row nobody can
// read back. This mirrors the driver it replaces, which refused for the same
// reason.
func (g *ArticleStoreGateway) SaveArticle(ctx context.Context, url, title, content string) (string, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("user context required to archive %s: %w", url, err)
	}

	resp, err := g.client.ArchiveArticle(ctx, connect.NewRequest(&datahubv1.ArchiveArticleRequest{
		Url:     url,
		Title:   title,
		Content: content,
		UserId:  user.UserID.String(),
	}))
	if err != nil {
		return "", fmt.Errorf("archive article %s: %w", url, err)
	}
	return resp.Msg.GetArticleId(), nil
}

// FetchArticleByURL returns the archived article, or (nil, nil) when the URL
// has not been archived.
//
// The nil-without-error shape is load-bearing: the fetch usecase reads it as
// "go get the page", and an error would make it give up instead.
func (g *ArticleStoreGateway) FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error) {
	resp, err := g.client.GetArticleByURL(ctx, connect.NewRequest(&datahubv1.GetArticleByURLRequest{
		Url:    articleURL,
		UserId: userIDFromContext(ctx),
	}))
	if err != nil {
		return nil, fmt.Errorf("get article by url %s: %w", articleURL, err)
	}
	return articleContentFromProto(resp.Msg.GetArticle()), nil
}

// FetchArticlesByURLs is the batch read. URLs with no archived article are
// absent from the map, which is what replaced the N+1 loop this path had.
func (g *ArticleStoreGateway) FetchArticlesByURLs(ctx context.Context, urls []string) (map[string]*domain.ArticleContent, error) {
	if len(urls) == 0 {
		return map[string]*domain.ArticleContent{}, nil
	}

	resp, err := g.client.BatchGetArticlesByURLs(ctx, connect.NewRequest(&datahubv1.BatchGetArticlesByURLsRequest{
		Urls:   urls,
		UserId: userIDFromContext(ctx),
	}))
	if err != nil {
		return nil, fmt.Errorf("batch get articles by urls (%d): %w", len(urls), err)
	}

	out := make(map[string]*domain.ArticleContent, len(resp.Msg.GetArticles()))
	for url, msg := range resp.Msg.GetArticles() {
		out[url] = articleContentFromProto(msg)
	}
	return out, nil
}

// SaveArticleSummary upserts the article_summaries row (catalog §2.K seam,
// wire capability W2-08).
//
// This is the last write alt-backend held against alt_db directly, and closing
// it needed two things the procedure did not have. The title is one: the driver
// took an article_title and this side has always passed a real one, while the
// only existing caller — pre-processor — passes none, so a request without the
// field would have blanked every title alt-backend had written.
//
// The versioning flag is the other, and it is the more interesting of the two.
// The procedure appends a summary_versions row and a SummaryVersionCreated
// event as part of the write. Both callers on this side already own their
// versioning: the stream-summarise path appends its own version under its own
// model, and the legacy summarise usecase deliberately appends none. Letting
// the provider version these too would have given one summary two versions in
// the first case and invented versions in the second — neither of which any
// test on either side would have failed on, because the extra rows are only
// visible in the Knowledge Home timeline. SUMMARY_VERSIONING_SKIP says so out
// loud instead.
func (g *ArticleStoreGateway) SaveArticleSummary(ctx context.Context, articleID, userID, articleTitle, summary string) error {
	_, err := g.client.SaveArticleSummary(ctx, connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{
		ArticleId:    articleID,
		UserId:       userID,
		ArticleTitle: articleTitle,
		Summary:      summary,
		// Language is left unset. The column does not exist; the field is
		// pre-processor's, and the driver has always ignored it.
		SummaryVersioning: datahubv1.SummaryVersioning_SUMMARY_VERSIONING_SKIP,
	}))
	if err != nil {
		return fmt.Errorf("save article summary %s: %w", articleID, err)
	}
	return nil
}

// FetchArticleByID returns the article, or (nil, nil) for an unknown id.
func (g *ArticleStoreGateway) FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error) {
	resp, err := g.client.GetArticleContentByID(ctx, connect.NewRequest(&datahubv1.GetArticleContentByIDRequest{
		ArticleId: articleID,
	}))
	if err != nil {
		return nil, fmt.Errorf("get article content %s: %w", articleID, err)
	}
	return articleContentFromProto(resp.Msg.GetArticle()), nil
}

// ArticleCursorGateway is the per-user article timeline (catalog §2.C W3-C4).
type ArticleCursorGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewArticleCursorGateway(client datahubv1connect.DataHubServiceClient) *ArticleCursorGateway {
	if client == nil {
		panic("datahub_gateway: ArticleCursorGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &ArticleCursorGateway{client: client}
}

// FetchArticlesWithCursor pages the signed-in user's articles, newest first.
//
// No user in the context is "authentication required", the same message the
// driver produced, because the alternative is a page of somebody else's
// articles.
func (g *ArticleCursorGateway) FetchArticlesWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.Article, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	resp, err := g.client.ListArticlesCursor(ctx, connect.NewRequest(&datahubv1.ListArticlesCursorRequest{
		UserId: user.UserID.String(),
		Cursor: timePtrToProto(cursor),
		Limit:  safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("list articles cursor: %w", err)
	}

	articles := make([]*domain.Article, 0, len(resp.Msg.GetArticles()))
	for _, msg := range resp.Msg.GetArticles() {
		article, convErr := userArticleFromProto(msg)
		if convErr != nil {
			return nil, convErr
		}
		articles = append(articles, article)
	}
	return articles, nil
}

// FetchArticleIDsWithCursor is the id-only walk the cached list path uses.
func (g *ArticleCursorGateway) FetchArticleIDsWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]uuid.UUID, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	resp, err := g.client.ListArticleIDsCursor(ctx, connect.NewRequest(&datahubv1.ListArticleIDsCursorRequest{
		UserId: user.UserID.String(),
		Cursor: timePtrToProto(cursor),
		Limit:  safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("list article ids cursor: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(resp.Msg.GetArticleIds()))
	for _, raw := range resp.Msg.GetArticleIds() {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("list article ids cursor returned %q, which is not a uuid: %w", raw, parseErr)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ArticleBatchGateway hydrates article ids into full articles
// (catalog §2.C W3-C5).
type ArticleBatchGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewArticleBatchGateway(client datahubv1connect.DataHubServiceClient) *ArticleBatchGateway {
	if client == nil {
		panic("datahub_gateway: ArticleBatchGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &ArticleBatchGateway{client: client}
}

// FetchArticlesByIDs keeps the requested order and omits unknown ids, which is
// what the callers that render the result positionally rely on.
func (g *ArticleBatchGateway) FetchArticlesByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error) {
	if len(articleIDs) == 0 {
		return []*domain.Article{}, nil
	}

	raw := make([]string, 0, len(articleIDs))
	for _, id := range articleIDs {
		raw = append(raw, id.String())
	}

	resp, err := g.client.BatchGetArticlesByIDs(ctx, connect.NewRequest(&datahubv1.BatchGetArticlesByIDsRequest{
		ArticleIds: raw,
	}))
	if err != nil {
		return nil, fmt.Errorf("batch get articles by ids (%d): %w", len(articleIDs), err)
	}

	articles := make([]*domain.Article, 0, len(resp.Msg.GetArticles()))
	for _, msg := range resp.Msg.GetArticles() {
		article, convErr := userArticleFromProto(msg)
		if convErr != nil {
			return nil, convErr
		}
		articles = append(articles, article)
	}
	return articles, nil
}

// LatestArticleGateway is the newest-article-per-feed read
// (catalog §2.C W3-C7).
type LatestArticleGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewLatestArticleGateway(client datahubv1connect.DataHubServiceClient) *LatestArticleGateway {
	if client == nil {
		panic("datahub_gateway: LatestArticleGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &LatestArticleGateway{client: client}
}

// FetchLatestArticleByFeedID returns (nil, nil) for a feed with no articles,
// which is the ordinary state of a feed registered but not yet crawled.
func (g *LatestArticleGateway) FetchLatestArticleByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error) {
	resp, err := g.client.GetLatestArticleByFeedID(ctx, connect.NewRequest(&datahubv1.GetLatestArticleByFeedIDRequest{
		FeedId: feedID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("get latest article for feed %s: %w", feedID, err)
	}
	return articleContentFromProto(resp.Msg.GetArticle()), nil
}

// ArticleURLLookupGateway resolves an article's source URL within one tenant
// (catalog §2.C W3-C8).
type ArticleURLLookupGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewArticleURLLookupGateway(client datahubv1connect.DataHubServiceClient) *ArticleURLLookupGateway {
	if client == nil {
		panic("datahub_gateway: ArticleURLLookupGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &ArticleURLLookupGateway{client: client}
}

// LookupArticleURL returns ("", nil) when the article is not in the user's
// tenant — deliberately the same answer as for one that does not exist.
func (g *ArticleURLLookupGateway) LookupArticleURL(ctx context.Context, articleID string, userID uuid.UUID) (string, error) {
	if articleID == "" {
		// Preserved from the driver: the Knowledge Trail action path calls
		// this with whatever id the event carried, and an empty one is a
		// missing link rather than a request worth a round trip.
		return "", nil
	}

	resp, err := g.client.LookupArticleURL(ctx, connect.NewRequest(&datahubv1.LookupArticleURLRequest{
		ArticleId: articleID,
		UserId:    userID.String(),
	}))
	if err != nil {
		return "", fmt.Errorf("lookup article url %s: %w", articleID, err)
	}
	return resp.Msg.GetUrl(), nil
}
