package di_test

import (
	"os/exec"
	"strings"
	"testing"
)

// ADR-000954 Wave 3's exit condition, expressed the only way that cannot be
// worked around: the *linker* decides.
//
// Every softer form of this check has a hole. A grep for `alt_db` misses an
// import added under an alias. A go/parser walk over one directory misses the
// package three hops down that pulls the driver in. A DI-level assertion that
// "the pool field is absent" misses a job constructing its own
// pgxpool.New. What has no hole is the transitive dependency graph of the
// binary: if alt/shared/driver/alt_db is not in it, no code path in
// cmd/backend can reach the database, whatever anybody writes next.
//
// The pgx entry matters as much as the alt_db one, and for a reason worth
// stating. alt_db is this repository's driver package and the obvious thing to
// forbid; pgx is the library underneath it, and a handler that wanted a
// database could reach one by calling pgxpool.New directly without ever
// touching alt_db. Forbidding both is what makes the property "this process
// cannot open a Postgres connection" rather than "this process does not use
// our repository type".
//
// cmd/datahub is the deliberate exception and is asserted positively below.
// A test that only forbade things would stay green if somebody removed the
// data plane's pool too, at which point every one of the 91 migrated
// capabilities would answer with an error and nothing here would notice.
func TestBinaryDependencyGraphs_DatabaseIsDataHubOnly(t *testing.T) {
	const (
		altDBPkg = "alt/shared/driver/alt_db"
		pgxPkg   = "github.com/jackc/pgx/v5"
		poolPkg  = "github.com/jackc/pgx/v5/pgxpool"
	)

	tests := []struct {
		name      string
		pkg       string
		forbidden []string
		required  []string
	}{
		{
			// The user-facing API. Every capability it needs is a procedure of
			// alt.datahub.v1.DataHubService since batch 6 (catalog §2.B …
			// §2.O).
			name:      "cmd/backend",
			pkg:       "alt/cmd/backend",
			forbidden: []string{altDBPkg, pgxPkg, poolPkg},
		},
		{
			// The scheduled jobs. Reached this state in batch 5 at the
			// container level; this pins it at the link level, which is where
			// a job that grew its own pool would have shown up.
			name:      "cmd/harvester",
			pkg:       "alt/cmd/harvester",
			forbidden: []string{altDBPkg, pgxPkg, poolPkg},
		},
		{
			// The data plane, and the whole point of the split: alt_db has
			// exactly one owner (ADR-000954 D3).
			name:     "cmd/datahub",
			pkg:      "alt/cmd/datahub",
			required: []string{altDBPkg, poolPkg},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := packageDeps(t, tt.pkg)

			for _, forbidden := range tt.forbidden {
				if deps[forbidden] {
					t.Errorf("%s must not depend on %s — ADR-000954 Wave 3 gives alt-data-hub "+
						"sole ownership of alt_db, and a dependency here means some package "+
						"in this binary can still open a connection. Route the capability "+
						"through alt.datahub.v1.DataHubService instead.", tt.pkg, forbidden)
				}
			}
			for _, required := range tt.required {
				if !deps[required] {
					t.Errorf("%s must depend on %s — it is the only binary that owns the "+
						"database, and without it every migrated capability answers an error",
						tt.pkg, required)
				}
			}
		})
	}
}

// packageDeps returns the transitive import set of one package, the linker's
// own answer rather than a re-derivation of it.
func packageDeps(t *testing.T, pkg string) map[string]bool {
	t.Helper()

	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, stderr)
	}

	deps := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			deps[line] = true
		}
	}
	return deps
}
