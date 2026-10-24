package sovereign_db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type snapshotTestRows struct {
	fakeEmptyRows
	scans []func(dest ...any) error
	idx   int
	err   error
}

func (r *snapshotTestRows) Next() bool {
	if r.idx < len(r.scans) {
		r.idx++
		return true
	}
	return false
}

func (r *snapshotTestRows) Scan(dest ...any) error {
	return r.scans[r.idx-1](dest...)
}

func (r *snapshotTestRows) Err() error { return r.err }

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

func TestSanitizedTableIdentifier(t *testing.T) {
	t.Run("accepts valid identifiers", func(t *testing.T) {
		ident, err := sanitizedTableIdentifier("knowledge_home_items")
		require.NoError(t, err)
		assert.Equal(t, `"knowledge_home_items"`, ident)
	})

	t.Run("rejects unsafe identifiers", func(t *testing.T) {
		for _, bad := range []string{"", "1table", "table-name", "public.table", "table; DROP TABLE users"} {
			_, err := sanitizedTableIdentifier(bad)
			require.Error(t, err, "expected error for %q", bad)
		}
	})
}

func TestInsertSnapshot(t *testing.T) {
	ctx := context.Background()
	snapID := uuid.New()
	createdAt := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

	validMeta := func() *SnapshotMetadata {
		return &SnapshotMetadata{
			SnapshotID:        snapID,
			SnapshotType:      "full",
			ProjectionVersion: 1,
			ProjectorBuildRef: "abc123",
			SchemaVersion:     "00032",
			SnapshotAt:        createdAt,
			EventSeqBoundary:  1000,
			SnapshotDataPath:  "/data/snapshots/snapshot_1",
			ItemsRowCount:     100,
			ItemsChecksum:     "sha256:abc",
			DigestRowCount:    10,
			DigestChecksum:    "sha256:def",
			RecallRowCount:    5,
			RecallChecksum:    "sha256:ghi",
			Status:            "valid",
		}
	}

	t.Run("scans RETURNING snapshot_id and created_at", func(t *testing.T) {
		returnedID := uuid.New()
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(dest ...interface{}) error {
						require.Len(t, dest, 2)
						*dest[0].(*uuid.UUID) = returnedID
						*dest[1].(*time.Time) = createdAt
						return nil
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		s := validMeta()
		err := repo.InsertSnapshot(ctx, s)
		require.NoError(t, err)
		require.Len(t, mock.queryRowCalls, 1)
		assert.Contains(t, mock.queryRowCalls[0].SQL, "RETURNING snapshot_id, created_at")
		assert.Equal(t, returnedID, s.SnapshotID)
		assert.Equal(t, createdAt, s.CreatedAt)
	})

	t.Run("validation failure skips query", func(t *testing.T) {
		mock := &mockPgx{}
		repo := &Repository{pool: mock}

		s := validMeta()
		s.ProjectorBuildRef = ""
		err := repo.InsertSnapshot(ctx, s)
		require.Error(t, err)
		assert.Empty(t, mock.queryRowCalls)
	})

	t.Run("propagates scan error", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(_ ...interface{}) error {
						return errors.New("db insert failed")
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		err := repo.InsertSnapshot(ctx, validMeta())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "InsertSnapshot")
	})
}

func TestUpdateSnapshotStatus(t *testing.T) {
	ctx := context.Background()
	snapID := uuid.New()

	t.Run("executes update", func(t *testing.T) {
		mock := &mockPgx{}
		repo := &Repository{pool: mock}

		err := repo.UpdateSnapshotStatus(ctx, snapID, "archived")
		require.NoError(t, err)
		require.Len(t, mock.execCalls, 1)
		assert.Equal(t, []interface{}{"archived", snapID}, mock.execCalls[0].Args)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockPgx{
			execFunc: func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
				return pgconn.CommandTag{}, errors.New("exec error")
			},
		}
		repo := &Repository{pool: mock}

		err := repo.UpdateSnapshotStatus(ctx, snapID, "archived")
		require.Error(t, err)
	})
}

