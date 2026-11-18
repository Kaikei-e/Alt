//! Test fakes for the themes pipeline.

use anyhow::Result;
use chrono::{DateTime, Utc};
use std::collections::{HashSet, VecDeque};
use std::sync::{Arc, Mutex};
use uuid::Uuid;

use super::ports::{ThemeArticleSource, ThemeGenerator, ThemeKnowledgePort, ThemesJobRunner};
use crate::clients::datahub::DataHubArticle;
use crate::clients::knowledge_sovereign::ReadingSourceEntry;
use crate::clients::knowledge_sovereign::publish::ThemePublishOutcome;
use crate::clients::news_creator::models::{
    ThemeGenerateOutcome, ThemeGenerateRequest, ThemeGenerateResponse, ThemeSentenceOutput,
    ThemeTimelineOutput,
};
use crate::store::dao::themes::UnpublishedThemeVersion;

pub use crate::pipeline::cards::fakes::{FakeCardVerifier, FakeEmbedCluster};

pub type ListSourcesCall = (Uuid, DateTime<Utc>, DateTime<Utc>);
pub type PublishVersionsCall = (Uuid, Uuid, Vec<UnpublishedThemeVersion>);

#[allow(private_interfaces)]
#[derive(Clone, Default)]
pub struct FakeThemeKnowledgePort {
    pub sources: Arc<Mutex<Vec<ReadingSourceEntry>>>,
    pub list_error: Arc<Mutex<Option<String>>>,
    pub list_calls: Arc<Mutex<Vec<ListSourcesCall>>>,
    pub publish_calls: Arc<Mutex<Vec<PublishVersionsCall>>>,
    pub fail_all_publishes: Arc<Mutex<bool>>,
    pub fail_publish_version_ids: Arc<Mutex<HashSet<Uuid>>>,
}

#[allow(private_interfaces)]
impl FakeThemeKnowledgePort {
    pub fn with_sources(sources: Vec<ReadingSourceEntry>) -> Self {
        Self {
            sources: Arc::new(Mutex::new(sources)),
            ..Self::default()
        }
    }

    pub fn set_fail_all_publishes(&self, fail: bool) {
        *self.fail_all_publishes.lock().unwrap() = fail;
    }

