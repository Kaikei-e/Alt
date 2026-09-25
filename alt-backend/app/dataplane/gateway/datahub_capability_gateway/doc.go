// Package datahub_capability_gateway adapts the alt_db drivers to the
// capability ports alt-data-hub serves (ADR-000954 Wave 3, catalog §2.A /
// §2.D / §2.E / §2.L / §2.O).
//
// It is deliberately thin. The drivers already hold the transaction
// boundaries and the ON CONFLICT clauses these capabilities are drawn around
// — FOR UPDATE SKIP LOCKED in the outbox claim, the upserts in the scraping
// and cache writes — and moving that SQL is a separate step: Wave 3's exit
// condition is that alt_db has no callers outside cmd/datahub, not that the
// files have been relocated. Relocating them in the same change would mix a
// behaviour-preserving file move into a commit that also moves a process
// boundary, and the two failure modes would be indistinguishable in a bisect.
//
// What this package does add is the shape the port asks for: domain types
// instead of driver structs, a status enum instead of a bare string, and a
// returned row instead of an argument mutated in place.
package datahub_capability_gateway
