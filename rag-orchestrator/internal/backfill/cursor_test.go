package backfill

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursorManager_LoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursor.json")
	manager := NewCursorManager(cursorPath)

	// Load non-existent cursor
	cursor, err := manager.Load()
	require.NoError(t, err)
	assert.True(t, cursor.IsEmpty())
	assert.Equal(t, CursorVersion, cursor.Version)

	// Save cursor
	now := time.Now().Truncate(time.Millisecond)
	cursor = Cursor{
		LastCreatedAt:  now,
		LastID:         "test-id-123",
		CurrentDate:    "2024-01-15",
		ProcessedCount: 100,
	}
	err = manager.Save(cursor)
	require.NoError(t, err)

	// Load saved cursor
	loaded, err := manager.Load()
	require.NoError(t, err)
	assert.Equal(t, CursorVersion, loaded.Version)
	assert.Equal(t, now.UTC(), loaded.LastCreatedAt.UTC())
	assert.Equal(t, "test-id-123", loaded.LastID)
	assert.Equal(t, "2024-01-15", loaded.CurrentDate)
	assert.Equal(t, 100, loaded.ProcessedCount)
	assert.False(t, loaded.UpdatedAt.IsZero())
}

func TestCursorManager_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursor.json")
	manager := NewCursorManager(cursorPath)

	cursor := Cursor{
		LastCreatedAt:  time.Now(),
		LastID:         "id-1",
		ProcessedCount: 50,
	}
	err := manager.Save(cursor)
	require.NoError(t, err)

	// Verify temp file doesn't exist after save
	tmpPath := cursorPath + ".tmp"
	_, err = os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(err))

	// Verify cursor file exists
	_, err = os.Stat(cursorPath)
	assert.NoError(t, err)
}

func TestCursorManager_Reset(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursor.json")
	manager := NewCursorManager(cursorPath)

	// Save a cursor
	cursor := Cursor{
		LastCreatedAt: time.Now(),
		LastID:        "test-id",
	}
	err := manager.Save(cursor)
	require.NoError(t, err)

	// Reset
	err = manager.Reset()
	require.NoError(t, err)

	// Verify file is gone
	_, err = os.Stat(cursorPath)
	assert.True(t, os.IsNotExist(err))

	// Load should return empty cursor
	loaded, err := manager.Load()
	require.NoError(t, err)
	assert.True(t, loaded.IsEmpty())
}

func TestCursorManager_Lock(t *testing.T) {
	tmpDir := t.TempDir()
	cursorPath := filepath.Join(tmpDir, "cursor.json")

	manager1 := NewCursorManager(cursorPath)
	manager2 := NewCursorManager(cursorPath)

	// First lock should succeed
	err := manager1.Lock()
	require.NoError(t, err)

	// Second lock should fail
	err = manager2.Lock()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "locked by another process")

	// Unlock first manager
	err = manager1.Unlock()
	require.NoError(t, err)

	// Now second lock should succeed
	err = manager2.Lock()
	require.NoError(t, err)

	err = manager2.Unlock()
	require.NoError(t, err)
}

func TestCursor_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		cursor   Cursor
		expected bool
	}{
		{
			name:     "empty cursor",
			cursor:   Cursor{},
			expected: true,
		},
		{
			name: "cursor with only ID",
			cursor: Cursor{
				LastID: "id-1",
			},
			expected: false,
		},
		{
			name: "cursor with only time",
			cursor: Cursor{
				LastCreatedAt: time.Now(),
			},
			expected: false,
		},
		{
			name: "cursor with both",
			cursor: Cursor{
				LastCreatedAt: time.Now(),
				LastID:        "id-1",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cursor.IsEmpty())
		})
	}
}
