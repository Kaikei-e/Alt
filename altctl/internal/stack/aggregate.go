package stack

import (
	"fmt"
	"os"
	"path/filepath"
)

// AggregateComposeFile is compose/compose.yaml -- the top-level `include:`
// aggregate that combines every stack meant to run together in the shared
// docker compose project. Lifecycle commands (up/restart/rebuild/logs/exec)
// use this single file as their `-f` argument and scope operations by
// service name instead of assembling per-stack `-f` subsets: several
// per-stack files transitively `include: pki.yaml`, whose pki-agent
// sidecars `depends_on` services scattered across many other stacks, so a
// narrow `-f` subset fails compose project validation even for an otherwise
// unrelated stack (empirically confirmed: `docker compose -f base.yaml -f
// db.yaml -f pgbouncer.yaml -f auth.yaml -f sovereign.yaml -f core.yaml
// config` rejects the project -- see Stack.AggregateCovered and
// internal/doctor/probe.go's aggregateComposeFile, which hit the same wall
// first). compose auto-starts each named service's transitive depends_on,
// so scoping by service name is sufficient without the file list needing to
// be "complete" for anything beyond the aggregate itself.
const AggregateComposeFile = "compose.yaml"

// aggregateCoveredFiles returns the set of compose filenames that
// compose/compose.yaml pulls in transitively (plus compose.yaml and
// base.yaml themselves, since base.yaml has no services of its own and is
// `include:`d by nearly every other file, but isn't named as a top-level
// `include:` entry of compose.yaml itself). A missing compose.yaml (e.g. a
// synthetic test fixture compose dir with no aggregate file at all) is not
// an error: it yields an empty set, so every stack is treated as outside
// the aggregate and lifecycle commands fall back to their own per-stack
// file lists -- the safe, pre-aggregate-strategy behavior.
func aggregateCoveredFiles(composeDir string) (map[string]bool, error) {
	path := filepath.Join(composeDir, AggregateComposeFile)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("checking aggregate compose file %s: %w", path, err)
	}

	includes, err := composeFileIncludes(path)
	if err != nil {
		return nil, err
	}
	covered := map[string]bool{AggregateComposeFile: true, "base.yaml": true}
	for _, f := range includes {
		covered[f] = true
	}
	return covered, nil
}
