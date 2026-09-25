package datahub_capability_gateway

import (
	"context"
	"fmt"

	"alt/shared/driver/alt_db"
)

// ---------------------------------------------------------------------------
// §2.O Automatic full-text fetch groundwork
// ---------------------------------------------------------------------------

type autoFulltextDriver interface {
	ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error)
	CheckArticleExistsByURLForUser(ctx context.Context, url string, userID string) (bool, string, error)
}

// AutoFulltextGateway implements datahub_capability_port.AutoFulltextPort.
type AutoFulltextGateway struct {
	db autoFulltextDriver
}

func NewAutoFulltextGateway(db *alt_db.AltDBRepository) *AutoFulltextGateway {
	return &AutoFulltextGateway{db: db}
}

func (g *AutoFulltextGateway) ListSubscribedUserIDsByFeedLinkID(ctx context.Context, feedLinkID string) ([]string, error) {
	ids, err := g.db.ListSubscribedUserIDsByFeedLinkID(ctx, feedLinkID)
	if err != nil {
		return nil, fmt.Errorf("list subscribed user ids for feed link %s: %w", feedLinkID, err)
	}
	return ids, nil
}

func (g *AutoFulltextGateway) CheckArticleExistsByURLForUser(ctx context.Context, url, userID string) (bool, string, error) {
	exists, articleID, err := g.db.CheckArticleExistsByURLForUser(ctx, url, userID)
	if err != nil {
		return false, "", fmt.Errorf("check article exists for user: %w", err)
	}
	return exists, articleID, nil
}
