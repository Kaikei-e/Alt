package datahub_gateway

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/orchestrator/driver/models"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// FeedGateway is the feeds table as alt-backend and alt-harvester see it
// (catalog §2.H).
//
// The methods keep the alt_db names and return orchestrator/driver/models.Feed
// because that is what the feed gateways above them already consume — the
// sanitising and RFC3339 formatting that turns a row into a domain.FeedItem is
// a pure function and stays on this side (ADR-000954 D4). Returning the driver
// model from an anti-corruption layer looks backwards; the alternative is to
// rewrite six gateways and their tests in the same commit that moves a process
// boundary, and then a bisect could not tell a mapping bug from a wiring one.
//
// Where the driver read the signed-in user from the context, these send it as
// an explicit field. The context does not survive the wire, and over Connect
// the peer certificate says "alt-backend" and nothing about whose feeds these
// are.
// feedDataHubClient isolates the data-hub RPCs called by FeedGateway.
type feedDataHubClient interface {
	RegisterFeeds(context.Context, *connect.Request[datahubv1.RegisterFeedsRequest]) (*connect.Response[datahubv1.RegisterFeedsResponse], error)
	GetFeedSummary(context.Context, *connect.Request[datahubv1.GetFeedSummaryRequest]) (*connect.Response[datahubv1.GetFeedSummaryResponse], error)
	GetArticleSummaryByArticleID(context.Context, *connect.Request[datahubv1.GetArticleSummaryByArticleIDRequest]) (*connect.Response[datahubv1.GetArticleSummaryByArticleIDResponse], error)
	GetFeedID(context.Context, *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error)
	SearchFeedsByTitle(context.Context, *connect.Request[datahubv1.SearchFeedsByTitleRequest]) (*connect.Response[datahubv1.SearchFeedsByTitleResponse], error)
	GetRandomFeed(context.Context, *connect.Request[datahubv1.GetRandomFeedRequest]) (*connect.Response[datahubv1.GetRandomFeedResponse], error)
	GetFeedURLsByArticleIDs(context.Context, *connect.Request[datahubv1.GetFeedURLsByArticleIDsRequest]) (*connect.Response[datahubv1.GetFeedURLsByArticleIDsResponse], error)
	BatchGetFeedTitlesByIDs(context.Context, *connect.Request[datahubv1.BatchGetFeedTitlesByIDsRequest]) (*connect.Response[datahubv1.BatchGetFeedTitlesByIDsResponse], error)
	GetInoreaderSummariesByURLs(context.Context, *connect.Request[datahubv1.GetInoreaderSummariesByURLsRequest]) (*connect.Response[datahubv1.GetInoreaderSummariesByURLsResponse], error)
	ListFeedsCursor(context.Context, *connect.Request[datahubv1.ListFeedsCursorRequest]) (*connect.Response[datahubv1.ListFeedsCursorResponse], error)
	ListFeedsPage(context.Context, *connect.Request[datahubv1.ListFeedsPageRequest]) (*connect.Response[datahubv1.ListFeedsPageResponse], error)
	ListFeedsLimit(context.Context, *connect.Request[datahubv1.ListFeedsLimitRequest]) (*connect.Response[datahubv1.ListFeedsLimitResponse], error)
	GetSingleFeed(context.Context, *connect.Request[datahubv1.GetSingleFeedRequest]) (*connect.Response[datahubv1.GetSingleFeedResponse], error)
	ListFeedsByFeedLinkID(context.Context, *connect.Request[datahubv1.ListFeedsByFeedLinkIDRequest]) (*connect.Response[datahubv1.ListFeedsByFeedLinkIDResponse], error)
}

type FeedGateway struct {
	client feedDataHubClient
}

