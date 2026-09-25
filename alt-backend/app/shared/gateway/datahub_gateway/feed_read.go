package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/orchestrator/driver/models"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

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
