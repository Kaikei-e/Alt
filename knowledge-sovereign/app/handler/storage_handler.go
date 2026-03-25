package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"knowledge-sovereign/driver/sovereign_db"
)

// StorageRepository defines storage metrics operations.
type StorageRepository interface {
	GetStorageStats(ctx context.Context) ([]sovereign_db.TableStorageInfo, error)
	ListPartitions(ctx context.Context, tableName string) ([]sovereign_db.PartitionInfo, error)
}

// StorageHandler provides HTTP endpoints for storage monitoring.
type StorageHandler struct {
	repo StorageRepository
}

// NewStorageHandler creates a new storage handler.
func NewStorageHandler(repo StorageRepository) *StorageHandler {
	return &StorageHandler{repo: repo}
}

// RegisterRoutes registers storage HTTP routes on the given mux.
func (h *StorageHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/storage/stats", h.handleStorageStats)
}

func (h *StorageHandler) handleStorageStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.repo.GetStorageStats(ctx)
	if err != nil {
		http.Error(w, `{"error": "failed to get storage stats"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
