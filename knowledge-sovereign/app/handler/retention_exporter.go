package handler

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"knowledge-sovereign/driver/sovereign_db"
)

// buildEligiblePartitionRows converts domain partition metadata to API rows.
func buildEligiblePartitionRows(tableName string, eligible []sovereign_db.PartitionInfo) []eligiblePartitionRow {
	rows := make([]eligiblePartitionRow, 0, len(eligible))
	for _, part := range eligible {
		rows = append(rows, eligiblePartitionRow{
			TableName:     tableName,
			PartitionName: part.Name,
			RangeStart:    part.RangeStart.Format(time.RFC3339),
			RangeEnd:      part.RangeEnd.Format(time.RFC3339),
			RowCount:      part.RowCount,
			SizeBytes:     part.SizeBytes,
		})
	}
	return rows
}

// buildRetentionLogEntry builds a durable audit record for a retention action.
func buildRetentionLogEntry(action retentionAction, dryRun bool, err error, refTime time.Time) sovereign_db.RetentionLogEntry {
	entry := sovereign_db.RetentionLogEntry{
		LogID:           uuid.New(),
		RunAt:           refTime,
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
	return entry
}

// exportPartition exports a partition table to a gzipped JSONL file with SHA-256 verification.
func (h *RetentionHandler) exportPartition(ctx context.Context, partitionName string, refTime time.Time) (string, int64, string, error) {
	if err := os.MkdirAll(h.archiveDir, 0o755); err != nil {
		return "", 0, "", fmt.Errorf("create archive dir: %w", err)
	}

	filePath := filepath.Join(h.archiveDir, fmt.Sprintf("%s_%s.jsonl.gz",
		partitionName, refTime.Format("20060102")))

	f, err := os.Create(filePath)
	if err != nil {
		return "", 0, "", fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.WarnContext(ctx, "failed to close archive file", "error", closeErr)
		}
	}()

	hasher := sha256.New()
	gzWriter := gzip.NewWriter(io.MultiWriter(f, hasher))

	rowCount, err := h.repo.ExportTableToWriter(ctx, partitionName, gzWriter)
	if err != nil {
		if closeErr := gzWriter.Close(); closeErr != nil {
			slog.WarnContext(ctx, "failed to close gzip writer", "error", closeErr)
		}
		if rmErr := os.Remove(filePath); rmErr != nil {
			slog.WarnContext(ctx, "failed to remove partial file", "error", rmErr)
		}
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
