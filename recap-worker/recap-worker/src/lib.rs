#![deny(warnings, clippy::all, clippy::pedantic)]
#![allow(
    clippy::module_name_repetitions,
    clippy::cast_precision_loss,
    clippy::cast_possible_truncation,
    clippy::cast_possible_wrap,
    clippy::items_after_statements,
    clippy::missing_errors_doc,
    clippy::missing_panics_doc,
    clippy::doc_markdown,
    clippy::redundant_closure,
    clippy::uninlined_format_args,
    clippy::option_if_let_else,
    clippy::or_fun_call,
    clippy::needless_pass_by_value,
    clippy::must_use_candidate,
    clippy::collapsible_if,
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
