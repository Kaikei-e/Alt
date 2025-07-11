#!/usr/bin/env bash
set -euo pipefail

# ログレベル抑制
export OLLAMA_LOG_LEVEL=ERROR
export LLAMA_LOG_LEVEL=0
export LLAMA_LOG_VERBOSITY=0

# 確実にホーム配下にディレクトリ作成
export OLLAMA_HOME="${HOME}/.ollama"
mkdir -p "$OLLAMA_HOME"

# サーバーをバックグラウンド起動 (最小ログ)
ollama serve --host 0.0.0.0 2>/dev/null &
SERVER_PID=$!

# 終了時のクリーンアップ設定
cleanup() {
  echo "Shutting down Ollama server (pid $SERVER_PID)..."
  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  exit 0
}
trap cleanup SIGINT SIGTERM

# サーバー準備待ち
echo "Waiting for Ollama server to start..."
until curl -s http://localhost:11434/api/tags >/dev/null 2>&1; do
  sleep 1
done

# モデルプリロード (完全抑制)
echo "Loading gemma3:4b model (quiet)..."
OLLAMA_LOG_LEVEL=ERROR ollama run gemma3:4b >/dev/null 2>&1 << 'EOF'
exit
EOF

# フォアグラウンドで再起動＋ログフィルタ
echo "Starting Ollama server in foreground..."
exec ollama serve --host 0.0.0.0 2>&1 \
  | grep -vE "INFO|print_info:|llama_model_load|load_tensors:|llama_context:|ggml_cuda_init:" || true