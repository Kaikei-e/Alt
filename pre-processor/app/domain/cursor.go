package domain

import (
	"time"
)

// Cursor represents pagination cursor for efficient pagination.
type Cursor struct {
	LastCreatedAt *time.Time
	LastID        string
}
