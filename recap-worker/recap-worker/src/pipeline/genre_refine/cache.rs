//! Tag-label graph cache for genre refinement.

use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use async_trait::async_trait;
use rustc_hash::FxHashMap;
use serde::{Deserialize, Serialize};
use tokio::sync::RwLock;
use tracing::error;

use crate::store::dao::RecapDao;
use crate::store::models::GraphEdgeRecord;

/// タグとジャンルの共起重み。
#[cfg_attr(not(test), allow(dead_code))]
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct LabelEdge {
    pub(crate) genre: String,
    pub(crate) tag: String,
    pub(crate) weight: f32,
}

impl LabelEdge {
    #[must_use]
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn new(genre: impl Into<String>, tag: impl Into<String>, weight: f32) -> Self {
        Self {
            genre: genre.into(),
            tag: tag.into(),
            weight,
        }
    }
}

/// タグラベル共起キャッシュ。
#[derive(Debug, Clone)]
pub(crate) struct TagLabelGraphCache {
    edges: Arc<FxHashMap<String, FxHashMap<String, f32>>>,
}

impl TagLabelGraphCache {
    #[must_use]
    pub(crate) fn empty() -> Self {
        Self {
            edges: Arc::new(FxHashMap::default()),
        }
    }

    #[must_use]
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn from_edges(edges: &[LabelEdge]) -> Self {
        let mut map: FxHashMap<String, FxHashMap<String, f32>> = FxHashMap::default();
        for edge in edges {
            let genre_key = edge.genre.to_lowercase();
            let tag_key = edge.tag.to_lowercase();
            map.entry(genre_key)
                .or_default()
                .insert(tag_key, edge.weight);
        }
        Self {
            edges: Arc::new(map),
        }
    }

    #[must_use]
    pub(crate) fn from_records(records: &[GraphEdgeRecord]) -> Self {
        let mut map: FxHashMap<String, FxHashMap<String, f32>> = FxHashMap::default();
        for record in records {
            let genre_key = record.genre.to_lowercase();
            let tag_key = record.tag.to_lowercase();
            map.entry(genre_key)
                .or_default()
                .insert(tag_key, record.weight);
        }
        Self {
            edges: Arc::new(map),
        }
    }

    #[must_use]
    pub(crate) fn weight(&self, genre: &str, tag: &str) -> Option<f32> {
        self.edges
            .get(&genre.to_lowercase())
            .and_then(|tags| tags.get(&tag.to_lowercase()).copied())
    }

    /// タグが関連付けられているジャンルとその重みを返す
    #[must_use]
    pub(crate) fn genres_for_tag(&self, tag: &str) -> Vec<(String, f32)> {
        let tag_lower = tag.to_lowercase();
        self.edges
            .iter()
            .filter_map(|(genre, tags)| tags.get(&tag_lower).map(|&weight| (genre.clone(), weight)))
            .collect()
    }

    /// Get statistics about the graph for debugging.
    #[must_use]
    pub(crate) fn debug_stats(&self) -> (usize, usize, Vec<String>) {
        let genre_count = self.edges.len();
        let total_tags: usize = self
            .edges
            .values()
            .map(std::collections::HashMap::len)
            .sum();
        let sample_tags: Vec<String> = self
            .edges
            .iter()
            .flat_map(|(genre, tags)| {
                tags.keys()
                    .take(3)
                    .map(move |tag| format!("{}:{}", genre, tag))
            })
            .take(10)
            .collect();
        (genre_count, total_tags, sample_tags)
    }
}

#[async_trait]
pub(crate) trait TagLabelGraphSource: Send + Sync {
    async fn snapshot(&self) -> Result<TagLabelGraphCache>;
}

pub(crate) struct DbTagLabelGraphSource {
    dao: Arc<dyn RecapDao>,
    window_label: String,
    ttl: Duration,
    state: RwLock<TagLabelGraphState>,
    refresh_mutex: tokio::sync::Mutex<()>, // Serialize refresh operations
}

struct TagLabelGraphState {
    cache: TagLabelGraphCache,
    loaded_at: Option<Instant>,
}

