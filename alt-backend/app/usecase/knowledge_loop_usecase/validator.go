// Package knowledge_loop_usecase implements the Knowledge Loop business-logic orchestration.
// Validators in this file enforce the canonical contract at the handler boundary so the
// DB CHECK constraints are a second line of defense, not the first.
package knowledge_loop_usecase

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// WhyMappingVersion is the historical exhaustive-mapping-table version. The
// authoritative copy now lives in knowledge-sovereign's
// usecase/knowledge_loop_projector package after the projection ownership move
// (ADR-000844 follow-up). This constant remains in alt-backend only for the
// boundary validators below, which still execute in the handler path.
const WhyMappingVersion = 4

var (
	// keyFormat pins the canonical identifier format: alphanumeric plus _ : -, up to 128 chars.
	// Applies to entry_key, source_item_key, and lens_mode_id.
	keyFormat = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)
)

// ErrInvalidArgument is the canonical validation failure. Handlers MUST wrap it without
// echoing the rejected value into the response body.
var ErrInvalidArgument = errors.New("invalid argument")

// ValidateKeyFormat returns ErrInvalidArgument if key does not match the canonical format.
// The rejected value is NOT included in the returned error message (security: F-005 no-echo).
func ValidateKeyFormat(field, key string) error {
	if !keyFormat.MatchString(key) {
		return fmt.Errorf("%w: %s format", ErrInvalidArgument, field)
	}
	return nil
}

// ValidateWhyText bounds why text at 1..512 chars (matches DB CHECK).
func ValidateWhyText(text string) error {
	n := len(text)
	if n < 1 || n > 512 {
		return fmt.Errorf("%w: why_text length", ErrInvalidArgument)
	}
	return nil
}

// ValidateEvidenceRefs caps the array at 8 entries (matches DB CHECK) and validates each ref_id.
func ValidateEvidenceRefs(refIDs []string, refLabels []string) error {
	if len(refIDs) > 8 {
		return fmt.Errorf("%w: evidence_refs length", ErrInvalidArgument)
	}
	for i, ref := range refIDs {
		if ref == "" || len(ref) > 128 {
			return fmt.Errorf("%w: evidence_refs[%d].ref_id", ErrInvalidArgument, i)
		}
	}
	return nil
}

// ValidateArtifactVersionRef requires at least one of (summary, tag_set, lens) version id.
// proto3 cannot express this; the server is the last line of defense.
func ValidateArtifactVersionRef(summary, tagSet, lens *string) error {
	if summary == nil && tagSet == nil && lens == nil {
		return fmt.Errorf("%w: artifact_version_ref requires at least one versioned artifact", ErrInvalidArgument)
	}
	return nil
}

// ValidateClientTransitionID checks that the provided key is a UUIDv7 and that its embedded
// timestamp is within a sane window: not older than 48h and not more than 5min in the future.
// Rejecting out-of-window keys makes stale-replay and clock-skew attacks harder.
func ValidateClientTransitionID(raw string, now time.Time) error {
	id, err := uuid.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: client_transition_id not a uuid", ErrInvalidArgument)
	}
	if id.Version() != 7 {
		return fmt.Errorf("%w: client_transition_id must be UUIDv7", ErrInvalidArgument)
	}
	// UUIDv7 timestamp is the first 48 bits in milliseconds since epoch.
	raw16, err := id.MarshalBinary()
	if err != nil {
		return fmt.Errorf("%w: client_transition_id binary", ErrInvalidArgument)
	}
	var unixMillis int64
	for i := 0; i < 6; i++ {
		unixMillis = (unixMillis << 8) | int64(raw16[i])
	}
	embedded := time.UnixMilli(unixMillis)
	if embedded.After(now.Add(5 * time.Minute)) {
		return fmt.Errorf("%w: client_transition_id timestamp is too far in the future", ErrInvalidArgument)
	}
	if embedded.Before(now.Add(-48 * time.Hour)) {
		return fmt.Errorf("%w: client_transition_id timestamp is older than 48h", ErrInvalidArgument)
	}
	return nil
}

// ValidateObservedProjectionRevision enforces positive revision numbers.
func ValidateObservedProjectionRevision(rev int64) error {
	if rev <= 0 {
		return fmt.Errorf("%w: observed_projection_revision must be > 0", ErrInvalidArgument)
	}
	return nil
}

// ValidateDwellTriggerTarget rejects every TRANSITION_TRIGGER_DWELL transition.
//
// Auto-OODA suppression (plan: Knowledge Loop 体験回復 — Pillar 1):
// passive viewing must NOT advance OODA stage. The frontend no longer fires
// dwell at all; this validator stays as a defensive guard so a future
// consumer cannot silently re-introduce passive stage advancement. Boyd's
// OODA model treats Orientation as a conscious step (see web research output)
// and Linear-style command-center UIs separate read-state from workflow
// status — dwell-as-advance contradicts both.
//
// toStage / trigger are passed as string to keep this package free of proto imports.
func ValidateDwellTriggerTarget(trigger, _ string) error {
	if trigger == "TRANSITION_TRIGGER_DWELL" {
		return fmt.Errorf("%w: dwell trigger is no longer accepted (Auto-OODA suppression)", ErrInvalidArgument)
	}
	return nil
}
