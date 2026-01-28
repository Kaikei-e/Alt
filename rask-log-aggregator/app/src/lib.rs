#![deny(warnings)]
#![deny(rust_2018_idioms)]
#![deny(rust_2024_compatibility)]
#![warn(clippy::pedantic)]
#![allow(clippy::module_name_repetitions)]
#![allow(clippy::missing_errors_doc)]
#![allow(clippy::doc_markdown)]

pub mod config;
pub mod domain;
pub mod error;
pub mod healthcheck;
pub mod log_exporter;

pub use healthcheck::{healthcheck, healthcheck_with_port};
