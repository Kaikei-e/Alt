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

// RetentionRepository defines the retention-specific operations.
type RetentionRepository interface {
	ListPartitions(ctx context.Context, tableName string) ([]sovereign_db.PartitionInfo, error)
	ExportTableToWriter(ctx context.Context, tableName string, w io.Writer) (int64, error)
	InsertRetentionLog(ctx context.Context, entry sovereign_db.RetentionLogEntry) error
	ListRetentionLogs(ctx context.Context, limit int) ([]sovereign_db.RetentionLogEntry, error)
	GetLatestValidSnapshot(ctx context.Context) (*sovereign_db.SnapshotMetadata, error)
	GetMaxEventSeq(ctx context.Context) (int64, error)
}

// RetentionHandler provides HTTP endpoints for retention operations.
type RetentionHandler struct {
	repo       RetentionRepository
	archiveDir string
	policy     sovereign_db.RetentionPolicy
}

// NewRetentionHandler creates a new retention handler.
func NewRetentionHandler(repo RetentionRepository, archiveDir string) *RetentionHandler {
	return &RetentionHandler{
		repo:       repo,
		archiveDir: archiveDir,
		policy:     sovereign_db.DefaultRetentionPolicy(),
	}
}

// RegisterRoutes registers retention HTTP routes on the given mux.
func (h *RetentionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/retention/run", h.handleRunRetention)
	mux.HandleFunc("GET /admin/retention/status", h.handleRetentionStatus)
	mux.HandleFunc("GET /admin/retention/eligible", h.handleEligiblePartitions)
}

type retentionRunRequest struct {
	DryRun bool `json:"dry_run"`
}

type retentionRunResponse struct {
	DryRun  bool              `json:"dry_run"`
	Actions []retentionAction `json:"actions"`
	Error   string            `json:"error,omitempty"`
}

type retentionAction struct {
	Action    string `json:"action"`
	Table     string `json:"table"`
	Partition string `json:"partition"`
	Rows      int64  `json:"rows"`
	Path      string `json:"path,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
	Status    string `json:"status"`
}

func (h *RetentionHandler) handleRunRetention(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req retentionRunRequest
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}
	// Default to dry-run for safety
	if r.Method == http.MethodPost && r.Body == nil {
		req.DryRun = true
	}

	result, err := h.RunRetention(ctx, req.DryRun)
	if err != nil {
		slog.ErrorContext(ctx, "retention run failed", "error", err)
		result.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *RetentionHandler) handleRetentionStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logs, err := h.repo.ListRetentionLogs(ctx, 20)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (h *RetentionHandler) handleEligiblePartitions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	type eligibleResult struct {
		Table    string                       `json:"table"`
		Eligible []sovereign_db.PartitionInfo `json:"eligible"`
	}

	var results []eligibleResult
	for _, tableName := range []string{"knowledge_events", "knowledge_user_events"} {
		parts, err := h.repo.ListPartitions(ctx, tableName)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		eligible := h.policy.PartitionsEligibleForArchive(tableName, parts, now)
		results = append(results, eligibleResult{Table: tableName, Eligible: eligible})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// RunRetention executes the retention cycle: export → verify → log.
// If dryRun is true, only reports what would be done without modifying data.
func (h *RetentionHandler) RunRetention(ctx context.Context, dryRun bool) (retentionRunResponse, error) {
	resp := retentionRunResponse{DryRun: dryRun}
	now := time.Now()

	// Safety check: require a valid snapshot before archiving
	snapshot, err := h.repo.GetLatestValidSnapshot(ctx)
	if err != nil || snapshot == nil {
		return resp, fmt.Errorf("no valid snapshot found; create a snapshot before running retention")
	}

	// Process each partitioned table
	for _, tableName := range []string{"knowledge_events", "knowledge_user_events"} {
		parts, err := h.repo.ListPartitions(ctx, tableName)
		if err != nil {
			return resp, fmt.Errorf("list partitions for %s: %w", tableName, err)
		}

		eligible := h.policy.PartitionsEligibleForArchive(tableName, parts, now)

		for _, part := range eligible {
			action := retentionAction{
				Action:    "export",
				Table:     tableName,
				Partition: part.Name,
				Status:    "planned",
			}

			if dryRun {
				action.Status = "dry_run"
				resp.Actions = append(resp.Actions, action)
				continue
			}

			// Export partition to JSONL.gz
			archivePath, rowCount, checksum, err := h.exportPartition(ctx, part.Name)
			if err != nil {
				action.Status = "failed"
				h.logAction(ctx, action, dryRun, err)
				resp.Actions = append(resp.Actions, action)
				return resp, fmt.Errorf("export %s: %w", part.Name, err)
			}

			action.Rows = rowCount
			action.Path = archivePath
			action.Checksum = checksum
			action.Status = "exported"
			h.logAction(ctx, action, dryRun, nil)
			resp.Actions = append(resp.Actions, action)
		}
	}

	return resp, nil
}

// exportPartition exports a partition table to a gzipped JSONL file.
func (h *RetentionHandler) exportPartition(ctx context.Context, partitionName string) (string, int64, string, error) {
	if err := os.MkdirAll(h.archiveDir, 0o755); err != nil {
		return "", 0, "", fmt.Errorf("create archive dir: %w", err)
	}

	filePath := filepath.Join(h.archiveDir, fmt.Sprintf("%s_%s.jsonl.gz",
		partitionName, time.Now().Format("20060102")))

	f, err := os.Create(filePath)
	if err != nil {
		return "", 0, "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	gzWriter := gzip.NewWriter(io.MultiWriter(f, hasher))

	rowCount, err := h.repo.ExportTableToWriter(ctx, partitionName, gzWriter)
	if err != nil {
		gzWriter.Close()
		os.Remove(filePath)
		return "", 0, "", fmt.Errorf("export: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return "", 0, "", fmt.Errorf("close gzip: %w", err)
	}

	checksum := fmt.Sprintf("sha256:%x", hasher.Sum(nil))

	slog.InfoContext(ctx, "partition exported",
		"partition", partitionName, "rows", rowCount,
		"checksum", checksum, "path", filePath)

	return filePath, rowCount, checksum, nil
}

func (h *RetentionHandler) logAction(ctx context.Context, action retentionAction, dryRun bool, err error) {
	entry := sovereign_db.RetentionLogEntry{
		LogID:           uuid.New(),
		RunAt:           time.Now(),
		Action:          action.Action,
		TargetTable:     action.Table,
		TargetPartition: action.Partition,
		RowsAffected:    action.Rows,
		ArchivePath:     action.Path,
		Checksum:        action.Checksum,
		DryRun:          dryRun,
		Status:          action.Status,
	}
	if err != nil {
		entry.Status = "failed"
		entry.ErrorMessage = err.Error()
	}
	if logErr := h.repo.InsertRetentionLog(ctx, entry); logErr != nil {
		slog.ErrorContext(ctx, "failed to log retention action", "error", logErr)
	}
}
