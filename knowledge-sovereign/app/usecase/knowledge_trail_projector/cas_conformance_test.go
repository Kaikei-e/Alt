package knowledge_trail_projector

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

	"knowledge-sovereign/driver/sovereign_db"
)

// TestFakeRepo_ConformsToTautoCASVectors replays every CAS|* vector emitted by
// specs/tauto/sovereign-projection against knowledge_trail_projector's fakeRepo.
func TestFakeRepo_ConformsToTautoCASVectors(t *testing.T) {
	ctx := context.Background()
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tsOf := func(n int64) time.Time { return epoch.Add(time.Duration(n) * time.Second) }

	raw, err := os.ReadFile("../projection_gap/testdata/projection_safety_vectors.txt")
	require.NoError(t, err)

	casCount := 0
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "|")
		if fields[0] != "CAS" {
			continue
		}
		casCount++
		require.Len(t, fields, 9)
		name := fields[1]
		initRow := fields[2]
		op := fields[3]
		toSeq, err := strconv.ParseInt(fields[4], 10, 64)
		require.NoError(t, err)
		wantApplied := fields[6] == "true"
		wantRow := fields[7]

		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo(nil)
			if initRow == "absent" {
				repo.checkpointExists = false
				repo.checkpoint = 0
				repo.checkpointAt = time.Time{}
			} else {
				parts := strings.Split(initRow, ":")
				require.Len(t, parts, 3)
				seq, err := strconv.ParseInt(parts[1], 10, 64)
				require.NoError(t, err)
				ts, err := strconv.ParseInt(parts[2], 10, 64)
				require.NoError(t, err)
				repo.checkpointExists = true
				repo.checkpoint = seq
				repo.checkpointAt = tsOf(ts)
			}

			tok, err := repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
			require.NoError(t, err)

			switch {
			case op == "none":
			case strings.HasPrefix(op, "rebuild:"):
				ts, err := strconv.ParseInt(strings.TrimPrefix(op, "rebuild:"), 10, 64)
				require.NoError(t, err)
				repo.checkpointExists = true
				repo.checkpoint = 0
				repo.checkpointAt = tsOf(ts)
			case strings.HasPrefix(op, "wire_update:"):
				parts := strings.Split(strings.TrimPrefix(op, "wire_update:"), ":")
				require.Len(t, parts, 2)
				wSeq, err := strconv.ParseInt(parts[0], 10, 64)
				require.NoError(t, err)
				wTS, err := strconv.ParseInt(parts[1], 10, 64)
				require.NoError(t, err)
				repo.checkpointExists = true
				repo.checkpoint = wSeq
				repo.checkpointAt = tsOf(wTS)
			case strings.HasPrefix(op, "reuse:"):
				parts := strings.Split(strings.TrimPrefix(op, "reuse:"), ":")
				require.Len(t, parts, 2)
				firstSeq, err := strconv.ParseInt(parts[0], 10, 64)
				require.NoError(t, err)
				ok, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, tok, firstSeq)
				require.NoError(t, err)
				require.True(t, ok)
			case strings.HasPrefix(op, "fabricate_present:"):
				parts := strings.Split(strings.TrimPrefix(op, "fabricate_present:"), ":")
				require.Len(t, parts, 2)
				fSeq, err := strconv.ParseInt(parts[0], 10, 64)
				require.NoError(t, err)
				fTS, err := strconv.ParseInt(parts[1], 10, 64)
				require.NoError(t, err)
				tok = sovereign_db.ProjectionCheckpoint{LastEventSeq: fSeq, UpdatedAt: tsOf(fTS), Exists: true}
			default:
				t.Fatalf("unknown CAS op %q", op)
			}

			applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, tok, toSeq)
			require.NoError(t, err)
			assert.Equal(t, wantApplied, applied)

			if wantRow == "absent" {
				assert.False(t, repo.checkpointExists)
			} else {
				parts := strings.Split(wantRow, ":")
				require.Len(t, parts, 3)
				wantSeq, err := strconv.ParseInt(parts[1], 10, 64)
				require.NoError(t, err)
				assert.True(t, repo.checkpointExists)
				assert.Equal(t, wantSeq, repo.checkpoint)
			}
		})
	}
	require.NoError(t, sc.Err())
	assert.GreaterOrEqual(t, casCount, 8)
}
