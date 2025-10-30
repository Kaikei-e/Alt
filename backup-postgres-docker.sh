#!/bin/bash
#
# Docker Compose PostgreSQL バックアップスクリプト
# .envファイルを参照してPostgreSQLデータベースをバックアップします
#
# 使用方法:
#   ./backup-postgres-docker.sh [オプション]
#
# オプション:
#   -f, --format FORMAT    バックアップ形式 (sql|custom|directory) [デフォルト: sql]
#   -o, --output DIR       出力ディレクトリ [デフォルト: ./backups]
#   -c, --compress         圧縮を有効にする
#   -h, --help             ヘルプを表示
#

set -euo pipefail

# デフォルト設定
BACKUP_FORMAT="sql"
OUTPUT_DIR="./backups"
COMPRESS=false
CONTAINER_NAME="alt-db"
SERVICE_NAME="db"

# ヘルプ関数
show_help() {
    cat << EOF
Docker Compose PostgreSQL バックアップスクリプト

使用方法:
    $0 [オプション]

オプション:
    -f, --format FORMAT    バックアップ形式 (sql|custom|directory) [デフォルト: sql]
    -o, --output DIR       出力ディレクトリ [デフォルト: ./backups]
    -c, --compress         圧縮を有効にする
    -h, --help             ヘルプを表示

バックアップ形式:
    sql        SQLダンプ形式（デフォルト）
    custom     PostgreSQLカスタム形式（推奨）
    directory  ディレクトリ形式

例:
    $0                                    # 基本的なSQLバックアップ
    $0 -f custom -c                       # 圧縮されたカスタム形式バックアップ
    $0 -o /path/to/backups -f directory   # ディレクトリ形式で指定ディレクトリに保存
EOF
}

# 引数解析
while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--format)
            BACKUP_FORMAT="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -c|--compress)
            COMPRESS=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "不明なオプション: $1" >&2
            show_help
            exit 1
            ;;
    esac
done

# .envファイルの存在確認と読み込み
ENV_FILE=".env"
if [[ ! -f "$ENV_FILE" ]]; then
    echo "❌ エラー: .envファイルが見つかりません" >&2
    echo "   make up を実行して .env ファイルを作成してください" >&2
    exit 1
fi

# .envファイルを読み込み
source "$ENV_FILE"

# 必要な環境変数の確認
required_vars=("POSTGRES_USER" "POSTGRES_PASSWORD" "POSTGRES_DB")
for var in "${required_vars[@]}"; do
    if [[ -z "${!var:-}" ]]; then
        echo "❌ エラー: 環境変数 $var が設定されていません" >&2
        exit 1
    fi
done

# 出力ディレクトリの作成
mkdir -p "$OUTPUT_DIR"

# タイムスタンプの生成
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# バックアップファイル名の生成
case "$BACKUP_FORMAT" in
    sql)
        BACKUP_FILENAME="postgres-backup-${TIMESTAMP}.sql"
        if [[ "$COMPRESS" == true ]]; then
            BACKUP_FILENAME="${BACKUP_FILENAME}.gz"
        fi
        ;;
    custom)
        BACKUP_FILENAME="postgres-backup-${TIMESTAMP}.dump"
        if [[ "$COMPRESS" == true ]]; then
            BACKUP_FILENAME="${BACKUP_FILENAME}.gz"
        fi
        ;;
    directory)
        BACKUP_FILENAME="postgres-backup-${TIMESTAMP}"
        ;;
    *)
        echo "❌ エラー: 無効なバックアップ形式: $BACKUP_FORMAT" >&2
        echo "   有効な形式: sql, custom, directory" >&2
        exit 1
        ;;
esac

BACKUP_PATH="$OUTPUT_DIR/$BACKUP_FILENAME"

# Docker Composeサービスの状態確認
echo "🔍 Docker Composeサービスの状態を確認中..."
if ! docker compose ps "$SERVICE_NAME" | grep -q "Up.*healthy\|Up.*running"; then
    echo "❌ エラー: PostgreSQLサービス ($SERVICE_NAME) が実行されていません" >&2
    echo "   make up を実行してサービスを開始してください" >&2
    exit 1
fi

# データベース接続テスト
echo "🔍 データベース接続をテスト中..."
if ! docker compose exec -T "$SERVICE_NAME" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; then
    echo "❌ エラー: データベースに接続できません" >&2
    exit 1
fi

echo "✅ データベース接続確認完了"

