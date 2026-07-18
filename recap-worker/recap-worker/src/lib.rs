// Clippy lint policy lives in Cargo.toml [lints.clippy] (DECREE §13).

pub mod analysis;
pub(crate) mod api;
pub mod app;
pub mod classification;
pub mod classifier;
pub(crate) mod clients;
pub mod config;
pub(crate) mod error;
pub mod evaluation;
// Not part of any bench/replay-bin/integration-test surface (unlike its
// sibling `pub` modules below) — only consumed internally via
// `classification::tokenizer`, so it doesn't need to be reachable
// crate-externally.
pub(crate) mod language_detection;
pub mod observability;
pub mod pipeline;
pub(crate) mod queue;
pub mod replay;
pub mod scheduler;
pub(crate) mod schema;
pub(crate) mod store;
pub mod tls;
pub mod util;

/// Populate the rust-bert `AllMiniLmL12V2` sentence-embedding model cache.
///
/// `SentenceEmbeddingsBuilder::remote(...)` writes downloaded weights and
/// tokenizer files under `$RUSTBERT_CACHE` (default `$HOME/.cache/.rustbert`).
/// Once the cache is primed, subsequent calls skip the HTTP fetch, which lets
/// the service boot in a network-isolated compose stack (staging `internal:
/// true`). Invoke this from a container running on an internet-connected
/// network, writing into a host-mounted cache volume that the runtime stack
/// consumes read-only.
pub async fn warmup_embedding_cache() -> anyhow::Result<()> {
    tokio::task::spawn_blocking(pipeline::embedding::EmbeddingService::new)
        .await
        .map_err(|e| anyhow::anyhow!("warmup task join failed: {e:?}"))?
        .map(|_| ())
}
