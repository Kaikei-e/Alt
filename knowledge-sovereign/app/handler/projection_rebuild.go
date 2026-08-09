package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"knowledge-sovereign/driver/sovereign_db"
)

// ProjectionRebuildRepository is the single destructive operation this handler
// exposes. It takes an allowlisted target, never a table name — see
// sovereign_db.ProjectionRebuildTarget.
type ProjectionRebuildRepository interface {
	RebuildProjection(ctx context.Context, target sovereign_db.ProjectionRebuildTarget) (sovereign_db.ProjectionRebuildResult, error)
}

// ProjectionRebuildHandler exposes the projection rebuild operation on the
// admin surface: empty a projection's read models and reset its projector
// checkpoint in one transaction, so the in-process projector re-folds the whole
// event log. Replaces the raw SQL in docs/runbooks/knowledge-trail-reproject.md.
type ProjectionRebuildHandler struct {
	repo ProjectionRebuildRepository
}

// NewProjectionRebuildHandler creates a new projection rebuild handler.
func NewProjectionRebuildHandler(repo ProjectionRebuildRepository) *ProjectionRebuildHandler {
	return &ProjectionRebuildHandler{repo: repo}
}

// RegisterRoutes registers projection rebuild routes on the given mux. Both sit
// under /admin/, which is the prefix the admin token gate covers.
func (h *ProjectionRebuildHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/projections/rebuild/targets", h.handleListTargets)
	mux.HandleFunc("POST /admin/projections/rebuild", h.handleRebuild)
}

type projectionRebuildRequest struct {
	Target string `json:"target"`
}

type rebuildTargetRow struct {
	Target        string   `json:"target"`
	Tables        []string `json:"tables"`
	ProjectorName string   `json:"projector_name"`
}

type rebuildTargetsResponse struct {
	Targets []rebuildTargetRow `json:"targets"`
}

// handleListTargets is the pre-flight preview: which tables a rebuild would
// empty and which checkpoint it would reset, before anything is destroyed.
func (h *ProjectionRebuildHandler) handleListTargets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	targets := sovereign_db.RebuildTargets()
	rows := make([]rebuildTargetRow, 0, len(targets))
	for _, t := range targets {
		rows = append(rows, rebuildTargetRow{
			Target:        t.Name(),
			Tables:        t.Tables(),
			ProjectorName: t.ProjectorName(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rebuildTargetsResponse{Targets: rows}); err != nil {
		slog.WarnContext(ctx, "failed to write rebuild targets response", "error", err)
	}
}

func (h *ProjectionRebuildHandler) handleRebuild(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// No default target. A destructive operation must be named explicitly, so
	// an empty or malformed body is a rejection rather than a rebuild of
	// whatever happens to come first in the allowlist.
	var req projectionRebuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			http.Error(w, fmt.Sprintf(`{"error": %q}`, "target is required"), http.StatusBadRequest)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error": %q}`, fmt.Sprintf("invalid request body: %v", err)), http.StatusBadRequest)
		return
	}

	target, err := sovereign_db.LookupRebuildTarget(req.Target)
	if err != nil {
		slog.WarnContext(ctx, "projection.rebuild.rejected", "requested_target", req.Target, "error", err)
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusBadRequest)
		return
	}

	result, err := h.repo.RebuildProjection(ctx, target)
	if err != nil {
		slog.ErrorContext(ctx, "projection.rebuild.failed", "target", target.Name(), "error", err)
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.WarnContext(ctx, "failed to write projection rebuild response", "error", err)
	}
}
