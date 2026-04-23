package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/port/knowledge_loop_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Mutation type constants mirror knowledge-sovereign/handler/rpc_knowledge_loop.go.
const (
	mutationKnowledgeLoopEntryUpsert   = "entry_upsert"
	mutationKnowledgeLoopSessionUpsert = "session_upsert"
	mutationKnowledgeLoopSurfaceUpsert = "surface_upsert"
)

// Compile-time assertions: Client implements all Knowledge Loop ports.
var (
	_ knowledge_loop_port.UpsertKnowledgeLoopEntryPort        = (*Client)(nil)
	_ knowledge_loop_port.UpsertKnowledgeLoopSessionStatePort = (*Client)(nil)
	_ knowledge_loop_port.UpsertKnowledgeLoopSurfacePort      = (*Client)(nil)
	_ knowledge_loop_port.GetKnowledgeLoopEntriesPort         = (*Client)(nil)
	_ knowledge_loop_port.GetKnowledgeLoopSessionStatePort    = (*Client)(nil)
	_ knowledge_loop_port.GetKnowledgeLoopSurfacesPort        = (*Client)(nil)
	_ knowledge_loop_port.ReserveTransitionIdempotencyPort    = (*Client)(nil)
)

// applyKnowledgeLoopMutation is the common write helper. It proto-marshals the payload
// message and dispatches to ApplyKnowledgeLoopMutation.
func (c *Client) applyKnowledgeLoopMutation(
	ctx context.Context,
	mutationType, entityID string,
	payloadProto proto.Message,
) (*knowledge_loop_port.UpsertResult, error) {
	if !c.enabled {
		return &knowledge_loop_port.UpsertResult{Applied: true, ProjectionRevision: 0, ProjectionSeqHiwater: 0}, nil
	}

	payload, err := proto.Marshal(payloadProto)
	if err != nil {
		return nil, fmt.Errorf("sovereign ApplyKnowledgeLoopMutation marshal %s: %w", mutationType, err)
	}
	resp, err := c.client.ApplyKnowledgeLoopMutation(ctx, connect.NewRequest(&sovereignv1.ApplyKnowledgeLoopMutationRequest{
		MutationType:   mutationType,
		EntityId:       entityID,
		Payload:        payload,
		IdempotencyKey: uuid.NewString(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ApplyKnowledgeLoopMutation %s: %w", mutationType, err)
	}
	if em := resp.Msg.ErrorMessage; em != "" {
		return nil, errors.New(em)
	}
	return &knowledge_loop_port.UpsertResult{
		Applied:              resp.Msg.Applied,
		SkippedBySeqHiwater:  resp.Msg.SkippedBySeqHiwater,
		ProjectionRevision:   resp.Msg.ProjectionRevision,
		ProjectionSeqHiwater: resp.Msg.ProjectionSeqHiwater,
	}, nil
}

// UpsertKnowledgeLoopEntry writes a projection row via sovereign.
func (c *Client) UpsertKnowledgeLoopEntry(
	ctx context.Context,
	entry *domain.KnowledgeLoopEntry,
) (*knowledge_loop_port.UpsertResult, error) {
	if entry == nil {
		return nil, errors.New("sovereign_client: nil KnowledgeLoopEntry")
	}
	return c.applyKnowledgeLoopMutation(ctx, mutationKnowledgeLoopEntryUpsert, entry.EntryKey, domainEntryToProto(entry))
}

// UpsertKnowledgeLoopSessionState writes session state via sovereign.
func (c *Client) UpsertKnowledgeLoopSessionState(
	ctx context.Context,
	state *domain.KnowledgeLoopSessionState,
) (*knowledge_loop_port.UpsertResult, error) {
	if state == nil {
		return nil, errors.New("sovereign_client: nil KnowledgeLoopSessionState")
	}
	return c.applyKnowledgeLoopMutation(ctx, mutationKnowledgeLoopSessionUpsert, state.UserID.String()+":"+state.LensModeID, domainSessionToProto(state))
}

// UpsertKnowledgeLoopSurface writes a surface-bucket row via sovereign.
func (c *Client) UpsertKnowledgeLoopSurface(
	ctx context.Context,
	surface *domain.KnowledgeLoopSurface,
) (*knowledge_loop_port.UpsertResult, error) {
	if surface == nil {
		return nil, errors.New("sovereign_client: nil KnowledgeLoopSurface")
	}
	return c.applyKnowledgeLoopMutation(ctx, mutationKnowledgeLoopSurfaceUpsert, surface.UserID.String()+":"+surface.LensModeID+":"+string(surface.SurfaceBucket), domainSurfaceToProto(surface))
}

// GetKnowledgeLoopEntries reads entries via sovereign.
func (c *Client) GetKnowledgeLoopEntries(
	ctx context.Context,
	q knowledge_loop_port.GetEntriesQuery,
) ([]*domain.KnowledgeLoopEntry, error) {
	if !c.enabled {
		return nil, nil
	}
	req := &sovereignv1.GetKnowledgeLoopEntriesRequest{
		TenantId:         q.TenantID.String(),
		UserId:           q.UserID.String(),
		LensModeId:       q.LensModeID,
		IncludeDismissed: q.IncludeDismissed,
		Limit:            int32(q.Limit),
	}
	if q.SurfaceBucket != nil {
		b := surfaceBucketToProto(*q.SurfaceBucket)
		req.SurfaceBucket = &b
	}
	resp, err := c.client.GetKnowledgeLoopEntries(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetKnowledgeLoopEntries: %w", err)
	}
	out := make([]*domain.KnowledgeLoopEntry, 0, len(resp.Msg.Entries))
	for _, pb := range resp.Msg.Entries {
		d, err := protoEntryToDomain(pb)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// GetKnowledgeLoopSessionState reads session state via sovereign.
func (c *Client) GetKnowledgeLoopSessionState(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	lensModeID string,
) (*domain.KnowledgeLoopSessionState, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetKnowledgeLoopSessionState(ctx, connect.NewRequest(&sovereignv1.GetKnowledgeLoopSessionStateRequest{
		TenantId:   tenantID.String(),
		UserId:     userID.String(),
		LensModeId: lensModeID,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetKnowledgeLoopSessionState: %w", err)
	}
	if resp.Msg.State == nil {
		return nil, nil
	}
	return protoSessionToDomain(resp.Msg.State)
}

// GetKnowledgeLoopSurfaces reads all surface buckets via sovereign.
func (c *Client) GetKnowledgeLoopSurfaces(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	lensModeID string,
) ([]*domain.KnowledgeLoopSurface, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetKnowledgeLoopSurfaces(ctx, connect.NewRequest(&sovereignv1.GetKnowledgeLoopSurfacesRequest{
		TenantId:   tenantID.String(),
		UserId:     userID.String(),
		LensModeId: lensModeID,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetKnowledgeLoopSurfaces: %w", err)
	}
	out := make([]*domain.KnowledgeLoopSurface, 0, len(resp.Msg.Surfaces))
	for _, pb := range resp.Msg.Surfaces {
		d, err := protoSurfaceToDomain(pb)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ReserveTransitionIdempotency is the ingest-side idempotency barrier via sovereign.
func (c *Client) ReserveTransitionIdempotency(
	ctx context.Context,
	userID uuid.UUID,
	clientTransitionID string,
) (bool, *knowledge_loop_port.CachedTransitionResponse, error) {
	if !c.enabled {
		return true, nil, nil
	}
	resp, err := c.client.ReserveKnowledgeLoopTransition(ctx, connect.NewRequest(&sovereignv1.ReserveKnowledgeLoopTransitionRequest{
		UserId:             userID.String(),
		ClientTransitionId: clientTransitionID,
	}))
	if err != nil {
		return false, nil, fmt.Errorf("sovereign ReserveKnowledgeLoopTransition: %w", err)
	}
	if resp.Msg.Reserved {
		return true, nil, nil
	}
	cached := &knowledge_loop_port.CachedTransitionResponse{
		CanonicalEntryKey:   resp.Msg.CachedCanonicalEntryKey,
		ResponsePayloadJSON: resp.Msg.CachedResponsePayload,
	}
	if resp.Msg.CachedCreatedAt != nil {
		cached.CreatedAt = resp.Msg.CachedCreatedAt.AsTime()
	}
	return false, cached, nil
}

// ---------- domain <-> proto mappers (alt.knowledge.loop.v1 equivalents) ----------

func domainEntryToProto(e *domain.KnowledgeLoopEntry) *sovereignv1.KnowledgeLoopEntry {
	pb := &sovereignv1.KnowledgeLoopEntry{
		UserId:               e.UserID.String(),
		TenantId:             e.TenantID.String(),
		LensModeId:           e.LensModeID,
		EntryKey:             e.EntryKey,
		SourceItemKey:        e.SourceItemKey,
		ProposedStage:        loopStageToProto(e.ProposedStage),
		SurfaceBucket:        surfaceBucketToProto(e.SurfaceBucket),
		ProjectionRevision:   e.ProjectionRevision,
		ProjectionSeqHiwater: e.ProjectionSeqHiwater,
		SourceEventSeq:       e.SourceEventSeq,
		FreshnessAt:          timestamppb.New(e.FreshnessAt),
		ArtifactVersionRef: &sovereignv1.KnowledgeLoopArtifactVersionRef{
			SummaryVersionId: e.ArtifactVersionRef.SummaryVersionID,
			TagSetVersionId:  e.ArtifactVersionRef.TagSetVersionID,
			LensVersionId:    e.ArtifactVersionRef.LensVersionID,
		},
		WhyPrimary: &sovereignv1.KnowledgeLoopWhyPayload{
			Kind: whyKindToProto(e.WhyKind),
			Text: e.WhyText,
		},
		ChangeSummary:        e.ChangeSummary,
		ContinueContext:      e.ContinueContext,
		DecisionOptions:      e.DecisionOptions,
		ActTargets:           e.ActTargets,
		SupersededByEntryKey: e.SupersededByEntryKey,
		DismissState:         dismissStateToProto(e.DismissState),
		RenderDepthHint:      int32(e.RenderDepthHint),
		LoopPriority:         loopPriorityToProto(e.LoopPriority),
	}
	if e.WhyConfidence != nil {
		pb.WhyPrimary.Confidence = e.WhyConfidence
	}
	if e.SourceObservedAt != nil {
		pb.SourceObservedAt = timestamppb.New(*e.SourceObservedAt)
	}
	for _, r := range e.WhyEvidenceRefs {
		pb.WhyPrimary.EvidenceRefs = append(pb.WhyPrimary.EvidenceRefs, &sovereignv1.KnowledgeLoopEvidenceRef{
			RefId: r.RefID,
			Label: r.Label,
		})
	}
	return pb
}

func domainSessionToProto(s *domain.KnowledgeLoopSessionState) *sovereignv1.KnowledgeLoopSessionState {
	return &sovereignv1.KnowledgeLoopSessionState{
		UserId:                s.UserID.String(),
		TenantId:              s.TenantID.String(),
		LensModeId:            s.LensModeID,
		CurrentStage:          loopStageToProto(s.CurrentStage),
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
}

func domainSurfaceToProto(s *domain.KnowledgeLoopSurface) *sovereignv1.KnowledgeLoopSurface {
	return &sovereignv1.KnowledgeLoopSurface{
		UserId:               s.UserID.String(),
		TenantId:             s.TenantID.String(),
		LensModeId:           s.LensModeID,
		SurfaceBucket:        surfaceBucketToProto(s.SurfaceBucket),
		PrimaryEntryKey:      s.PrimaryEntryKey,
		SecondaryEntryKeys:   s.SecondaryEntryKeys,
		ProjectionRevision:   s.ProjectionRevision,
		ProjectionSeqHiwater: s.ProjectionSeqHiwater,
		FreshnessAt:          timestamppb.New(s.FreshnessAt),
		ServiceQuality:       serviceQualityToProto(s.ServiceQuality),
		LoopHealth:           s.LoopHealth,
	}
}

func protoEntryToDomain(pb *sovereignv1.KnowledgeLoopEntry) (*domain.KnowledgeLoopEntry, error) {
	userID, err := uuid.Parse(pb.UserId)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tenantID, err := uuid.Parse(pb.TenantId)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}
	e := &domain.KnowledgeLoopEntry{
		UserID:               userID,
		TenantID:             tenantID,
		LensModeID:           pb.LensModeId,
		EntryKey:             pb.EntryKey,
		SourceItemKey:        pb.SourceItemKey,
		ProposedStage:        loopStageFromProto(pb.ProposedStage),
		SurfaceBucket:        surfaceBucketFromProto(pb.SurfaceBucket),
		ProjectionRevision:   pb.ProjectionRevision,
		ProjectionSeqHiwater: pb.ProjectionSeqHiwater,
		SourceEventSeq:       pb.SourceEventSeq,
		ChangeSummary:        pb.ChangeSummary,
		ContinueContext:      pb.ContinueContext,
		DecisionOptions:      pb.DecisionOptions,
		ActTargets:           pb.ActTargets,
		SupersededByEntryKey: pb.SupersededByEntryKey,
		DismissState:         dismissStateFromProto(pb.DismissState),
		RenderDepthHint:      domain.RenderDepthHint(pb.RenderDepthHint),
		LoopPriority:         loopPriorityFromProto(pb.LoopPriority),
	}
	if pb.FreshnessAt != nil {
		e.FreshnessAt = pb.FreshnessAt.AsTime()
	}
	if pb.SourceObservedAt != nil {
		t := pb.SourceObservedAt.AsTime()
		e.SourceObservedAt = &t
	}
	if pb.ArtifactVersionRef != nil {
		e.ArtifactVersionRef = domain.ArtifactVersionRef{
			SummaryVersionID: pb.ArtifactVersionRef.SummaryVersionId,
			TagSetVersionID:  pb.ArtifactVersionRef.TagSetVersionId,
			LensVersionID:    pb.ArtifactVersionRef.LensVersionId,
		}
	}
	if pb.WhyPrimary != nil {
		e.WhyKind = whyKindFromProto(pb.WhyPrimary.Kind)
		e.WhyText = pb.WhyPrimary.Text
		e.WhyConfidence = pb.WhyPrimary.Confidence
		for _, r := range pb.WhyPrimary.EvidenceRefs {
			e.WhyEvidenceRefIDs = append(e.WhyEvidenceRefIDs, r.RefId)
			e.WhyEvidenceRefs = append(e.WhyEvidenceRefs, domain.EvidenceRef{RefID: r.RefId, Label: r.Label})
		}
	}
	return e, nil
}

func protoSessionToDomain(pb *sovereignv1.KnowledgeLoopSessionState) (*domain.KnowledgeLoopSessionState, error) {
	userID, err := uuid.Parse(pb.UserId)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tenantID, err := uuid.Parse(pb.TenantId)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}
	s := &domain.KnowledgeLoopSessionState{
		UserID:               userID,
		TenantID:             tenantID,
		LensModeID:           pb.LensModeId,
		CurrentStage:         loopStageFromProto(pb.CurrentStage),
		FocusedEntryKey:      pb.FocusedEntryKey,
		ForegroundEntryKey:   pb.ForegroundEntryKey,
		LastObservedEntryKey: pb.LastObservedEntryKey,
		LastOrientedEntryKey: pb.LastOrientedEntryKey,
		LastDecidedEntryKey:  pb.LastDecidedEntryKey,
		LastActedEntryKey:    pb.LastActedEntryKey,
		LastReturnedEntryKey: pb.LastReturnedEntryKey,
		LastDeferredEntryKey: pb.LastDeferredEntryKey,
		ProjectionRevision:   pb.ProjectionRevision,
		ProjectionSeqHiwater: pb.ProjectionSeqHiwater,
	}
	if pb.CurrentStageEnteredAt != nil {
		s.CurrentStageEnteredAt = pb.CurrentStageEnteredAt.AsTime()
	}
	return s, nil
}

func protoSurfaceToDomain(pb *sovereignv1.KnowledgeLoopSurface) (*domain.KnowledgeLoopSurface, error) {
	userID, err := uuid.Parse(pb.UserId)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tenantID, err := uuid.Parse(pb.TenantId)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}
	s := &domain.KnowledgeLoopSurface{
		UserID:               userID,
		TenantID:             tenantID,
		LensModeID:           pb.LensModeId,
		SurfaceBucket:        surfaceBucketFromProto(pb.SurfaceBucket),
		PrimaryEntryKey:      pb.PrimaryEntryKey,
		SecondaryEntryKeys:   pb.SecondaryEntryKeys,
		ProjectionRevision:   pb.ProjectionRevision,
		ProjectionSeqHiwater: pb.ProjectionSeqHiwater,
		ServiceQuality:       serviceQualityFromProto(pb.ServiceQuality),
		LoopHealth:           pb.LoopHealth,
	}
	if pb.FreshnessAt != nil {
		s.FreshnessAt = pb.FreshnessAt.AsTime()
	}
	return s, nil
}

// ---------- enum mappers (domain <-> proto) ----------

func loopStageToProto(s domain.LoopStage) sovereignv1.LoopStage {
	switch s {
	case domain.LoopStageObserve:
		return sovereignv1.LoopStage_LOOP_STAGE_OBSERVE
	case domain.LoopStageOrient:
		return sovereignv1.LoopStage_LOOP_STAGE_ORIENT
	case domain.LoopStageDecide:
		return sovereignv1.LoopStage_LOOP_STAGE_DECIDE
	case domain.LoopStageAct:
		return sovereignv1.LoopStage_LOOP_STAGE_ACT
	}
	return sovereignv1.LoopStage_LOOP_STAGE_UNSPECIFIED
}

func loopStageFromProto(s sovereignv1.LoopStage) domain.LoopStage {
	switch s {
	case sovereignv1.LoopStage_LOOP_STAGE_OBSERVE:
		return domain.LoopStageObserve
	case sovereignv1.LoopStage_LOOP_STAGE_ORIENT:
		return domain.LoopStageOrient
	case sovereignv1.LoopStage_LOOP_STAGE_DECIDE:
		return domain.LoopStageDecide
	case sovereignv1.LoopStage_LOOP_STAGE_ACT:
		return domain.LoopStageAct
	}
	return domain.LoopStageObserve
}

func surfaceBucketToProto(b domain.SurfaceBucket) sovereignv1.SurfaceBucket {
	switch b {
	case domain.SurfaceNow:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW
	case domain.SurfaceContinue:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	case domain.SurfaceChanged:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED
	case domain.SurfaceReview:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	}
	return sovereignv1.SurfaceBucket_SURFACE_BUCKET_UNSPECIFIED
}

func surfaceBucketFromProto(b sovereignv1.SurfaceBucket) domain.SurfaceBucket {
	switch b {
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW:
		return domain.SurfaceNow
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE:
		return domain.SurfaceContinue
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED:
		return domain.SurfaceChanged
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW:
		return domain.SurfaceReview
	}
	return domain.SurfaceNow
}

func dismissStateToProto(d domain.DismissState) sovereignv1.DismissState {
	switch d {
	case domain.DismissActive:
		return sovereignv1.DismissState_DISMISS_STATE_ACTIVE
	case domain.DismissDeferred:
		return sovereignv1.DismissState_DISMISS_STATE_DEFERRED
	case domain.DismissDismissed:
		return sovereignv1.DismissState_DISMISS_STATE_DISMISSED
	case domain.DismissCompleted:
		return sovereignv1.DismissState_DISMISS_STATE_COMPLETED
	}
	return sovereignv1.DismissState_DISMISS_STATE_ACTIVE
}

func dismissStateFromProto(d sovereignv1.DismissState) domain.DismissState {
	switch d {
	case sovereignv1.DismissState_DISMISS_STATE_ACTIVE:
		return domain.DismissActive
	case sovereignv1.DismissState_DISMISS_STATE_DEFERRED:
		return domain.DismissDeferred
	case sovereignv1.DismissState_DISMISS_STATE_DISMISSED:
		return domain.DismissDismissed
	case sovereignv1.DismissState_DISMISS_STATE_COMPLETED:
		return domain.DismissCompleted
	}
	return domain.DismissActive
}

func whyKindToProto(k domain.WhyKind) sovereignv1.WhyKind {
	switch k {
	case domain.WhyKindSource:
		return sovereignv1.WhyKind_WHY_KIND_SOURCE
	case domain.WhyKindPattern:
		return sovereignv1.WhyKind_WHY_KIND_PATTERN
	case domain.WhyKindRecall:
		return sovereignv1.WhyKind_WHY_KIND_RECALL
	case domain.WhyKindChange:
		return sovereignv1.WhyKind_WHY_KIND_CHANGE
	}
	return sovereignv1.WhyKind_WHY_KIND_SOURCE
}

func whyKindFromProto(k sovereignv1.WhyKind) domain.WhyKind {
	switch k {
	case sovereignv1.WhyKind_WHY_KIND_SOURCE:
		return domain.WhyKindSource
	case sovereignv1.WhyKind_WHY_KIND_PATTERN:
		return domain.WhyKindPattern
	case sovereignv1.WhyKind_WHY_KIND_RECALL:
		return domain.WhyKindRecall
	case sovereignv1.WhyKind_WHY_KIND_CHANGE:
		return domain.WhyKindChange
	}
	return domain.WhyKindSource
}

func loopPriorityToProto(p domain.LoopPriority) sovereignv1.LoopPriority {
	switch p {
	case domain.LoopPriorityCritical:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL
	case domain.LoopPriorityContinuing:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CONTINUING
	case domain.LoopPriorityConfirm:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CONFIRM
	case domain.LoopPriorityReference:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_REFERENCE
	}
	return sovereignv1.LoopPriority_LOOP_PRIORITY_REFERENCE
}

func loopPriorityFromProto(p sovereignv1.LoopPriority) domain.LoopPriority {
	switch p {
	case sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL:
		return domain.LoopPriorityCritical
	case sovereignv1.LoopPriority_LOOP_PRIORITY_CONTINUING:
		return domain.LoopPriorityContinuing
	case sovereignv1.LoopPriority_LOOP_PRIORITY_CONFIRM:
		return domain.LoopPriorityConfirm
	case sovereignv1.LoopPriority_LOOP_PRIORITY_REFERENCE:
		return domain.LoopPriorityReference
	}
	return domain.LoopPriorityReference
}

func serviceQualityToProto(q domain.LoopServiceQuality) sovereignv1.LoopServiceQuality {
	switch q {
	case domain.LoopQualityFull:
		return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FULL
	case domain.LoopQualityDegraded:
		return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_DEGRADED
	case domain.LoopQualityFallback:
		return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FALLBACK
	}
	return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FULL
}

func serviceQualityFromProto(q sovereignv1.LoopServiceQuality) domain.LoopServiceQuality {
	switch q {
	case sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FULL:
		return domain.LoopQualityFull
	case sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_DEGRADED:
		return domain.LoopQualityDegraded
	case sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FALLBACK:
		return domain.LoopQualityFallback
	}
	return domain.LoopQualityFull
}

// --- JSON helper (kept for consistency with existing sovereign_client patterns) ---

var _ = json.Marshal // no-op reference; JSON payloads are not used for Knowledge Loop
