package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"knowledge-sovereign/driver/sovereign_db"
)

// KnowledgeLoopReprojectRepository is the narrow port the handler depends on.
// Defined locally so the handler can be tested against a fake without pulling
// the full Repository surface.
type KnowledgeLoopReprojectRepository interface {
	TruncateKnowledgeLoopProjections(ctx context.Context) (sovereign_db.KnowledgeLoopReprojectResult, error)
}

// KnowledgeLoopReprojectHandler exposes the Knowledge Loop full-reproject
// procedure as an admin HTTP endpoint. The runbook
// (docs/runbooks/knowledge-loop-reproject.md) is the source of truth; this
// handler just wraps the in-transaction TRUNCATE + checkpoint reset so an
// operator can trigger it from /admin/knowledge-home without reaching for
// psql.
type KnowledgeLoopReprojectHandler struct {
	repo KnowledgeLoopReprojectRepository
}

func NewKnowledgeLoopReprojectHandler(repo KnowledgeLoopReprojectRepository) *KnowledgeLoopReprojectHandler {
	return &KnowledgeLoopReprojectHandler{repo: repo}
}

// RegisterRoutes wires the admin endpoint onto the metrics-port mux. The
// route mirrors the retention handler's `/admin/...` convention so an
// operator running on the metrics port sees both controls under the same
// path prefix.
func (h *KnowledgeLoopReprojectHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/knowledge-loop/reproject", h.handleReproject)
}

type knowledgeLoopReprojectResponse struct {
	OK                     bool   `json:"ok"`
	EntriesTruncated       int64  `json:"entries_truncated"`
	SessionStateTruncated  int64  `json:"session_state_truncated"`
	SurfacesTruncated      int64  `json:"surfaces_truncated"`
	CheckpointReset        bool   `json:"checkpoint_reset"`
	ProjectorWillRunOnTick string `json:"projector_will_run_on_tick"`
	Error                  string `json:"error,omitempty"`
}

func (h *KnowledgeLoopReprojectHandler) handleReproject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slog.InfoContext(ctx, "knowledge-loop reproject requested")

	res, err := h.repo.TruncateKnowledgeLoopProjections(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "knowledge-loop reproject failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(knowledgeLoopReprojectResponse{
			OK:    false,
			Error: err.Error(),
		})
		return
	}

	slog.InfoContext(ctx, "knowledge-loop reproject completed",
		"entries_truncated", res.EntriesTruncated,
		"session_state_truncated", res.SessionTruncated,
		"surfaces_truncated", res.SurfacesTruncated,
		"checkpoint_reset", res.CheckpointReset,
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(knowledgeLoopReprojectResponse{
		OK:                     true,
		EntriesTruncated:       res.EntriesTruncated,
		SessionStateTruncated:  res.SessionTruncated,
		SurfacesTruncated:      res.SurfacesTruncated,
		CheckpointReset:        res.CheckpointReset,
		ProjectorWillRunOnTick: "Projector picks up from event_seq=0 on next scheduler tick (~5s).",
	})
}
