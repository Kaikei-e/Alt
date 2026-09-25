package knowledge_home

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
)

// StreamKnowledgeHomeUpdates streams real-time updates for the home feed.
func (h *Handler) StreamKnowledgeHomeUpdates(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.StreamKnowledgeHomeUpdatesRequest],
	stream *connect.ServerStream[knowledgehomev1.StreamKnowledgeHomeUpdatesResponse],
) error {
	if err := h.streamFlagGuard(ctx); err != nil {
		return err
	}
	user, _ := middleware.GetUserContext(ctx)

	var lensIDPtr *uuid.UUID
	var lensIDStr string
	if req.Msg.LensId != nil && *req.Msg.LensId != "" {
		parsedLensID, err := parseUUID(*req.Msg.LensId, "lens_id")
		if err != nil {
			return err
		}
		lensIDPtr = &parsedLensID
		lensIDStr = parsedLensID.String()
	}

	// Resolve the lens once per connection. The resolved filter is held for
	// the lifetime of the stream so the subscriber sees a stable view —
	// switching lenses requires a new connection (matches the unary
	// GetKnowledgeHome contract).
	var lensFilter *domain.KnowledgeHomeLensFilter
	if h.resolveLensPort != nil {
		resolved, resolveErr := h.resolveLensPort.ResolveKnowledgeHomeLens(ctx, user.UserID, lensIDPtr)
		if resolveErr != nil {
			// When the caller named a specific lens that we cannot resolve,
			// surface NotFound. PermissionDenied would let an attacker probe
			// for lens existence; NotFound treats unknown and unauthorized
			// the same way.
			if lensIDPtr != nil {
				h.logger.WarnContext(ctx, "alt.knowledge_home.stream_lens_resolve_failed",
					"user_id", user.UserID, "lens_id", lensIDStr, "error", resolveErr)
				return connect.NewError(connect.CodeNotFound,
					fmt.Errorf("lens not found"))
			}
			// No explicit lens requested — fall back to no filter rather
			// than fail the stream.
			h.logger.WarnContext(ctx, "alt.knowledge_home.stream_default_lens_resolve_failed",
				"user_id", user.UserID, "error", resolveErr)
		} else {
			lensFilter = resolved
		}
	}

	lastSeq, err := h.initialStreamSeq(ctx, user.TenantID, user.UserID)
	if err != nil {
		return errorhandler.HandleUpstreamError(ctx, h.logger, err, "StreamKnowledgeHomeUpdates")
	}

	if h.metrics != nil {
		h.metrics.StreamConnectionsTotal.Add(ctx, 1)
		if h.metrics.Snapshot != nil {
			h.metrics.Snapshot.RecordStreamConnection()
		}
	}

	recordDelivery := func() {
		if h.metrics != nil {
			h.metrics.StreamDeliveriesTotal.Add(ctx, 1)
			if h.metrics.Snapshot != nil {
				h.metrics.Snapshot.RecordStreamDelivery()
			}
		}
	}

	h.logger.InfoContext(ctx, "alt.knowledge_home.stream_started",
		"user_id", user.UserID,
		"tenant_id", user.TenantID,
		"lens_id", lensIDStr,
		"lens_resolved", lensFilter != nil,
		"start_seq", lastSeq)

	// Send immediate heartbeat so the client receives the first byte instantly.
	// Without this, the first Send() is delayed until updateTicker (5s) or
	// heartbeatTicker (10s) fires, causing 5-10s upstream header time in nginx.
	if err := stream.Send(&knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
		EventType:  "heartbeat",
		OccurredAt: time.Now().Format(time.RFC3339),
	}); err != nil {
		return err
	}

	updateTicker := time.NewTicker(5 * time.Second)
	defer updateTicker.Stop()

	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	// Close long-lived idle connections after timeout.
	staleTimer := time.NewTimer(30 * time.Minute)
	defer staleTimer.Stop()

	var consecutiveErrors int

	for {
		select {
		case <-ctx.Done():
			if h.metrics != nil {
				h.metrics.StreamDisconnectsTotal.Add(ctx, 1)
				if h.metrics.Snapshot != nil {
					h.metrics.Snapshot.RecordStreamDisconnect()
				}
			}
			h.logger.InfoContext(ctx, "alt.knowledge_home.stream_ended",
				"user_id", user.UserID, "reason", ctx.Err())
			return nil

		case <-staleTimer.C:
			// Notify client to reconnect with jitter after expiration.
			reconnectMs := int32(5000 + time.Now().UnixMilli()%3000)
			if err := stream.Send(&knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
				EventType:        "stream_expired",
				OccurredAt:       time.Now().Format(time.RFC3339),
				ReconnectAfterMs: &reconnectMs,
			}); err != nil {
				return err
			}
			recordDelivery()
			h.logger.InfoContext(ctx, "alt.knowledge_home.stream_expired",
				"user_id", user.UserID, "reconnect_ms", reconnectMs)
			return nil

		case <-updateTicker.C:
			if h.eventsForUserPort == nil {
				continue
			}
			events, err := h.eventsForUserPort.ListKnowledgeEventsSinceForUser(ctx, user.TenantID, user.UserID, lastSeq, 50)
			if err != nil {
				consecutiveErrors++
				h.logger.ErrorContext(ctx, "stream: failed to fetch events", "error", err, "consecutive_errors", consecutiveErrors)
				// Fall back to unary queries after repeated fetch failures.
				if consecutiveErrors >= 3 {
					reconnectMs := int32(10000)
					if err := stream.Send(&knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
						EventType:        "fallback_to_unary",
						OccurredAt:       time.Now().Format(time.RFC3339),
						ReconnectAfterMs: &reconnectMs,
					}); err == nil {
						recordDelivery()
					}
					return nil
				}
				continue
			}
			consecutiveErrors = 0

			// Apply lens filter at delivery time. The events are already
			// scoped to (tenant, user) by the DB query above; lens filter
			// drops article-aggregate events whose underlying article would
			// not appear in the subscriber's lens-filtered Home view. This
			// keeps the projector reproject-safe — lens preference never
			// influences the projection.
			//
			// `lastSeq` advances over the *fetched* range (including dropped
			// events) so the next tick does not re-fetch what we already
			// evaluated.
			highestFetchedSeq := lastSeq
			if len(events) > 0 {
				highestFetchedSeq = events[len(events)-1].EventSeq
			}
			events = h.filterEventsByLens(ctx, events, user.TenantID, user.UserID, lensFilter)
			// Advance the cursor over dropped events too so the next tick
			// does not re-evaluate them.
			if highestFetchedSeq > lastSeq && len(events) == 0 {
				lastSeq = highestFetchedSeq
			}

			coalesced := coalesceStreamEvents(events)

			// High-water mark: if too many distinct items changed, send a single
			// digest_changed so the client re-fetches instead of processing N updates.
			if len(coalesced) > streamHighWaterMark {
				update := &knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
					EventType:  "digest_changed",
					OccurredAt: time.Now().Format(time.RFC3339),
				}
				if err := stream.Send(update); err != nil {
					return err
				}
				recordDelivery()
				lastSeq = events[len(events)-1].EventSeq
				continue
			}

			for _, event := range coalesced {
				update := buildStreamResponse(event)
				h.enrichRecallChangedUpdate(ctx, user.UserID, update)
				if err := stream.Send(update); err != nil {
					return err
				}
				recordDelivery()
				if h.metrics != nil {
					h.metrics.StreamUpdateLagSeconds.Record(ctx, time.Since(event.OccurredAt).Seconds())
				}
				if event.EventSeq > lastSeq {
					lastSeq = event.EventSeq
				}
			}

		case <-heartbeatTicker.C:
			if err := stream.Send(&knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
				EventType:  "heartbeat",
				OccurredAt: time.Now().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
	}
}

