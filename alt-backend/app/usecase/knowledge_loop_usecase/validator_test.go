package knowledge_loop_usecase

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateKeyFormat(t *testing.T) {
	require.NoError(t, ValidateKeyFormat("entry_key", "article:42"))
	require.NoError(t, ValidateKeyFormat("entry_key", "article_42-v2"))
	require.ErrorIs(t, ValidateKeyFormat("entry_key", ""), ErrInvalidArgument)
	require.ErrorIs(t, ValidateKeyFormat("entry_key", "has space"), ErrInvalidArgument)
	require.ErrorIs(t, ValidateKeyFormat("entry_key", "has/slash"), ErrInvalidArgument)

	err := ValidateKeyFormat("entry_key", "x'; DROP TABLE knowledge_loop_entries; --")
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.NotContains(t, err.Error(), "DROP TABLE", "rejected value MUST NOT be echoed into the error")
}

func TestValidateWhyText_Bounds(t *testing.T) {
	require.NoError(t, ValidateWhyText("a"))
	require.NoError(t, ValidateWhyText(strings.Repeat("x", 512)))
	require.ErrorIs(t, ValidateWhyText(""), ErrInvalidArgument)
	require.ErrorIs(t, ValidateWhyText(strings.Repeat("x", 513)), ErrInvalidArgument)
}

func TestValidateEvidenceRefs_CappedAt8(t *testing.T) {
	makeRefs := func(n int) []string {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = "ref-x"
		}
		return ids
	}
	require.NoError(t, ValidateEvidenceRefs(nil, nil))
	require.NoError(t, ValidateEvidenceRefs(makeRefs(8), nil))
	require.ErrorIs(t, ValidateEvidenceRefs(makeRefs(9), nil), ErrInvalidArgument)
}

func TestValidateArtifactVersionRef_AtLeastOneRequired(t *testing.T) {
	lens := "lens-v1"
	require.NoError(t, ValidateArtifactVersionRef(nil, nil, &lens))
	require.ErrorIs(t, ValidateArtifactVersionRef(nil, nil, nil), ErrInvalidArgument)
}

func TestValidateClientTransitionID_RejectsNonV7(t *testing.T) {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	// UUIDv4 (random) must be rejected.
	v4 := uuid.New().String()
	require.ErrorIs(t, ValidateClientTransitionID(v4, now), ErrInvalidArgument)

	// Malformed string rejected.
	require.ErrorIs(t, ValidateClientTransitionID("not-a-uuid", now), ErrInvalidArgument)
}

func TestValidateClientTransitionID_AcceptsFreshV7(t *testing.T) {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	v7, err := uuid.NewV7()
	require.NoError(t, err)
	// The just-generated UUIDv7 carries a current-ish timestamp; set `now` near it to pass.
	ok := ValidateClientTransitionID(v7.String(), time.Now())
	require.NoError(t, ok)
	_ = now
}

func TestValidateClientTransitionID_RejectsStale(t *testing.T) {
	// Forge a UUIDv7 with a very old embedded timestamp.
	var raw [16]byte
	stale := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC) // 5 days ago from 2026-04-23
	ms := stale.UnixMilli()
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	raw[6] = 0x70 // version 7 in the high nibble of byte 6
	raw[8] = 0x80 // variant bits
	id, err := uuid.FromBytes(raw[:])
	require.NoError(t, err)

	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	require.ErrorIs(t, ValidateClientTransitionID(id.String(), now), ErrInvalidArgument)
}

func TestValidateDwellTriggerTarget(t *testing.T) {
	// Auto-OODA suppression: TRANSITION_TRIGGER_DWELL is rejected for every
	// to_stage. The frontend no longer sends dwell at all (passive viewing
	// must not advance OODA stage — see plan: image-zed-agent-pasted-image-
	// name-image-pure-stearns.md, Pillar 1). The validator stays as a
	// defensive guard so a future consumer cannot silently re-introduce
	// passive stage advancement.
	for _, target := range []string{
		"LOOP_STAGE_OBSERVE",
		"LOOP_STAGE_ORIENT",
		"LOOP_STAGE_DECIDE",
		"LOOP_STAGE_ACT",
	} {
		err := ValidateDwellTriggerTarget("TRANSITION_TRIGGER_DWELL", target)
		require.Error(t, err, "dwell→%s must be rejected", target)
		require.True(t, errors.Is(err, ErrInvalidArgument))
	}

	// Non-dwell triggers pass through untouched.
	require.NoError(t, ValidateDwellTriggerTarget("TRANSITION_TRIGGER_USER_TAP", "LOOP_STAGE_ACT"))
	require.NoError(t, ValidateDwellTriggerTarget("TRANSITION_TRIGGER_USER_TAP", "LOOP_STAGE_ORIENT"))
	require.NoError(t, ValidateDwellTriggerTarget("TRANSITION_TRIGGER_KEYBOARD", "LOOP_STAGE_OBSERVE"))
	require.NoError(t, ValidateDwellTriggerTarget("TRANSITION_TRIGGER_DEFER", "LOOP_STAGE_OBSERVE"))
	require.NoError(t, ValidateDwellTriggerTarget("TRANSITION_TRIGGER_UNSPECIFIED", "LOOP_STAGE_OBSERVE"))
}

func TestValidateObservedProjectionRevision(t *testing.T) {
	require.NoError(t, ValidateObservedProjectionRevision(1))
	require.ErrorIs(t, ValidateObservedProjectionRevision(0), ErrInvalidArgument)
	require.ErrorIs(t, ValidateObservedProjectionRevision(-1), ErrInvalidArgument)
}
