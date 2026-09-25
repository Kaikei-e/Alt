package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/utils/safeconv"
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// === Projection reads ===

func (c *Client) GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error) {
	if !c.enabled {
		return nil, "", false, nil
	}

	req := &sovereignv1.GetKnowledgeHomeItemsRequest{
		UserId: userID.String(),
		Cursor: cursor,
		Limit:  safeconv.Int32(limit),
	}
	if filter != nil {
		sourceIDs := make([]string, len(filter.SourceIDs))
		for i, id := range filter.SourceIDs {
			sourceIDs[i] = id.String()
		}
		req.Filter = &sovereignv1.LensFilter{
			QueryText:  filter.QueryText,
			TagIds:     filter.TagNames,
			SourceIds:  sourceIDs,
			TimeWindow: filter.TimeWindow,
		}
	}

	resp, err := c.client.GetKnowledgeHomeItems(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, "", false, fmt.Errorf("sovereign GetKnowledgeHomeItems: %w", err)
	}

	items := make([]domain.KnowledgeHomeItem, len(resp.Msg.Items))
	for i, pb := range resp.Msg.Items {
		items[i] = protoToHomeItem(pb)
	}
	return items, resp.Msg.NextCursor, resp.Msg.HasMore, nil
}

func (c *Client) GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (domain.TodayDigest, error) {
	if !c.enabled {
		return domain.TodayDigest{UserID: userID, DigestDate: date}, nil
	}

	resp, err := c.client.GetTodayDigest(ctx, connect.NewRequest(&sovereignv1.GetTodayDigestRequest{
		UserId: userID.String(),
		Date:   date.Format("2006-01-02"),
	}))
	if err != nil {
		return domain.TodayDigest{}, fmt.Errorf("sovereign GetTodayDigest: %w", err)
	}

	return protoToTodayDigest(resp.Msg.Digest, userID, date), nil
}

func (c *Client) GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]domain.RecallCandidate, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetRecallCandidates(ctx, connect.NewRequest(&sovereignv1.GetRecallCandidatesRequest{
		UserId: userID.String(), Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetRecallCandidates: %w", err)
	}
	candidates := make([]domain.RecallCandidate, len(resp.Msg.Candidates))
	for i, pb := range resp.Msg.Candidates {
		candidates[i] = protoToRecallCandidate(pb)
	}
	return candidates, nil
}

func (c *Client) ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListDistinctUserIDs(ctx, connect.NewRequest(&sovereignv1.ListDistinctUserIDsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListDistinctUserIDs: %w", err)
	}
	ids := make([]uuid.UUID, len(resp.Msg.UserIds))
	for i, s := range resp.Msg.UserIds {
		ids[i] = parseUUID(s)
	}
	return ids, nil
}

func (c *Client) CountNeedToKnowItems(ctx context.Context, userID uuid.UUID, date time.Time) (int, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.CountNeedToKnowItems(ctx, connect.NewRequest(&sovereignv1.CountNeedToKnowItemsRequest{
		UserId: userID.String(), Date: date.Format("2006-01-02"),
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign CountNeedToKnowItems: %w", err)
	}
	return int(resp.Msg.Count), nil
}

func (c *Client) GetProjectionFreshness(ctx context.Context, projectorName string) (*time.Time, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetProjectionFreshness(ctx, connect.NewRequest(&sovereignv1.GetProjectionFreshnessRequest{
		ProjectorName: projectorName,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetProjectionFreshness: %w", err)
	}
	if !resp.Msg.Found || resp.Msg.UpdatedAt == nil {
		return nil, nil
	}
	t := resp.Msg.UpdatedAt.AsTime()
	return &t, nil
}