func (h *Handler) initialStreamSeq(ctx context.Context, tenantID, userID uuid.UUID) (int64, error) {
	if h.latestSeqPort == nil {
		return 0, nil
	}

	return h.latestSeqPort.GetLatestKnowledgeEventSeqForUser(ctx, tenantID, userID)
}

// filterEventsByLens drops article-aggregate events whose underlying article
// is not visible in the subscriber's lens-filtered Home view. Non-article
// aggregates (home_session, recap) are user-scoped at the DB layer and are
// always passed through.
//
// Fail-closed semantics: if the visibility check fails, article events are
// dropped for this tick — never delivered. Non-article events still flow.
func (h *Handler) filterEventsByLens(
	ctx context.Context,
	events []domain.KnowledgeEvent,
	tenantID, userID uuid.UUID,
	filter *domain.KnowledgeHomeLensFilter,
) []domain.KnowledgeEvent {
	if filter == nil || h.lensVisibilityPort == nil || len(events) == 0 {
		return events
	}

	seen := make(map[uuid.UUID]struct{}, len(events))
	articleIDs := make([]uuid.UUID, 0, len(events))
	for _, e := range events {
		if e.AggregateType != domain.AggregateArticle {
			continue
		}
		id, err := uuid.Parse(e.AggregateID)
		if err != nil {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		articleIDs = append(articleIDs, id)
	}

	if len(articleIDs) == 0 {
		return events
	}

	visibility, err := h.lensVisibilityPort.AreArticlesVisibleInLens(ctx, tenantID, userID, articleIDs, filter)
	if err != nil {
		h.logger.WarnContext(ctx, "alt.knowledge_home.stream_lens_visibility_failed",
			"user_id", userID, "tenant_id", tenantID, "error", err)
		return dropArticleAggregates(events)
	}

	return applyLensVisibility(events, visibility)
}

// streamFlagGuard checks authentication and feature flag for stream access.
// Extracted to allow unit testing without a real ServerStream.
func (h *Handler) streamFlagGuard(ctx context.Context) error {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagStreamUpdates, user.UserID) {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("stream updates is not enabled for this user"))
	}
	return nil
}

func (h *Handler) enrichRecallChangedUpdate(
	ctx context.Context,
	userID uuid.UUID,
	update *knowledgehomev1.StreamKnowledgeHomeUpdatesResponse,
) {
	if update == nil || update.EventType != "recall_changed" || update.RecallChange == nil || h.recallRailUsecase == nil {
		return
	}

	candidates, err := h.recallRailUsecase.Execute(ctx, userID, 5)
	if err != nil {
		h.logger.ErrorContext(ctx, "stream: failed to fetch recall candidates", "error", err)
		return
	}

	for _, candidate := range candidates {
		if candidate.ItemKey == update.RecallChange.ItemKey {
			update.RecallChange = convertRecallCandidateToProto(candidate)
			return
		}
	}
}
