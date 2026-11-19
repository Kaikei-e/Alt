//! Themes pipeline module.

pub mod adapters;
#[cfg(test)]
pub mod fakes;
pub mod params;
pub mod pipeline;
pub mod ports;

pub use adapters::{
    DataHubThemeArticleSource, NewsCreatorThemeGenerator, SovereignThemeKnowledgePort,
    SubworkerCardVerifier, SubworkerEmbedCluster,
};
#[cfg(test)]
pub use fakes::{
    FakeCardVerifier, FakeEmbedCluster, FakeThemeArticleSource, FakeThemeGenerator,
    FakeThemeKnowledgePort, FakeThemesJobRunner,
};
pub use params::ThemesParams;
pub use pipeline::{ThemesPipeline, ThemesPipelineDao, ThemesPipelineResult, ThemesRunner};
pub use ports::{
    CardVerifier, EmbedCluster, ThemeArticleSource, ThemeGenerator, ThemeKnowledgePort,
    ThemesJobRunner,
};
