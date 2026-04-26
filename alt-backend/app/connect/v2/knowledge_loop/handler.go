// Package knowledge_loop provides the Connect-RPC handler for KnowledgeLoopService.
// The handler derives user_id / tenant_id exclusively from the JWT-backed user context
// and ignores any corresponding fields in the request body.
package knowledge_loop

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	loopv1 "alt/gen/proto/alt/knowledge/loop/v1"
	"alt/gen/proto/alt/knowledge/loop/v1/knowledgeloopv1connect"
	"alt/port/knowledge_event_port"
	"alt/usecase/knowledge_loop_usecase"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Stream wall-clock constants. Picked to match the Knowledge Home stream contract
// so both subscriptions drain in the same reconnect cadence.
const (
	streamPollInterval   = 5 * time.Second
	streamHeartbeatEvery = 10 * time.Second
	streamStaleTimeout   = 30 * time.Minute
	streamFetchBatchSize = 50
)

// Handler implements knowledgeloopv1connect.KnowledgeLoopServiceHandler.
type Handler struct {
	getUsecase        *knowledge_loop_usecase.GetKnowledgeLoopUsecase
	transitionUsecase *knowledge_loop_usecase.TransitionKnowledgeLoopUsecase
	eventsForUserPort knowledge_event_port.ListKnowledgeEventsForUserPort
	logger            *slog.Logger
}

// Compile-time interface verification.
var _ knowledgeloopv1connect.KnowledgeLoopServiceHandler = (*Handler)(nil)

// NewHandler constructs the Knowledge Loop handler.
//
// eventsForUserPort is optional: when nil, StreamKnowledgeLoopUpdates sends a
// terminal StreamExpired immediately so the client exits the subscription loop
// gracefully instead of hanging. Tests may pass nil; production wiring injects
// the sovereign-backed port.
func NewHandler(
	getUsecase *knowledge_loop_usecase.GetKnowledgeLoopUsecase,
	transitionUsecase *knowledge_loop_usecase.TransitionKnowledgeLoopUsecase,
	eventsForUserPort knowledge_event_port.ListKnowledgeEventsForUserPort,
	logger *slog.Logger,
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		getUsecase:        getUsecase,
		transitionUsecase: transitionUsecase,
		eventsForUserPort: eventsForUserPort,
		logger:            logger,
	}
}

