//! Test fakes for CardsPipeline.

use anyhow::Result;
use chrono::{DateTime, Utc};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use uuid::Uuid;

use super::ports::{CardGenerator, CardVerifier, EmbedCluster, FeedSource, GenreTagger};
use crate::clients::alt_backend::AltBackendFeed;
use crate::clients::news_creator::models::{
    CardContent, CardGenerateOutcome, CardGenerateRequest, CardGenerateResponse,
    CardGenerationMetadata, CardSentence,
};
use crate::clients::subworker::cards::{
    ClusterOutput, ClusterStoriesRequest, ClusterStoriesResponse, VerifyAttribution,
    VerifyCardRequest, VerifyCardResponse, VerifyEmbeddingInfo, VerifyFiller, VerifySentenceResult,
    VerifySpecificity,
};

#[derive(Clone, Default)]
pub struct FakeFeedSource {
    pub feeds: Arc<Mutex<Vec<AltBackendFeed>>>,
    pub read_feed_ids: Arc<Mutex<Vec<Uuid>>>,
    pub fail_after_n_fetches: Arc<Mutex<Option<usize>>>,
    pub fetch_count: Arc<Mutex<usize>>,
    pub read_window_feeds: Arc<Mutex<Option<Vec<AltBackendFeed>>>>,
}

impl FakeFeedSource {
    pub fn new(feeds: Vec<AltBackendFeed>) -> Self {
        Self {
            feeds: Arc::new(Mutex::new(feeds)),
            read_feed_ids: Arc::new(Mutex::new(Vec::new())),
            fail_after_n_fetches: Arc::new(Mutex::new(None)),
            fetch_count: Arc::new(Mutex::new(0)),
            read_window_feeds: Arc::new(Mutex::new(None)),
        }
    }

    pub fn with_read_ids(feeds: Vec<AltBackendFeed>, read_ids: Vec<Uuid>) -> Self {
        Self {
            feeds: Arc::new(Mutex::new(feeds)),
            read_feed_ids: Arc::new(Mutex::new(read_ids)),
            fail_after_n_fetches: Arc::new(Mutex::new(None)),
            fetch_count: Arc::new(Mutex::new(0)),
            read_window_feeds: Arc::new(Mutex::new(None)),
        }
    }

    pub fn with_fail_after_n_fetches(
        feeds: Vec<AltBackendFeed>,
        read_ids: Vec<Uuid>,
        n: usize,
    ) -> Self {
        Self {
            feeds: Arc::new(Mutex::new(feeds)),
            read_feed_ids: Arc::new(Mutex::new(read_ids)),
            fail_after_n_fetches: Arc::new(Mutex::new(Some(n))),
            fetch_count: Arc::new(Mutex::new(0)),
            read_window_feeds: Arc::new(Mutex::new(None)),
        }
    }

    pub fn with_read_window_feeds(
        window_feeds: Vec<AltBackendFeed>,
        read_feed_ids: Vec<Uuid>,
        read_feeds: Vec<AltBackendFeed>,
    ) -> Self {
        Self {
            feeds: Arc::new(Mutex::new(window_feeds)),
            read_feed_ids: Arc::new(Mutex::new(read_feed_ids)),
            fail_after_n_fetches: Arc::new(Mutex::new(None)),
            fetch_count: Arc::new(Mutex::new(0)),
            read_window_feeds: Arc::new(Mutex::new(Some(read_feeds))),
        }
    }
}

#[async_trait::async_trait]
impl FeedSource for FakeFeedSource {
    async fn fetch_all_feeds_in_window(
        &self,
        _from: DateTime<Utc>,
        _to: DateTime<Utc>,
    ) -> Result<Vec<AltBackendFeed>> {
        let mut count = self.fetch_count.lock().unwrap();
        *count += 1;
        if let Some(limit) = *self.fail_after_n_fetches.lock().unwrap() {
            if *count > limit {
                anyhow::bail!(
                    "alt-backend returned error status 400 Bad Request: {{\"code\":\"invalid_argument\",\"message\":\"date range exceeds 8 days\"}}"
                );
            }
        }
        if *count == 2 {
            if let Some(ref r) = *self.read_window_feeds.lock().unwrap() {
                return Ok(r.clone());
            }
        }
        Ok(self.feeds.lock().unwrap().clone())
    }

    async fn get_all_read_feed_ids(
        &self,
        _user_id: Uuid,
        _since: Option<DateTime<Utc>>,
    ) -> Result<Vec<Uuid>> {
        Ok(self.read_feed_ids.lock().unwrap().clone())
    }
}

pub type FixedEmbeddingMap = Arc<Mutex<Option<HashMap<String, Vec<f32>>>>>;

