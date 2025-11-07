/// 部分成功ハンドリングと不足ジャンル検出。
use std::collections::HashSet;

use uuid::Uuid;

use crate::pipeline::dispatch::{DispatchResult, GenreResult};

/// 部分成功の分析結果。
#[derive(Debug, Clone)]
pub(crate) struct PartialSuccessAnalysis {
    pub(crate) job_id: Uuid,
    pub(crate) successful_genres: Vec<String>,
    pub(crate) failed_genres: Vec<String>,
    pub(crate) missing_genres: Vec<String>,
    pub(crate) can_retry: bool,
}

impl PartialSuccessAnalysis {
    /// DispatchResultから部分成功を分析する。
    pub(crate) fn from_dispatch_result(
        job_id: Uuid,
        result: &DispatchResult,
        expected_genres: &[String],
    ) -> Self {
        let successful: Vec<String> = result
            .genre_results
            .iter()
            .filter(|(_, gr)| gr.error.is_none() && gr.summary_response_id.is_some())
            .map(|(genre, _)| genre.clone())
            .collect();

        let failed: Vec<String> = result
            .genre_results
            .iter()
            .filter(|(_, gr)| gr.error.is_some())
            .map(|(genre, _)| genre.clone())
            .collect();

        let successful_set: HashSet<String> = successful.iter().cloned().collect();
        let expected_set: HashSet<String> = expected_genres.iter().cloned().collect();

        let missing: Vec<String> = expected_set.difference(&successful_set).cloned().collect();

        // リトライ可能かどうか（致命的エラーがない場合）
        let can_retry = failed.iter().all(|genre| {
            result
                .genre_results
                .get(genre)
                .and_then(|gr| gr.error.as_ref())
                .map(|e| {
                    // 簡易的な判定：エラーメッセージに"fatal"が含まれていなければリトライ可能
                    !e.to_lowercase().contains("fatal")
                })
                .unwrap_or(true)
        });

        Self {
            job_id,
            successful_genres: successful,
            failed_genres: failed,
            missing_genres: missing,
            can_retry,
        }
    }

    /// すべて成功したかどうか。
    pub(crate) fn is_complete(&self) -> bool {
        self.failed_genres.is_empty() && self.missing_genres.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pipeline::dispatch::GenreResult;
    use std::collections::HashMap;

    #[test]
    fn partial_success_identifies_missing_genres() {
        let job_id = Uuid::new_v4();
        let mut genre_results = HashMap::new();

        genre_results.insert(
            "ai".to_string(),
            GenreResult {
                genre: "ai".to_string(),
                clustering_response: None,
                summary_response_id: Some("resp-1".to_string()),
                error: None,
            },
        );

        let result = DispatchResult {
            job_id,
            genre_results,
            success_count: 1,
            failure_count: 0,
        };

        let analysis = PartialSuccessAnalysis::from_dispatch_result(
            job_id,
            &result,
            &["ai".to_string(), "tech".to_string()],
        );

        assert_eq!(analysis.successful_genres, vec!["ai"]);
        assert_eq!(analysis.missing_genres, vec!["tech"]);
        assert!(!analysis.is_complete());
    }

    #[test]
    fn partial_success_identifies_failed_genres() {
        let job_id = Uuid::new_v4();
        let mut genre_results = HashMap::new();

        genre_results.insert(
            "ai".to_string(),
            GenreResult {
                genre: "ai".to_string(),
                clustering_response: None,
                summary_response_id: None,
                error: Some("Clustering failed".to_string()),
            },
        );

        let result = DispatchResult {
            job_id,
            genre_results,
            success_count: 0,
            failure_count: 1,
        };

        let analysis =
            PartialSuccessAnalysis::from_dispatch_result(job_id, &result, &["ai".to_string()]);

        assert_eq!(analysis.failed_genres, vec!["ai"]);
        assert!(!analysis.is_complete());
    }
}