// GetKnowledgeLoop returns foreground entries, surfaces, and session state.
func (h *Handler) GetKnowledgeLoop(
	ctx context.Context,
	req *connect.Request[loopv1.GetKnowledgeLoopRequest],
) (*connect.Response[loopv1.GetKnowledgeLoopResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	fgLimit := 3
	if req.Msg.ForegroundLimit != nil {
		fgLimit = int(*req.Msg.ForegroundLimit)
	}

	result, err := h.getUsecase.Execute(ctx, user.TenantID, user.UserID, req.Msg.LensModeId, fgLimit)
	if err != nil {
		if errors.Is(err, knowledge_loop_usecase.ErrInvalidArgument) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid argument"))
		}
		h.logger.ErrorContext(ctx, "get_knowledge_loop failed", "err", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal"))
	}

	resp := &loopv1.GetKnowledgeLoopResponse{
		ForegroundEntries:     toProtoEntries(result.ForegroundEntries),
		BucketEntries:         toProtoEntries(result.BucketEntries),
		Surfaces:              toProtoSurfaces(result.Surfaces),
		SessionState:          toProtoSessionState(result.SessionState),
		OverallServiceQuality: loopv1.ServiceQuality_SERVICE_QUALITY_FULL,
		GeneratedAt:           timestamppb.Now(),
		ProjectionSeqHiwater:  result.ProjectionSeqHiwater,
	}
	return connect.NewResponse(resp), nil
}

// TransitionKnowledgeLoop validates and records a stage transition with idempotency.
func (h *Handler) TransitionKnowledgeLoop(
	ctx context.Context,
	req *connect.Request[loopv1.TransitionKnowledgeLoopRequest],
) (*connect.Response[loopv1.TransitionKnowledgeLoopResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	in := knowledge_loop_usecase.TransitionInput{
		TenantID:             user.TenantID,
		UserID:               user.UserID,
		LensModeID:           req.Msg.LensModeId,
		ClientTransitionID:   req.Msg.ClientTransitionId,
		EntryKey:             req.Msg.EntryKey,
		FromStage:            req.Msg.FromStage.String(),
		ToStage:              req.Msg.ToStage.String(),
		Trigger:              req.Msg.Trigger.String(),
		ObservedProjRevision: req.Msg.ObservedProjectionRevision,
	}

	result, err := h.transitionUsecase.Execute(ctx, in)
	if err != nil {
		switch {
		case errors.Is(err, knowledge_loop_usecase.ErrInvalidArgument):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid argument"))
		case errors.Is(err, knowledge_loop_usecase.ErrRateLimited):
			// 429 at the BFF so clients honour Retry-After. The warn log gives ops
			// a signal distinct from 5xx while keeping the error envelope terse.
			h.logger.WarnContext(ctx, "transition_knowledge_loop rate limited",
				"user_id", user.UserID, "entry_key", req.Msg.EntryKey)
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("rate limited"))
		case errors.Is(err, context.DeadlineExceeded):
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("deadline exceeded"))
		case errors.Is(err, context.Canceled):
			return nil, connect.NewError(connect.CodeCanceled, errors.New("canceled"))
		case errors.Is(err, knowledge_loop_usecase.ErrUpstreamUnavailable):
			h.logger.WarnContext(ctx, "transition_knowledge_loop upstream unavailable", "err", err)
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("upstream unavailable"))
		default:
			h.logger.ErrorContext(ctx, "transition_knowledge_loop failed", "err", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal"))
		}
	}

	resp := &loopv1.TransitionKnowledgeLoopResponse{
		Accepted: result.Accepted,
	}
	if result.CanonicalEntryKey != nil {
		resp.CanonicalEntryKey = result.CanonicalEntryKey
	}
	if result.Message != nil {
		resp.Message = result.Message
	}
	h.logger.InfoContext(ctx, "alt.knowledge_loop.transition_ok",
		"user_id", user.UserID,
		"entry_key", req.Msg.EntryKey,
		"from", req.Msg.FromStage.String(),
		"to", req.Msg.ToStage.String(),
		"accepted", result.Accepted,
	)
	return connect.NewResponse(resp), nil
}

