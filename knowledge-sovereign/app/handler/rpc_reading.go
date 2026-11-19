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
