#!/usr/bin/env bash
set -euo pipefail

# --- privilege drop ----------------------------------------------------------
# root で起動された場合は所有権を直してから ollama-user で再実行
# gosuを使用してユーザー切り替え（Dockerのベストプラクティス）
# gosuは環境変数を自動的に引き継ぐため、明示的な設定は不要
if [ "$(id -u)" -eq 0 ] && [ "${OLLAMA_ENTRYPOINT_RERUN:-0}" != "1" ]; then
  TARGET_USER="ollama-user"
  USER_HOME="$(getent passwd "$TARGET_USER" | cut -d: -f6)"
  export HOME="$USER_HOME"
  export OLLAMA_HOME="${OLLAMA_HOME:-${USER_HOME}/.ollama}"
  mkdir -p "$OLLAMA_HOME"
  chown -R "$TARGET_USER":"$TARGET_USER" "$OLLAMA_HOME"

  # GPU関連の環境変数を確実に設定（compose.yamlで設定されている環境変数を考慮）
  # gosuは環境変数を自動的に引き継ぐため、ここで設定した変数は引き継がれる
  export NVIDIA_VISIBLE_DEVICES="${NVIDIA_VISIBLE_DEVICES:-all}"
  export NVIDIA_DRIVER_CAPABILITIES="${NVIDIA_DRIVER_CAPABILITIES:-compute,utility}"
  export LD_LIBRARY_PATH="/usr/lib/ollama/cuda_v12:/usr/lib/ollama/cuda_v13:/usr/local/nvidia/lib:/usr/local/nvidia/lib64:${LD_LIBRARY_PATH:-}"

  export OLLAMA_ENTRYPOINT_RERUN=1
  # gosuを使用してユーザー切り替え（環境変数は自動的に引き継がれる）
  # gosuはDockerコンテナ内でのユーザー切り替えに最適化されており、環境変数を保持する
  exec gosu "$TARGET_USER" /usr/local/bin/entrypoint.sh
fi

# --- Ollama server configuration --------------------------------------------
# コンテキスト長を統一することでランナー再利用を改善し、メモリ使用を安定化
# CUDAライブラリのパスを設定（compose.yamlで設定されている環境変数も考慮）
export NVIDIA_VISIBLE_DEVICES="${NVIDIA_VISIBLE_DEVICES:-all}"
export NVIDIA_DRIVER_CAPABILITIES="${NVIDIA_DRIVER_CAPABILITIES:-compute,utility}"
export LD_LIBRARY_PATH="/usr/lib/ollama/cuda_v12:/usr/lib/ollama/cuda_v13:/usr/local/nvidia/lib:/usr/local/nvidia/lib64:${LD_LIBRARY_PATH:-}"
export OLLAMA_HOST="${OLLAMA_HOST:-127.0.0.1:11435}"   # 明示的にポート11435を指定
export OLLAMA_CONTEXT_LENGTH="${OLLAMA_CONTEXT_LENGTH:-16384}" # デフォルトは16K（通常のAI Summary用、80KはRecap時のみ）
export OLLAMA_NUM_PARALLEL="${OLLAMA_NUM_PARALLEL:-1}"         # 8GB では並列 1 が安定
# 最適化: 16Kモデルのみを常時GPU上にロード（80Kはオンデマンド）
# セマフォのロジック（OllamaGateway）により同時に1つのリクエストのみが処理されるため、
# 16Kと80Kモデルが同時に使用されることはない。OLLAMA_MAX_LOADED_MODELS=1により、
# 同時に1つのモデルのみがGPUメモリにロードされる。
export OLLAMA_MAX_LOADED_MODELS_FORCE="${OLLAMA_MAX_LOADED_MODELS_FORCE:-1}"
export OLLAMA_MAX_LOADED_MODELS="$OLLAMA_MAX_LOADED_MODELS_FORCE"
export OLLAMA_KEEP_ALIVE="${OLLAMA_KEEP_ALIVE:-24h}"   # 24時間保持して16KモデルをGPU上に確実に保持
export OLLAMA_ORIGINS="${OLLAMA_ORIGINS:-*}"
export OLLAMA_LOAD_TIMEOUT="${OLLAMA_LOAD_TIMEOUT:-10m}"       # モデル読み込みタイムアウト
# RTX 4060最適化: CPUスレッド数（環境変数で調整可能、デフォルトは12）
export OLLAMA_NUM_THREAD="${OLLAMA_NUM_THREAD:-12}"              # CPUスレッド数

