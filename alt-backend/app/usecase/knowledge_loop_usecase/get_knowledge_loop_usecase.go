package knowledge_loop_usecase

import (
	"alt/domain"
	"alt/port/knowledge_loop_port"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// GetKnowledgeLoopUsecase orchestrates the read path: foreground entries + session state
// + surface summaries for the authenticated user's current lens.
// Tenant and user id MUST come from the handler's JWT-derived user context.
type GetKnowledgeLoopUsecase struct {
	entriesPort  knowledge_loop_port.GetKnowledgeLoopEntriesPort
	sessionPort  knowledge_loop_port.GetKnowledgeLoopSessionStatePort
	surfacesPort knowledge_loop_port.GetKnowledgeLoopSurfacesPort
}

// NewGetKnowledgeLoopUsecase constructs the usecase.
func NewGetKnowledgeLoopUsecase(
	entriesPort knowledge_loop_port.GetKnowledgeLoopEntriesPort,
	sessionPort knowledge_loop_port.GetKnowledgeLoopSessionStatePort,
	surfacesPort knowledge_loop_port.GetKnowledgeLoopSurfacesPort,
) *GetKnowledgeLoopUsecase {
	return &GetKnowledgeLoopUsecase{
		entriesPort:  entriesPort,
		sessionPort:  sessionPort,
		surfacesPort: surfacesPort,
	}
}

// GetKnowledgeLoopResult bundles the three projections for a single RPC response.
type GetKnowledgeLoopResult struct {
	ForegroundEntries    []*domain.KnowledgeLoopEntry
	SessionState         *domain.KnowledgeLoopSessionState
	Surfaces             []*domain.KnowledgeLoopSurface
	ProjectionSeqHiwater int64
}

// Execute reads the three projections. lens_mode_id is validated by the caller.
func (u *GetKnowledgeLoopUsecase) Execute(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	lensModeID string,
	foregroundLimit int,
) (*GetKnowledgeLoopResult, error) {
	if foregroundLimit <= 0 || foregroundLimit > 5 {
		foregroundLimit = 3
	}
	if err := ValidateKeyFormat("lens_mode_id", lensModeID); err != nil {
		return nil, err
	}

	nowBucket := domain.SurfaceNow
	entries, err := u.entriesPort.GetKnowledgeLoopEntries(ctx, knowledge_loop_port.GetEntriesQuery{
		TenantID:      tenantID,
		UserID:        userID,
		LensModeID:    lensModeID,
		SurfaceBucket: &nowBucket,
		Limit:         foregroundLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("get_knowledge_loop: entries: %w", err)
	}

	session, err := u.sessionPort.GetKnowledgeLoopSessionState(ctx, tenantID, userID, lensModeID)
	if err != nil {
		return nil, fmt.Errorf("get_knowledge_loop: session: %w", err)
	}

	surfaces, err := u.surfacesPort.GetKnowledgeLoopSurfaces(ctx, tenantID, userID, lensModeID)
	if err != nil {
		return nil, fmt.Errorf("get_knowledge_loop: surfaces: %w", err)
	}

	// Pick the max seq_hiwater across entries / session / surfaces so the client can resume.
	maxSeq := int64(0)
	for _, e := range entries {
		if e.ProjectionSeqHiwater > maxSeq {
			maxSeq = e.ProjectionSeqHiwater
		}
	}
	if session != nil && session.ProjectionSeqHiwater > maxSeq {
		maxSeq = session.ProjectionSeqHiwater
	}
	for _, s := range surfaces {
		if s.ProjectionSeqHiwater > maxSeq {
			maxSeq = s.ProjectionSeqHiwater
		}
	}

	return &GetKnowledgeLoopResult{
		ForegroundEntries:    entries,
		SessionState:         session,
		Surfaces:             surfaces,
		ProjectionSeqHiwater: maxSeq,
	}, nil
}
