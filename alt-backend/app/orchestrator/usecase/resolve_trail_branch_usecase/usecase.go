// Package resolve_trail_branch_usecase records a user's response to a proposed
// Knowledge Trail branch (taken or dismissed) by appending an idempotent
// trail.branch_resolved.v1 event. Taking a branch closes the loop; the sovereign
// projector transitions the branch out of the open set (trail closure).
package resolve_trail_branch_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"alt/domain"
	"alt/shared/port/knowledge_event_port"

	"github.com/google/uuid"
)

// EventTrailBranchResolved is the user-action event type. It mirrors the
// constant the sovereign projector matches; the value is the cross-service
// contract pinned by the consumer pact.
const EventTrailBranchResolved = "trail.branch_resolved.v1"

// ErrInvalidRequest wraps client-side validation failures so the handler can map
// them to InvalidArgument (vs an append failure, which is Internal).
var ErrInvalidRequest = errors.New("invalid resolve-branch request")

var uuidv7Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ResolveTrailBranchUsecase appends branch_resolved events.
type ResolveTrailBranchUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
}

func NewResolveTrailBranchUsecase(appendPort knowledge_event_port.AppendKnowledgeEventPort) *ResolveTrailBranchUsecase {
	return &ResolveTrailBranchUsecase{appendPort: appendPort}
}

// Execute validates and appends a branch resolution. Idempotent via
// clientResolutionID (a UUIDv7); a replay returns no error and appends nothing
// new (the dedupe registry absorbs it).
func (u *ResolveTrailBranchUsecase) Execute(ctx context.Context, userID, tenantID uuid.UUID, branchKey, resolution, clientResolutionID string) error {
	branchKey = strings.TrimSpace(branchKey)
	if branchKey == "" {
		return fmt.Errorf("%w: branch_key required", ErrInvalidRequest)
	}
	if resolution != "taken" && resolution != "dismissed" {
		return fmt.Errorf("%w: resolution must be taken or dismissed", ErrInvalidRequest)
	}
	if !uuidv7Re.MatchString(strings.ToLower(strings.TrimSpace(clientResolutionID))) {
		return fmt.Errorf("%w: client_resolution_id must be UUIDv7", ErrInvalidRequest)
	}

	payload, _ := json.Marshal(map[string]string{
		"branch_key": branchKey,
		"resolution": resolution,
	})
	uid := userID
	evt := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     EventTrailBranchResolved,
		AggregateType: "trail_branch",
		AggregateID:   branchKey,
		DedupeKey:     EventTrailBranchResolved + ":" + clientResolutionID,
		Payload:       payload,
	}
	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, evt); err != nil {
		return fmt.Errorf("resolve trail branch: %w", err)
	}
	return nil
}
