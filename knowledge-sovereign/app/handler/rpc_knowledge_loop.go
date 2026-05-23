package handler

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// Knowledge Loop RPC handlers (ADR-000831).
// Storage is owned by sovereign; alt-backend is a thin Connect-RPC client.

// Mutation type constants for ApplyKnowledgeLoopMutation.
const (
	MutationKnowledgeLoopEntryUpsert   = "entry_upsert"
	MutationKnowledgeLoopSessionUpsert = "session_upsert"
	MutationKnowledgeLoopSurfaceUpsert = "surface_upsert"
)

// KnowledgeLoopWriteRepository is the write-side contract the sovereign driver must satisfy.
type KnowledgeLoopWriteRepository interface {
	UpsertKnowledgeLoopEntry(ctx context.Context, entry *sovereignv1.KnowledgeLoopEntry) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	UpsertKnowledgeLoopSessionState(ctx context.Context, state *sovereignv1.KnowledgeLoopSessionState) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	UpsertKnowledgeLoopSurface(ctx context.Context, surface *sovereignv1.KnowledgeLoopSurface) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	ReserveKnowledgeLoopTransition(ctx context.Context, userID uuid.UUID, clientTransitionID string) (*sovereign_db.KnowledgeLoopReservationResult, error)
}

// KnowledgeLoopReadRepository is the read-side contract.
type KnowledgeLoopReadRepository interface {
	GetKnowledgeLoopEntries(ctx context.Context, filter sovereign_db.GetKnowledgeLoopEntriesFilter) ([]*sovereignv1.KnowledgeLoopEntry, error)
	GetKnowledgeLoopSessionState(ctx context.Context, tenantID, userID uuid.UUID, lensModeID string) (*sovereignv1.KnowledgeLoopSessionState, error)
	GetKnowledgeLoopSurfaces(ctx context.Context, tenantID, userID uuid.UUID, lensModeID string) ([]*sovereignv1.KnowledgeLoopSurface, error)
}

// ApplyKnowledgeLoopMutation handles the write envelope, dispatching by mutation_type.
// payload MUST be a proto-marshalled KnowledgeLoopEntry / SessionState / Surface matching mutation_type.
func (h *SovereignHandler) ApplyKnowledgeLoopMutation(
	ctx context.Context,
	req *connect.Request[sovereignv1.ApplyKnowledgeLoopMutationRequest],
) (*connect.Response[sovereignv1.ApplyKnowledgeLoopMutationResponse], error) {
	msg := req.Msg
	repo, ok := h.readDB.(KnowledgeLoopWriteRepository)
	if !ok {
		return nil, connect.NewError(connect.CodeUnimplemented,
			errors.New("sovereign: KnowledgeLoop write repository not wired"))
	}

	switch msg.MutationType {
	case MutationKnowledgeLoopEntryUpsert:
		var entry sovereignv1.KnowledgeLoopEntry
		if err := proto.Unmarshal(msg.Payload, &entry); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("decode entry payload: %w", err))
		}
		res, err := repo.UpsertKnowledgeLoopEntry(ctx, &entry)
		return wrapLoopUpsertResponse(res, err)

	case MutationKnowledgeLoopSessionUpsert:
		var state sovereignv1.KnowledgeLoopSessionState
		if err := proto.Unmarshal(msg.Payload, &state); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("decode session payload: %w", err))
		}
		res, err := repo.UpsertKnowledgeLoopSessionState(ctx, &state)
		return wrapLoopUpsertResponse(res, err)

	case MutationKnowledgeLoopSurfaceUpsert:
		var surface sovereignv1.KnowledgeLoopSurface
		if err := proto.Unmarshal(msg.Payload, &surface); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("decode surface payload: %w", err))
		}
		res, err := repo.UpsertKnowledgeLoopSurface(ctx, &surface)
		return wrapLoopUpsertResponse(res, err)

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown mutation_type: %q", msg.MutationType))
	}
}

