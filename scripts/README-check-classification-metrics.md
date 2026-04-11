# Classificationメトリクス確認手順

## 概要
Classificationタブが空白になる問題を調査するため、データベースにclassificationメトリクスが保存されているか確認します。

## データベース接続方法

### Docker Compose環境の場合

recap-dbコンテナに接続してSQLクエリを実行します：

```bash
# recap-dbコンテナに接続
docker compose exec recap-db psql -U ${RECAP_DB_USER} -d ${RECAP_DB_NAME}

# または、環境変数を確認してから接続
docker compose exec recap-db psql -U recap_user -d recap_db
```

### SQLクエリの実行

接続後、以下のSQLクエリを実行します：

```sql
-- 過去7日間のclassificationメトリクスの件数
SELECT COUNT(*) as count
FROM recap_system_metrics
WHERE metric_type = 'classification'
  AND timestamp > NOW() - INTERVAL '7 days';
```

または、事前に作成したSQLファイルを使用：

```bash
# SQLファイルをコンテナにコピーして実行
docker compose cp scripts/check_classification_metrics.sql recap-db:/tmp/check.sql
docker compose exec recap-db psql -U ${RECAP_DB_USER} -d ${RECAP_DB_NAME} -f /tmp/check.sql
```

## 確認項目

1. **過去7日間のデータが存在するか**
   - COUNT(*) > 0 であれば、データは保存されている
   - COUNT(*) = 0 であれば、データが保存されていない可能性

2. **最新のタイムスタンプ**
   - 最新のデータが7日以内であれば、正常に保存されている
   - 7日以上古い場合は、最近のジョブでメトリクスが保存されていない可能性

3. **メトリクスの内容**
   - metricsカラムにaccuracy、macro_f1、hamming_lossなどの値が含まれているか確認

## トラブルシューティング

### データが存在しない場合

1. `recap-subworker`のログを確認：
   ```bash
   docker compose logs recap-subworker | grep -i "classification\|system_metrics"
   ```

2. `recap-subworker/recap_subworker/app/routers/evaluation.py`の280-284行目で保存処理が実行されているか確認

3. エラーログがないか確認：
   ```bash
   docker compose logs recap-subworker | grep -i "error\|warning"
   ```

### データは存在するが表示されない場合

1. フロントエンドの時間範囲設定を「7d」に変更して確認
2. ブラウザの開発者ツールでAPIリクエスト/レスポンスを確認
3. `recap-worker`のAPIエンドポイントが正しく動作しているか確認
