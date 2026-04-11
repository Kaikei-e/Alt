#!/bin/bash
# 記事HTML→テキスト移行スクリプトのラッパー

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 環境変数の読み込み（.envファイルがあれば）
if [ -f "../.env" ]; then
    export $(grep -v '^#' ../.env | xargs)
fi

# Python仮想環境の確認
if [ -d "venv" ]; then
    source venv/bin/activate
fi

# 依存関係のインストール確認
if ! python3 -c "import psycopg2, bs4, readability, tqdm" 2>/dev/null; then
    echo "必要な依存関係をインストール中..."
    pip install -r requirements.txt
fi

# スクリプト実行
python3 migrate_articles_to_text.py "$@"

