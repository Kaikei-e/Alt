pub(crate) mod cadence;
pub mod daemon;
pub(crate) mod jobs;
pub(crate) mod ledger;

/// Self-check type alias so the `spawn_jst_batch_daemon` signature-drift
/// guard below stays readable under `clippy::type_complexity`.
type JstBatchDaemonSpawnFn = fn(
    Scheduler,
    Vec<String>,
    u32,
    Option<crate::config::KnowledgeOwnerIds>,
    tokio_util::sync::CancellationToken,
) -> tokio::task::JoinHandle<()>;

#[allow(dead_code)]
const _SPAWN_JST_BATCH_DAEMON_GUARD: JstBatchDaemonSpawnFn = daemon::spawn_jst_batch_daemon;
pub(crate) use jobs::{JobContext, Scheduler};
