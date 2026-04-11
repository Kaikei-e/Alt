# Classificationメトリクス調査レポート

## 調査日時
2025-12-12

## 調査結果サマリー

### データベース確認結果

1. **過去7日間のclassificationメトリクス**: 1件のみ
2. **最新のタイムスタンプ**: 2025-12-11 06:14:07（約4日前）
3. **全期間のclassificationメトリクス**: 1件のみ
4. **他のメトリクスとの比較**:
   - clustering: 669件
   - summarization: 316件
   - classification: 1件（非常に少ない）

### 根本原因

**classificationメトリクスは定期的なジョブ実行では自動保存されていません。**

#### 現在の実装状況

1. **recap-subworker** (`recap_subworker/app/routers/evaluation.py`)
   - `/v1/evaluation/genres` エンドポイントでclassificationメトリクスを保存
   - ただし、このエンドポイントは**手動でAPIを呼び出す必要がある**
   - 定期的なrecapジョブの実行フローには含まれていない

2. **recap-worker** (`recap-worker/src/pipeline/dispatch.rs`)
   - clusteringメトリクス: 自動保存（220行目）
   - summarizationメトリクス: 自動保存（591行目）
   - **classificationメトリクス: 保存処理なし**

3. **ログ確認結果**
   - recap-workerのログにはclusteringの実行記録は多数あるが、classificationのevaluation実行記録は見当たらない
   - recap-subworkerのログにもclassificationメトリクス保存に関する記録がない

### 問題の詳細

7日間のジョブを実行していても、以下の理由でclassificationメトリクスが保存されない：

1. recap-workerのパイプラインにはclassificationメトリクスを保存する処理が実装されていない
2. classificationメトリクスは`/v1/evaluation/genres` APIを手動で呼び出した場合のみ保存される
3. 定期的なrecapジョブの実行フローでは、このevaluation APIは呼び出されない

### 解決策

#### オプション1: recap-workerのパイプラインにclassificationメトリクス保存処理を追加（推奨）

recap-workerのパイプラインで、genre evaluationを実行してclassificationメトリクスを保存する処理を追加する。

**実装場所**: `recap-worker/recap-worker/src/pipeline/dispatch.rs` または `recap-worker/recap-worker/src/pipeline/genre.rs`

**実装内容**:
- 各genreの処理後に、recap-subworkerの`/v1/evaluation/genres` APIを呼び出す
- または、recap-worker内で直接evaluationを実行してメトリクスを保存

#### オプション2: 定期的なevaluationジョブを追加

schedulerに定期的にevaluation APIを呼び出すジョブを追加する。

**実装場所**: `recap-worker/recap-worker/src/scheduler/`

#### オプション3: 既存のevaluation結果を活用

既に`recap_genre_evaluation_runs`テーブルに評価結果が保存されている場合、それを`recap_system_metrics`に変換するバッチ処理を追加する。

### 推奨アクション

1. **短期的対応**: 時間範囲オプションに「7d」を追加（✅ 完了）
2. **中期的対応**: recap-workerのパイプラインにclassificationメトリクス保存処理を追加
3. **長期的対応**: 定期的なevaluationジョブの実装を検討

### 確認用SQLクエリ

```sql
-- 過去7日間のclassificationメトリクスの件数
SELECT COUNT(*) as count
FROM recap_system_metrics
WHERE metric_type = 'classification'
  AND timestamp > NOW() - INTERVAL '7 days';

-- 最新のclassificationメトリクスのタイムスタンプ
SELECT MAX(timestamp) as latest_timestamp
FROM recap_system_metrics
WHERE metric_type = 'classification';

-- メトリクスタイプ別の件数
SELECT metric_type, COUNT(*) as count
FROM recap_system_metrics
GROUP BY metric_type
ORDER BY count DESC;
```