# バックアップ実行
echo "📦 バックアップを開始します..."
echo "   データベース: $POSTGRES_DB"
echo "   ユーザー: $POSTGRES_USER"
echo "   形式: $BACKUP_FORMAT"
echo "   出力先: $BACKUP_PATH"
if [[ "$COMPRESS" == true ]]; then
    echo "   圧縮: 有効"
fi

# バックアップコマンドの実行
case "$BACKUP_FORMAT" in
    sql)
        if [[ "$COMPRESS" == true ]]; then
            docker compose exec -T "$SERVICE_NAME" pg_dump \
                -U "$POSTGRES_USER" \
                --clean \
                --if-exists \
                --create \
                --verbose \
                "$POSTGRES_DB" | gzip > "$BACKUP_PATH"
        else
            docker compose exec -T "$SERVICE_NAME" pg_dump \
                -U "$POSTGRES_USER" \
                --clean \
                --if-exists \
                --create \
                --verbose \
                "$POSTGRES_DB" > "$BACKUP_PATH"
        fi
        ;;
    custom)
        if [[ "$COMPRESS" == true ]]; then
            docker compose exec -T "$SERVICE_NAME" pg_dump \
                -U "$POSTGRES_USER" \
                --format=custom \
                --compress=9 \
                --verbose \
                --file="/tmp/backup.dump" \
                "$POSTGRES_DB"
            docker compose exec -T "$SERVICE_NAME" cat /tmp/backup.dump | gzip > "$BACKUP_PATH"
            docker compose exec "$SERVICE_NAME" rm -f /tmp/backup.dump
        else
            docker compose exec -T "$SERVICE_NAME" pg_dump \
                -U "$POSTGRES_USER" \
                --format=custom \
                --verbose \
                --file="/tmp/backup.dump" \
                "$POSTGRES_DB"
            docker compose exec -T "$SERVICE_NAME" cat /tmp/backup.dump > "$BACKUP_PATH"
            docker compose exec "$SERVICE_NAME" rm -f /tmp/backup.dump
        fi
        ;;
    directory)
        # ディレクトリ形式の場合は一時ディレクトリを作成
        TEMP_DIR="/tmp/postgres-backup-${TIMESTAMP}"
        docker compose exec "$SERVICE_NAME" mkdir -p "$TEMP_DIR"
        docker compose exec -T "$SERVICE_NAME" pg_dump \
            -U "$POSTGRES_USER" \
            --format=directory \
            --verbose \
            --file="$TEMP_DIR" \
            "$POSTGRES_DB"

        # ディレクトリをローカルにコピー
        docker compose cp "$SERVICE_NAME:$TEMP_DIR" "$BACKUP_PATH"

        # 一時ディレクトリを削除
        docker compose exec "$SERVICE_NAME" rm -rf "$TEMP_DIR"
        ;;
esac

# バックアップ完了確認
if [[ -f "$BACKUP_PATH" ]] || [[ -d "$BACKUP_PATH" ]]; then
    echo ""
    echo "✅ バックアップが正常に完了しました"
    echo "   ファイル: $BACKUP_PATH"

    if [[ -f "$BACKUP_PATH" ]]; then
        echo "   サイズ: $(ls -lh "$BACKUP_PATH" | awk '{print $5}')"
    elif [[ -d "$BACKUP_PATH" ]]; then
        echo "   サイズ: $(du -sh "$BACKUP_PATH" | awk '{print $1}')"
    fi

    echo ""
    echo "📋 復元方法:"
    case "$BACKUP_FORMAT" in
        sql)
            if [[ "$COMPRESS" == true ]]; then
                echo "   gunzip -c $BACKUP_PATH | docker compose exec -T $SERVICE_NAME psql -U $POSTGRES_USER -d $POSTGRES_DB"
            else
                echo "   docker compose exec -T $SERVICE_NAME psql -U $POSTGRES_USER -d $POSTGRES_DB < $BACKUP_PATH"
            fi
            ;;
        custom)
            if [[ "$COMPRESS" == true ]]; then
                echo "   gunzip -c $BACKUP_PATH | docker compose exec -T $SERVICE_NAME pg_restore -U $POSTGRES_USER -d $POSTGRES_DB"
            else
                echo "   docker compose exec -T $SERVICE_NAME pg_restore -U $POSTGRES_USER -d $POSTGRES_DB < $BACKUP_PATH"
            fi
            ;;
        directory)
            echo "   docker compose cp $BACKUP_PATH $SERVICE_NAME:/tmp/restore"
            echo "   docker compose exec $SERVICE_NAME pg_restore -U $POSTGRES_USER -d $POSTGRES_DB /tmp/restore"
            ;;
    esac
else
    echo "❌ バックアップが失敗しました" >&2
    exit 1
fi