#[derive(Clone, Default)]
pub struct FakeEmbedCluster {
    pub fixed_embeddings: Arc<Mutex<Option<Vec<Vec<f32>>>>>,
    pub fixed_map: FixedEmbeddingMap,
    pub fixed_cluster_response: Arc<Mutex<Option<ClusterStoriesResponse>>>,
    pub embed_calls: Arc<Mutex<Vec<Vec<String>>>>,
}

impl FakeEmbedCluster {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_fixed(embeddings: Vec<Vec<f32>>, response: ClusterStoriesResponse) -> Self {
        Self {
            fixed_embeddings: Arc::new(Mutex::new(Some(embeddings))),
            fixed_map: Arc::new(Mutex::new(None)),
            fixed_cluster_response: Arc::new(Mutex::new(Some(response))),
            embed_calls: Arc::new(Mutex::new(Vec::new())),
        }
    }

    pub fn with_fixed_map(
        map: HashMap<String, Vec<f32>>,
        response: ClusterStoriesResponse,
    ) -> Self {
        Self {
            fixed_embeddings: Arc::new(Mutex::new(None)),
            fixed_map: Arc::new(Mutex::new(Some(map))),
            fixed_cluster_response: Arc::new(Mutex::new(Some(response))),
            embed_calls: Arc::new(Mutex::new(Vec::new())),
        }
    }
}

#[async_trait::async_trait]
impl EmbedCluster for FakeEmbedCluster {
    async fn embed(&self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        self.embed_calls.lock().unwrap().push(texts.to_vec());
        if let Some(map) = self.fixed_map.lock().unwrap().as_ref() {
            let mut results = Vec::with_capacity(texts.len());
            for t in texts {
                if let Some(v) = map.get(t) {
                    results.push(v.clone());
                } else {
                    let h = xxhash_rust::xxh3::xxh3_64(t.as_bytes());
                    let mut v = vec![0.0_f32; 4];
                    let idx = (h as usize) % 4;
                    v[idx] = 1.0;
                    let frac = ((h >> 8) & 0xFF) as f32 / 1000.0;
                    v[(idx + 1) % 4] = frac;
                    results.push(v);
                }
            }
            return Ok(results);
        }
        if let Some(embs) = self.fixed_embeddings.lock().unwrap().as_ref() {
            return Ok(embs.clone());
        }
        // Deterministic pseudo-embeddings distinct per text hash so they are independent of batch ordering
        Ok(texts
            .iter()
            .map(|t| {
                let h = xxhash_rust::xxh3::xxh3_64(t.as_bytes());
                let mut v = vec![0.0_f32; 4];
                let idx = (h as usize) % 4;
                v[idx] = 1.0;
                let frac = ((h >> 8) & 0xFF) as f32 / 1000.0;
                v[(idx + 1) % 4] = frac;
                v
            })
            .collect())
    }

    async fn cluster_stories(&self, req: &ClusterStoriesRequest) -> Result<ClusterStoriesResponse> {
        if let Some(resp) = self.fixed_cluster_response.lock().unwrap().as_ref() {
            return Ok(resp.clone());
        }
        // Default: put all items into cluster 0
        let member_ids: Vec<Uuid> = req.items.iter().map(|it| it.id).collect();
        Ok(ClusterStoriesResponse {
            clusters: vec![ClusterOutput {
                cluster_id: 0,
                member_ids,
                centroid: vec![0.1, 0.2, 0.3, 0.4],
            }],
            params: req.params.clone(),
        })
    }
}

#[derive(Clone, Default)]
pub struct FakeGenreTagger {
    pub fixed_scores: Arc<Mutex<Option<HashMap<String, f32>>>>,
    pub calls: Arc<Mutex<Vec<String>>>,
    pub should_fail: Arc<Mutex<bool>>,
    pub in_flight: Arc<std::sync::atomic::AtomicUsize>,
    pub max_in_flight: Arc<std::sync::atomic::AtomicUsize>,
}

impl FakeGenreTagger {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_fixed(scores: HashMap<String, f32>) -> Self {
        Self {
            fixed_scores: Arc::new(Mutex::new(Some(scores))),
            calls: Arc::new(Mutex::new(Vec::new())),
            should_fail: Arc::new(Mutex::new(false)),
            in_flight: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            max_in_flight: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        }
    }

    pub fn with_failure() -> Self {
        Self {
            fixed_scores: Arc::new(Mutex::new(None)),
            calls: Arc::new(Mutex::new(Vec::new())),
            should_fail: Arc::new(Mutex::new(true)),
            in_flight: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            max_in_flight: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        }
    }
}

