#!/usr/bin/env bash
set -euo pipefail

# --- privilege drop ----------------------------------------------------------
# root で起動された場合は所有権を直してから ollama-user で再実行
if [ "$(id -u)" -eq 0 ] && [ "${OLLAMA_ENTRYPOINT_RERUN:-0}" != "1" ]; then
  TARGET_USER="ollama-user"
  USER_HOME="$(getent passwd "$TARGET_USER" | cut -d: -f6)"
  export HOME="$USER_HOME"
  export OLLAMA_HOME="${OLLAMA_HOME:-${USER_HOME}/.ollama}"
  mkdir -p "$OLLAMA_HOME"
  chown -R "$TARGET_USER":"$TARGET_USER" "$OLLAMA_HOME"

  export OLLAMA_ENTRYPOINT_RERUN=1
  exec su -p "$TARGET_USER" -c "/usr/local/bin/entrypoint.sh"
fi

# --- Ollama server configuration --------------------------------------------
# GPUメモリ最適化: 7GBギリギリまで使用（80K時は約7.0-7.5 GiB使用予定）
# コンテキスト長を統一することでランナー再利用を改善し、メモリ使用を安定化
export OLLAMA_HOST="${OLLAMA_HOST:-127.0.0.1:11435}"   # 明示的にポート11435を指定
export OLLAMA_CONTEXT_LENGTH="${OLLAMA_CONTEXT_LENGTH:-75200}" # 75Kコンテキスト（7GBギリギリまで使用）
export OLLAMA_NUM_PARALLEL="${OLLAMA_NUM_PARALLEL:-1}"         # 8GB では並列 1 が安定
export OLLAMA_MAX_LOADED_MODELS="${OLLAMA_MAX_LOADED_MODELS:-1}" # 同じコンテキスト長なら再利用される
export OLLAMA_KEEP_ALIVE="${OLLAMA_KEEP_ALIVE:-24h}"   # 24時間保持してランナー再利用を促進
export OLLAMA_ORIGINS="${OLLAMA_ORIGINS:-*}"
export OLLAMA_LOAD_TIMEOUT="${OLLAMA_LOAD_TIMEOUT:-10m}"       # モデル読み込みタイムアウト
export OLLAMA_NUM_THREAD="${OLLAMA_NUM_THREAD:-8}"              # CPUスレッド数

# 速度とメモリ効率：Flash Attention + KV キャッシュ量子化
export OLLAMA_FLASH_ATTENTION="${OLLAMA_FLASH_ATTENTION:-1}"   # 有効化                  :contentReference[oaicite:3]{index=3}
export OLLAMA_KV_CACHE_TYPE="${OLLAMA_KV_CACHE_TYPE:-q8_0}"    # q8_0（必要なら q4_0）   :contentReference[oaicite:4]{index=4}

# ログ抑制（Ollama は OLLAMA_DEBUG を使う）
export OLLAMA_DEBUG="${OLLAMA_DEBUG:-ERROR}"                    # 例: DEBUG/INFO/WARN/ERROR  :contentReference[oaicite:5]{index=5}

# FastAPI → Ollama 内部URL
export LLM_SERVICE_URL="${LLM_SERVICE_URL:-http://127.0.0.1:11435}"

# HOME / キャッシュ
export HOME="$(getent passwd "$(id -u)" | cut -d: -f6)"
export OLLAMA_HOME="${OLLAMA_HOME:-${HOME}/.ollama}"
export OLLAMA_MODELS="$OLLAMA_HOME"
mkdir -p "$OLLAMA_HOME"

echo "Starting Ollama server with configuration:"
echo "  OLLAMA_HOST: $OLLAMA_HOST (internal)"
echo "  OLLAMA_HOME: $OLLAMA_HOME"
echo "  OLLAMA_CONTEXT_LENGTH: $OLLAMA_CONTEXT_LENGTH"
echo "  OLLAMA_NUM_PARALLEL: $OLLAMA_NUM_PARALLEL"
echo "  OLLAMA_FLASH_ATTENTION: $OLLAMA_FLASH_ATTENTION"
echo "  OLLAMA_KV_CACHE_TYPE: $OLLAMA_KV_CACHE_TYPE"
echo "  FastAPI will be exposed on port 11434"

# --- start Ollama in background ---------------------------------------------
ollama serve &
SERVER_PID=$!

# 待機（/api/tags が 200 を返すまで）
echo "Waiting for Ollama server to start..."
for i in $(seq 1 30); do
  if curl -fs "http://$OLLAMA_HOST/api/tags" >/dev/null 2>&1; then
    echo "  Server is up after $i seconds"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done

if ! curl -fs "http://$OLLAMA_HOST/api/tags" >/dev/null 2>&1; then
  echo "Error: Ollama server did not start in time"
  kill "$SERVER_PID" 2>/dev/null || true
  exit 1
fi

# --- ensure model (always pull latest) ----------------------------------------
echo "Ensuring gemma3:4b model is up to date..."
if ! ollama list 2>/dev/null | grep -q "gemma3:4b"; then
  echo "Pulling gemma3:4b model (this may take a few minutes)..."
  if ! ollama pull gemma3:4b; then
    echo "Warning: Failed to pull model"
  else
    echo "  Model gemma3:4b pulled successfully"
  fi
else
  echo "  Model gemma3:4b exists, pulling latest version..."
  # Always pull to ensure we have the latest version
  if ! ollama pull gemma3:4b; then
    echo "Warning: Failed to update model (using existing version)"
  else
    echo "  Model gemma3:4b updated to latest version"
  fi
fi

# 事前ロード（/api/chat 推奨。テンプレ適用＆将来の呼び出しに近い）  :contentReference[oaicite:6]{index=6}
echo "Preloading gemma3:4b model..."
curl -sS -X POST "http://$OLLAMA_HOST/api/chat" \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemma3:4b","messages":[{"role":"user","content":"Hello"}],"stream":false}' \
  >/dev/null 2>&1 || echo "Warning: Failed to preload model"

echo "Ollama server is ready with gemma3:4b model!"

# --- start FastAPI (public) --------------------------------------------------
echo "Starting FastAPI application on port 11434..."
cd /home/ollama-user/app
export OLLAMA_BASE_URL="$LLM_SERVICE_URL"
python3 -m uvicorn main:app --host 0.0.0.0 --port 11434 --log-level info &
FASTAPI_PID=$!
echo "FastAPI application started (PID: $FASTAPI_PID)"

# --- signal handling & wait --------------------------------------------------
trap 'kill $SERVER_PID $FASTAPI_PID 2>/dev/null || true' SIGTERM SIGINT
wait -n "$SERVER_PID" "$FASTAPI_PID" || true
kill $SERVER_PID $FASTAPI_PID 2>/dev/null || true
wait || true
