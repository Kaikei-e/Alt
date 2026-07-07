package handler

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseUUIDField_RejectsMalformedInput pins the fix for the silent-
// fallback bug: parseUUID used to swallow the parse error and return
// uuid.Nil, letting malformed event_id/tenant_id/user_id get written to
// knowledge_events or used to query with a Nil UUID (Rule 8: no silent
// fallback). parseUUIDField must surface the error instead.
func TestParseUUIDField_RejectsMalformedInput(t *testing.T) {
	_, err := parseUUIDField("user_id", "not-a-uuid")
	require.Error(t, err, "malformed UUID must error, not silently return uuid.Nil")
	assert.Contains(t, err.Error(), "user_id")
}

func TestParseUUIDField_ParsesValidUUID(t *testing.T) {
	id := uuid.New()
	got, err := parseUUIDField("user_id", id.String())
	require.NoError(t, err)
	assert.Equal(t, id, got)
}

func TestParseUUIDPtrField_EmptyStringReturnsNil(t *testing.T) {
	got, err := parseUUIDPtrField("correlation_id", "")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParseUUIDPtrField_RejectsMalformedInput(t *testing.T) {
	_, err := parseUUIDPtrField("correlation_id", "not-a-uuid")
	require.Error(t, err, "malformed non-empty UUID must error, not silently return nil")
	assert.Contains(t, err.Error(), "correlation_id")
}

func TestParseUUIDPtrField_ParsesValidUUID(t *testing.T) {
	id := uuid.New()
	got, err := parseUUIDPtrField("correlation_id", id.String())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, id, *got)
}
