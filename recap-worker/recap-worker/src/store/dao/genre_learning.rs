use anyhow::{Context, Result};
use sqlx::types::Json;
use sqlx::{PgPool, Row};

use super::super::models::{GenreLearningRecord, GraphEdgeRecord};

pub(crate) struct RecapDao;

impl RecapDao {
    /// タグ-ジャンル共起グラフを読み込む。
    pub async fn load_tag_label_graph(
        pool: &PgPool,
        window_label: &str,
    ) -> Result<Vec<GraphEdgeRecord>> {
        let rows = sqlx::query(
            r"
            SELECT genre, tag, weight
            FROM tag_label_graph
            WHERE window_label = $1
            ",
        )
        .bind(window_label)
        .fetch_all(pool)
        .await
        .context("failed to load tag_label_graph entries")?;

        let mut edges = Vec::with_capacity(rows.len());
        for row in rows {
            edges.push(GraphEdgeRecord {
                genre: row.try_get("genre")?,
                tag: row.try_get("tag")?,
                weight: row.try_get::<f32, _>("weight")?,
            });
        }

        Ok(edges)
    }

    /// ジャンル学習レコードを保存する。
    pub async fn upsert_genre_learning_record(
        pool: &PgPool,
        record: &GenreLearningRecord,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_genre_learning_results
                (job_id, article_id, coarse_candidates, refine_decision, tag_profile,
                 graph_context, feedback, telemetry, timestamps)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
            ON CONFLICT (job_id, article_id) DO UPDATE SET
                coarse_candidates = EXCLUDED.coarse_candidates,
                refine_decision = EXCLUDED.refine_decision,
                tag_profile = EXCLUDED.tag_profile,
                graph_context = EXCLUDED.graph_context,
                feedback = COALESCE(EXCLUDED.feedback, recap_genre_learning_results.feedback),
                telemetry = EXCLUDED.telemetry,
                timestamps = EXCLUDED.timestamps,
                updated_at = NOW()
            ",
        )
        .bind(record.job_id)
        .bind(&record.article_id)
        .bind(Json(record.coarse_candidates.clone()))
        .bind(Json(record.refine_decision.clone()))
        .bind(Json(record.tag_profile.clone()))
        .bind(Json(record.graph_context.clone()))
        .bind(record.feedback.clone().map(Json))
        .bind(record.telemetry.clone().map(Json))
        .bind(Json(record.timestamps.clone()))
        .execute(pool)
        .await
        .context("failed to upsert recap_genre_learning_results")?;

        Ok(())
    }

    /// 複数のジャンル学習レコードをバルクでupsertする。
    ///
    /// バッチサイズ（100件）ごとにトランザクションを分けて処理することで、
    /// 接続プールの使用を効率化し、大量データの処理を高速化する。
    pub async fn upsert_genre_learning_records_bulk(
        pool: &PgPool,
        records: &[GenreLearningRecord],
    ) -> Result<()> {
        if records.is_empty() {
            return Ok(());
        }

        const BATCH_SIZE: usize = 100;

        for chunk in records.chunks(BATCH_SIZE) {
            let mut tx = pool
                .begin()
                .await
                .context("failed to begin transaction for bulk upsert")?;

            for record in chunk {
                sqlx::query(
                    r"
                    INSERT INTO recap_genre_learning_results
                        (job_id, article_id, coarse_candidates, refine_decision, tag_profile,
                         graph_context, feedback, telemetry, timestamps)
                    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
                    ON CONFLICT (job_id, article_id) DO UPDATE SET
                        coarse_candidates = EXCLUDED.coarse_candidates,
                        refine_decision = EXCLUDED.refine_decision,
                        tag_profile = EXCLUDED.tag_profile,
                        graph_context = EXCLUDED.graph_context,
                        feedback = COALESCE(EXCLUDED.feedback, recap_genre_learning_results.feedback),
                        telemetry = EXCLUDED.telemetry,
                        timestamps = EXCLUDED.timestamps,
                        updated_at = NOW()
                    ",
                )
                .bind(record.job_id)
                .bind(&record.article_id)
                .bind(Json(record.coarse_candidates.clone()))
                .bind(Json(record.refine_decision.clone()))
                .bind(Json(record.tag_profile.clone()))
                .bind(Json(record.graph_context.clone()))
                .bind(record.feedback.clone().map(Json))
                .bind(record.telemetry.clone().map(Json))
                .bind(Json(record.timestamps.clone()))
                .execute(&mut *tx)
                .await
                .context("failed to upsert recap_genre_learning_results record in bulk")?;
            }

            tx.commit()
                .await
                .context("failed to commit bulk upsert transaction")?;
        }

        Ok(())
    }
}
