package handler

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"knowledge-sovereign/driver/sovereign_db"
)

// SnapshotRepository defines the snapshot-specific operations.
type SnapshotRepository interface {
	InsertSnapshot(ctx context.Context, s *sovereign_db.SnapshotMetadata) error
	UpdateSnapshotStatus(ctx context.Context, snapshotID uuid.UUID, status string) error
	GetLatestValidSnapshot(ctx context.Context) (*sovereign_db.SnapshotMetadata, error)
	ListSnapshots(ctx context.Context, limit int) ([]sovereign_db.SnapshotMetadata, error)
	ExportTableToWriter(ctx context.Context, tableName string, w io.Writer) (int64, error)
	GetMaxEventSeq(ctx context.Context) (int64, error)
	GetTableRowCount(ctx context.Context, tableName string) (int, error)
}

// SnapshotHandler provides HTTP endpoints for snapshot operations.
type SnapshotHandler struct {
	repo              SnapshotRepository
	snapshotDir       string
	projectorBuildRef string
	schemaVersion     string
}

// NewSnapshotHandler creates a new snapshot handler.
func NewSnapshotHandler(repo SnapshotRepository, snapshotDir, buildRef, schemaVersion string) *SnapshotHandler {
	return &SnapshotHandler{
		repo:              repo,
		snapshotDir:       snapshotDir,
		projectorBuildRef: buildRef,
		schemaVersion:     schemaVersion,
	}
}

// RegisterRoutes registers snapshot HTTP routes on the given mux.
func (h *SnapshotHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/snapshots/create", h.handleCreateSnapshot)
	mux.HandleFunc("GET /admin/snapshots/list", h.handleListSnapshots)
	mux.HandleFunc("GET /admin/snapshots/latest", h.handleGetLatestSnapshot)
}

func (h *SnapshotHandler) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	snapshot, err := h.CreateSnapshot(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "snapshot creation failed", "error", err)
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}

func (h *SnapshotHandler) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	snapshots, err := h.repo.ListSnapshots(ctx, 20)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshots)
}

func (h *SnapshotHandler) handleGetLatestSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	snapshot, err := h.repo.GetLatestValidSnapshot(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}

// CreateSnapshot exports all projection tables and records the snapshot.
func (h *SnapshotHandler) CreateSnapshot(ctx context.Context) (*sovereign_db.SnapshotMetadata, error) {
	// 1. Get current max event_seq (snapshot boundary)
	maxSeq, err := h.repo.GetMaxEventSeq(ctx)
	if err != nil {
		return nil, fmt.Errorf("get max event seq: %w", err)
	}
	if maxSeq <= 0 {
		return nil, fmt.Errorf("no events found, cannot create snapshot")
	}

	snapshotID := uuid.New()
	now := time.Now()
	dateStr := now.Format("20060102_150405")
	baseDir := filepath.Join(h.snapshotDir, fmt.Sprintf("snapshot_%s", dateStr))

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}

	// 2. Export each projection table
	tables := []struct {
		name     string
		filename string
	}{
		{"knowledge_home_items", "knowledge_home_items.jsonl.gz"},
		{"today_digest_view", "today_digest_view.jsonl.gz"},
		{"recall_candidate_view", "recall_candidate_view.jsonl.gz"},
	}

	var itemsCount, digestCount, recallCount int
	var itemsChecksum, digestChecksum, recallChecksum string

	for i, t := range tables {
		filePath := filepath.Join(baseDir, t.filename)
		count, checksum, err := h.exportTable(ctx, t.name, filePath)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", t.name, err)
		}
		slog.InfoContext(ctx, "table exported",
			"table", t.name, "rows", count, "checksum", checksum, "path", filePath)

		switch i {
		case 0:
			itemsCount, itemsChecksum = count, checksum
		case 1:
			digestCount, digestChecksum = count, checksum
		case 2:
			recallCount, recallChecksum = count, checksum
		}
	}

	// 3. Get active projection version
	projVersion := 1 // default

	// 4. Record snapshot metadata
	meta := &sovereign_db.SnapshotMetadata{
		SnapshotID:        snapshotID,
		SnapshotType:      "full",
		ProjectionVersion: projVersion,
		ProjectorBuildRef: h.projectorBuildRef,
		SchemaVersion:     h.schemaVersion,
		SnapshotAt:        now,
		EventSeqBoundary:  maxSeq,
		SnapshotDataPath:  baseDir,
		ItemsRowCount:     itemsCount,
		ItemsChecksum:     itemsChecksum,
		DigestRowCount:    digestCount,
		DigestChecksum:    digestChecksum,
		RecallRowCount:    recallCount,
		RecallChecksum:    recallChecksum,
		Status:            "valid",
	}

	if err := h.repo.InsertSnapshot(ctx, meta); err != nil {
		return nil, fmt.Errorf("save snapshot metadata: %w", err)
	}

	slog.InfoContext(ctx, "snapshot created",
		"snapshot_id", snapshotID,
		"event_seq_boundary", maxSeq,
		"path", baseDir)

	return meta, nil
}

// exportTable exports a table to a gzipped JSONL file and returns row count + SHA-256 checksum.
func (h *SnapshotHandler) exportTable(ctx context.Context, tableName, filePath string) (int, string, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return 0, "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	gzWriter := gzip.NewWriter(io.MultiWriter(f, hasher))
	defer gzWriter.Close()

	rowCount, err := h.repo.ExportTableToWriter(ctx, tableName, gzWriter)
	if err != nil {
		return 0, "", fmt.Errorf("export: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return 0, "", fmt.Errorf("close gzip: %w", err)
	}

	checksum := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
	return int(rowCount), checksum, nil
}
