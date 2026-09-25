package datahubapi

import (
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// requiredUUID rejects both an absent and an unparseable identifier at the
// delivery layer, so the caller gets InvalidArgument rather than an Internal
// wrapping whatever the query did with a zero UUID.
func requiredUUID(raw, field string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s is required", field))
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a uuid: %w", field, err))
	}
	return id, nil
}

// optionalUUID maps an empty field to nil — "no scope", a different query —
// while still refusing a value that was sent and does not parse. Silently
// treating a malformed user id as "unscoped" would widen a tenant-scoped read
// into a global one.
func optionalUUID(raw, field string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a uuid: %w", field, err))
	}
	return &id, nil
}

// keysetCursor enforces the all-or-nothing rule both backfill walks share.
//
// Accepting half a cursor would restart the walk from the beginning, and the
// jobs that drive these would re-emit every knowledge event they had already
// emitted — a silent duplicate replay rather than a visible failure.
func keysetCursor(ts *timestamppb.Timestamp, rawID, tsField, idField string) (*time.Time, *uuid.UUID, error) {
	hasTS := ts != nil && ts.IsValid()
	hasID := rawID != ""

	switch {
	case !hasTS && !hasID:
		return nil, nil, nil
	case hasTS != hasID:
		return nil, nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%s and %s must be sent together: half a keyset cursor would restart the walk from the beginning", tsField, idField))
	}

	id, err := uuid.Parse(rawID)
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a uuid: %w", idField, err))
	}
	t := ts.AsTime()
	return &t, &id, nil
}
