# Recap Database Atlas Migrations

このディレクトリは Recap Worker が利用する `recap-db` 用の Atlas マイグレーションを管理します。Docker Compose (`compose/recap.yaml`) または Docker コンテナ経由でマイグレーションを実行します。

## 構成

```
recap-migration-atlas/
  docker/             # Atlas 実行用コンテナの Dockerfile とスクリプト
  migrations/         # Atlas 形式の SQL マイグレーションと設定ファイル
```

## 使い方

### Docker Compose 経由

```bash
# マイグレーション適用
docker compose -f compose/compose.yaml -p alt run --rm recap-db-migrator apply

# ステータス確認
docker compose -f compose/compose.yaml -p alt run --rm recap-db-migrator status
```

### 直接コンテナ実行

1. Atlas 用の環境変数を設定します（例）:
   ```bash
   export DATABASE_URL="postgres://user:password@recap-db:5432/recap"
   export ATLAS_REVISIONS_SCHEMA=public
   ```

2. Docker イメージをビルドして実行します:
   ```bash
   docker build -t recap-db-migrator ./recap-migration-atlas/docker
   docker run --rm \
     -e DATABASE_URL \
     -e ATLAS_REVISIONS_SCHEMA \
     recap-db-migrator apply
   ```

3. 既存スキーマを Atlas に取り込む必要がある場合は `MIGRATE_BASELINE_VERSION` を指定して `status` を実行し、Atlas の guidance に従って baseline を行ってください。

> **Note**: `atlas.sum` の生成・更新は `docker/scripts/hash.sh` を使って実施します。
