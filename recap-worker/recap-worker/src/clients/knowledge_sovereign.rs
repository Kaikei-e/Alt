//! Knowledge Sovereign client for recap-worker.
//!
//! recap-worker is a consumer of knowledge-sovereign's `AppendKnowledgeEvent`
//! Connect-RPC endpoint. Wave 4-B (Knowledge Loop, ADR-000853 / canonical
//! contract §6.4.1) introduced the `recap.topic_snapshotted.v1` event so
//! Surface Planner v2's resolver can credit Loop entries with the
//! `topic_overlap_count` signal — RecapTopicSnapshot-bound, replay-safe.
//!
//! ## Wire format
//!
//! Although the provider is implemented in Go with connect-go, the Connect-RPC
//! over HTTP/1.1 + JSON convention (ADR-000764) means the consumer can speak
//! it directly with `reqwest`: a normal POST whose body is the JSON-encoded
//! request message. We avoid `prost` / `tonic` here for parity with the
//! existing recap-worker clients (news_creator, subworker, tag_generator),
//! all of which use reqwest + JSON without proto bindings.
//!
//! ## Idempotency
//!
//! `dedupe_key` folds in the user id + snapshot id so retries against the
//! sovereign unique index resolve to a no-op rather than a duplicate row.

mod client;

#[cfg(test)]
mod contract;

pub(crate) use client::{KnowledgeSovereignClient, TopicSnapshottedInput};
