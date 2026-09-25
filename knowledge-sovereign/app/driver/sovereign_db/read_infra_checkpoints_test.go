package sovereign_db

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAdvanceCheckpointInsertArgs(t *testing.T) {
	args := buildAdvanceCheckpointInsertArgs("test-projector", 500)
	require.Len(t, args, 2)
	assert.Equal(t, "test-projector", args[0])
	assert.EqualValues(t, 500, args[1])
}

func TestBuildAdvanceCheckpointCASArgs(t *testing.T) {
	t0 := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
	from := ProjectionCheckpoint{
		LastEventSeq: 100,
		UpdatedAt:    t0,
		Exists:       true,
	}

	args := buildAdvanceCheckpointCASArgs("test-projector", from, 200)
	require.Len(t, args, 4)
	assert.Equal(t, "test-projector", args[0])
	assert.EqualValues(t, 100, args[1])
	assert.EqualValues(t, 200, args[2])
	assert.Equal(t, t0, args[3])
}

func TestLiveProjectorNames_NotEmpty(t *testing.T) {
	names := liveProjectorNames()
	assert.NotEmpty(t, names)
	targets := RebuildTargets()
	assert.Equal(t, len(targets), len(names))
}

func TestScanProjectionCheckpoint_Success(t *testing.T) {
	t0 := time.Date(2026, 9, 25, 14, 0, 0, 0, time.UTC)
	mock := &mockRow{scanFunc: func(dest ...interface{}) error {
		require.Len(t, dest, 2)
		*dest[0].(*int64) = 999
		*dest[1].(*time.Time) = t0
		return nil
	}}

	cp, err := scanProjectionCheckpoint(mock)
	require.NoError(t, err)
	assert.EqualValues(t, 999, cp.LastEventSeq)
	assert.Equal(t, t0, cp.UpdatedAt)
	assert.True(t, cp.Exists)
}

func TestScanProjectionCheckpoint_Error(t *testing.T) {
	mock := &mockRow{scanFunc: func(_ ...interface{}) error {
		return errors.New("scan failure")
	}}

	_, err := scanProjectionCheckpoint(mock)
	require.Error(t, err)
}
