#![deny(warnings)]
#![deny(rust_2018_idioms)]
#![deny(rust_2024_compatibility)]
#![warn(clippy::pedantic)]

pub mod adapter;
pub mod app;
pub mod auth;
pub mod config;
pub mod domain;
pub mod error;
pub mod handler;
pub mod healthcheck;
pub mod log_exporter;
pub mod otlp;
pub mod port;

#[cfg(test)]
pub mod test_support;

pub use healthcheck::{healthcheck, healthcheck_with_port};