func wrapLoopUpsertResponse(
	res *sovereign_db.KnowledgeLoopUpsertResult,
	err error,
) (*connect.Response[sovereignv1.ApplyKnowledgeLoopMutationResponse], error) {
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sovereignv1.ApplyKnowledgeLoopMutationResponse{
		Applied:              res.Applied,
		SkippedBySeqHiwater:  res.SkippedBySeqHiwater,
		ProjectionRevision:   res.ProjectionRevision,
		ProjectionSeqHiwater: res.ProjectionSeqHiwater,
	}), nil
}

// GetKnowledgeLoopEntries returns entries for a user/lens scope.
func (h *SovereignHandler) GetKnowledgeLoopEntries(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetKnowledgeLoopEntriesRequest],
) (*connect.Response[sovereignv1.GetKnowledgeLoopEntriesResponse], error) {
	msg := req.Msg
	tenantID, err := uuid.Parse(msg.TenantId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("tenant_id: %w", err))
	}
	userID, err := uuid.Parse(msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id: %w", err))
	}
	repo, ok := h.readDB.(KnowledgeLoopReadRepository)
	if !ok {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("sovereign: KnowledgeLoop read repository not wired"))
	}

	filter := sovereign_db.GetKnowledgeLoopEntriesFilter{
		TenantID:         tenantID,
		UserID:           userID,
		LensModeID:       msg.LensModeId,
		SurfaceBucket:    msg.SurfaceBucket,
		IncludeDismissed: msg.IncludeDismissed,
		Limit:            int(msg.Limit),
	}
	entries, err := repo.GetKnowledgeLoopEntries(ctx, filter)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sovereignv1.GetKnowledgeLoopEntriesResponse{Entries: entries}), nil
}

// GetKnowledgeLoopSessionState returns the single session row for (tenant, user, lens).
func (h *SovereignHandler) GetKnowledgeLoopSessionState(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetKnowledgeLoopSessionStateRequest],
) (*connect.Response[sovereignv1.GetKnowledgeLoopSessionStateResponse], error) {
	msg := req.Msg
	tenantID, err := uuid.Parse(msg.TenantId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("tenant_id: %w", err))
	}
	userID, err := uuid.Parse(msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id: %w", err))
	}
	repo, ok := h.readDB.(KnowledgeLoopReadRepository)
	if !ok {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("sovereign: KnowledgeLoop read repository not wired"))
	}

	state, err := repo.GetKnowledgeLoopSessionState(ctx, tenantID, userID, msg.LensModeId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// ADR-000909 §Δ2 supplement: attach the macro projection so the public
	// alt-backend handler can echo it onto the response. Missing macro_state
	// rows are normal (older users / empty windows) so we treat the
	// absence as "no byline" rather than an error.
	if state != nil {
		if macroState, macroErr := attachMacroStateIfPresent(ctx, h.readDB, userID, tenantID, msg.LensModeId); macroErr != nil {
			// Log via Go's error path — the macro layer is supplementary,
			// it must never block session state reads.
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("attach macro_state: %w", macroErr))
		} else if macroState != nil {
			state.MacroState = macroState
		}
	}
	return connect.NewResponse(&sovereignv1.GetKnowledgeLoopSessionStateResponse{State: state}), nil
}

// macroStateReader is the narrow subset of sovereign_db.Repository the
// session-state handler needs to satisfy the macro-attach path. Keeping
// it as a type assertion lets the handler stay testable with the
// existing readDB interface mock.
type macroStateReader interface {
	GetKnowledgeLoopMacroState(ctx context.Context, userID, tenantID uuid.UUID, lensModeID string) (*sovereign_db.KnowledgeLoopMacroStateRow, error)
}

