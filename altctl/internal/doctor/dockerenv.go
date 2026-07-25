package doctor

import "os"

// EnsureDockerGroupIDEnv makes sure DOCKER_GROUP_ID is present in the
// process environment for the duration of a read-only `docker compose`
// introspection call, injecting a harmless placeholder when it's unset, and
// returns a restore func that undoes the injection afterward (safe to call
// even when nothing was injected -- it's a no-op then).
//
// compose/logging.yaml requires DOCKER_GROUP_ID to even parse
// (`${DOCKER_GROUP_ID:?...}`), which otherwise hard-fails any command that
// touches the aggregate compose file (compose/compose.yaml's `include:`, or
// any per-stack file that transitively includes it) at config-parse time --
// before the docker daemon is even reached -- for a user who isn't touching
// the logging stack at all. cmd/doctor.go's newDoctorExecutor originated
// this workaround for its own read-only probe; cmd/status.go hits the exact
// same landmine (H1: `altctl status` aggregates every stack's compose file,
// including logging.yaml, before ever calling the daemon) and shares this
// helper instead of re-deriving the mechanism.
//
// This must ONLY be used to wrap read-only compose introspection (ps /
// config / logs). Commands that actually start containers must never call
// this -- a genuinely unset DOCKER_GROUP_ID there must fail loudly
// (altctl/CLAUDE.md Critical Rule 9 -- fail-fast startup config), not be
// silently patched over.
func EnsureDockerGroupIDEnv() (restore func()) {
	if os.Getenv("DOCKER_GROUP_ID") != "" {
		return func() {}
	}
	os.Setenv("DOCKER_GROUP_ID", "0")
	return func() { os.Unsetenv("DOCKER_GROUP_ID") }
}
