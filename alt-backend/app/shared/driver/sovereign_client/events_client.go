package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/utils/safeconv"
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// === Events ===

func (c *Client) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListKnowledgeEvents(ctx, connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
		AfterSeq: afterSeq, Limit: safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListKnowledgeEventsSince: %w", err)
	}
	return protoToEvents(resp.Msg.Events), nil
}

func (c *Client) ListKnowledgeEventsSinceForUser(ctx context.Context, tenantID, userID uuid.UUID, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListKnowledgeEvents(ctx, connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
		AfterSeq: afterSeq,
		Limit:    safeconv.Int32(limit),
		UserId:   userID.String(),
		TenantId: tenantID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListKnowledgeEventsSinceForUser: %w", err)
	}
	return protoToEvents(resp.Msg.Events), nil
}

func (c *Client) GetLatestKnowledgeEventSeqForUser(ctx context.Context, tenantID, userID uuid.UUID) (int64, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetLatestEventSeq(ctx, connect.NewRequest(&sovereignv1.GetLatestEventSeqRequest{
		UserId:   userID.String(),
		TenantId: tenantID.String(),
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetLatestEventSeqForUser: %w", err)
	}
	return resp.Msg.EventSeq, nil
}

// AreArticlesVisibleInLens implements knowledge_event_port.IsArticleVisibleInLensPort.
func (c *Client) AreArticlesVisibleInLens(ctx context.Context, tenantID, userID uuid.UUID, articleIDs []uuid.UUID, filter *domain.KnowledgeHomeLensFilter) (map[uuid.UUID]bool, error) {
	if !c.enabled {
		return map[uuid.UUID]bool{}, nil
	}
	if len(articleIDs) == 0 {
		return map[uuid.UUID]bool{}, nil
	}

	idStrings := make([]string, len(articleIDs))
	for i, id := range articleIDs {
		idStrings[i] = id.String()
	}

	req := &sovereignv1.AreArticlesVisibleInLensRequest{
		TenantId:   tenantID.String(),
		UserId:     userID.String(),
		ArticleIds: idStrings,
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

	resp, err := c.client.AreArticlesVisibleInLens(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("sovereign AreArticlesVisibleInLens: %w", err)
	}

	out := make(map[uuid.UUID]bool, len(resp.Msg.Visibility))
	for idStr, visible := range resp.Msg.Visibility {
		id, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			continue
		}
		out[id] = visible
	}
	return out, nil
}

func (c *Client) AppendKnowledgeEvent(ctx context.Context, event domain.KnowledgeEvent) (int64, error) {
	if !c.enabled {
		// Finding [11]: must match ApplyProjectionMutation / ApplyRecallMutation
		// / ApplyCurationMutation in client.go — a disabled client rejects the
		// call instead of faking a successful append. Returning (0, nil) here
		// made "SOVEREIGN_URL unset" indistinguishable from "event appended,
		// zero-seq dedupe hit" for callers like knowledge_url_backfill_usecase
		// that treat eventSeq==0 as a duplicate-detection signal (ADR-869) —
		// those callers check err != nil first, so ErrSovereignDisabled is
		// caught before ever reaching that eventSeq==0 interpretation.
		return 0, ErrSovereignDisabled
	}
	pbEvent := domainEventToProto(event)
	resp, err := c.client.AppendKnowledgeEvent(ctx, connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
		Event: pbEvent,
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign AppendKnowledgeEvent: %w", err)
	}
	if resp == nil || resp.Msg == nil {
		return 0, nil
	}
	return resp.Msg.GetEventSeq(), nil
}