func NewFeedGateway(client datahubv1connect.DataHubServiceClient) *FeedGateway {
	if client == nil {
		panic("datahub_gateway: FeedGateway requires a DataHubService client — " +
			"a nil client would make every feed list fail identically to a user with no subscriptions " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &FeedGateway{client: client}
}

// registerFeedsBatchLimit mirrors data-hub's maxFeedRegistrationBatch; an
// hourly poll routinely exceeds it, so polls are sent as ≤limit chunks. Each
// chunk is one provider transaction — safe because the upsert is idempotent.
const registerFeedsBatchLimit = 2000

// RegisterMultipleFeedsWithState upserts a poll's items and reports, per item,
// whether the row was new. Result order matches the input order.
func (g *FeedGateway) RegisterMultipleFeedsWithState(ctx context.Context, feeds []models.Feed) ([]domain.FeedRegistrationResult, error) {
	if len(feeds) == 0 {
		return nil, nil
	}

	items := make([]*datahubv1.FeedRegistration, 0, len(feeds))
	for _, f := range feeds {
		items = append(items, &datahubv1.FeedRegistration{
			Title:       f.Title,
			Description: f.Description,
			WebsiteUrl:  f.WebsiteURL,
			PubDate:     timeToProto(f.PubDate),
			CreatedAt:   timeToProto(f.CreatedAt),
			UpdatedAt:   timeToProto(f.UpdatedAt),
			FeedLinkId:  f.FeedLinkID,
			OgImageUrl:  f.OgImageURL,
		})
	}

	results := make([]domain.FeedRegistrationResult, 0, len(items))
	for start := 0; start < len(items); start += registerFeedsBatchLimit {
		end := min(start+registerFeedsBatchLimit, len(items))
		resp, err := g.client.RegisterFeeds(ctx, connect.NewRequest(&datahubv1.RegisterFeedsRequest{Feeds: items[start:end]}))
		if err != nil {
			return nil, fmt.Errorf("register feeds %d-%d of %d: %w", start+1, end, len(items), err)
		}
		for _, r := range resp.Msg.GetResults() {
			results = append(results, domain.FeedRegistrationResult{FeedID: r.GetFeedId(), Created: r.GetCreated()})
		}
	}
	return results, nil
}

// RegisterMultipleFeeds is the id-only form the collector job uses.
func (g *FeedGateway) RegisterMultipleFeeds(ctx context.Context, feeds []models.Feed) ([]string, error) {
	results, err := g.RegisterMultipleFeedsWithState(ctx, feeds)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.FeedID)
	}
	return ids, nil
}

// FetchFeedSummary returns (nil, nil) when no summary has been generated.
//
// The driver answered pgx.ErrNoRows for that, and the summarise path read the
// error as "go generate one". Over an RPC the same fact has to be an unset
// field, or every unsummarised article would look like a data plane fault.
func (g *FeedGateway) FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	if feedURL == nil {
		return nil, fmt.Errorf("fetch feed summary: nil url")
	}

	resp, err := g.client.GetFeedSummary(ctx, connect.NewRequest(&datahubv1.GetFeedSummaryRequest{
		FeedUrl: feedURL.String(),
		UserId:  optionalUserID(ctx),
	}))
	if err != nil {
		return nil, fmt.Errorf("get feed summary for %s: %w", feedURL, err)
	}
	return feedSummaryFromProto(resp.Msg.GetSummary()), nil
}

func (g *FeedGateway) FetchArticleSummaryByArticleID(ctx context.Context, articleID string) (*domain.FeedSummary, error) {
	resp, err := g.client.GetArticleSummaryByArticleID(ctx, connect.NewRequest(&datahubv1.GetArticleSummaryByArticleIDRequest{
		ArticleId: articleID,
		UserId:    optionalUserID(ctx),
	}))
	if err != nil {
		return nil, fmt.Errorf("get summary for article %s: %w", articleID, err)
	}
	return feedSummaryFromProto(resp.Msg.GetSummary()), nil
}

