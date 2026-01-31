// モジュールの公開と型の再エクスポート
pub mod article;
pub mod config;
pub mod evaluation;
pub mod genre_learning;
pub mod job;
pub mod metrics;
pub mod morning;
pub mod output;
pub mod pulse;
pub mod stage;
pub mod subworker;
pub mod types;

// New modular architecture
pub mod traits;
pub mod impls;
mod compat;

#[cfg(test)]
pub mod mock;
#[cfg(test)]
mod tests;

pub mod job_status;

// 型の再エクスポート - 新アーキテクチャを優先
pub use impls::UnifiedDao;
pub use types::{GenreStatus, JobStatus, PipelineStage, TriggerSource};

// Backward compatibility re-exports
// RecapDao now comes from the compat module (blanket impl over focused traits)
pub use compat::RecapDao;

// RecapDaoImpl is now an alias to UnifiedDao for backward compatibility
pub type RecapDaoImpl = UnifiedDao;

// Re-export focused traits for new code
// Note: These traits are for new code patterns. Existing code can continue using RecapDao.
#[allow(unused_imports)]
pub use traits::{
    ArticleDao, ConfigDao, EvaluationDao, GenreLearningDao, JobDao, JobStatusDao as JobStatusDaoTrait,
    MetricsDao, MorningDao, OutputDao, PulseDao, StageDao, SubworkerDao,
};

