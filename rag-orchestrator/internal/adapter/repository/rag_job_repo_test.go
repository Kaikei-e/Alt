package repository

import (
	"testing"
	"time"

	"rag-orchestrator/internal/backfill"
)

func TestStaleProcessingLease_ExceedsRebuildJobTimeoutWithMargin(t *testing.T) {
	rebuildTimeout := backfill.DefaultRebuildConfig().JobTimeout
	const minMargin = time.Minute

	if staleProcessingLease <= rebuildTimeout+minMargin {
		t.Fatalf("staleProcessingLease (%s) must exceed DefaultRebuildConfig().JobTimeout (%s) by more than %s so in-flight rebuild jobs are not reclaimed before timeout and status update",
			staleProcessingLease, rebuildTimeout, minMargin)
	}
}
