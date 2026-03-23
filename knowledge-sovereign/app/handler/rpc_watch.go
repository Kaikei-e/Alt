package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

const (
	watchChannel          = "knowledge_projector"
	watchHeartbeatInterval = 3 * time.Second
)

// WatchProjectorEvents implements server-streaming RPC.
// Sovereign LISTENs on its own DB and pushes notifications to the client.
func (h *SovereignHandler) WatchProjectorEvents(
	ctx context.Context,
	req *connect.Request[sovereignv1.WatchProjectorEventsRequest],
	stream *connect.ServerStream[sovereignv1.ProjectorEventNotification],
) error {
	if h.databaseURL == "" {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("database URL not configured for LISTEN"))
	}

	conn, err := pgx.Connect(ctx, h.databaseURL)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("connect for LISTEN: %w", err))
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+watchChannel); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("LISTEN %s: %w", watchChannel, err))
	}

	slog.Info("WatchProjectorEvents: client connected",
		"projector_name", req.Msg.ProjectorName)

	for {
		waitCtx, cancel := context.WithTimeout(ctx, watchHeartbeatInterval)
		notification, err := conn.WaitForNotification(waitCtx)
		cancel()

		if ctx.Err() != nil {
			slog.Info("WatchProjectorEvents: client disconnected",
				"projector_name", req.Msg.ProjectorName)
			return nil
		}

		if err != nil {
			// Timeout — send heartbeat
			if err := stream.Send(&sovereignv1.ProjectorEventNotification{
				LatestEventSeq: 0,
				OccurredAt:     timestamppb.Now(),
			}); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}
			continue
		}

		// Parse event_seq from notification payload
		var eventSeq int64
		if notification.Payload != "" {
			eventSeq, _ = strconv.ParseInt(notification.Payload, 10, 64)
		}

		if err := stream.Send(&sovereignv1.ProjectorEventNotification{
			LatestEventSeq: eventSeq,
			OccurredAt:     timestamppb.Now(),
		}); err != nil {
			return fmt.Errorf("send notification: %w", err)
		}
	}
}
