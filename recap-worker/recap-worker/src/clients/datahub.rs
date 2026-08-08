//! alt-data-hub client for recap-worker.
//!
//! Today this covers one procedure — `EnqueueNotification`, the sink of the
//! notification-outbox relay. The wire convention is the same one every other
//! recap-worker client uses (ADR-000764): Connect-RPC unary over HTTP/1.1 with
//! a protojson body, spoken directly with `reqwest` rather than through
//! generated `prost`/`tonic` stubs, so there is nothing to regenerate when the
//! proto moves and one place — `clients/datahub/contract.rs` — where the wire
//! shape is asserted.

mod client;

#[cfg(test)]
mod contract;

pub(crate) use client::{DataHubClient, NotificationEnqueue, recap_ready_payload};
