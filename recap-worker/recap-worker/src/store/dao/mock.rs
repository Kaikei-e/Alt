// テスト用のモックRecapDao実装
// プロダクションコードから分離して、テスト専用のモックを提供

#[cfg(test)]
use anyhow::Result;
#[cfg(test)]
use sqlx::PgPool;
#[cfg(test)]
use uuid::Uuid;

#[cfg(test)]
/// テスト用のモックRecapDao（DB接続なしで動作）
#[allow(dead_code)]
#[derive(Clone)]
pub(crate) struct MockRecapDao;

#[cfg(test)]
#[allow(dead_code, clippy::unused_self)]
impl MockRecapDao {
    #[allow(dead_code)]
    pub(crate) fn new() -> Self {
        Self
    }

    #[allow(dead_code, clippy::unused_self)]
    pub(crate) fn pool(&self) -> &PgPool {
        // このメソッドは使われないが、型チェックを通すために必要
        // 実際には呼ばれない想定
        panic!("MockRecapDao::pool() should not be called in tests")
    }

    #[allow(dead_code)]
    pub(crate) fn save_stage_state(
        &self,
        _job_id: Uuid,
        _stage: &str,
        _state_data: &serde_json::Value,
    ) -> std::future::Ready<Result<()>> {
        // モック: DB操作をスキップ（常に成功）
        std::future::ready(Ok(()))
    }

    #[allow(dead_code)]
    pub(crate) fn load_stage_state(
        &self,
        _job_id: Uuid,
        _stage: &str,
    ) -> std::future::Ready<Result<Option<serde_json::Value>>> {
        // モック: リジューム時は常にNoneを返す（新規実行をシミュレート）
        std::future::ready(Ok(None))
    }

    #[allow(dead_code)]
    pub(crate) fn update_job_status(
        &self,
        _job_id: Uuid,
        _status: super::JobStatus,
        _last_stage: Option<&str>,
    ) -> std::future::Ready<Result<()>> {
        // モック: DB操作をスキップ
        std::future::ready(Ok(()))
    }

    #[allow(dead_code)]
    pub(crate) fn insert_stage_log(
        &self,
        _job_id: Uuid,
        _stage: &str,
        _status: &str,
        _message: Option<&str>,
    ) -> std::future::Ready<Result<()>> {
        // モック: DB操作をスキップ
        std::future::ready(Ok(()))
    }

    #[allow(dead_code, clippy::unused_self)]
    pub(crate) fn insert_failed_task(
        &self,
        _job_id: Uuid,
        _stage: &str,
        _payload: Option<&serde_json::Value>,
        _error: Option<&str>,
    ) -> std::future::Ready<Result<()>> {
        // モック: DB操作をスキップ
        std::future::ready(Ok(()))
    }

    #[allow(dead_code, clippy::unused_self)]
    pub(crate) fn get_articles_by_ids(
        &self,
        _job_id: Uuid,
        _article_ids: &[String],
    ) -> std::future::Ready<Result<Vec<super::article::FetchedArticleData>>> {
        // モック: 空のリストを返す（リジューム時は使われない想定）
        std::future::ready(Ok(vec![]))
    }
}
