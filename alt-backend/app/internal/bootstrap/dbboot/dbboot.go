// Package dbboot opens the alt-db connection pool for the one binary that
// owns it.
//
// It is a package rather than an option on internal/bootstrap, and that is the
// whole design. As a flag — `bootstrap.Options{RequireDB: true}` — the driver
// stayed in every binary's import graph whether the flag was set or not, so
// "cmd/backend has no database" was a claim about a boolean that any future
// caller could flip, and about a package that was still linked in and still
// read DB_HOST when something called it. As a separate package the property is
// enforced by the linker: cmd/backend and cmd/harvester do not import this, so
// alt/shared/driver/alt_db is not in their binaries at all, and no environment
// variable they are given can be read by code that is not there.
//
// di/import_boundary_test.go is what keeps that true across future edits.
package dbboot

import (
	"context"
	"fmt"
	"log/slog"

	"alt/internal/bootstrap"
	"alt/shared/driver/alt_db"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MustOpen opens the pool or fails the process.
//
// Fail-fast rather than warn-and-limp (CLAUDE.md rule 9): alt-data-hub with no
// database serves every one of its procedures an error, and a process in that
// state passing its own health check would be reported healthy while the whole
// deployment above it degraded.
//
// The close is registered on the runtime rather than returned as a second
// value, so it runs in the same reverse-acquisition order as the OTel shutdown
// that precedes it.
func MustOpen(ctx context.Context, rt *bootstrap.Runtime, serviceName string) *pgxpool.Pool {
	log := rt.Log
	if log == nil {
		log = slog.Default()
	}

	pool, err := alt_db.InitDBConnectionPool(ctx)
	if err != nil {
		log.ErrorContext(ctx, "failed to connect to database", "error", err, "service", serviceName)
		panic(fmt.Errorf("init db connection pool for %s: %w", serviceName, err))
	}

	rt.AddShutdownHook(func(context.Context) error {
		pool.Close()
		return nil
	})
	return pool
}
