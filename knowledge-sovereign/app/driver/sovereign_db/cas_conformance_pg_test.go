//go:build integration

package sovereign_db

// Replays every CAS|* golden vector emitted by specs/tauto/sovereign-projection
// against a real PostgreSQL instance carrying the Atlas migration history
// (ADR-000965 / ADR-000967 / ADR-001031 Phase 1).

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

func TestCASConformance_ReplaysTautoVectorsAgainstRealPostgreSQL(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../usecase/projection_gap/testdata/projection_safety_vectors.txt")
	require.NoError(t, err, "read golden vector file")

	var vectors [][]string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "|")
		if fields[0] == "CAS" {
			require.Len(t, fields, 9, "malformed CAS vector %q", line)
			vectors = append(vectors, fields)
		}
	}
	require.NoError(t, sc.Err())
	require.GreaterOrEqual(t, len(vectors), 8, "expected at least 8 CAS vectors")

	for _, fields := range vectors {
		name := fields[1]
		initRow := fields[2]
		op := fields[3]
		toSeq, err := strconv.ParseInt(fields[4], 10, 64)
		require.NoError(t, err)
		wantApplied := fields[6] == "true"
		wantRow := fields[7]
		wantEmptied := fields[8] == "true"

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := pgtest.NewDB(t)
			repo := NewRepository(db)
			target := pgTarget(t, "knowledge-home")

			// Seed read models so wantEmptied is falsifiable on real tables.
			pgSeedProjections(ctx, t, db, 0, 0)
			if initRow == "absent" {
				pgExec(ctx, t, db, `DELETE FROM knowledge_projection_checkpoints WHERE projector_name = $1`, pgHomeProjector)
			} else {
				parts := strings.Split(initRow, ":")
				require.Len(t, parts, 3)
				initSeq, err := strconv.ParseInt(parts[1], 10, 64)
				require.NoError(t, err)
				// Seed with fixed past timestamp pgCheckpointAt so subsequent
				// server-side now() writes are strictly newer (tok_ts < write_ts).
				pgSetCheckpoint(ctx, t, db, pgHomeProjector, initSeq)
			}

			tok, err := repo.ReadProjectionCheckpointForAdvance(ctx, pgHomeProjector)
			require.NoError(t, err)

			switch {
			case op == "none":
			case strings.HasPrefix(op, "rebuild:"):
				_, err := repo.RebuildProjection(ctx, target)
				require.NoError(t, err)
			case strings.HasPrefix(op, "wire_update:"):
				parts := strings.Split(strings.TrimPrefix(op, "wire_update:"), ":")
				require.Len(t, parts, 2)
				wireSeq, err := strconv.ParseInt(parts[0], 10, 64)
				require.NoError(t, err)
				require.NoError(t, repo.UpdateProjectionCheckpoint(ctx, pgHomeProjector, wireSeq))
			case strings.HasPrefix(op, "reuse:"):
				parts := strings.Split(strings.TrimPrefix(op, "reuse:"), ":")
				require.Len(t, parts, 2)
				firstSeq, err := strconv.ParseInt(parts[0], 10, 64)
				require.NoError(t, err)
				appliedFirst, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, tok, firstSeq)
				require.NoError(t, err)
				require.True(t, appliedFirst, "precondition: first advance must consume token")
				// Ensure server clock advances past microsecond granularity before reuse attempt.
				time.Sleep(time.Millisecond)
			case strings.HasPrefix(op, "fabricate_present:"):
				parts := strings.Split(strings.TrimPrefix(op, "fabricate_present:"), ":")
				require.Len(t, parts, 2)
				fakeSeq, err := strconv.ParseInt(parts[0], 10, 64)
				require.NoError(t, err)
				tok = ProjectionCheckpoint{
					LastEventSeq: fakeSeq,
					UpdatedAt:    pgCheckpointAt,
					Exists:       true,
				}
			default:
				t.Fatalf("unknown CAS op %q", op)
			}

			applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, tok, toSeq)
			require.NoError(t, err)
			assert.Equal(t, wantApplied, applied)

			gotSeq, gotUpdatedAt, gotExists := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
			if wantRow == "absent" {
				assert.False(t, gotExists)
			} else {
				parts := strings.Split(wantRow, ":")
				require.Len(t, parts, 3)
				wantSeq, err := strconv.ParseInt(parts[1], 10, 64)
				require.NoError(t, err)
				assert.True(t, gotExists)
				assert.Equal(t, wantSeq, gotSeq)
				if wantApplied && initRow != "absent" {
					assert.True(t, gotUpdatedAt.After(pgCheckpointAt), "applied advance must move updated_at witness forward")
				}
			}

			for _, table := range target.Tables() {
				if wantEmptied {
					assert.Zero(t, pgCountRows(ctx, t, db, table), "rebuild must leave %s empty", table)
				} else {
					assert.EqualValues(t, pgSeedRows, pgCountRows(ctx, t, db, table), "non-rebuild op must leave %s intact", table)
				}
			}
		})
	}
}
