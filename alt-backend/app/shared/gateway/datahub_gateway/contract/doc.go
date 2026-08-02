// Package contract holds the consumer-driven contract tests for the two
// in-family callers of services.datahub.v1.DataHubService.
//
// alt-backend and alt-harvester are built from the same Go module as
// alt-data-hub, which is exactly why these pacts exist. A shared module makes
// it possible to change a message and a handler in one commit and have
// everything compile, so "it builds" says nothing about whether the deployed
// provider still answers what the deployed consumer sends — the three binaries
// ship as three containers and can be rolled independently. The compiler is
// not the contract; this is (CLAUDE.md rule 7).
//
// Two consumers, two pact files, because they exercise disjoint halves of the
// surface: alt-harvester drives the outbox and the retention/backfill jobs,
// alt-backend drives the article-serving reads. Publishing them under one name
// would let a harvester-only capability break while the backend's verification
// stayed green.
package contract
