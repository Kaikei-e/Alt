package knowledge_loop_port

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// UpsertKnowledgeLoopEntryPort applies a new projection row or revises an existing one.
// Implementations MUST enforce the seq-hiwater guard: updates where the current row's
// projection_seq_hiwater exceeds the incoming source_event_seq are no-ops, so that
// out-of-order event replay cannot overwrite newer projection state.
type UpsertKnowledgeLoopEntryPort interface {
	UpsertKnowledgeLoopEntry(ctx context.Context, entry *domain.KnowledgeLoopEntry) (*UpsertResult, error)
}

// UpsertResult reports what the repository actually did for a given upsert attempt.
type UpsertResult struct {
	Applied              bool  // true = insert or update committed
	SkippedBySeqHiwater  bool  // true = existing seq_hiwater >= incoming; no mutation
	ProjectionRevision   int64 // post-apply revision (0 when skipped)
	ProjectionSeqHiwater int64 // post-apply seq_hiwater (0 when skipped)
}

// GetKnowledgeLoopEntriesPort reads a user's Loop entries (scoped by tenant/lens).
// MUST read through the knowledge_loop_entries_public view (projected_at is invisible).
type GetKnowledgeLoopEntriesPort interface {
	GetKnowledgeLoopEntries(ctx context.Context, q GetEntriesQuery) ([]*domain.KnowledgeLoopEntry, error)
}

// GetEntriesQuery scopes a read. tenant_id and user_id MUST come from verified JWT claims.
type GetEntriesQuery struct {
	TenantID         uuid.UUID
	UserID           uuid.UUID
	LensModeID       string
	SurfaceBucket    *domain.SurfaceBucket // optional filter
	IncludeDismissed bool
	Limit            int
}

// UpsertKnowledgeLoopSessionStatePort writes session state. The triggering event's
// occurred_at (not wall-clock) MUST be passed as current_stage_entered_at.
type UpsertKnowledgeLoopSessionStatePort interface {
	UpsertKnowledgeLoopSessionState(ctx context.Context, state *domain.KnowledgeLoopSessionState) (*UpsertResult, error)
}

// GetKnowledgeLoopSessionStatePort reads session state for a given (user, lens).
type GetKnowledgeLoopSessionStatePort interface {
	GetKnowledgeLoopSessionState(ctx context.Context, tenantID, userID uuid.UUID, lensModeID string) (*domain.KnowledgeLoopSessionState, error)
}

// UpsertKnowledgeLoopSurfacePort writes a per-bucket surface summary.
type UpsertKnowledgeLoopSurfacePort interface {
	UpsertKnowledgeLoopSurface(ctx context.Context, surface *domain.KnowledgeLoopSurface) (*UpsertResult, error)
}

// GetKnowledgeLoopSurfacesPort reads all buckets for a (user, lens).
type GetKnowledgeLoopSurfacesPort interface {
	GetKnowledgeLoopSurfaces(ctx context.Context, tenantID, userID uuid.UUID, lensModeID string) ([]*domain.KnowledgeLoopSurface, error)
}

// ReserveTransitionIdempotencyPort is the ingest-side idempotency barrier.
// Returns (false, nil) if the (user_id, client_transition_id) key has already been seen;
// in that case the caller MUST return the cached response rather than re-append the event.
// Returns (true, nil) if the key was freshly claimed.
//
// This table is NOT a projection: full reproject does NOT touch it.
type ReserveTransitionIdempotencyPort interface {
	ReserveTransitionIdempotency(ctx context.Context, userID uuid.UUID, clientTransitionID string) (reserved bool, cached *CachedTransitionResponse, err error)
}

// CachedTransitionResponse replays the original accepted response for a duplicate request.
type CachedTransitionResponse struct {
	CanonicalEntryKey   *string
	ResponsePayloadJSON []byte
	CreatedAt           time.Time
}