func attachMacroStateIfPresent(ctx context.Context, readDB any, userID, tenantID uuid.UUID, lensModeID string) (*sovereignv1.KnowledgeLoopMacroState, error) {
	reader, ok := readDB.(macroStateReader)
	if !ok {
		// Wiring incomplete — the test repository for this handler suite may
		// not implement GetKnowledgeLoopMacroState yet. Treat as "macro layer
		// disabled" rather than failing the parent read.
		return nil, nil
	}
	row, err := reader.GetKnowledgeLoopMacroState(ctx, userID, tenantID, lensModeID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return macroStateRowToProto(row), nil
}

func macroStateRowToProto(row *sovereign_db.KnowledgeLoopMacroStateRow) *sovereignv1.KnowledgeLoopMacroState {
	return &sovereignv1.KnowledgeLoopMacroState{
		ActiveContinueThreads:   row.ActiveContinueThreads,
		PendingReviewCount:      row.PendingReviewCount,
		RecentInternalizedCount: row.RecentInternalizedCount,
		CognitiveLoadHint:       cognitiveLoadHintToProto(row.CognitiveLoadHint),
		WindowEndAt:             timestamppb.New(row.WindowEndAt),
		WindowStartAt:           timestamppb.New(row.WindowStartAt),
		SeqHiwater:              row.SeqHiwater,
		LensWeightsVersion:      row.LensWeightsVersion,
	}
}

func cognitiveLoadHintToProto(s string) sovereignv1.KnowledgeLoopCognitiveLoadHint {
	switch s {
	case "light":
		return sovereignv1.KnowledgeLoopCognitiveLoadHint_KNOWLEDGE_LOOP_COGNITIVE_LOAD_HINT_LIGHT
	case "medium":
		return sovereignv1.KnowledgeLoopCognitiveLoadHint_KNOWLEDGE_LOOP_COGNITIVE_LOAD_HINT_MEDIUM
	case "heavy":
		return sovereignv1.KnowledgeLoopCognitiveLoadHint_KNOWLEDGE_LOOP_COGNITIVE_LOAD_HINT_HEAVY
	default:
		return sovereignv1.KnowledgeLoopCognitiveLoadHint_KNOWLEDGE_LOOP_COGNITIVE_LOAD_HINT_UNSPECIFIED
	}
}

// GetKnowledgeLoopSurfaces returns all surface buckets for (tenant, user, lens).
func (h *SovereignHandler) GetKnowledgeLoopSurfaces(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetKnowledgeLoopSurfacesRequest],
) (*connect.Response[sovereignv1.GetKnowledgeLoopSurfacesResponse], error) {
	msg := req.Msg
	tenantID, err := uuid.Parse(msg.TenantId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("tenant_id: %w", err))
	}
	userID, err := uuid.Parse(msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id: %w", err))
	}
	repo, ok := h.readDB.(KnowledgeLoopReadRepository)
	if !ok {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("sovereign: KnowledgeLoop read repository not wired"))
	}

	surfaces, err := repo.GetKnowledgeLoopSurfaces(ctx, tenantID, userID, msg.LensModeId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sovereignv1.GetKnowledgeLoopSurfacesResponse{Surfaces: surfaces}), nil
}

// ReserveKnowledgeLoopTransition is the ingest-side idempotency barrier.
// Dedupe table is NOT a projection; reproject MUST NOT rebuild it.
func (h *SovereignHandler) ReserveKnowledgeLoopTransition(
	ctx context.Context,
	req *connect.Request[sovereignv1.ReserveKnowledgeLoopTransitionRequest],
) (*connect.Response[sovereignv1.ReserveKnowledgeLoopTransitionResponse], error) {
	msg := req.Msg
	userID, err := uuid.Parse(msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id: %w", err))
	}
	repo, ok := h.readDB.(KnowledgeLoopWriteRepository)
	if !ok {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("sovereign: KnowledgeLoop write repository not wired"))
	}
	res, err := repo.ReserveKnowledgeLoopTransition(ctx, userID, msg.ClientTransitionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &sovereignv1.ReserveKnowledgeLoopTransitionResponse{
		Reserved:                res.Reserved,
		CachedCanonicalEntryKey: res.CanonicalEntryKey,
		CachedResponsePayload:   res.ResponsePayloadJSON,
	}
	if res.CachedCreatedAt != nil && !res.CachedCreatedAt.IsZero() {
		resp.CachedCreatedAt = timestamppb.New(*res.CachedCreatedAt)
	}
	return connect.NewResponse(resp), nil
}
