// Package datahub_capability_port declares what alt-data-hub needs from
// alt_db to serve the capabilities ADR-000954 moved off the callers'
// direct database access (catalog §2.A / §2.D / §2.E / §2.L / §2.O).
//
// One package, one interface per capability group, mirroring the single
// anti-corruption package on the consumer side. Each interface is the exact
// set of operations one group of procedures performs — nothing wider, so a
// handler cannot reach a query it has no procedure for.
package datahub_capability_port