// StreamKnowledgeLoopUpdates polls knowledge_events scoped to (tenant, user) and
// emits Knowledge Loop stream frames. The handler is read-only: it never mutates
// projection state (immutable-design-guard F3). All frame classification is
// delegated to knowledge_loop_usecase.ClassifyLoopStreamUpdate.
//
// Termination paths:
//   - ctx.Done: client disconnected.
//   - staleTimer (30m): send StreamExpired("stale") and close so the client
//     reconnects with a fresh JWT. Also fires when JWT exp is reached.
//   - repeated event-fetch failures (>=3): send StreamExpired("upstream_error").
func (h *Handler) StreamKnowledgeLoopUpdates(
	ctx context.Context,
	req *connect.Request[loopv1.StreamKnowledgeLoopUpdatesRequest],
	stream *connect.ServerStream[loopv1.StreamKnowledgeLoopUpdatesResponse],
) error {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	if h.eventsForUserPort == nil {
		return sendStreamExpired(stream, "stream not wired")
	}

	lastSeq := int64(0)
	if req.Msg.ResumeFromSeq != nil && *req.Msg.ResumeFromSeq > 0 {
		// Subtract one because the port returns events strictly greater than afterSeq.
		lastSeq = *req.Msg.ResumeFromSeq - 1
	}

	// Send an immediate heartbeat so the HTTP response headers flush without
	// waiting the full poll interval. Without this, nginx holds the first byte
	// for up to streamPollInterval, breaking proxy health checks.
	if err := stream.Send(&loopv1.StreamKnowledgeLoopUpdatesResponse{
		ProjectionSeqHiwater: lastSeq,
	}); err != nil {
		return err
	}

	h.logger.InfoContext(ctx, "alt.knowledge_loop.stream_started",
		"user_id", user.UserID, "tenant_id", user.TenantID, "start_seq", lastSeq)

	updateTicker := time.NewTicker(streamPollInterval)
	defer updateTicker.Stop()
	heartbeatTicker := time.NewTicker(streamHeartbeatEvery)
	defer heartbeatTicker.Stop()
	staleTimer := time.NewTimer(streamStaleTimeout)
	defer staleTimer.Stop()

	// JWT exp enforcement (canonical contract §9 "Stream auth expiry"): the
	// stream must terminate when the JWT embedded in the request expires, not
	// just when the stale timer fires. A dedicated ticker polls the wall clock
	// against the cached ExpiresAt so expiry is detected within ~1s.
	jwtExpiry := user.ExpiresAt
	jwtCheckTicker := time.NewTicker(time.Second)
	defer jwtCheckTicker.Stop()

	consecutiveErrors := 0
	for {
		select {
		case <-ctx.Done():
			h.logger.InfoContext(ctx, "alt.knowledge_loop.stream_ended",
				"user_id", user.UserID, "reason", ctx.Err())
			return nil

		case <-jwtCheckTicker.C:
			if !jwtExpiry.IsZero() && time.Now().After(jwtExpiry) {
				h.logger.InfoContext(ctx, "alt.knowledge_loop.stream_jwt_expired",
					"user_id", user.UserID, "jwt_exp", jwtExpiry)
				return sendStreamExpired(stream, "jwt_expired")
			}

		case <-staleTimer.C:
			return sendStreamExpired(stream, "stale")

		case <-heartbeatTicker.C:
			// projection_seq_hiwater serves as the heartbeat payload; the update
			// oneof is intentionally left unset so clients can distinguish
			// keep-alive frames from real updates.
			if err := stream.Send(&loopv1.StreamKnowledgeLoopUpdatesResponse{
				ProjectionSeqHiwater: lastSeq,
			}); err != nil {
				return err
			}

		case <-updateTicker.C:
			events, fetchErr := h.eventsForUserPort.ListKnowledgeEventsSinceForUser(
				ctx, user.TenantID, user.UserID, lastSeq, streamFetchBatchSize,
			)
			if fetchErr != nil {
				consecutiveErrors++
				h.logger.ErrorContext(ctx, "alt.knowledge_loop.stream_fetch_failed",
					"err", fetchErr, "consecutive_errors", consecutiveErrors)
				if consecutiveErrors >= 3 {
					return sendStreamExpired(stream, "upstream_error")
				}
				continue
			}
			consecutiveErrors = 0

			for i := range events {
				ev := events[i]
				frame, ok := knowledge_loop_usecase.ClassifyLoopStreamUpdate(&ev)
				if !ok {
					if ev.EventSeq > lastSeq {
						lastSeq = ev.EventSeq
					}
					continue
				}
				msg := frameToProto(frame, lastSeq)
				if msg == nil {
					if ev.EventSeq > lastSeq {
						lastSeq = ev.EventSeq
					}
					continue
				}
				if err := stream.Send(msg); err != nil {
					return err
				}
				if ev.EventSeq > lastSeq {
					lastSeq = ev.EventSeq
				}
			}
		}
	}
}

// sendStreamExpired emits a terminal StreamExpired envelope with the given reason.
// Callers return immediately after invoking this; the stream is closed by the
// runtime when the handler returns.
func sendStreamExpired(stream *connect.ServerStream[loopv1.StreamKnowledgeLoopUpdatesResponse], reason string) error {
	expired := &loopv1.StreamKnowledgeLoopUpdatesResponse{
		Update: &loopv1.StreamKnowledgeLoopUpdatesResponse_StreamExpired{
			StreamExpired: &loopv1.StreamExpired{Reason: reason},
		},
	}
	return stream.Send(expired)
}