# 速度とメモリ効率：KV キャッシュ量子化（RTX 4060最適化）
# 注意: Gemma3:4Bはq8_0が極端に遅くなる場合があるため、デフォルトでは無効化
# 必要に応じて環境変数で有効化: export OLLAMA_KV_CACHE_TYPE=q8_0
# export OLLAMA_KV_CACHE_TYPE="${OLLAMA_KV_CACHE_TYPE:-q8_0}"    # q8_0（必要なら q4_0）

# バッチサイズ: RTX 4060最適化（1024に統一、config.pyと一致）
export OLLAMA_NUM_BATCH="${OLLAMA_NUM_BATCH:-1024}"

## 公式：OLLAMA_DEBUG=1 のように指定（レベル文字列ではない）
export OLLAMA_DEBUG="${OLLAMA_DEBUG:-0}"

# Flash Attention: コンテキストが伸びるほどメモリを減らすための機能
export OLLAMA_FLASH_ATTENTION="${OLLAMA_FLASH_ATTENTION:-1}"

# FastAPI → Ollama 内部URL
export LLM_SERVICE_URL="${LLM_SERVICE_URL:-http://127.0.0.1:11435}"

# HOME / キャッシュ
export HOME="$(getent passwd "$(id -u)" | cut -d: -f6)"
export OLLAMA_HOME="${OLLAMA_HOME:-${HOME}/.ollama}"
export OLLAMA_MODELS="$OLLAMA_HOME"
mkdir -p "$OLLAMA_HOME"

# --- GPU availability check (critical: fail fast if GPU is not available) ---
echo "Checking GPU availability..."
if [ ! -e /dev/nvidia0 ] && [ ! -e /dev/nvidia-uvm ] && [ ! -e /dev/nvidiactl ]; then
  echo "ERROR: GPU devices not found in container!"
  echo "  Expected: /dev/nvidia0, /dev/nvidia-uvm, or /dev/nvidiactl"
  echo "  This indicates GPU is not properly passed to the container."
  echo "  Please check Docker Compose GPU configuration (gpus: all or deploy.resources.reservations.devices)."
  exit 1
fi
echo "  GPU devices found: OK"

echo "Starting Ollama server with configuration:"
echo "  OLLAMA_HOST: $OLLAMA_HOST (internal)"
echo "  OLLAMA_HOME: $OLLAMA_HOME"
echo "  OLLAMA_CONTEXT_LENGTH: $OLLAMA_CONTEXT_LENGTH"
echo "  OLLAMA_NUM_PARALLEL: $OLLAMA_NUM_PARALLEL"
echo "  OLLAMA_MAX_LOADED_MODELS: $OLLAMA_MAX_LOADED_MODELS"
echo "  OLLAMA_NUM_BATCH: $OLLAMA_NUM_BATCH"
echo "  OLLAMA_NUM_THREAD: $OLLAMA_NUM_THREAD"
echo "  NVIDIA_VISIBLE_DEVICES: ${NVIDIA_VISIBLE_DEVICES:-not set}"
echo "  NVIDIA_DRIVER_CAPABILITIES: ${NVIDIA_DRIVER_CAPABILITIES:-not set}"
echo "  LD_LIBRARY_PATH: $LD_LIBRARY_PATH"
echo "  FastAPI will be exposed on port 11434"

