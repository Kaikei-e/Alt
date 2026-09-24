package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"knowledge-sovereign/driver/sovereign_db"
)

func TestBuildEligiblePartitionRows(t *testing.T) {
	start1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	end2 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		tableName string
		eligible  []sovereign_db.PartitionInfo
		wantLen   int
	}{
		{
			name:      "empty list",
			tableName: "knowledge_events",
			eligible:  nil,
			wantLen:   0,
		},
		{
			name:      "multiple partitions",
			tableName: "knowledge_events",
			eligible: []sovereign_db.PartitionInfo{
				{Name: "ke_p1", RangeStart: start1, RangeEnd: end1, RowCount: 100, SizeBytes: 2048},
				{Name: "ke_p2", RangeStart: start2, RangeEnd: end2, RowCount: 200, SizeBytes: 4096},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := buildEligiblePartitionRows(tt.tableName, tt.eligible)
			require.Len(t, rows, tt.wantLen)
			if tt.wantLen > 0 {
				assert.Equal(t, tt.tableName, rows[0].TableName)
				assert.Equal(t, "ke_p1", rows[0].PartitionName)
				assert.Equal(t, start1.Format(time.RFC3339), rows[0].RangeStart)
				assert.Equal(t, end1.Format(time.RFC3339), rows[0].RangeEnd)
				assert.Equal(t, int64(100), rows[0].RowCount)
				assert.Equal(t, int64(2048), rows[0].SizeBytes)
			}
		})
	}
}

func TestBuildRetentionLogEntry(t *testing.T) {
	refTime := time.Date(2026, 9, 25, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		action     retentionAction
		dryRun     bool
		err        error
		wantStatus string
		wantErrMsg string
		wantRows   int64
		wantTable  string
		wantPart   string
		wantPath   string
		wantChksum string
	}{
		{
			name: "successful export action",
			action: retentionAction{
				Action:    "export",
				Table:     "knowledge_events",
				Partition: "ke_2026_01",
				Rows:      500,
				Path:      "/archives/ke_2026_01.jsonl.gz",
				Checksum:  "sha256:abcd",
				Status:    "exported",
			},
			dryRun:     false,
			err:        nil,
			wantStatus: "exported",
			wantRows:   500,
			wantTable:  "knowledge_events",
			wantPart:   "ke_2026_01",
			wantPath:   "/archives/ke_2026_01.jsonl.gz",
			wantChksum: "sha256:abcd",
		},
		{
			name: "failed action overrides status and sets error message",
			action: retentionAction{
				Action:    "export",
				Table:     "knowledge_events",
				Partition: "ke_2026_02",
				Status:    "failed",
			},
			dryRun:     false,
			err:        errors.New("disk full"),
			wantStatus: "failed",
			wantErrMsg: "disk full",
			wantTable:  "knowledge_events",
			wantPart:   "ke_2026_02",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := buildRetentionLogEntry(tt.action, tt.dryRun, tt.err, refTime)
			assert.NotEqual(t, uuid.Nil, entry.LogID)
			assert.Equal(t, refTime, entry.RunAt)
			assert.Equal(t, tt.action.Action, entry.Action)
			assert.Equal(t, tt.wantTable, entry.TargetTable)
			assert.Equal(t, tt.wantPart, entry.TargetPartition)
			assert.Equal(t, tt.wantRows, entry.RowsAffected)
			assert.Equal(t, tt.wantPath, entry.ArchivePath)
			assert.Equal(t, tt.wantChksum, entry.Checksum)
			assert.Equal(t, tt.dryRun, entry.DryRun)
			assert.Equal(t, tt.wantStatus, entry.Status)
			assert.Equal(t, tt.wantErrMsg, entry.ErrorMessage)
		})
	}
}
