//! 新しい分類パイプライン: Centroid-based Classification + Graph Label Propagation

pub mod graph;
pub mod workflow;

pub use graph::GraphPropagator;
pub use workflow::{ClassificationPipeline, GoldenItem};
