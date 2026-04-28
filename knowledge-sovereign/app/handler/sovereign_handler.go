package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// Mutation type constants matching alt-backend's knowledge_sovereign_port.
const (
	MutationUpsertHomeItem        = "upsert_home_item"
	MutationDismissHomeItem       = "dismiss_home_item"
	MutationClearSupersede        = "clear_supersede"
	MutationUpsertTodayDigest     = "upsert_today_digest"
	MutationUpsertRecallCandidate = "upsert_recall_candidate"
	// MutationPatchHomeItemURL is the corrective-event patch path.
	// Updates only the `url` column on knowledge_home_items, preserving
	// every other field. Used by alt-backend's projector when consuming
	// `ArticleUrlBackfilled` events that repair historical legacy-keyed
	// payloads.
	MutationPatchHomeItemURL = "patch_home_item_url"

	MutationUpsertCandidate  = "upsert_candidate"
	MutationSnoozeCandidate  = "snooze_candidate"
	MutationDismissCandidate = "dismiss_candidate"

	MutationDismissCuration = "dismiss_curation"
)

// MutationRepository defines the database operations used by the handler.
type MutationRepository interface {
	UpsertKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error
	DismissKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error
	ClearSupersedeState(ctx context.Context, payload json.RawMessage) error
	UpsertTodayDigest(ctx context.Context, payload json.RawMessage) error
	UpsertRecallCandidate(ctx context.Context, payload json.RawMessage) error
	SnoozeRecallCandidate(ctx context.Context, payload json.RawMessage) error
	DismissRecallCandidate(ctx context.Context, payload json.RawMessage) error
	// PatchKnowledgeHomeItemURL applies a single-column URL patch to one
	// knowledge_home_items row. Used by the corrective ArticleUrlBackfilled
	// projector branch.
	PatchKnowledgeHomeItemURL(ctx context.Context, payload json.RawMessage) error
}

// SovereignHandler implements the Connect-RPC KnowledgeSovereignService.
type SovereignHandler struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
	repo        MutationRepository
	readDB      ReadDB
	databaseURL string // for LISTEN/NOTIFY connections
}

// ReadDB defines all read/write operations beyond generic mutations.
// Satisfied by *sovereign_db.Repository.
type ReadDB interface {
	MutationRepository
	ReadOperations
}

// NewSovereignHandler creates a new sovereign handler.
func NewSovereignHandler(repo ReadDB, opts ...Option) *SovereignHandler {
	h := &SovereignHandler{repo: repo, readDB: repo}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Option configures the SovereignHandler.
type Option func(*SovereignHandler)

// WithDatabaseURL sets the database URL for LISTEN/NOTIFY connections.
func WithDatabaseURL(url string) Option {
	return func(h *SovereignHandler) { h.databaseURL = url }
}

// ApplyProjectionMutation dispatches a projection mutation to the repository.
func (h *SovereignHandler) ApplyProjectionMutation(
	ctx context.Context,
	req *connect.Request[sovereignv1.ApplyProjectionMutationRequest],
) (*connect.Response[sovereignv1.ApplyProjectionMutationResponse], error) {
	msg := req.Msg
	payload := json.RawMessage(msg.Payload)

	var err error
	switch msg.MutationType {
	case MutationUpsertHomeItem:
		err = h.repo.UpsertKnowledgeHomeItem(ctx, payload)
	case MutationDismissHomeItem:
		err = h.repo.DismissKnowledgeHomeItem(ctx, payload)
	case MutationClearSupersede:
		err = h.repo.ClearSupersedeState(ctx, payload)
	case MutationUpsertTodayDigest:
		err = h.repo.UpsertTodayDigest(ctx, payload)
	case MutationUpsertRecallCandidate:
		err = h.repo.UpsertRecallCandidate(ctx, payload)
	case MutationPatchHomeItemURL:
		err = h.repo.PatchKnowledgeHomeItemURL(ctx, payload)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown projection mutation type: %s", msg.MutationType))
	}

	if err != nil {
		slog.ErrorContext(ctx, "projection mutation failed",
			"type", msg.MutationType, "entity_id", msg.EntityId, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sovereignv1.ApplyProjectionMutationResponse{Success: true}), nil
}

// ApplyRecallMutation dispatches a recall mutation to the repository.
func (h *SovereignHandler) ApplyRecallMutation(
	ctx context.Context,
	req *connect.Request[sovereignv1.ApplyRecallMutationRequest],
) (*connect.Response[sovereignv1.ApplyRecallMutationResponse], error) {
	msg := req.Msg
	payload := json.RawMessage(msg.Payload)

	var err error
	switch msg.MutationType {
	case MutationUpsertCandidate:
		err = h.repo.UpsertRecallCandidate(ctx, payload)
	case MutationSnoozeCandidate:
		err = h.repo.SnoozeRecallCandidate(ctx, payload)
	case MutationDismissCandidate:
		err = h.repo.DismissRecallCandidate(ctx, payload)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown recall mutation type: %s", msg.MutationType))
	}

	if err != nil {
		slog.ErrorContext(ctx, "recall mutation failed",
			"type", msg.MutationType, "entity_id", msg.EntityId, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sovereignv1.ApplyRecallMutationResponse{Success: true}), nil
}

// ApplyCurationMutation dispatches a curation mutation to the repository.
func (h *SovereignHandler) ApplyCurationMutation(
	ctx context.Context,
	req *connect.Request[sovereignv1.ApplyCurationMutationRequest],
) (*connect.Response[sovereignv1.ApplyCurationMutationResponse], error) {
	msg := req.Msg
	payload := json.RawMessage(msg.Payload)

	var err error
	switch msg.MutationType {
	case MutationDismissCuration:
		err = h.repo.DismissKnowledgeHomeItem(ctx, payload)
	default:
		// Lens mutations are fire-and-forget logging; ack without DB op for now.
		slog.InfoContext(ctx, "curation mutation received",
			"type", msg.MutationType, "entity_id", msg.EntityId)
	}

	if err != nil {
		slog.ErrorContext(ctx, "curation mutation failed",
			"type", msg.MutationType, "entity_id", msg.EntityId, "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&sovereignv1.ApplyCurationMutationResponse{Success: true}), nil
}
