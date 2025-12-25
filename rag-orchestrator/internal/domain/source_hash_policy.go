package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SourceHashPolicy defines the logic to compute a stable hash for a document source.
// It ensures idempotency: same title+body (normalized) -> same hash.
type SourceHashPolicy interface {
	Compute(title, body string) string
}

type sourceHashPolicy struct{}

// NewSourceHashPolicy creates a new instance of the default SourceHashPolicy.
func NewSourceHashPolicy() SourceHashPolicy {
	return &sourceHashPolicy{}
}

// Compute returns the SHA-256 hash of the normalized content.
// Normalization currently means trimming leading/trailing whitespace.
func (p *sourceHashPolicy) Compute(title, body string) string {
	normalizedTitle := strings.TrimSpace(title)
	normalizedBody := strings.TrimSpace(body)

	// Combine components. Using a separator to avoid ambiguity (e.g., "A"+"B" vs "AB"+"")
	// Using null byte or similar as separator might be safer, but for simple text, a newline or specific delimiter is okay.
	// Let's use a null byte to be robust.
	content := normalizedTitle + "\x00" + normalizedBody

	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
