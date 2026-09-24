package sovereign_db

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateProjectionVersionArgs(t *testing.T) {
	t0 := time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC)
	v := ProjectionVersion{
		Version:     3,
		Description: "next-gen projection",
		Status:      "inactive",
		CreatedAt:   t0,
		ActivatedAt: nil,
	}

	args := buildCreateProjectionVersionArgs(v)
	require.Len(t, args, 5)
	assert.Equal(t, 3, args[0])
	assert.Equal(t, "next-gen projection", args[1])
	assert.Equal(t, "inactive", args[2])
	assert.Equal(t, t0, args[3])
	assert.Nil(t, args[4])
}

func TestScanProjectionVersion_Success(t *testing.T) {
	t0 := time.Date(2026, 9, 25, 8, 0, 0, 0, time.UTC)
	mock := &mockRow{scanFunc: func(dest ...interface{}) error {
		require.Len(t, dest, 5)
		*dest[0].(*int) = 4
		*dest[1].(*string) = "test desc"
		*dest[2].(*string) = "active"
		*dest[3].(*time.Time) = t0
		*dest[4].(**time.Time) = &t0
		return nil
	}}

	v, err := scanProjectionVersion(mock)
	require.NoError(t, err)
	assert.Equal(t, 4, v.Version)
	assert.Equal(t, "test desc", v.Description)
	assert.Equal(t, "active", v.Status)
	assert.Equal(t, t0, v.CreatedAt)
	require.NotNil(t, v.ActivatedAt)
	assert.Equal(t, t0, *v.ActivatedAt)
}

func TestScanProjectionVersion_Error(t *testing.T) {
	mock := &mockRow{scanFunc: func(_ ...interface{}) error {
		return errors.New("scan failure")
	}}

	_, err := scanProjectionVersion(mock)
	require.Error(t, err)
}