    pub fn set_list_error(&self, err: Option<String>) {
        *self.list_error.lock().unwrap() = err;
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl ThemeKnowledgePort for FakeThemeKnowledgePort {
    async fn list_reading_sources_in_window(
        &self,
        user_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<ReadingSourceEntry>> {
        self.list_calls.lock().unwrap().push((user_id, from, to));
        if let Some(ref err) = *self.list_error.lock().unwrap() {
            anyhow::bail!("{err}");
        }
        Ok(self.sources.lock().unwrap().clone())
    }

    async fn publish_theme_versions(
        &self,
        user_id: Uuid,
        tenant_id: Uuid,
        versions: &[UnpublishedThemeVersion],
    ) -> ThemePublishOutcome {
        self.publish_calls
            .lock()
            .unwrap()
            .push((user_id, tenant_id, versions.to_vec()));
        let fail_all = *self.fail_all_publishes.lock().unwrap();
        let fail_ids = self.fail_publish_version_ids.lock().unwrap();
        let mut outcome = ThemePublishOutcome::default();
        for v in versions {
            if fail_all || fail_ids.contains(&v.version_id) {
                outcome.failed += 1;
            } else {
                outcome.succeeded_version_ids.push(v.version_id);
            }
        }
        outcome
    }
}

#[allow(private_interfaces)]
#[derive(Clone, Default)]
pub struct FakeThemeArticleSource {
    pub articles: Arc<Mutex<Vec<DataHubArticle>>>,
    pub fetch_error: Arc<Mutex<Option<String>>>,
    pub fetch_calls: Arc<Mutex<Vec<Vec<Uuid>>>>,
}

#[allow(private_interfaces)]
impl FakeThemeArticleSource {
    pub fn with_articles(articles: Vec<DataHubArticle>) -> Self {
        Self {
            articles: Arc::new(Mutex::new(articles)),
            ..Self::default()
        }
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl ThemeArticleSource for FakeThemeArticleSource {
    async fn batch_get_articles_by_ids(&self, article_ids: &[Uuid]) -> Result<Vec<DataHubArticle>> {
        self.fetch_calls.lock().unwrap().push(article_ids.to_vec());
        if let Some(ref err) = *self.fetch_error.lock().unwrap() {
            anyhow::bail!("{err}");
        }
        let wanted: HashSet<Uuid> = article_ids.iter().copied().collect();
        Ok(self
            .articles
            .lock()
            .unwrap()
            .iter()
            .filter(|a| wanted.contains(&a.id))
            .cloned()
            .collect())
    }
}

#[allow(private_interfaces)]
#[derive(Clone, Default)]
pub struct FakeThemeGenerator {
    pub outcomes: Arc<Mutex<VecDeque<Result<ThemeGenerateOutcome>>>>,
    pub requests: Arc<Mutex<Vec<ThemeGenerateRequest>>>,
}

#[allow(private_interfaces)]
impl FakeThemeGenerator {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_outcomes(outcomes: Vec<Result<ThemeGenerateOutcome>>) -> Self {
        Self {
            outcomes: Arc::new(Mutex::new(outcomes.into())),
            requests: Arc::new(Mutex::new(Vec::new())),
        }
    }
}

#[allow(private_interfaces)]
#[async_trait::async_trait]
impl ThemeGenerator for FakeThemeGenerator {
    async fn generate_theme(&self, req: &ThemeGenerateRequest) -> Result<ThemeGenerateOutcome> {
        self.requests.lock().unwrap().push(req.clone());
        if let Some(next) = self.outcomes.lock().unwrap().pop_front() {
            return next;
        }

        let first_n = req.items.first().map_or(1, |i| i.n);
        let first_date = req
            .items
            .first()
            .and_then(|i| i.published_at.clone())
            .unwrap_or_else(|| "2026-11-18".to_string());

        let what_changed = if req.new_item_ns.is_empty() {
            Vec::new()
        } else {
            vec![ThemeSentenceOutput {
                text: "Added recent developments from newly read articles. [1]".to_string(),
                refs: vec![req.new_item_ns[0]],
            }]
        };

        Ok(ThemeGenerateOutcome::Success(ThemeGenerateResponse {
            title: format!("Theme {}", req.theme_id),
            key_points: vec![ThemeSentenceOutput {
                text: "Core point grounded in the first source. [1]".to_string(),
                refs: vec![first_n],
            }],
            timeline: vec![ThemeTimelineOutput {
                date: first_date,
                text: "Initial milestone observed. [1]".to_string(),
                refs: vec![first_n],
            }],
            what_changed,
            open_questions: vec![ThemeSentenceOutput {
                text: "What remains unresolved across sources? [1]".to_string(),
                refs: vec![first_n],
            }],
            model: "gemma4-e4b".to_string(),
        }))
    }
}

pub type ThemeRunInvocation = (Uuid, DateTime<Utc>);

#[derive(Clone, Default)]
pub struct FakeThemesJobRunner {
    pub runs: Arc<Mutex<Vec<ThemeRunInvocation>>>,
    pub should_fail: Arc<Mutex<bool>>,
    pub notify_on_start: Arc<tokio::sync::Notify>,
    pub notify_on_finish: Arc<tokio::sync::Notify>,
    pub pause_signal: Arc<tokio::sync::Notify>,
    pub hold: Arc<Mutex<bool>>,
}

impl FakeThemesJobRunner {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn set_hold(&self, hold: bool) {
        *self.hold.lock().unwrap() = hold;
    }

    pub fn release(&self) {
        self.pause_signal.notify_waiters();
    }

    pub fn invocations(&self) -> Vec<ThemeRunInvocation> {
        self.runs.lock().unwrap().clone()
    }
}

#[async_trait::async_trait]
impl ThemesJobRunner for FakeThemesJobRunner {
    async fn run_themes(&self, run_id: Uuid, run_time: DateTime<Utc>) -> Result<()> {
        self.runs.lock().unwrap().push((run_id, run_time));
        self.notify_on_start.notify_waiters();

        if *self.hold.lock().unwrap() {
            self.pause_signal.notified().await;
        }

        let res = if *self.should_fail.lock().unwrap() {
            anyhow::bail!("fake themes runner failure");
        } else {
            Ok(())
        };

        self.notify_on_finish.notify_waiters();
        res
    }
}
