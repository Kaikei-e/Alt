use anyhow::{Context, Result, anyhow};
use serde::{Deserialize, Serialize};
use tracing::{debug, warn};
use uuid::Uuid;

use super::SubworkerClient;

/// 評価リクエスト
#[derive(Debug, Clone, Serialize)]
pub(crate) struct EvaluateRequest {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) golden_data_path: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) weights_path: Option<String>,
    pub(crate) use_bootstrap: bool,
    pub(crate) n_bootstrap: i32,
    pub(crate) use_cross_validation: bool,
    pub(crate) n_folds: i32,
    pub(crate) save_to_db: bool,
}

/// 信頼区間
#[allow(dead_code)] // serdeで受け取るDTO: 利用フィールドは呼び出し側に依存
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ConfidenceInterval {
    pub(crate) point: f64,
    pub(crate) lower: f64,
    pub(crate) upper: f64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) width: Option<f64>,
}

/// ジャンル別メトリクス
#[allow(dead_code)] // serdeで受け取るDTO: 全フィールドを常に参照するとは限らない
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct PerGenreMetric {
    pub(crate) tp: i32,
    pub(crate) fp: i32,
    #[serde(rename = "fn")]
    pub(crate) fn_count: i32,
    pub(crate) support: i32,
    pub(crate) precision: f64,
    pub(crate) recall: f64,
    pub(crate) f1: f64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) threshold: Option<f64>,
}

/// 評価レスポンス
#[allow(dead_code)] // serdeで受け取るDTO: 一部フィールドは将来利用/デバッグ用
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct EvaluateResponse {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) run_id: Option<Uuid>,
    pub(crate) accuracy: f64,
    pub(crate) macro_precision: f64,
    pub(crate) macro_recall: f64,
    pub(crate) macro_f1: f64,
    pub(crate) micro_precision: f64,
    pub(crate) micro_recall: f64,
    pub(crate) micro_f1: f64,
    #[serde(rename = "per_genre_metrics")]
    pub(crate) per_genre_metrics: std::collections::HashMap<String, PerGenreMetric>,
    pub(crate) total_samples: i32,
}

impl SubworkerClient {
    /// ジャンル分類の評価を実行する
    pub(crate) async fn evaluate_genres(
        &self,
        request: &EvaluateRequest,
    ) -> Result<EvaluateResponse> {
        let mut url = self.base_url.clone();
        url.path_segments_mut()
            .map_err(|()| anyhow!("subworker base URL must be absolute"))?
            .extend(["v1", "evaluation", "genres"]);

        debug!(
            url = %url,
            use_bootstrap = request.use_bootstrap,
            n_bootstrap = request.n_bootstrap,
            "calling subworker evaluation API"
        );

        let response = self
            .client
            .post(url.clone())
            .json(request)
            .send()
            .await
            .context("failed to send evaluation request")?;

        if !response.status().is_success() {
            let status = response.status();
            let error_text = response
                .text()
                .await
                .unwrap_or_else(|_| "failed to read error response".to_string());
            warn!(
                url = %url,
                status = %status,
                error = %error_text,
                "evaluation API returned error status"
            );
            return Err(anyhow::anyhow!(
                "evaluation API returned status {}: {}",
                status,
                error_text
            ));
        }

        let eval_response: EvaluateResponse = response
            .json()
            .await
            .context("failed to parse evaluation response")?;

        debug!(
            accuracy = eval_response.accuracy,
            macro_f1 = eval_response.macro_f1,
            genre_count = eval_response.per_genre_metrics.len(),
            "evaluation completed successfully"
        );

        Ok(eval_response)
    }
}
