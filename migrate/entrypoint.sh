#!/bin/sh
set -eu

# --- 設定可能なパラメータ ---
MIGRATE_BIN="${MIGRATE_BIN:-/usr/local/bin/migrate}"
MIGRATE_PATH="${MIGRATE_PATH:-/migrations}"
DB_URL="${DB_URL:?環境変数 DB_URL が設定されていません。}"
MAX_RETRIES="${MIGRATE_MAX_RETRIES:-12}"
RETRY_INTERVAL="${MIGRATE_RETRY_INTERVAL:-5}"
# タイムアウト（1回の migrate 実行あたり秒）
MIGRATE_TIMEOUT="${MIGRATE_TIMEOUT:-60}"

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
echo "  タイムアウト: ${MIGRATE_TIMEOUT}s"

current_retry=0
while [ "$current_retry" -lt "$MAX_RETRIES" ]; do
  attempt=$((current_retry + 1))
  echo "[INFO] 試行 ${attempt}/${MAX_RETRIES} — migrate を実行しています..."

  # タイムアウト付きで migrate を呼び出し
  if timeout "${MIGRATE_TIMEOUT}" \
       "${MIGRATE_BIN}" \
         -path "${MIGRATE_PATH}" \
         -database "${DB_URL}" up; then
    echo "[SUCCESS] マイグレーション成功（または変更なし）。"
    exit 0
  else
    code=$?
    echo "[WARN] マイグレーション失敗 (exit code: ${code})."
    current_retry=$((current_retry + 1))
    if [ "$current_retry" -ge "$MAX_RETRIES" ]; then
      echo "[ERROR] 最大試行回数に達しました。終了コード: ${code}"
      exit "${code}"
    fi
    echo "[INFO] ${RETRY_INTERVAL}s 後に再試行します..."
    sleep "${RETRY_INTERVAL}"
  fi
done

# ここには到達しないはず
exit 1
