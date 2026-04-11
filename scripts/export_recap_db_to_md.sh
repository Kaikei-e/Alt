#!/bin/bash
# recap-dbのデータをMarkdownファイルに書き出すスクリプト

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_FILE="${1:-recap_db_export.md}"
OUTPUT_PATH="$PROJECT_ROOT/$OUTPUT_FILE"

# .envファイルから接続情報を読み込む
if [ -f "$PROJECT_ROOT/.env" ]; then
    export $(grep -E "^RECAP_DB_" "$PROJECT_ROOT/.env" | xargs)
fi

# デフォルト値
RECAP_DB_HOST="${RECAP_DB_HOST:-recap-db}"
RECAP_DB_PORT="${RECAP_DB_PORT:-5432}"
RECAP_DB_USER="${RECAP_DB_USER:-recap_user}"
RECAP_DB_PASSWORD="${RECAP_DB_PASSWORD:-}"
RECAP_DB_NAME="${RECAP_DB_NAME:-recap}"

# Dockerコンテナ内で実行する場合
if docker compose ps recap-db | grep -q "Up"; then
    echo "Dockerコンテナ経由でデータを取得します..."

    # recap-subworkerコンテナを使用（Python環境がある）
    SCRIPT_NAME="export_recap_db_to_md_docker.py"
    CONTAINER_SCRIPT="/tmp/$SCRIPT_NAME"

    # スクリプトをコンテナにコピー
    docker compose cp "$SCRIPT_DIR/$SCRIPT_NAME" "recap-subworker:$CONTAINER_SCRIPT"

    # コンテナ内でpsycopg2-binaryをインストール（必要に応じて）
    docker compose exec recap-subworker uv pip install psycopg2-binary --quiet 2>/dev/null || true

    # コンテナ内で実行
    docker compose exec -e RECAP_DB_HOST="$RECAP_DB_HOST" \
                        -e RECAP_DB_PORT="$RECAP_DB_PORT" \
                        -e RECAP_DB_USER="$RECAP_DB_USER" \
                        -e RECAP_DB_PASSWORD="$RECAP_DB_PASSWORD" \
                        -e RECAP_DB_NAME="$RECAP_DB_NAME" \
                        recap-subworker python3 "$CONTAINER_SCRIPT" /tmp/recap_db_export.md

    # 結果をホストにコピー
    docker compose cp "recap-subworker:/tmp/recap_db_export.md" "$OUTPUT_PATH"

    echo "✓ エクスポート完了: $OUTPUT_PATH"
else
    echo "エラー: recap-dbコンテナが起動していません"
    echo "起動するには: docker compose --profile recap up -d recap-db"
    exit 1
fi
