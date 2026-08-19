package sovereign_db

import (
	"context"
	"fmt"
)

// PingDB is the cheap round-trip /health/deep uses. The error is wrapped
// without the DSN — callers must still not put err.Error() on the wire.
func (r *Repository) PingDB(ctx context.Context) error {
	var n int
	if err := r.pool.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	return nil
}

// PingProjectors reports whether in-process projectors have a checkpoint row.
// ready=false means they have not ticked yet (warn, not a transport failure).
func (r *Repository) PingProjectors(ctx context.Context, names []string) (bool, error) {
	for _, name := range names {
		cp, err := r.ReadProjectionCheckpointForAdvance(ctx, name)
		if err != nil {
			return false, fmt.Errorf("projector ping: %w", err)
		}
		if !cp.Exists {
			return false, nil
		}
	}
	return true, nil
}
