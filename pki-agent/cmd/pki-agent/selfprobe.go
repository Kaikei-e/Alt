package main

// This file previously housed a consecutive-failure counter that drove a
// tick-loop `os.Exit(1)` on netns-orphan detection. That design was removed
// after live testing (ADR-000802 follow-up): Docker's `restart:
// unless-stopped` policy keeps the stale `network_mode: service:<parent>`
// reference after a self-exit, so on restart the sidecar tries to rejoin
// the *old* parent's netns — which has already been garbage-collected —
// and the restart fails with `No such container: <old parent id>`.
//
// Wave 4 Pattern B cutover retired the shared-netns topology entirely.
// Inbound TLS lives in the parent process; pki-agent is cert-only on its
// own netns. There is no netns-orphan class left for a healthcheck to
// detect, and scripts/cascade-pki-sidecars.sh is a hard-failing tombstone.
