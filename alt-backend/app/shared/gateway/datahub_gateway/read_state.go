package datahub_gateway

import (
	"context"
	"fmt"
	"net/url"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// ReadStateGateway is the per-user state kept beside the feed list: what has
// been read, what is subscribed, what is starred (catalog §2.I).
//
// One gateway for four ports — UpdateFeedStatusPort, UpdateArticleStatusPort,
// SubscriptionPort, RegisterFavoriteFeedPort — because they are one question
// asked four ways and every call carries the same tenant. Splitting it would
// mean four Connect clients over one connection and four places to forget the
// user id.
//
// Caching is not here. The 5-second read-state cache and the 30-second
// subscription cache stay in user_read_state_gateway: a TTL is flow
// orchestration and belongs to the caller (ADR-000954 D4), and moving it
// across the boundary would make one process's staleness another process's
// problem.
type ReadStateGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewReadStateGateway(client datahubv1connect.DataHubServiceClient) *ReadStateGateway {
	if client == nil {
		panic("datahub_gateway: ReadStateGateway requires a DataHubService client — " +
			"a nil client would make every read mark, subscription and favourite " +
			"fail identically to a user who has done none of those things " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &ReadStateGateway{client: client}
}

// UpdateFeedStatus marks a feed read. It satisfies
// feed_status_port.UpdateFeedStatusPort.
func (g *ReadStateGateway) UpdateFeedStatus(ctx context.Context, feedURL url.URL, userID uuid.UUID) error {
	_, err := g.client.MarkFeedRead(ctx, connect.NewRequest(&datahubv1.MarkFeedReadRequest{
		FeedUrl: feedURL.String(),
		UserId:  userID.String(),
	}))
	if err != nil {
		return feedAbsenceError(err, fmt.Sprintf("mark feed %q read", feedURL.String()))
	}
	return nil
}

// MarkArticleAsRead marks the feed an article belongs to read. It satisfies
// article_status_port.UpdateArticleStatusPort.
//
// userID is a parameter rather than something this gateway reads from the
// context, and the usecase resolves it — the same shape FeedsReadingStatusUsecase
// has always had. The gateway is a mapper; who is authenticated is the
// usecase's business.
func (g *ReadStateGateway) MarkArticleAsRead(ctx context.Context, articleURL url.URL, userID uuid.UUID) error {
	_, err := g.client.MarkArticleRead(ctx, connect.NewRequest(&datahubv1.MarkArticleReadRequest{
		ArticleUrl: articleURL.String(),
		UserId:     userID.String(),
	}))
	if err != nil {
		return feedAbsenceError(err, fmt.Sprintf("mark article %q read", articleURL.String()))
	}
	return nil
}

// GetReadFeedIDs and GetAllReadFeedIDs return the set shape
// user_read_state_gateway's cache is built around.
//
// The wire carries a list because absence is the meaning there — an unread
// feed is simply not in it — while the caller wants membership lookups. The
// map is rebuilt here rather than pushed onto the caller so that swapping the
// driver for this gateway stays a DI change.
func (g *ReadStateGateway) GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(feedIDs) == 0 {
		return map[uuid.UUID]bool{}, nil
	}

	resp, err := g.client.GetReadFeedIDs(ctx, connect.NewRequest(&datahubv1.GetReadFeedIDsRequest{
		UserId:  userID.String(),
		FeedIds: uuidsToStrings(feedIDs),
	}))
	if err != nil {
		return nil, fmt.Errorf("get read feed ids for user %s: %w", userID, err)
	}
	return readFeedIDSet(resp.Msg.GetReadFeedIds())
}

func (g *ReadStateGateway) GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	resp, err := g.client.GetAllReadFeedIDs(ctx, connect.NewRequest(&datahubv1.GetAllReadFeedIDsRequest{
		UserId: userID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("get all read feed ids for user %s: %w", userID, err)
	}
	return readFeedIDSet(resp.Msg.GetReadFeedIds())
}

// GetUserSubscriptions returns the feed links the user follows. The name is
// the driver method it replaces, so the exclusion list the feed pager builds
// from it needs no edit.
func (g *ReadStateGateway) GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	resp, err := g.client.GetUserSubscribedFeedLinkIDs(ctx, connect.NewRequest(&datahubv1.GetUserSubscribedFeedLinkIDsRequest{
		UserId: userID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("get subscribed feed link ids for user %s: %w", userID, err)
	}

	ids := make([]uuid.UUID, 0, len(resp.Msg.GetFeedLinkIds()))
	for _, raw := range resp.Msg.GetFeedLinkIds() {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("subscribed feed link id: %w", parseErr)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ListSubscriptions, Subscribe and Unsubscribe satisfy
// subscription_port.SubscriptionPort.
func (g *ReadStateGateway) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	resp, err := g.client.ListSubscriptions(ctx, connect.NewRequest(&datahubv1.ListSubscriptionsRequest{
		UserId: userID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("list subscriptions for user %s: %w", userID, err)
	}

	sources := make([]*domain.FeedSource, 0, len(resp.Msg.GetSubscriptions()))
	for _, s := range resp.Msg.GetSubscriptions() {
		sources = append(sources, &domain.FeedSource{
			ID:           s.GetFeedLinkId(),
			URL:          s.GetUrl(),
			IsSubscribed: s.GetIsSubscribed(),
			// Zero for an unfollowed link, because there is no date on which
			// nobody started following it. The screen already renders the
			// zero time as "not following".
			CreatedAt: timeFromProto(s.GetSubscribedAt()),
		})
	}
	return sources, nil
}

func (g *ReadStateGateway) Subscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	_, err := g.client.Subscribe(ctx, connect.NewRequest(&datahubv1.SubscribeRequest{
		UserId:     userID.String(),
		FeedLinkId: feedLinkID.String(),
	}))
	if err != nil {
		return fmt.Errorf("subscribe user %s to feed link %s: %w", userID, feedLinkID, err)
	}
	return nil
}

func (g *ReadStateGateway) Unsubscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	_, err := g.client.Unsubscribe(ctx, connect.NewRequest(&datahubv1.UnsubscribeRequest{
		UserId:     userID.String(),
		FeedLinkId: feedLinkID.String(),
	}))
	if err != nil {
		return fmt.Errorf("unsubscribe user %s from feed link %s: %w", userID, feedLinkID, err)
	}
	return nil
}

// RegisterFavoriteFeed and RemoveFavoriteFeed satisfy
// register_favorite_feed_port.RegisterFavoriteFeedPort.
//
// The URL validation that used to sit in front of these — parse, require a
// scheme — is a pure function and stays with the caller's gateway wrapper
// (ADR-000954 D4).
func (g *ReadStateGateway) RegisterFavoriteFeed(ctx context.Context, feedURL string, userID uuid.UUID) error {
	_, err := g.client.AddFavoriteFeed(ctx, connect.NewRequest(&datahubv1.AddFavoriteFeedRequest{
		FeedUrl: feedURL,
		UserId:  userID.String(),
	}))
	if err != nil {
		return feedAbsenceError(err, fmt.Sprintf("add favorite feed %q", feedURL))
	}
	return nil
}

func (g *ReadStateGateway) RemoveFavoriteFeed(ctx context.Context, feedURL string, userID uuid.UUID) error {
	_, err := g.client.RemoveFavoriteFeed(ctx, connect.NewRequest(&datahubv1.RemoveFavoriteFeedRequest{
		FeedUrl: feedURL,
		UserId:  userID.String(),
	}))
	if err != nil {
		return feedAbsenceError(err, fmt.Sprintf("remove favorite feed %q", feedURL))
	}
	return nil
}

// feedAbsenceError turns the provider's NotFound back into the domain error
// the callers above this gateway already branch on.
//
// It is one function for all four writes because the provider derives NotFound
// the same way in all four (catalog §4-5): a URL that names no feed. Before
// the split, one of them raised domain.ErrFeedNotFound and another raised
// pgx.ErrNoRows for the identical situation, and each caller had learned which
// — this is the point where that stops being something a caller has to know.
//
// Both the domain error and the upstream error stay in the chain: dropping the
// latter would strip the Connect code, and a caller that does not branch on
// domain.ErrFeedNotFound would then report an absence as an internal fault.
func feedAbsenceError(err error, action string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return fmt.Errorf("%s: %w: %w", action, domain.ErrFeedNotFound, err)
	}
	return fmt.Errorf("%s: %w", action, err)
}

func readFeedIDSet(raw []string) (map[uuid.UUID]bool, error) {
	set := make(map[uuid.UUID]bool, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("read feed id: %w", err)
		}
		set[id] = true
	}
	return set, nil
}

func uuidsToStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