impl TagLabelGraphState {
    fn is_fresh(&self, ttl: Duration) -> bool {
        self.loaded_at
            .is_some_and(|instant| instant.elapsed() < ttl)
    }
}

impl DbTagLabelGraphSource {
    pub(crate) fn new(
        dao: Arc<dyn RecapDao>,
        window_label: impl Into<String>,
        ttl: Duration,
    ) -> Self {
        Self {
            dao,
            window_label: window_label.into(),
            ttl,
            state: RwLock::new(TagLabelGraphState {
                cache: TagLabelGraphCache::empty(),
                loaded_at: None,
            }),
            refresh_mutex: tokio::sync::Mutex::new(()),
        }
    }

    pub(crate) async fn preload(&self) -> Result<()> {
        self.refresh().await
    }

    async fn refresh(&self) -> Result<()> {
        let records = self
            .dao
            .load_tag_label_graph(&self.window_label)
            .await
            .with_context(|| {
                format!(
                    "failed to load tag_label_graph window {}",
                    self.window_label
                )
            })?;
        let cache = TagLabelGraphCache::from_records(&records);

        // Debug: Log graph statistics
        let total_edges = records.len();
        let genre_count = records
            .iter()
            .map(|r| &r.genre)
            .collect::<std::collections::HashSet<_>>()
            .len();
        let tag_count = records
            .iter()
            .map(|r| &r.tag)
            .collect::<std::collections::HashSet<_>>()
            .len();

        // Sample tags by genre for better debugging
        let mut genre_samples: std::collections::HashMap<String, Vec<String>> =
            std::collections::HashMap::new();
        for record in records.iter().take(50) {
            genre_samples
                .entry(record.genre.clone())
                .or_default()
                .push(record.tag.clone());
        }
        let sample_by_genre: Vec<String> = genre_samples
            .iter()
            .map(|(genre, tags)| {
                let tag_list: String = tags
                    .iter()
                    .take(3)
                    .map(std::string::String::as_str)
                    .collect::<Vec<_>>()
                    .join(", ");
                format!("{}:[{}]", genre, tag_list)
            })
            .collect();

        tracing::info!(
            window = %self.window_label,
            total_edges = total_edges,
            genre_count = genre_count,
            tag_count = tag_count,
            sample_by_genre = ?sample_by_genre,
            "loaded tag_label_graph"
        );

        let mut guard = self.state.write().await;
        guard.cache = cache;
        guard.loaded_at = Some(Instant::now());
        Ok(())
    }
}

#[async_trait]
impl TagLabelGraphSource for DbTagLabelGraphSource {
    async fn snapshot(&self) -> Result<TagLabelGraphCache> {
        // Fast path: check if cache is fresh
        {
            let guard = self.state.read().await;
            if guard.is_fresh(self.ttl) {
                return Ok(guard.cache.clone());
            }
        }

        // Serialize refresh operations to prevent connection pool exhaustion
        let _refresh_guard = self.refresh_mutex.lock().await;

        // Double-check after acquiring mutex (another task may have refreshed)
        {
            let guard = self.state.read().await;
            if guard.is_fresh(self.ttl) {
                return Ok(guard.cache.clone());
            }
        }

        // Perform refresh
        if let Err(err) = self.refresh().await {
            {
                let guard = self.state.read().await;
                if guard.loaded_at.is_some() {
                    error!(
                        window = %self.window_label,
                        error = ?err,
                        "serving stale tag label graph after refresh failure"
                    );
                    return Ok(guard.cache.clone());
                }
            }
            return Err(err);
        }

        let guard = self.state.read().await;
        Ok(guard.cache.clone())
    }
}

#[cfg(test)]
pub(crate) struct StaticTagLabelGraphSource {
    cache: TagLabelGraphCache,
}

#[cfg(test)]
impl StaticTagLabelGraphSource {
    pub(crate) fn new(cache: TagLabelGraphCache) -> Self {
        Self { cache }
    }
}

#[cfg(test)]
#[async_trait]
impl TagLabelGraphSource for StaticTagLabelGraphSource {
    async fn snapshot(&self) -> Result<TagLabelGraphCache> {
        Ok(self.cache.clone())
    }
}