# --- start Ollama in background ---------------------------------------------
# ログは捨てない（GPU discovery failure等を拾うため）
# ユーザーのホームディレクトリにログを保存（権限エラーを回避）
OLLAMA_LOG_DIR="${OLLAMA_HOME}/logs"
mkdir -p "$OLLAMA_LOG_DIR"
ollama serve 2>&1 | tee -a "$OLLAMA_LOG_DIR/ollama.log" &
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

# --- ensure base model (always pull latest) ----------------------------------------
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

# --- create model variants with fixed num_ctx ----------------------------------------
echo "Creating model variants with fixed num_ctx (16K, 80K)..."

# Find Modelfile directory (same directory as entrypoint.sh)
MODELFILE_DIR="$(dirname "$0")"
# if [ ! -f "$MODELFILE_DIR/Modelfile.gemma3-4b-8k" ]; then  # 8kモデルは使用しない
#   # Try current directory
#   MODELFILE_DIR="."
# fi
if [ ! -f "$MODELFILE_DIR/Modelfile.gemma3-4b-16k" ]; then
  # Try current directory
  MODELFILE_DIR="."
fi

create_model() {
  local model_name=$1
  local modelfile=$2
  echo "Creating model $model_name from $modelfile..."
  if ollama list 2>/dev/null | grep -q "^$model_name"; then
    echo "  Model $model_name already exists, skipping creation"
  else
    if [ -f "$modelfile" ]; then
      if ollama create "$model_name" -f "$modelfile"; then
        echo "  Model $model_name created successfully"
      else
        echo "  Warning: Failed to create model $model_name"
      fi
    else
      echo "  Warning: Modelfile $modelfile not found, skipping $model_name"
    fi
  fi
}

# Create 2 models: 16K, and 80K
# create_model "gemma3-4b-8k" "$MODELFILE_DIR/Modelfile.gemma3-4b-8k"  # 8kモデルは使用しない
create_model "gemma3-4b-16k" "$MODELFILE_DIR/Modelfile.gemma3-4b-16k"
create_model "gemma3-4b-80k" "$MODELFILE_DIR/Modelfile.gemma3-4b-80k"

echo "Model variants created (if needed)."

# --- preload 16K model only (80K is loaded on-demand) ------------------------
# RTX 4060最適化: 16Kモデルのみを常時GPU上にロード（80Kはオンデマンドでロード）
echo "Preloading 16K model only (80K will be loaded on-demand)..."
# 16Kモデルを実行して、確実にGPU上にロード・保持されるようにする
# APIを使用してkeep_aliveを指定（より確実にGPUにロードされる）
echo "  Loading 16K model (attempt 1/3)..."
if curl -s -X POST http://127.0.0.1:11435/api/generate \
  -d '{"model":"gemma3-4b-16k","prompt":"ping","stream":false,"keep_alive":"24h","options":{"num_predict":1}}' \
  >/dev/null 2>&1; then
  echo "  16K model preloaded successfully (will be kept in GPU memory)"
  sleep 2
  echo "  Verifying 16K model is loaded (attempt 2/3)..."
  if curl -s -X POST http://127.0.0.1:11435/api/generate \
    -d '{"model":"gemma3-4b-16k","prompt":"ping","stream":false,"keep_alive":"24h","options":{"num_predict":1}}' \
    >/dev/null 2>&1; then
    echo "  16K model confirmed to be loaded in GPU memory (keep_alive: 24h)"
  else
    echo "  Warning: 16K model second preload check failed"
  fi
else
  echo "  Warning: Failed to preload 16K model (will load on first request)"
fi

