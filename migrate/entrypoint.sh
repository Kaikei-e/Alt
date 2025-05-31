#!/bin/sh
set -e

DB_URL="${DB_URL}"
MAX_RETRIES="${MIGRATE_MAX_RETRIES:-12}"      # 最大リトライ回数 (デフォルト12回)
RETRY_INTERVAL="${MIGRATE_RETRY_INTERVAL:-5}" # リトライ間隔（秒） (デフォルト5秒)

# DB_URLが設定されているか確認
if [ -z "$DB_URL" ]; then
  echo "エラー: 環境変数 DB_URL が設定されていません。"
  exit 1
fi

echo "データベースマイグレーションを開始します..."
echo "DB接続先URL (マスクされていません。本番環境では注意): ${DB_URL}"
echo "最大リトライ回数: ${MAX_RETRIES}"
echo "リトライ間隔: ${RETRY_INTERVAL}秒"

current_retry=0
until [ "$current_retry" -ge "$MAX_RETRIES" ]; do
  echo "マイグレーション実行試行 (試行回数: $((current_retry + 1))/${MAX_RETRIES})..."
  # go-migrateコマンドを実行
  # migrate -path /migrations -database "${DB_URL}" up # -verbose オプションを追加しても良い
  if migrate -path /migrations -database "${DB_URL}" up; then
    echo "マイグレーション成功 (または変更なし)。"
    exit 0 # 正常終了
  else
    exit_code=$?
    current_retry=$((current_retry + 1))
    if [ "$current_retry" -ge "$MAX_RETRIES" ]; then
      echo "マイグレーション失敗 (最大試行回数超過)。終了コード: ${exit_code}"
      exit ${exit_code} # 最終的な失敗
    fi
    echo "マイグレーション失敗。終了コード: ${exit_code}。${RETRY_INTERVAL}秒後に再試行します..."
    sleep "${RETRY_INTERVAL}"
  fi
done