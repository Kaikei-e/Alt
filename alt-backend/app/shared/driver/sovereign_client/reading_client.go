package sovereign_client

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
)

// GetReadingQueue fetches the user's active (non-removed, non-expired as of
// asOf) Read Today queue entries from knowledge-sovereign.
func (c *Client) GetReadingQueue(ctx context.Context, userID uuid.UUID, asOf time.Time) ([]domain.ReadingQueueEntry, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetReadingQueue(ctx, connect.NewRequest(&sovereignv1.GetReadingQueueRequest{
		UserId: userID.String(),
		AsOf:   timestamppb.New(asOf),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetReadingQueue: %w", err)
	}

	out := make([]domain.ReadingQueueEntry, 0, len(resp.Msg.Items))
	for _, pb := range resp.Msg.Items {
		articleID, err := uuid.Parse(pb.GetArticleId())
		if err != nil {
			return nil, fmt.Errorf("sovereign GetReadingQueue invalid article_id %q: %w", pb.GetArticleId(), err)
		}
		var queuedAt, expiresAt time.Time
		if pb.GetQueuedAt() != nil {
			queuedAt = pb.GetQueuedAt().AsTime()
		}
		if pb.GetExpiresAt() != nil {
			expiresAt = pb.GetExpiresAt().AsTime()
		}
		out = append(out, domain.ReadingQueueEntry{
			ArticleID:          articleID,
			QueuedAt:           queuedAt,
			ExpiresAt:          expiresAt,
			QueuePolicyVersion: int(pb.GetQueuePolicyVersion()),
		})
	}
	return out, nil
}

// CountReadingSourcesSince returns the count of articles that counted as read
// on or after since.
func (c *Client) CountReadingSourcesSince(ctx context.Context, userID uuid.UUID, since time.Time) (int, error) {
	if !c.enabled {
		return 0, nil
	}

	resp, err := c.client.CountReadingSourcesSince(ctx, connect.NewRequest(&sovereignv1.CountReadingSourcesSinceRequest{
		UserId: userID.String(),
		Since:  timestamppb.New(since),
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign CountReadingSourcesSince: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}