# --- GPU usage verification (critical: fail if GPU is not being used) ---
echo "Verifying GPU usage..."
sleep 3  # Wait for logs to be written
OLLAMA_LOG_FILE="${OLLAMA_LOG_DIR}/ollama.log"
if [ -f "$OLLAMA_LOG_FILE" ]; then
  # First, check for positive GPU usage indicators (most recent entries)
  # Check the last 100 lines to focus on recent model loading
  RECENT_LOG=$(tail -n 100 "$OLLAMA_LOG_FILE" 2>/dev/null)

  # Positive check: look for GPU usage indicators in recent logs
  if echo "$RECENT_LOG" | grep -q "offloaded [1-9]" 2>/dev/null || \
     echo "$RECENT_LOG" | grep -q "offloaded [0-9][0-9]" 2>/dev/null; then
    # Found positive GPU usage - extract the actual offload count
    OFFLOAD_LINE=$(echo "$RECENT_LOG" | grep -o "offloaded [0-9]*/[0-9]*" | tail -1)
    if [ -n "$OFFLOAD_LINE" ]; then
      OFFLOADED=$(echo "$OFFLOAD_LINE" | grep -o "[0-9]*/" | grep -o "[0-9]*")
      TOTAL=$(echo "$OFFLOAD_LINE" | grep -o "/[0-9]*" | grep -o "[0-9]*")
      if [ -n "$OFFLOADED" ] && [ -n "$TOTAL" ] && [ "$OFFLOADED" -gt 0 ]; then
        echo "  GPU usage confirmed: $OFFLOAD_LINE layers to GPU"
      else
        echo "  Warning: Could not parse GPU offload information"
      fi
    fi
  elif echo "$RECENT_LOG" | grep -q "inference compute id=gpu" 2>/dev/null || \
       echo "$RECENT_LOG" | grep -q "load_backend: loaded.*CUDA" 2>/dev/null; then
    echo "  GPU usage confirmed: CUDA backend loaded"
  else
    # No positive indicators found - check for negative indicators
    # Check for CPU-only execution indicators
    if echo "$RECENT_LOG" | grep -q "inference compute id=cpu" 2>/dev/null; then
      echo "ERROR: Ollama is running on CPU instead of GPU!"
      echo "  Found 'inference compute id=cpu' in recent logs"
      echo "  This indicates GPU is not being used."
      echo "  Please check:"
      echo "    1. Docker Compose GPU configuration (gpus: all)"
      echo "    2. nvidia-container-toolkit installation on host"
      echo "    3. Log file: $OLLAMA_LOG_FILE"
      exit 1
    fi

    # Check for zero GPU layers in recent logs (only if no positive indicators)
    if echo "$RECENT_LOG" | grep -q "offloaded 0/[0-9]" 2>/dev/null; then
      echo "ERROR: No model layers are offloaded to GPU!"
      echo "  Found 'offloaded 0/X' in recent logs (0 layers on GPU)"
      echo "  This indicates GPU is not being used for inference."
      echo "  Please check:"
      echo "    1. Docker Compose GPU configuration (gpus: all)"
      echo "    2. nvidia-container-toolkit installation on host"
      echo "    3. Log file: $OLLAMA_LOG_FILE"
      exit 1
    fi

    # Check for zero VRAM
    if echo "$RECENT_LOG" | grep -q 'total vram="0 B"' 2>/dev/null; then
      echo "ERROR: VRAM is reported as 0 B!"
      echo "  Found 'total vram=\"0 B\"' in recent logs"
      echo "  This indicates GPU is not recognized."
      echo "  Please check:"
      echo "    1. Docker Compose GPU configuration (gpus: all)"
      echo "    2. nvidia-container-toolkit installation on host"
      echo "    3. Log file: $OLLAMA_LOG_FILE"
      exit 1
    fi

    echo "  Warning: Could not find explicit GPU usage indicators in recent logs"
    echo "  (This may be OK if model hasn't been loaded yet)"
  fi
else
  echo "  Warning: Log file not found yet: $OLLAMA_LOG_FILE"
  echo "  (GPU verification will be skipped)"
fi

echo "Ollama server is ready!"

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
