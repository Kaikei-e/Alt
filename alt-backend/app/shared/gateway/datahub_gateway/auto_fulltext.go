package datahub_gateway

import (
	"context"
	"fmt"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
)

// AutoFulltextGateway is the groundwork for automatic full-text fetch
// (catalog §2.O).
//
// Both capabilities are reachable from here and from nowhere else in the
// repository: the two driver methods behind them have had no caller since they
// were written. They are migrated with the rest of §2.O rather than left on
// the direct alt_db path because Wave 3's exit condition is that alt_db has no
// callers outside cmd/datahub — an unused direct call still fails that check,
// and leaving it would mean the eventual feature is built against a driver
// that is about to be deleted.
type AutoFulltextGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewAutoFulltextGateway(client datahubv1connect.DataHubServiceClient) *AutoFulltextGateway {
	if client == nil {
		panic("datahub_gateway: AutoFulltextGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &AutoFulltextGateway{client: client}
}

// ListSubscribedUserIDsByFeedLinkID returns the users subscribed to a feed
// link, so one fetched article can be fanned out to each of them.
func (g *AutoFulltextGateway) ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error) {
	resp, err := g.client.ListSubscribedUserIDsByFeedLinkID(ctx,
		connect.NewRequest(&datahubv1.ListSubscribedUserIDsByFeedLinkIDRequest{FeedLinkId: feedLinkID}))
	if err != nil {
		return nil, fmt.Errorf("list subscribed user ids for feed link %s: %w", feedLinkID, err)
	}
	return resp.Msg.GetUserIds(), nil
}

// CheckArticleExistsByURLForUser is the tenant-scoped existence check.
func (g *AutoFulltextGateway) CheckArticleExistsByURLForUser(ctx context.Context, url, userID string) (bool, string, error) {
	resp, err := g.client.CheckArticleExistsByURLForUser(ctx,
		connect.NewRequest(&datahubv1.CheckArticleExistsByURLForUserRequest{Url: url, UserId: userID}))
	if err != nil {
		return false, "", fmt.Errorf("check article exists for user: %w", err)
	}
	return resp.Msg.GetExists(), resp.Msg.GetArticleId(), nil
}
