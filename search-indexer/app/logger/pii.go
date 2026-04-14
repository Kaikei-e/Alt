package logger

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashQuery returns a short stable hex digest of a search query, safe to ship
// to the log aggregator. It breaks the link between a user's query history
// and the user_id while preserving cardinality for debugging. 8 bytes (16 hex
// chars) give ample collision resistance for log correlation without
// reversing the original text.
func HashQuery(query string) string {
	sum := sha256.Sum256([]byte(query))
	return hex.EncodeToString(sum[:8])
}