// GetFeedIDByURL resolves a feed's id from its website URL (wire capability
// W2-10).
//
// The error travels unwrapped so that the caller's normalization retry can read
// the Connect code off it. That retry — try the literal URL, then its canonical
// form — is the one thing this lookup's caller does that the provider does not,
// and it depends on telling NotFound from a fault.
func (g *FeedGateway) GetFeedIDByURL(ctx context.Context, feedURL string) (string, error) {
	resp, err := g.client.GetFeedID(ctx, connect.NewRequest(&datahubv1.GetFeedIDRequest{
		FeedUrl: feedURL,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.GetFeedId(), nil
}

// SearchFeedsByTitle re-derives the domain.FeedItem shape its caller expects.
//
// pub_date is already the driver's resolved value — it substitutes created_at
// for a NULL — so the published string is formatted from it here without a
// second fallback.
func (g *FeedGateway) SearchFeedsByTitle(ctx context.Context, query, userID string) ([]*domain.FeedItem, error) {
	resp, err := g.client.SearchFeedsByTitle(ctx, connect.NewRequest(&datahubv1.SearchFeedsByTitleRequest{
		Query:  query,
		UserId: userID,
	}))
	if err != nil {
		return nil, fmt.Errorf("search feeds by title %q: %w", query, err)
	}

	items := make([]*domain.FeedItem, 0, len(resp.Msg.GetFeeds()))
	for _, f := range resp.Msg.GetFeeds() {
		published := timeFromProto(f.GetPubDate())
		items = append(items, &domain.FeedItem{
			Title:           f.GetTitle(),
			Description:     f.GetDescription(),
			Link:            f.GetWebsiteUrl(),
			Published:       published.Format(time.RFC3339),
			PublishedParsed: published,
		})
	}
	return items, nil
}

// FetchRandomFeed returns (nil, nil) when nothing is tagged yet, which the Tag
// Trail entry point renders rather than reports.
func (g *FeedGateway) FetchRandomFeed(ctx context.Context) (*domain.Feed, error) {
	resp, err := g.client.GetRandomFeed(ctx, connect.NewRequest(&datahubv1.GetRandomFeedRequest{}))
	if err != nil {
		return nil, fmt.Errorf("get random feed: %w", err)
	}

	f := resp.Msg.GetFeed()
	if f == nil {
		return nil, nil
	}
	id, err := parseUUID(f.GetId())
	if err != nil {
		return nil, fmt.Errorf("random feed id: %w", err)
	}
	return &domain.Feed{
		ID:          id,
		Title:       f.GetTitle(),
		Description: f.GetDescription(),
		WebsiteURL:  f.GetWebsiteUrl(),
	}, nil
}

func (g *FeedGateway) GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]domain.FeedAndArticle, error) {
	if len(articleIDs) == 0 {
		return nil, nil
	}

	resp, err := g.client.GetFeedURLsByArticleIDs(ctx, connect.NewRequest(&datahubv1.GetFeedURLsByArticleIDsRequest{
		ArticleIds: articleIDs,
	}))
	if err != nil {
		return nil, fmt.Errorf("get feed urls for %d articles: %w", len(articleIDs), err)
	}

	pairs := make([]domain.FeedAndArticle, 0, len(resp.Msg.GetPairs()))
	for _, p := range resp.Msg.GetPairs() {
		pairs = append(pairs, domain.FeedAndArticle{
			FeedID:       p.GetFeedId(),
			ArticleID:    p.GetArticleId(),
			URL:          p.GetUrl(),
			FeedTitle:    p.GetFeedTitle(),
			ArticleTitle: p.GetArticleTitle(),
		})
	}
	return pairs, nil
}

// FetchFeedTitlesByIDs omits unknown ids. The Morning Letter enrichment
// defaults the byline itself, and an empty-string entry would claim the feed
// exists with no title.
func (g *FeedGateway) FetchFeedTitlesByIDs(ctx context.Context, feedIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	if len(feedIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}

	raw := make([]string, 0, len(feedIDs))
	for _, id := range feedIDs {
		raw = append(raw, id.String())
	}

	resp, err := g.client.BatchGetFeedTitlesByIDs(ctx, connect.NewRequest(&datahubv1.BatchGetFeedTitlesByIDsRequest{
		FeedIds: raw,
	}))
	if err != nil {
		return nil, fmt.Errorf("batch get feed titles (%d): %w", len(feedIDs), err)
	}

	out := make(map[uuid.UUID]string, len(resp.Msg.GetTitles()))
	for rawID, title := range resp.Msg.GetTitles() {
		id, parseErr := parseUUID(rawID)
		if parseErr != nil {
			return nil, fmt.Errorf("feed title key: %w", parseErr)
		}
		out[id] = title
	}
	return out, nil
}

func (g *FeedGateway) FetchInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*models.InoreaderSummary, error) {
	if len(urls) == 0 {
		return []*models.InoreaderSummary{}, nil
	}

	resp, err := g.client.GetInoreaderSummariesByURLs(ctx, connect.NewRequest(&datahubv1.GetInoreaderSummariesByURLsRequest{
		Urls: urls,
	}))
	if err != nil {
		return nil, fmt.Errorf("get inoreader summaries (%d urls): %w", len(urls), err)
	}

	out := make([]*models.InoreaderSummary, 0, len(resp.Msg.GetSummaries()))
	for _, s := range resp.Msg.GetSummaries() {
		out = append(out, &models.InoreaderSummary{
			ArticleURL:  s.GetArticleUrl(),
			Title:       s.GetTitle(),
			Author:      s.Author,
			Content:     s.GetContent(),
			ContentType: s.GetContentType(),
			PublishedAt: timeFromProto(s.GetPublishedAt()),
			FetchedAt:   timeFromProto(s.GetFetchedAt()),
			InoreaderID: s.GetInoreaderId(),
		})
	}
	return out, nil
}

// optionalUserID maps "no signed-in user" to an absent field rather than an
// empty string.
//
// The two summary reads have always fallen back to an unscoped query for
// service-to-service callers, and the proto says so with an optional field.
// Sending "" instead would make the absence indistinguishable from a caller
// that sent a blank user id.
func optionalUserID(ctx context.Context) *string {
	id := userIDFromContext(ctx)
	if id == "" {
		return nil
	}
	return &id
}
