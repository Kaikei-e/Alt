package sovereign_db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// TestWithUserContext_RejectsNilUserID pins Wave 4-D Phase 1's defensive
// stance: a Nil user id must NEVER be bound to alt.user_id. Once Phase 2
// ships the RLS policy, binding the zero uuid would be a stealth bypass
// (the SQL `current_setting('alt.user_id', true)::uuid` returns nil and
// the policy could match rows with user_id IS NULL in some schemas).
// Failing fast at the helper boundary keeps the contract tight.
func TestWithUserContext_RejectsNilUserID(t *testing.T) {
	r := &Repository{pool: &mockPgx{}}

	called := false
	err := r.withUserContext(context.Background(), uuid.Nil, func(_ pgx.Tx) error {
		called = true
		return nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil user_id")
	require.False(t, called, "callback must NOT run when user_id is nil")
}
