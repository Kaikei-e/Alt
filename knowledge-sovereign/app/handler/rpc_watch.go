package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// WatchProjectorEvents implements server-streaming RPC.
// Sovereign LISTENs on its own DB and pushes notifications to the client.
func (h *SovereignHandler) WatchProjectorEvents(
	ctx context.Context,
	req *connect.Request[sovereignv1.WatchProjectorEventsRequest],
	stream *connect.ServerStream[sovereignv1.WatchProjectorEventsResponse],
) error {
	if h.databaseURL == "" {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("database URL not configured for LISTEN"))
	}

	watcher, err := h.watcherOpener(ctx, h.databaseURL)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("open projector watcher: %w", err))
	}
	defer func() {
		if closeErr := watcher.Close(context.Background()); closeErr != nil {
			slog.Warn("WatchProjectorEvents: close watcher failed", "error", closeErr)
		}
	}()

	slog.Info("WatchProjectorEvents: client connected",
		"projector_name", req.Msg.ProjectorName)

	for {
		payload, isTimeout, err := watcher.WaitForNotification(ctx, sovereign_db.WatchHeartbeatInterval)
		if ctx.Err() != nil {
			slog.Info("WatchProjectorEvents: client disconnected",
				"projector_name", req.Msg.ProjectorName)
			return nil
		}

		if err != nil {
			return fmt.Errorf("WaitForNotification: %w", err)
		}

		if isTimeout {
			// Timeout — send heartbeat
			if err := stream.Send(&sovereignv1.WatchProjectorEventsResponse{
				LatestEventSeq: 0,
				OccurredAt:     timestamppb.Now(),
			}); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}
			continue
		}

		// Parse event_seq from notification payload
		var eventSeq int64
		if payload != "" {
			eventSeq, _ = strconv.ParseInt(payload, 10, 64)
		}

		if err := stream.Send(&sovereignv1.WatchProjectorEventsResponse{
			LatestEventSeq: eventSeq,
			OccurredAt:     timestamppb.Now(),
		}); err != nil {
			return fmt.Errorf("send notification: %w", err)
		}
	}
}
