//! Topic Cards pipeline module.
//!
//! Standalone pipeline producing `recap_card_candidates` from raw feed items:
//! snapshot → normalize → noise → dedup → embed → cluster → rank → persist.

pub mod adapters;
pub mod dedup;
#[cfg(test)]
pub mod fakes;
pub mod noise;
pub mod normalize;
pub mod params;
pub mod pipeline;
pub mod ports;
pub mod rank;

pub use adapters::{
    AltBackendFeedSource, NewsCreatorCardGenerator, SubworkerCardVerifier, SubworkerEmbedCluster,
    SubworkerGenreTagger,
};
#[cfg(test)]
pub use fakes::{
    FakeCardGenerator, FakeCardVerifier, FakeCardsMlPort, FakeEmbedCluster, FakeFeedSource,
    FakeGenreTagger,
};
pub use params::{CardsParams, DEFAULT_PARAMS_VERSION};
pub use pipeline::{CardsPipeline, CardsPipelineDao, CardsPipelineResult, ReplayResult};
pub use ports::{CardGenerator, CardVerifier, CardsMlPort, EmbedCluster, FeedSource, GenreTagger};
pub use rank::{compute_cluster_fingerprint, rank_candidates};
