package domain

import "fmt"

const (
	// IdempotencyLastWriteWins means the mutation is inherently idempotent (upsert).
	IdempotencyLastWriteWins = "last_write_wins"
	// IdempotencySkipIfApplied means the mutation should be skipped if already applied.
	IdempotencySkipIfApplied = "skip_if_applied"
)

// BuildIdempotencyKey creates a deterministic key for deduplication.
func BuildIdempotencyKey(mutationType, entityID string) string {
	return fmt.Sprintf("%s:%s", mutationType, entityID)
}

// GetIdempotencyPolicy returns the idempotency policy for a mutation type.
func GetIdempotencyPolicy(mutationType string) string {
	switch mutationType {
	case "upsert_home_item", "upsert_today_digest", "upsert_recall_candidate", "upsert_candidate":
		return IdempotencyLastWriteWins
	default:
		return IdempotencySkipIfApplied
	}
}