func TestGetLatestValidSnapshot(t *testing.T) {
	ctx := context.Background()
	snapID := uuid.New()
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	t.Run("returns snapshot when found", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(dest ...interface{}) error {
						*dest[0].(*uuid.UUID) = snapID
						*dest[1].(*string) = "full"
						*dest[2].(*int) = 2
						*dest[3].(*string) = "build-1"
						*dest[4].(*string) = "00032"
						*dest[5].(*time.Time) = now
						*dest[6].(*int64) = 500
						*dest[7].(*string) = "/data/snapshots/snap1"
						*dest[8].(*int) = 10
						*dest[9].(*string) = "sha256:1"
						*dest[10].(*int) = 2
						*dest[11].(*string) = "sha256:2"
						*dest[12].(*int) = 3
						*dest[13].(*string) = "sha256:3"
						*dest[14].(*time.Time) = now
						*dest[15].(*string) = "valid"
						return nil
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetLatestValidSnapshot(ctx)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, snapID, got.SnapshotID)
		assert.Equal(t, "00032", got.SchemaVersion)
		assert.Equal(t, int64(500), got.EventSeqBoundary)
		assert.Equal(t, "valid", got.Status)
	})

	t.Run("returns nil on ErrNoRows", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(_ ...interface{}) error {
						return pgx.ErrNoRows
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetLatestValidSnapshot(ctx)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestGetMaxEventSeq(t *testing.T) {
	ctx := context.Background()
	mock := &mockPgx{
		queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
			return &mockRow{
				scanFunc: func(dest ...interface{}) error {
					*dest[0].(*int64) = 12345
					return nil
				},
			}
		},
	}
	repo := &Repository{pool: mock}

	seq, err := repo.GetMaxEventSeq(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), seq)
}

func TestGetTableRowCount(t *testing.T) {
	ctx := context.Background()

	t.Run("returns row count for valid table name", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(dest ...interface{}) error {
						*dest[0].(*int) = 42
						return nil
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		count, err := repo.GetTableRowCount(ctx, "knowledge_home_items")
		require.NoError(t, err)
		assert.Equal(t, 42, count)
		require.Len(t, mock.queryRowCalls, 1)
		assert.Equal(t, `SELECT COUNT(*) FROM "knowledge_home_items"`, mock.queryRowCalls[0].SQL)
	})

	t.Run("rejects invalid table name", func(t *testing.T) {
		mock := &mockPgx{}
		repo := &Repository{pool: mock}

		_, err := repo.GetTableRowCount(ctx, "invalid-table; DROP TABLE x")
		require.Error(t, err)
		assert.Empty(t, mock.queryRowCalls)
	})
}

func TestGetStorageStats(t *testing.T) {
	ctx := context.Background()
	mock := &mockPgx{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &snapshotTestRows{
				scans: []func(dest ...any) error{
					func(dest ...any) error {
						*dest[0].(*string) = "knowledge_events"
						*dest[1].(*int64) = 50000
						*dest[2].(*string) = "64 MB"
						*dest[3].(*string) = "48 MB"
						*dest[4].(*string) = "16 MB"
						*dest[5].(*int64) = 67108864
						*dest[6].(*bool) = true
						return nil
					},
				},
			}, nil
		},
	}
	repo := &Repository{pool: mock}

	stats, err := repo.GetStorageStats(ctx)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "knowledge_events", stats[0].TableName)
	assert.Equal(t, int64(50000), stats[0].RowCount)
	assert.Equal(t, "64 MB", stats[0].TotalSize)
	assert.Equal(t, "48 MB", stats[0].TableSize)
	assert.Equal(t, "16 MB", stats[0].IndexSize)
	assert.Equal(t, int64(67108864), stats[0].TotalBytes)
	assert.True(t, stats[0].IsPartitioned)
}
