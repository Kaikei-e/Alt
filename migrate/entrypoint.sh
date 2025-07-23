#!/bin/sh
set -eu

# --- 設定可能なパラメータ ---
MIGRATE_BIN="${MIGRATE_BIN:-/usr/local/bin/migrate}"
MIGRATE_PATH="${MIGRATE_PATH:-/migrations}"
DB_URL="${DB_URL:?環境変数 DB_URL が設定されていません。}"
MAX_RETRIES="${MIGRATE_MAX_RETRIES:-3}"
RETRY_INTERVAL="${MIGRATE_RETRY_INTERVAL:-10}"
# migrateネイティブのlock-timeout（秒）- 2025年ベストプラクティスに基づき300秒に延長
LOCK_TIMEOUT="${MIGRATE_LOCK_TIMEOUT:-300}"

# --- 内部関数 ---
# DB_URL のパスワードをマスクして表示
mask_url() {
  # postgresql://user:pass@host/db?… の形式を想定
  echo "$DB_URL" | sed -E 's#(://[^:]+:)[^@]+@#\1****@#'
}

# 終了時に呼ばれるハンドラ
cleanup() {
  echo "[INFO] 受信したシグナルでシャットダウンします…"
  exit 0
}

# シグナル捕捉
trap cleanup INT TERM

# --- 処理開始 ---
echo "[INFO] データベースマイグレーションを開始します..."
echo "  DB 接続先URL: $(mask_url)"
echo "  マイグレーションパス: ${MIGRATE_PATH}"
echo "  migrate バイナリ: ${MIGRATE_BIN}"
echo "  最大試行回数: ${MAX_RETRIES}"
echo "  リトライ間隔: ${RETRY_INTERVAL}s"
echo "  ロックタイムアウト: ${LOCK_TIMEOUT}s"

current_retry=0
while [ "$current_retry" -lt "$MAX_RETRIES" ]; do
  attempt=$((current_retry + 1))
  echo "[INFO] 試行 ${attempt}/${MAX_RETRIES} — migrate を実行しています..."
  echo "[DEBUG] ロックタイムアウト: ${LOCK_TIMEOUT}秒, パス: ${MIGRATE_PATH}"

  # 開始時刻を記録してパフォーマンス測定
  start_time=$(date +%s)
  
  # migrateのネイティブlock-timeoutを使用（詳細ログ付き）
  if "${MIGRATE_BIN}" \
       -path "${MIGRATE_PATH}" \
       -database "${DB_URL}" \
       -lock-timeout "${LOCK_TIMEOUT}" \
       -verbose \
       up; then
    end_time=$(date +%s)
    duration=$((end_time - start_time))
    echo "[SUCCESS] マイグレーション成功（実行時間: ${duration}秒）"
    exit 0
  else
    code=$?
    end_time=$(date +%s)
    duration=$((end_time - start_time))
    echo "[WARN] マイグレーション失敗 (exit code: ${code}, 実行時間: ${duration}秒)."
    
    # エラーコード124（タイムアウト）の特別処理
    if [ "$code" = "124" ]; then
      echo "[ERROR] タイムアウトが発生しました。データベースロックまたはネットワーク問題の可能性があります。"
    fi
    
    current_retry=$((current_retry + 1))
    if [ "$current_retry" -ge "$MAX_RETRIES" ]; then
      echo "[ERROR] 最大試行回数に達しました。終了コード: ${code}"
      echo "[DEBUG] 問題の診断: exit code ${code} - 詳細はKubernetesログを確認してください。"
      exit "${code}"
    fi
    
    # 指数バックオフの実装（2025年ベストプラクティス）
    backoff_time=$((RETRY_INTERVAL * attempt))
    echo "[INFO] ${backoff_time}s 後に再試行します（指数バックオフ）..."
    sleep "${backoff_time}"
  fi
done

# ここには到達しないはず
exit 1
