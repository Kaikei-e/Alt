-- Classificationメトリクスの確認用SQLクエリ
-- データベースにclassificationメトリクスが保存されているか確認します

-- 1. 過去7日間のclassificationメトリクスの件数
SELECT COUNT(*) as count
FROM recap_system_metrics
WHERE metric_type = 'classification'
  AND timestamp > NOW() - INTERVAL '7 days';

-- 2. 最新のclassificationメトリクスのタイムスタンプ
SELECT MAX(timestamp) as latest_timestamp
FROM recap_system_metrics
WHERE metric_type = 'classification';

-- 3. 過去7日間のclassificationメトリクスの詳細（最新10件）
SELECT job_id, timestamp, metrics
FROM recap_system_metrics
WHERE metric_type = 'classification'
  AND timestamp > NOW() - INTERVAL '7 days'
ORDER BY timestamp DESC
LIMIT 10;

-- 4. 全期間のclassificationメトリクスの件数
SELECT COUNT(*) as total_count
FROM recap_system_metrics
WHERE metric_type = 'classification';

-- 5. メトリクスタイプ別の件数（確認用）
SELECT metric_type, COUNT(*) as count
FROM recap_system_metrics
GROUP BY metric_type
ORDER BY count DESC;
