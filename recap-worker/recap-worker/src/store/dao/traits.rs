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

pub use article::ArticleDao;
pub use config::ConfigDao;
pub use evaluation::EvaluationDao;
pub use genre_learning::GenreLearningDao;
pub use job::JobDao;
pub use job_status::JobStatusDao;
pub use metrics::MetricsDao;
pub use morning::MorningDao;
pub use output::OutputDao;
pub use pulse::PulseDao;
pub use stage::StageDao;
pub use subworker::SubworkerDao;
