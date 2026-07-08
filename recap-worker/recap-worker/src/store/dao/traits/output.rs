//! OutputDao trait - Recap output operations

use std::collections::HashMap;
use std::future::Future;

use anyhow::Result;
use serde_json::Value;
use uuid::Uuid;

use crate::store::models::{
    ClusterWithEvidence, GenreWithSummary, PersistedGenre, RecapFinalSection, RecapJob,
    RecapOutput, RecapSearchHit,
};

/// OutputDao - リキャップ出力のためのデータアクセス層
#[allow(dead_code)]
pub trait OutputDao: Send + Sync {
    /// 最終セクションを保存する
    fn save_final_section(
        &self,
        section: &RecapFinalSection,
    ) -> impl Future<Output = Result<i64>> + Send;

    /// リキャップ出力を挿入/更新する
    fn upsert_recap_output(&self, output: &RecapOutput) -> impl Future<Output = Result<()>> + Send;

    /// リキャップ出力のボディJSONを取得する
    fn get_recap_output_body_json(
        &self,
        job_id: Uuid,
        genre: &str,
    ) -> impl Future<Output = Result<Option<Value>>> + Send;

    /// 最新の完了済みジョブを取得する
    fn get_latest_completed_job(
        &self,
        window_days: i32,
    ) -> impl Future<Output = Result<Option<RecapJob>>> + Send;

    /// ジョブごとのジャンルを取得する
    fn get_genres_by_job(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<Vec<GenreWithSummary>>> + Send;

    /// ジョブごとのクラスタを取得する
    fn get_clusters_by_job(
        &self,
        job_id: Uuid,
    ) -> impl Future<Output = Result<HashMap<String, Vec<ClusterWithEvidence>>>> + Send;

    /// top_termsでRecapジャンルを横断検索する
    fn search_recaps_by_term(
        &self,
        term: &str,
        limit: i32,
    ) -> impl Future<Output = Result<Vec<RecapSearchHit>>> + Send;

    /// 指定 run の `recap_subworker_sentences` を `(article_id -> Vec<sentence DB id>)` に集約して返す。
    /// citation reconciliation で `references[n-1].article_id` を sentence id に解決するために使う。
    fn get_sentence_ids_by_run(
        &self,
        run_id: i64,
    ) -> impl Future<Output = Result<HashMap<String, Vec<i64>>>> + Send;

    /// `recap_outputs` と `recap_sections` (genre pointer) を単一トランザクションで
    /// 書き込む。別々の auto-commit にすると片方の失敗でもう片方だけ残り得るため。
    fn persist_genre_output(
        &self,
        output: &RecapOutput,
        genre: &PersistedGenre,
    ) -> impl Future<Output = Result<()>> + Send;
}
