package handler

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// GetReadingQueue returns active, non-expired queue items for the user as of
// the caller-supplied timestamp.
func (h *SovereignHandler) GetReadingQueue(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetReadingQueueRequest],
) (*connect.Response[sovereignv1.GetReadingQueueResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if msg.AsOf == nil || !msg.AsOf.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("as_of timestamp is required"))
	}

	items, err := h.readDB.GetActiveReadingQueue(ctx, userID, msg.AsOf.AsTime())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetReadingQueue: %w", err))
	}

	pbItems := make([]*sovereignv1.ReadingQueueEntry, 0, len(items))
	for _, it := range items {
		pbItems = append(pbItems, &sovereignv1.ReadingQueueEntry{
			ArticleId:          it.ArticleID.String(),
			QueuedAt:           timestamppb.New(it.QueuedAt),
			ExpiresAt:          timestamppb.New(it.ExpiresAt),
			QueuePolicyVersion: safeIntToInt32(it.QueuePolicyVersion),
		})
	}

	return connect.NewResponse(&sovereignv1.GetReadingQueueResponse{
		Items: pbItems,
	}), nil
}

// CountReadingSourcesSince returns the count of articles that counted as read
// on or after the caller-supplied timestamp.
func (h *SovereignHandler) CountReadingSourcesSince(
	ctx context.Context,
	req *connect.Request[sovereignv1.CountReadingSourcesSinceRequest],
) (*connect.Response[sovereignv1.CountReadingSourcesSinceResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if msg.Since == nil || !msg.Since.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("since timestamp is required"))
	}

	count, err := h.readDB.CountReadingSourcesSince(ctx, userID, msg.Since.AsTime())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CountReadingSourcesSince: %w", err))
	}

	return connect.NewResponse(&sovereignv1.CountReadingSourcesSinceResponse{
		Count: safeIntToInt32(count),
	}), nil
}

// ListReadingSourcesInWindow returns reading_sources entries across all rule
// versions for the calling user within the requested time window.
func (h *SovereignHandler) ListReadingSourcesInWindow(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListReadingSourcesInWindowRequest],
) (*connect.Response[sovereignv1.ListReadingSourcesInWindowResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if msg.From == nil || !msg.From.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("from timestamp is required"))
	}
	if msg.To == nil || !msg.To.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("to timestamp is required"))
	}
	from := msg.From.AsTime()
	to := msg.To.AsTime()
	if to.Before(from) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("to timestamp must not be before from"))
	}

	sources, err := h.readDB.ListReadingSourcesInWindow(ctx, userID, from, to)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListReadingSourcesInWindow: %w", err))
	}

	entries := make([]*sovereignv1.ReadingSourceEntry, 0, len(sources))
	for _, s := range sources {
		entries = append(entries, &sovereignv1.ReadingSourceEntry{
			ArticleId:       s.ArticleID.String(),
			FirstReadAt:     timestamppb.New(s.FirstReadAt),
			ReadRuleVersion: safeIntToInt32(s.ReadRuleVersion),
		})
	}

	return connect.NewResponse(&sovereignv1.ListReadingSourcesInWindowResponse{
		Entries: entries,
	}), nil
}
