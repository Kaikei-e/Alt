package sovereign_db

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotMetadata_Validate(t *testing.T) {
	validMeta := SnapshotMetadata{
		SnapshotID:        uuid.New(),
		SnapshotType:      "full",
		ProjectionVersion: 1,
		ProjectorBuildRef: "abc123",
		SchemaVersion:     "00008",
		SnapshotAt:        time.Now(),
		EventSeqBoundary:  1000,
		SnapshotDataPath:  "/data/snapshots/snapshot_20260325.jsonl.gz",
		ItemsRowCount:     100,
		ItemsChecksum:     "sha256:abc",
		DigestRowCount:    10,
		DigestChecksum:    "sha256:def",
		RecallRowCount:    5,
		RecallChecksum:    "sha256:ghi",
	}

	t.Run("valid metadata passes", func(t *testing.T) {
		err := validMeta.Validate()
		require.NoError(t, err)
	})

	t.Run("empty projector_build_ref fails", func(t *testing.T) {
		m := validMeta
		m.ProjectorBuildRef = ""
		err := m.Validate()
		assert.ErrorContains(t, err, "projector_build_ref")
	})

	t.Run("empty schema_version fails", func(t *testing.T) {
		m := validMeta
		m.SchemaVersion = ""
		err := m.Validate()
		assert.ErrorContains(t, err, "schema_version")
	})

	t.Run("zero event_seq_boundary fails", func(t *testing.T) {
		m := validMeta
		m.EventSeqBoundary = 0
		err := m.Validate()
		assert.ErrorContains(t, err, "event_seq_boundary")
	})

	t.Run("empty snapshot_data_path fails", func(t *testing.T) {
		m := validMeta
		m.SnapshotDataPath = ""
		err := m.Validate()
		assert.ErrorContains(t, err, "snapshot_data_path")
	})

	t.Run("empty checksum fails", func(t *testing.T) {
		m := validMeta
		m.ItemsChecksum = ""
		err := m.Validate()
		assert.ErrorContains(t, err, "items_checksum")
	})
}

func TestSnapshotMetadata_IsCompatibleWith(t *testing.T) {
	base := SnapshotMetadata{
		SchemaVersion:     "00008",
		ProjectorBuildRef: "abc123",
	}

	t.Run("same schema and projector is compatible", func(t *testing.T) {
		assert.True(t, base.IsCompatibleWith("00008", "abc123"))
	})

	t.Run("different schema is incompatible", func(t *testing.T) {
		assert.False(t, base.IsCompatibleWith("00009", "abc123"))
	})

	t.Run("different projector is incompatible", func(t *testing.T) {
		assert.False(t, base.IsCompatibleWith("00008", "def456"))
	})
}