// frameToProto converts a classified StreamUpdateFrame into the proto oneof.
// Returns nil for unhandled kinds so the caller can skip the send without
// breaking the outer polling loop.
func frameToProto(frame knowledge_loop_usecase.StreamUpdateFrame, seqHiwater int64) *loopv1.StreamKnowledgeLoopUpdatesResponse {
	msg := &loopv1.StreamKnowledgeLoopUpdatesResponse{ProjectionSeqHiwater: seqHiwater}
	switch frame.Kind {
	case knowledge_loop_usecase.StreamUpdateKindAppended:
		msg.Update = &loopv1.StreamKnowledgeLoopUpdatesResponse_Appended{
			Appended: &loopv1.EntryAppended{EntryKey: frame.EntryKey, Revision: frame.Revision},
		}
	case knowledge_loop_usecase.StreamUpdateKindRevised:
		msg.Update = &loopv1.StreamKnowledgeLoopUpdatesResponse_Revised{
			Revised: &loopv1.EntryRevised{EntryKey: frame.EntryKey, Revision: frame.Revision},
		}
	case knowledge_loop_usecase.StreamUpdateKindSuperseded:
		msg.Update = &loopv1.StreamKnowledgeLoopUpdatesResponse_Superseded{
			Superseded: &loopv1.EntrySuperseded{
				EntryKey:    frame.EntryKey,
				NewEntryKey: frame.NewEntryKey,
				Revision:    frame.Revision,
			},
		}
	case knowledge_loop_usecase.StreamUpdateKindWithdrawn:
		msg.Update = &loopv1.StreamKnowledgeLoopUpdatesResponse_Withdrawn{
			Withdrawn: &loopv1.EntryWithdrawn{EntryKey: frame.EntryKey, Revision: frame.Revision},
		}
	case knowledge_loop_usecase.StreamUpdateKindSurfaceRebalanced:
		// Loop transition events affect session state globally; emit on the NOW
		// bucket so clients re-fetch the foreground plane (Continue/Changed/Review
		// are low-frequency in comparison).
		msg.Update = &loopv1.StreamKnowledgeLoopUpdatesResponse_Rebalanced{
			Rebalanced: &loopv1.SurfaceRebalanced{
				SurfaceBucket: loopv1.SurfaceBucket_SURFACE_BUCKET_NOW,
				Revision:      frame.Revision,
			},
		}
	default:
		return nil
	}
	return msg
}

// ============================================================================
// proto mappers
// ============================================================================

func toProtoEntries(in []*domain.KnowledgeLoopEntry) []*loopv1.KnowledgeLoopEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]*loopv1.KnowledgeLoopEntry, 0, len(in))
	for _, e := range in {
		out = append(out, toProtoEntry(e))
	}
	return out
}

func toProtoEntry(e *domain.KnowledgeLoopEntry) *loopv1.KnowledgeLoopEntry {
	if e == nil {
		return nil
	}
	pb := &loopv1.KnowledgeLoopEntry{
		EntryKey:             e.EntryKey,
		SourceItemKey:        e.SourceItemKey,
		ProposedStage:        mapLoopStage(e.ProposedStage),
		SurfaceBucket:        mapSurfaceBucket(e.SurfaceBucket),
		ProjectionRevision:   e.ProjectionRevision,
		ProjectionSeqHiwater: e.ProjectionSeqHiwater,
		SourceEventSeq:       e.SourceEventSeq,
		FreshnessAt:          timestamppb.New(e.FreshnessAt),
		DismissState:         mapDismissState(e.DismissState),
		RenderDepthHint:      mapRenderDepth(e.RenderDepthHint),
		LoopPriority:         mapLoopPriority(e.LoopPriority),
		WhyPrimary: &loopv1.WhyPayload{
			Kind: mapWhyKind(e.WhyKind),
			Text: e.WhyText,
		},
		ArtifactVersionRef: &loopv1.ArtifactVersionRef{
			SummaryVersionId: e.ArtifactVersionRef.SummaryVersionID,
			TagSetVersionId:  e.ArtifactVersionRef.TagSetVersionID,
			LensVersionId:    e.ArtifactVersionRef.LensVersionID,
		},
	}
	if e.WhyConfidence != nil {
		pb.WhyPrimary.Confidence = e.WhyConfidence
	}
	for _, ref := range e.WhyEvidenceRefs {
		pb.WhyEvidenceRefs = append(pb.WhyEvidenceRefs, &loopv1.EvidenceRef{
			RefId: ref.RefID,
			Label: ref.Label,
		})
	}
	if e.SourceObservedAt != nil {
		pb.SourceObservedAt = timestamppb.New(*e.SourceObservedAt)
	}
	if e.SupersededByEntryKey != nil {
		pb.SupersededByEntryKey = e.SupersededByEntryKey
	}
	pb.ChangeSummary = decodeChangeSummary(e.ChangeSummary)
	pb.ContinueContext = decodeContinueContext(e.ContinueContext)
	pb.DecisionOptions = decodeDecisionOptions(e.DecisionOptions)
	pb.ActTargets = decodeActTargets(e.ActTargets)
	return pb
}

