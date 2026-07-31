package datahub_gateway

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/orchestrator/driver/models"
	"alt/utils/safeconv"

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
type FeedGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewFeedGateway(client datahubv1connect.DataHubServiceClient) *FeedGateway {
	if client == nil {
		panic("datahub_gateway: FeedGateway requires a DataHubService client — " +
			"a nil client would make every feed list fail identically to a user with no subscriptions " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &FeedGateway{client: client}
}

// RegisterMultipleFeedsWithState upserts a poll's items in one provider
// transaction and reports, per item, whether the row was new.
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

	resp, err := g.client.RegisterFeeds(ctx, connect.NewRequest(&datahubv1.RegisterFeedsRequest{Feeds: items}))
	if err != nil {
		return nil, fmt.Errorf("register %d feeds: %w", len(feeds), err)
	}

	results := make([]domain.FeedRegistrationResult, 0, len(resp.Msg.GetResults()))
	for _, r := range resp.Msg.GetResults() {
		results = append(results, domain.FeedRegistrationResult{FeedID: r.GetFeedId(), Created: r.GetCreated()})
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

func (g *FeedGateway) FetchAllFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	return g.listCursor(ctx, datahubv1.FeedScope_FEED_SCOPE_ALL, cursor, limit, excludeFeedLinkIDs)
}

func (g *FeedGateway) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	return g.listCursor(ctx, datahubv1.FeedScope_FEED_SCOPE_UNREAD, cursor, limit, excludeFeedLinkIDs)
}

// FetchReadFeedsListCursor pages by read_at, so it takes no exclusion list —
// the "hide this source" filter only exists on the two timeline scopes.
func (g *FeedGateway) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	return g.listCursor(ctx, datahubv1.FeedScope_FEED_SCOPE_READ, cursor, limit, nil)
}

func (g *FeedGateway) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	return g.listCursor(ctx, datahubv1.FeedScope_FEED_SCOPE_FAVORITE, cursor, limit, nil)
}

// listCursor refuses to run without a signed-in user rather than sending an
// empty user_id and letting the provider answer InvalidArgument.
//
// The four scopes are all "one person's feeds". A request with no user has no
// meaning to fall back to, and failing here names the missing thing instead of
// surfacing a Connect error from the other side of the network.
func (g *FeedGateway) listCursor(ctx context.Context, scope datahubv1.FeedScope, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user context required to list feeds (%s): %w", scope.String(), err)
	}

	excludes := make([]string, 0, len(excludeFeedLinkIDs))
	for _, id := range excludeFeedLinkIDs {
		excludes = append(excludes, id.String())
	}

	resp, err := g.client.ListFeedsCursor(ctx, connect.NewRequest(&datahubv1.ListFeedsCursorRequest{
		Scope:              scope,
		UserId:             user.UserID.String(),
		Cursor:             timePtrToProto(cursor),
		Limit:              safeconv.Int32(limit),
		ExcludeFeedLinkIds: excludes,
	}))
	if err != nil {
		return nil, fmt.Errorf("list feeds cursor (%s): %w", scope.String(), err)
	}
	return feedModelsFromProto(resp.Msg.GetFeeds()), nil
}

// FetchUnreadFeedsListPage is the legacy offset pager, user-scoped.
func (g *FeedGateway) FetchUnreadFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user context required to list unread feeds page: %w", err)
	}
	return g.listPage(ctx, page, true, user.UserID.String())
}

// FetchFeedsListPage is the same pager without the read filter, and therefore
// without a user.
func (g *FeedGateway) FetchFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	return g.listPage(ctx, page, false, "")
}

func (g *FeedGateway) listPage(ctx context.Context, page int, unreadOnly bool, userID string) ([]*models.Feed, error) {
	resp, err := g.client.ListFeedsPage(ctx, connect.NewRequest(&datahubv1.ListFeedsPageRequest{
		Page:       safeconv.Int32(page),
		UnreadOnly: unreadOnly,
		UserId:     userID,
	}))
	if err != nil {
		return nil, fmt.Errorf("list feeds page %d: %w", page, err)
	}
	return feedModelsFromProto(resp.Msg.GetFeeds()), nil
}

// FetchFeedsList is the unbounded list, which the provider caps at its own
// standing ceiling. Zero is how that is asked for; it does not mean "no
// limit".
func (g *FeedGateway) FetchFeedsList(ctx context.Context) ([]*models.Feed, error) {
	return g.listLimit(ctx, 0)
}

func (g *FeedGateway) FetchFeedsListLimit(ctx context.Context, limit int) ([]*models.Feed, error) {
	return g.listLimit(ctx, limit)
}

func (g *FeedGateway) listLimit(ctx context.Context, limit int) ([]*models.Feed, error) {
	resp, err := g.client.ListFeedsLimit(ctx, connect.NewRequest(&datahubv1.ListFeedsLimitRequest{
		Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("list feeds (limit %d): %w", limit, err)
	}
	return feedModelsFromProto(resp.Msg.GetFeeds()), nil
}

// GetSingleFeed returns the newest feed, or (nil, nil) when there are none.
func (g *FeedGateway) GetSingleFeed(ctx context.Context) (*models.Feed, error) {
	resp, err := g.client.GetSingleFeed(ctx, connect.NewRequest(&datahubv1.GetSingleFeedRequest{}))
	if err != nil {
		return nil, fmt.Errorf("get single feed: %w", err)
	}
	return feedModelFromProto(resp.Msg.GetFeed()), nil
}

// FetchFeedsByFeedLinkID returns the feeds one subscription produced, in the
// row shape the page-cache invalidator consumes.
func (g *FeedGateway) FetchFeedsByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*domain.FeedRow, error) {
	resp, err := g.client.ListFeedsByFeedLinkID(ctx, connect.NewRequest(&datahubv1.ListFeedsByFeedLinkIDRequest{
		FeedLinkId: feedLinkID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("list feeds for feed link %s: %w", feedLinkID, err)
	}

	rows := make([]*domain.FeedRow, 0, len(resp.Msg.GetFeeds()))
	for _, f := range resp.Msg.GetFeeds() {
		rows = append(rows, feedRowFromProto(f))
	}
	return rows, nil
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

func feedRowFromProto(f *datahubv1.Feed) *domain.FeedRow {
	if f == nil {
		return nil
	}
	return &domain.FeedRow{
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