#[async_trait::async_trait]
impl GenreTagger for FakeGenreTagger {
    async fn tag_genre(&self, text: &str) -> Result<HashMap<String, f32>> {
        self.calls.lock().unwrap().push(text.to_string());
        let cur = self
            .in_flight
            .fetch_add(1, std::sync::atomic::Ordering::SeqCst)
            + 1;
        self.max_in_flight
            .fetch_max(cur, std::sync::atomic::Ordering::SeqCst);
        tokio::task::yield_now().await;

        let res = if *self.should_fail.lock().unwrap() {
            Err(anyhow::anyhow!("simulated classifier failure"))
        } else if let Some(scores) = self.fixed_scores.lock().unwrap().as_ref() {
            Ok(scores.clone())
        } else {
            let mut map = HashMap::new();
            map.insert("technology".to_string(), 0.95);
            Ok(map)
        };

        self.in_flight
            .fetch_sub(1, std::sync::atomic::Ordering::SeqCst);
        res
    }

    async fn tag_genres(
        &self,
        texts: &[String],
        concurrency: usize,
    ) -> Result<Vec<HashMap<String, f32>>> {
        use futures::stream::{self, StreamExt, TryStreamExt};
        let concurrency = concurrency.max(1);
        stream::iter(texts.iter().cloned())
            .map(|t| async move { self.tag_genre(&t).await })
            .buffered(concurrency)
            .try_collect()
            .await
    }
}

#[allow(private_interfaces)]
#[derive(Clone, Default)]
pub struct FakeCardGenerator {
    pub responses: Arc<Mutex<Vec<CardGenerateOutcome>>>,
    pub requests: Arc<Mutex<Vec<CardGenerateRequest>>>,
}

#[allow(private_interfaces)]
impl FakeCardGenerator {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_responses(responses: Vec<CardGenerateOutcome>) -> Self {
        Self {
            responses: Arc::new(Mutex::new(responses)),
            requests: Arc::new(Mutex::new(Vec::new())),
        }
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl CardGenerator for FakeCardGenerator {
    async fn generate_card(&self, req: &CardGenerateRequest) -> Result<CardGenerateOutcome> {
        self.requests.lock().unwrap().push(req.clone());
        let mut resps = self.responses.lock().unwrap();
        if !resps.is_empty() {
            return Ok(resps.remove(0));
        }

        // Default synthetic card response
        let what_sentences: Vec<CardSentence> = req
            .items
            .iter()
            .take(2)
            .map(|item| CardSentence {
                text: format!("{}に関する出来事が発生しました。[1]", item.title),
                refs: vec![item.n],
            })
            .collect();

        Ok(CardGenerateOutcome::Success(CardGenerateResponse {
            card: CardContent {
                headline_ja: "主要なテクノロジー動向の進展".to_string(),
                what_ja: what_sentences,
                why_ja: Some(CardSentence {
                    text: "業界全体に重要な影響を与える。[1]".to_string(),
                    refs: vec![1],
                }),
                used_refs: vec![1],
            },
            generation: CardGenerationMetadata {
                model: "gemma4-e4b".to_string(),
                prompt_version: req.prompt_version.clone(),
                cache_hit: false,
                prompt_tokens: 100,
                completion_tokens: 50,
                ms: 120,
                raw_text: "raw text".to_string(),
            },
        }))
    }
}

#[derive(Clone, Default)]
pub struct FakeCardVerifier {
    pub responses: Arc<Mutex<Vec<VerifyCardResponse>>>,
    pub requests: Arc<Mutex<Vec<VerifyCardRequest>>>,
}

impl FakeCardVerifier {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_responses(responses: Vec<VerifyCardResponse>) -> Self {
        Self {
            responses: Arc::new(Mutex::new(responses)),
            requests: Arc::new(Mutex::new(Vec::new())),
        }
    }
}

#[async_trait::async_trait]
impl CardVerifier for FakeCardVerifier {
    async fn verify_card(&self, req: &VerifyCardRequest) -> Result<VerifyCardResponse> {
        self.requests.lock().unwrap().push(req.clone());
        let mut resps = self.responses.lock().unwrap();
        if !resps.is_empty() {
            return Ok(resps.remove(0));
        }

        // Default: all sentences pass
        let sentences = req
            .sentences
            .iter()
            .map(|s| VerifySentenceResult {
                idx: s.idx,
                attribution: VerifyAttribution {
                    max_cos: 0.85,
                    best_n: s.refs.first().copied(),
                    pass: true,
                },
                filler: VerifyFiller {
                    matched: vec![],
                    pass: true,
                },
                specificity: VerifySpecificity {
                    proper_nouns: 1,
                    numbers: 0,
                    tokens: 5,
                    density: 0.2,
                },
            })
            .collect();

        Ok(VerifyCardResponse {
            sentences,
            why_hint_present: true,
            embedding: VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        })
    }
}
