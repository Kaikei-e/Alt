package backfill

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

const CursorVersion = 1

// Cursor represents the current position in backfill processing.
type Cursor struct {
	Version        int       `json:"version"`
	LastCreatedAt  time.Time `json:"last_created_at"`
	LastID         string    `json:"last_id"`
	CurrentDate    string    `json:"current_date,omitempty"`
	ProcessedCount int       `json:"processed_count"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// IsEmpty returns true if the cursor has no position set.
func (c Cursor) IsEmpty() bool {
	return c.LastCreatedAt.IsZero() && c.LastID == ""
}

// CursorManager handles cursor persistence with atomic writes and file locking.
type CursorManager struct {
	filePath string
	lockFile *os.File
}

// NewCursorManager creates a new CursorManager for the given file path.
func NewCursorManager(filePath string) *CursorManager {
	return &CursorManager{
		filePath: filePath,
	}
}

// Lock acquires an exclusive lock on the cursor file.
// Returns an error if the lock is already held by another process.
func (m *CursorManager) Lock() error {
	lockPath := m.filePath + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("cursor is locked by another process")
		}
		return fmt.Errorf("acquire lock: %w", err)
	}

	m.lockFile = f
	return nil
}

// Unlock releases the exclusive lock on the cursor file.
func (m *CursorManager) Unlock() error {
	if m.lockFile == nil {
		return nil
	}

	if err := syscall.Flock(int(m.lockFile.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("release lock: %w", err)
	}

	if err := m.lockFile.Close(); err != nil {
		return fmt.Errorf("close lock file: %w", err)
	}

	m.lockFile = nil

	// Remove lock file
	lockPath := m.filePath + ".lock"
	_ = os.Remove(lockPath)

	return nil
}

// Load reads the cursor from disk.
// Returns an empty cursor if the file doesn't exist or is empty.
func (m *CursorManager) Load() (Cursor, error) {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Cursor{Version: CursorVersion}, nil
		}
		return Cursor{}, fmt.Errorf("read cursor file: %w", err)
	}

	// Handle empty file
	if len(data) == 0 {
		return Cursor{Version: CursorVersion}, nil
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return Cursor{}, fmt.Errorf("parse cursor file: %w", err)
	}

	// Handle version migration if needed
	if cursor.Version == 0 {
		cursor.Version = CursorVersion
	}

	return cursor, nil
}

// Save writes the cursor to disk atomically.
// Uses write-to-temp-then-rename pattern to prevent corruption.
func (m *CursorManager) Save(cursor Cursor) error {
	cursor.Version = CursorVersion
	cursor.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cursor: %w", err)
	}

	// Write to temporary file
	tmpPath := m.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp cursor file: %w", err)
	}

	// Atomically rename to target path
	if err := os.Rename(tmpPath, m.filePath); err != nil {
		// Clean up temp file on failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename cursor file: %w", err)
	}

	return nil
}

// Reset clears the cursor file.
func (m *CursorManager) Reset() error {
	if err := os.Remove(m.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cursor file: %w", err)
	}
	return nil
}

// FilePath returns the cursor file path.
func (m *CursorManager) FilePath() string {
	return m.filePath
}
