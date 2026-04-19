package main

// This file previously housed a consecutive-failure counter that drove a
// tick-loop `os.Exit(1)` on netns-orphan detection. That design was removed
// after live testing (ADR-000802 follow-up): Docker's `restart:
// unless-stopped` policy keeps the stale `network_mode: service:<parent>`
// reference after a self-exit, so on restart the sidecar tries to rejoin
// the *old* parent's netns — which has already been garbage-collected —
// and the restart fails with `No such container: <old parent id>`. The
// container ends up exited and unrecoverable, making the symptom worse
// rather than better.
//
// The detection half of the feature lives on in `probeNetns` (see
// healthcheck.go) and is invoked by the Docker HEALTHCHECK subcommand, so
// an orphan is now visible as `State.Health.Status = unhealthy` within the
// 15 s healthcheck interval. Self-heal requires an external force-recreate
// (alt-deploy cascade per ADR-000783, or an operator running
// `docker compose up --no-deps --force-recreate pki-agent-<svc>` manually).
