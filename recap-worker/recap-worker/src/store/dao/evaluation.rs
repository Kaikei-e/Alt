use anyhow::{Context, Result};
use sqlx::PgPool;
use uuid::Uuid;

use super::super::models::{GenreEvaluationMetric, GenreEvaluationRun};

pub(crate) struct RecapDao;

impl RecapDao {
    /// ジャンル評価実行のメタデータとメトリクスを保存する。
    ///
    /// トランザクション内で実行され、run_idとper-genreメトリクスを一括で保存します。
    pub async fn save_genre_evaluation(
        pool: &PgPool,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> Result<()> {
        let mut tx = pool
            .begin()
            .await
            .context("failed to begin transaction for genre evaluation")?;

        // Insert run metadata
        sqlx::query(
            r"
            INSERT INTO recap_genre_evaluation_runs
                (run_id, dataset_path, total_items, macro_precision, macro_recall, macro_f1,
                 summary_tp, summary_fp, summary_fn,
                 micro_precision, micro_recall, micro_f1, weighted_f1,
                 macro_f1_valid, valid_genre_count, undefined_genre_count)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
            ",
        )
        .bind(run.run_id)
        .bind(&run.dataset_path)
        .bind(run.total_items)
        .bind(run.macro_precision)
        .bind(run.macro_recall)
        .bind(run.macro_f1)
        .bind(run.summary_tp)
        .bind(run.summary_fp)
        .bind(run.summary_fn)
        .bind(run.micro_precision)
        .bind(run.micro_recall)
        .bind(run.micro_f1)
        .bind(run.weighted_f1)
        .bind(run.macro_f1_valid)
        .bind(run.valid_genre_count)
        .bind(run.undefined_genre_count)
        .execute(&mut *tx)
        .await
        .context("failed to insert genre evaluation run")?;

        // Bulk insert per-genre metrics
        for metric in metrics {
            sqlx::query(
                r"
                INSERT INTO recap_genre_evaluation_metrics
                    (run_id, genre, tp, fp, fn_count, precision, recall, f1_score)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                ",
            )
            .bind(run.run_id)
            .bind(&metric.genre)
            .bind(metric.tp)
            .bind(metric.fp)
            .bind(metric.fn_count)
            .bind(metric.precision)
            .bind(metric.recall)
            .bind(metric.f1_score)
            .execute(&mut *tx)
            .await
            .with_context(|| {
                format!(
                    "failed to insert genre evaluation metric for genre: {}",
                    metric.genre
                )
            })?;
        }

        tx.commit()
            .await
            .context("failed to commit genre evaluation transaction")?;

        Ok(())
    }

    /// 指定されたrun_idの評価結果を取得する
    #[allow(clippy::too_many_lines)]
    pub async fn get_genre_evaluation(
        pool: &PgPool,
        run_id: Uuid,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        // Get run metadata
        let run_row = sqlx::query_as::<
            _,
            (
                uuid::Uuid,
                String,
                i32,
                f64,
                f64,
                f64,
                i32,
                i32,
                i32,
                Option<f64>,
                Option<f64>,
                Option<f64>,
                Option<f64>,
                Option<f64>,
                Option<i32>,
                Option<i32>,
            ),
        >(
            r"
            SELECT run_id, dataset_path, total_items, macro_precision, macro_recall, macro_f1,
                   summary_tp, summary_fp, summary_fn,
                   micro_precision, micro_recall, micro_f1, weighted_f1,
                   macro_f1_valid, valid_genre_count, undefined_genre_count
            FROM recap_genre_evaluation_runs
            WHERE run_id = $1
            ",
        )
        .bind(run_id)
        .fetch_optional(pool)
        .await
        .context("failed to fetch genre evaluation run")?;

        let Some((
            run_id,
            dataset_path,
            total_items,
            macro_precision,
            macro_recall,
            macro_f1,
            summary_true_positives,
            summary_false_positives,
            summary_false_negatives,
            micro_precision_val,
            micro_recall_val,
            micro_f1_val,
            weighted_f1,
            macro_f1_valid,
            valid_genre_count,
            undefined_genre_count,
        )) = run_row
        else {
            return Ok(None);
        };

        let run = GenreEvaluationRun {
            run_id,
            dataset_path,
            total_items,
            macro_precision,
            macro_recall,
            macro_f1,
            summary_tp: summary_true_positives,
            summary_fp: summary_false_positives,
            summary_fn: summary_false_negatives,
            micro_precision: micro_precision_val,
            micro_recall: micro_recall_val,
            micro_f1: micro_f1_val,
            weighted_f1,
            macro_f1_valid,
            valid_genre_count,
            undefined_genre_count,
        };

        // Get per-genre metrics
        let metric_rows = sqlx::query_as::<_, (String, i32, i32, i32, f64, f64, f64)>(
            r"
            SELECT genre, tp, fp, fn_count, precision, recall, f1_score
            FROM recap_genre_evaluation_metrics
            WHERE run_id = $1
            ORDER BY genre
            ",
        )
        .bind(run_id)
        .fetch_all(pool)
        .await
        .context("failed to fetch genre evaluation metrics")?;

        let metrics = metric_rows
            .into_iter()
            .map(
                |(genre, tp, fp, fn_count, precision, recall, f1_score)| GenreEvaluationMetric {
                    genre,
                    tp,
                    fp,
                    fn_count,
                    precision,
                    recall,
                    f1_score,
                },
            )
            .collect();

        Ok(Some((run, metrics)))
    }

    /// 最新の評価結果を取得する
    pub async fn get_latest_genre_evaluation(
        pool: &PgPool,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        // Get latest run_id
        let run_id_row = sqlx::query_as::<_, (uuid::Uuid,)>(
            r"
            SELECT run_id
            FROM recap_genre_evaluation_runs
            ORDER BY created_at DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch latest genre evaluation run_id")?;

        let Some((run_id,)) = run_id_row else {
            return Ok(None);
        };

        Self::get_genre_evaluation(pool, run_id).await
    }
}
