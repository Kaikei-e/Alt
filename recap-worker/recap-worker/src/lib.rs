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
pub(crate) mod notification;
pub mod observability;
pub mod pipeline;
pub(crate) mod pki;
pub(crate) mod queue;
pub mod replay;
pub mod scheduler;
pub(crate) mod schema;
pub mod startup;
pub(crate) mod store;
pub mod tls;
pub mod util;

/// Verify the baked `AllMiniLmL12V2` sentence-embedding model loads.
///
/// The model directory ships inside the image and is read with
/// `SentenceEmbeddingsBuilder::local`, so nothing is downloaded and no network
/// is touched. This subcommand exists to fail a bad image at build or deploy
/// time rather than at the first recap job: it names any missing file and
/// exercises the real `tch` load path.
pub async fn warmup_embedding_cache() -> anyhow::Result<()> {
    tokio::task::spawn_blocking(pipeline::embedding::EmbeddingService::new)
        .await
        .map_err(|e| anyhow::anyhow!("warmup task join failed: {e:?}"))?
        .map(|_| ())
}
