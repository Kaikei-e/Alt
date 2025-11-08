#![deny(warnings, clippy::all, clippy::pedantic)]
#![allow(clippy::module_name_repetitions)]

pub(crate) mod api;
pub mod app;
pub mod classification;
pub(crate) mod clients;
pub mod config;
pub mod evaluation;
pub mod observability;
pub mod pipeline;
pub(crate) mod scheduler;
pub(crate) mod schema;
pub(crate) mod store;
pub mod util;
