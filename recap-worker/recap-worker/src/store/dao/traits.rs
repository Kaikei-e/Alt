//! DAO trait definitions
//!
//! This module contains focused, single-responsibility traits for data access operations.
//! Each trait corresponds to a specific domain area of the application.

mod article;
mod config;
mod evaluation;
mod genre_learning;
mod job;
mod job_status;
mod metrics;
mod morning;
mod output;
mod pulse;
mod stage;
mod subworker;

pub(crate) use article::ArticleDao;
pub(crate) use config::ConfigDao;
pub(crate) use evaluation::EvaluationDao;
pub(crate) use genre_learning::GenreLearningDao;
pub(crate) use job::JobDao;
pub(crate) use job_status::JobStatusDao;
pub(crate) use metrics::MetricsDao;
pub(crate) use morning::MorningDao;
pub(crate) use output::OutputDao;
pub(crate) use pulse::PulseDao;
pub(crate) use stage::StageDao;
pub(crate) use subworker::SubworkerDao;
