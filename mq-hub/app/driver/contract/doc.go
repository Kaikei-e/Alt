//go:build contract

// Package contract contains Consumer-Driven Contract tests for mq-hub.
//
// These tests are isolated from normal unit tests via the "contract" build tag.
// Run them with:
//
//	go test -tags=contract ./driver/contract/ -v
//
// Generated pact files are written to ../../../../pacts/ and later verified by
// consumer-side message contract tests in search-indexer and tag-generator.
package contract
