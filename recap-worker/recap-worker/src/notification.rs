//! Notification outbox relay.
//!
//! The producer half lives in `store::dao::notification_outbox` and writes
//! inside the recap completion's transaction. This module carries those rows
//! to alt-data-hub, which owns the per-device push queue.

pub(crate) mod relay;

pub(crate) use relay::{NotificationRelay, RelayConfig};