// ============================================================================
// JSONB → proto decoders
//
// These decoders tolerate malformed payloads (swallow json.Unmarshal errors,
// return nil/empty). The projector and sovereign storage guarantee the shape,
// but a stale row or a migration-in-flight row should not crash the handler.
// ============================================================================

func decodeChangeSummary(b []byte) *loopv1.ChangeSummary {
	if len(b) == 0 {
		return nil
	}
	var raw struct {
		Summary               string   `json:"summary"`
		ChangedFields         []string `json:"changed_fields"`
		PreviousEntryKey      *string  `json:"previous_entry_key"`
		AddedPhrases          []string `json:"added_phrases"`
		RemovedPhrases        []string `json:"removed_phrases"`
		UnchangedPhrasesCount *uint32  `json:"unchanged_phrases_count"`
		AddedTags             []string `json:"added_tags"`
		RemovedTags           []string `json:"removed_tags"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	return &loopv1.ChangeSummary{
		Summary:               raw.Summary,
		ChangedFields:         raw.ChangedFields,
		PreviousEntryKey:      raw.PreviousEntryKey,
		AddedPhrases:          raw.AddedPhrases,
		RemovedPhrases:        raw.RemovedPhrases,
		UnchangedPhrasesCount: raw.UnchangedPhrasesCount,
		AddedTags:             raw.AddedTags,
		RemovedTags:           raw.RemovedTags,
	}
}

func decodeContinueContext(b []byte) *loopv1.ContinueContext {
	if len(b) == 0 {
		return nil
	}
	var raw struct {
		Summary            string   `json:"summary"`
		RecentActionLabels []string `json:"recent_action_labels"`
		LastInteractedAt   *string  `json:"last_interacted_at"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	pb := &loopv1.ContinueContext{
		Summary:            raw.Summary,
		RecentActionLabels: raw.RecentActionLabels,
	}
	if raw.LastInteractedAt != nil {
		if t, err := time.Parse(time.RFC3339, *raw.LastInteractedAt); err == nil {
			pb.LastInteractedAt = timestamppb.New(t)
		}
	}
	return pb
}

func decodeDecisionOptions(b []byte) []*loopv1.DecisionOption {
	if len(b) == 0 {
		return nil
	}
	var raw []struct {
		ActionID string  `json:"action_id"`
		Intent   string  `json:"intent"`
		Label    *string `json:"label"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	out := make([]*loopv1.DecisionOption, 0, len(raw))
	for _, r := range raw {
		out = append(out, &loopv1.DecisionOption{
			ActionId: r.ActionID,
			Intent:   mapDecisionIntent(r.Intent),
			Label:    r.Label,
		})
	}
	return out
}

func decodeActTargets(b []byte) []*loopv1.ActTarget {
	if len(b) == 0 {
		return nil
	}
	var raw []struct {
		TargetType string  `json:"target_type"`
		TargetRef  string  `json:"target_ref"`
		Route      *string `json:"route"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	out := make([]*loopv1.ActTarget, 0, len(raw))
	for _, r := range raw {
		out = append(out, &loopv1.ActTarget{
			TargetType: mapActTargetType(r.TargetType),
			TargetRef:  r.TargetRef,
			Route:      r.Route,
		})
	}
	return out
}

func mapDecisionIntent(s string) loopv1.DecisionIntent {
	switch s {
	case "open":
		return loopv1.DecisionIntent_DECISION_INTENT_OPEN
	case "ask":
		return loopv1.DecisionIntent_DECISION_INTENT_ASK
	case "save":
		return loopv1.DecisionIntent_DECISION_INTENT_SAVE
	case "compare":
		return loopv1.DecisionIntent_DECISION_INTENT_COMPARE
	case "revisit":
		return loopv1.DecisionIntent_DECISION_INTENT_REVISIT
	case "snooze":
		return loopv1.DecisionIntent_DECISION_INTENT_SNOOZE
	default:
		return loopv1.DecisionIntent_DECISION_INTENT_UNSPECIFIED
	}
}

func mapActTargetType(s string) loopv1.ActTargetType {
	switch s {
	case "article":
		return loopv1.ActTargetType_ACT_TARGET_TYPE_ARTICLE
	case "ask":
		return loopv1.ActTargetType_ACT_TARGET_TYPE_ASK
	case "recap":
		return loopv1.ActTargetType_ACT_TARGET_TYPE_RECAP
	case "diff":
		return loopv1.ActTargetType_ACT_TARGET_TYPE_DIFF
	case "cluster":
		return loopv1.ActTargetType_ACT_TARGET_TYPE_CLUSTER
	default:
		return loopv1.ActTargetType_ACT_TARGET_TYPE_UNSPECIFIED
	}
}

func toProtoSurfaces(in []*domain.KnowledgeLoopSurface) []*loopv1.SurfaceState {
	if len(in) == 0 {
		return nil
	}
	out := make([]*loopv1.SurfaceState, 0, len(in))
	for _, s := range in {
		out = append(out, &loopv1.SurfaceState{
			SurfaceBucket:        mapSurfaceBucket(s.SurfaceBucket),
			PrimaryEntryKey:      s.PrimaryEntryKey,
			SecondaryEntryKeys:   s.SecondaryEntryKeys,
			ProjectionRevision:   s.ProjectionRevision,
			ProjectionSeqHiwater: s.ProjectionSeqHiwater,
			FreshnessAt:          timestamppb.New(s.FreshnessAt),
			ServiceQuality:       mapServiceQuality(s.ServiceQuality),
		})
	}
	return out
}

func toProtoSessionState(s *domain.KnowledgeLoopSessionState) *loopv1.KnowledgeLoopSessionState {
	if s == nil {
		return nil
	}
	pb := &loopv1.KnowledgeLoopSessionState{
		CurrentStage:          mapLoopStage(s.CurrentStage),
		CurrentStageEnteredAt: timestamppb.New(s.CurrentStageEnteredAt),
		FocusedEntryKey:       s.FocusedEntryKey,
		ForegroundEntryKey:    s.ForegroundEntryKey,
		LastObservedEntryKey:  s.LastObservedEntryKey,
		LastOrientedEntryKey:  s.LastOrientedEntryKey,
		LastDecidedEntryKey:   s.LastDecidedEntryKey,
		LastActedEntryKey:     s.LastActedEntryKey,
		LastReturnedEntryKey:  s.LastReturnedEntryKey,
		LastDeferredEntryKey:  s.LastDeferredEntryKey,
		ProjectionRevision:    s.ProjectionRevision,
		ProjectionSeqHiwater:  s.ProjectionSeqHiwater,
	}
	return pb
}

func mapLoopStage(s domain.LoopStage) loopv1.LoopStage {
	switch s {
	case domain.LoopStageObserve:
		return loopv1.LoopStage_LOOP_STAGE_OBSERVE
	case domain.LoopStageOrient:
		return loopv1.LoopStage_LOOP_STAGE_ORIENT
	case domain.LoopStageDecide:
		return loopv1.LoopStage_LOOP_STAGE_DECIDE
	case domain.LoopStageAct:
		return loopv1.LoopStage_LOOP_STAGE_ACT
	default:
		return loopv1.LoopStage_LOOP_STAGE_UNSPECIFIED
	}
}

func mapSurfaceBucket(b domain.SurfaceBucket) loopv1.SurfaceBucket {
	switch b {
	case domain.SurfaceNow:
		return loopv1.SurfaceBucket_SURFACE_BUCKET_NOW
	case domain.SurfaceContinue:
		return loopv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	case domain.SurfaceChanged:
		return loopv1.SurfaceBucket_SURFACE_BUCKET_CHANGED
	case domain.SurfaceReview:
		return loopv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	default:
		return loopv1.SurfaceBucket_SURFACE_BUCKET_UNSPECIFIED
	}
}

func mapDismissState(d domain.DismissState) loopv1.DismissState {
	switch d {
	case domain.DismissActive:
		return loopv1.DismissState_DISMISS_STATE_ACTIVE
	case domain.DismissDeferred:
		return loopv1.DismissState_DISMISS_STATE_DEFERRED
	case domain.DismissDismissed:
		return loopv1.DismissState_DISMISS_STATE_DISMISSED
	case domain.DismissCompleted:
		return loopv1.DismissState_DISMISS_STATE_COMPLETED
	default:
		return loopv1.DismissState_DISMISS_STATE_UNSPECIFIED
	}
}

func mapRenderDepth(d domain.RenderDepthHint) loopv1.RenderDepthHint {
	switch d {
	case domain.RenderDepthFlat:
		return loopv1.RenderDepthHint_RENDER_DEPTH_HINT_FLAT
	case domain.RenderDepthLight:
		return loopv1.RenderDepthHint_RENDER_DEPTH_HINT_LIGHT
	case domain.RenderDepthStrong:
		return loopv1.RenderDepthHint_RENDER_DEPTH_HINT_STRONG
	case domain.RenderDepthCritical:
		return loopv1.RenderDepthHint_RENDER_DEPTH_HINT_CRITICAL
	default:
		return loopv1.RenderDepthHint_RENDER_DEPTH_HINT_UNSPECIFIED
	}
}

func mapLoopPriority(p domain.LoopPriority) loopv1.LoopPriority {
	switch p {
	case domain.LoopPriorityCritical:
		return loopv1.LoopPriority_LOOP_PRIORITY_CRITICAL
	case domain.LoopPriorityContinuing:
		return loopv1.LoopPriority_LOOP_PRIORITY_CONTINUING
	case domain.LoopPriorityConfirm:
		return loopv1.LoopPriority_LOOP_PRIORITY_CONFIRM
	case domain.LoopPriorityReference:
		return loopv1.LoopPriority_LOOP_PRIORITY_REFERENCE
	default:
		return loopv1.LoopPriority_LOOP_PRIORITY_UNSPECIFIED
	}
}

func mapWhyKind(k domain.WhyKind) loopv1.WhyKind {
	switch k {
	case domain.WhyKindSource:
		return loopv1.WhyKind_WHY_KIND_SOURCE
	case domain.WhyKindPattern:
		return loopv1.WhyKind_WHY_KIND_PATTERN
	case domain.WhyKindRecall:
		return loopv1.WhyKind_WHY_KIND_RECALL
	case domain.WhyKindChange:
		return loopv1.WhyKind_WHY_KIND_CHANGE
	default:
		return loopv1.WhyKind_WHY_KIND_UNSPECIFIED
	}
}

func mapServiceQuality(q domain.LoopServiceQuality) loopv1.ServiceQuality {
	switch q {
	case domain.LoopQualityFull:
		return loopv1.ServiceQuality_SERVICE_QUALITY_FULL
	case domain.LoopQualityDegraded:
		return loopv1.ServiceQuality_SERVICE_QUALITY_DEGRADED
	case domain.LoopQualityFallback:
		return loopv1.ServiceQuality_SERVICE_QUALITY_FALLBACK
	default:
		return loopv1.ServiceQuality_SERVICE_QUALITY_UNSPECIFIED
	}
}
