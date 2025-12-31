#![deny(warnings, clippy::all, clippy::pedantic)]
#![allow(
    // Acceptable for trait naming consistency (e.g., RecapDao, RecapDaoImpl)
    clippy::module_name_repetitions,

    // Required for ML/statistics: f64 → f32 conversions for BERT embeddings and centroid calculations
    clippy::cast_precision_loss,

    // Necessary for embedding dimension conversions: usize ↔ i32/u32 for array indexing
    clippy::cast_possible_truncation,

    // Required for signed/unsigned conversions in database queries and pagination
    clippy::cast_possible_wrap,

    // Domain logic often requires helper declarations mid-function for readability
    clippy::items_after_statements,

    // Error context via anyhow::Context already provides sufficient documentation
    clippy::missing_errors_doc,

    // Panic paths are defensive (e.g., mutex poisoning), not part of normal flow
    clippy::missing_panics_doc,

    // Technical identifiers (e.g., XXH3, BERT, TF-IDF) don't need markdown formatting
    clippy::doc_markdown,

    // Explicit closures improve clarity for complex async chains
    clippy::redundant_closure,

    // Named format args reduce readability for long messages with many placeholders
    clippy::uninlined_format_args,

    // if-let-else patterns are clearer than map_or for error handling flows
    clippy::option_if_let_else,

    // or_else() allocation overhead negligible; or() preferred for readability
    clippy::or_fun_call,

    // Pass-by-value necessary for async trait methods (Arc, Config types)
    clippy::needless_pass_by_value,

    // Too noisy: many utility methods return useful values but aren't always used
    clippy::must_use_candidate,

    // Nested conditions improve readability when branches are semantically distinct
    clippy::collapsible_if,

    // for x in iter.iter() is clearer than for x in &iter for consistency
    clippy::explicit_iter_loop
)]

pub mod analysis;
pub(crate) mod api;
pub mod app;
pub mod classification;
pub mod classifier;
pub(crate) mod clients;
pub mod config;
pub mod evaluation;
pub mod language_detection;
pub mod observability;
pub mod pipeline;
pub(crate) mod queue;
pub mod replay;
pub mod scheduler;
pub(crate) mod schema;
pub(crate) mod store;
pub mod util;
