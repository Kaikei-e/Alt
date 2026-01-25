#!/usr/bin/env bash
set -euo pipefail

# entrypoint-backend.sh - Ollama-only entrypoint
# Separated from original entrypoint.sh for the news-creator-backend service

# --- privilege drop ----------------------------------------------------------
# root で起動された場合は所有権を直してから ollama-user で再実行
if [ "$(id -u)" -eq 0 ] && [ "${OLLAMA_ENTRYPOINT_RERUN:-0}" != "1" ]; then
  TARGET_USER="ollama-user"
  USER_HOME="$(getent passwd "$TARGET_USER" | cut -d: -f6)"
  export HOME="$USER_HOME"
  export OLLAMA_HOME="${OLLAMA_HOME:-${USER_HOME}/.ollama}"
  mkdir -p "$OLLAMA_HOME"
  chown -R "$TARGET_USER":"$TARGET_USER" "$OLLAMA_HOME"

  # GPU関連の環境変数を確実に設定
  export NVIDIA_VISIBLE_DEVICES="${NVIDIA_VISIBLE_DEVICES:-all}"
  export NVIDIA_DRIVER_CAPABILITIES="${NVIDIA_DRIVER_CAPABILITIES:-compute,utility}"
  export LD_LIBRARY_PATH="/usr/lib/ollama/cuda_v12:/usr/lib/ollama/cuda_v13:/usr/local/nvidia/lib:/usr/local/nvidia/lib64:${LD_LIBRARY_PATH:-}"

  export OLLAMA_ENTRYPOINT_RERUN=1
  exec gosu "$TARGET_USER" /usr/local/bin/entrypoint-backend.sh
fi

# --- Ollama server configuration --------------------------------------------
export NVIDIA_VISIBLE_DEVICES="${NVIDIA_VISIBLE_DEVICES:-all}"
export NVIDIA_DRIVER_CAPABILITIES="${NVIDIA_DRIVER_CAPABILITIES:-compute,utility}"
export LD_LIBRARY_PATH="/usr/lib/ollama/cuda_v12:/usr/lib/ollama/cuda_v13:/usr/local/nvidia/lib:/usr/local/nvidia/lib64:${LD_LIBRARY_PATH:-}"
export OLLAMA_HOST="${OLLAMA_HOST:-0.0.0.0:11435}"
export OLLAMA_CONTEXT_LENGTH="${OLLAMA_CONTEXT_LENGTH:-8192}"
export OLLAMA_NUM_PARALLEL="${OLLAMA_NUM_PARALLEL:-2}"
export OLLAMA_MAX_LOADED_MODELS_FORCE="${OLLAMA_MAX_LOADED_MODELS_FORCE:-1}"
export OLLAMA_MAX_LOADED_MODELS="$OLLAMA_MAX_LOADED_MODELS_FORCE"
export OLLAMA_KEEP_ALIVE="${OLLAMA_KEEP_ALIVE:-24h}"
export OLLAMA_ORIGINS="${OLLAMA_ORIGINS:-*}"
export OLLAMA_LOAD_TIMEOUT="${OLLAMA_LOAD_TIMEOUT:-10m}"
export OLLAMA_NUM_THREAD="${OLLAMA_NUM_THREAD:-12}"
export OLLAMA_NUM_BATCH="${OLLAMA_NUM_BATCH:-1024}"
export OLLAMA_DEBUG="${OLLAMA_DEBUG:-0}"
export OLLAMA_FLASH_ATTENTION="${OLLAMA_FLASH_ATTENTION:-1}"

# HOME / キャッシュ
export HOME="$(getent passwd "$(id -u)" | cut -d: -f6)"
export OLLAMA_HOME="${OLLAMA_HOME:-${HOME}/.ollama}"
export OLLAMA_MODELS="$OLLAMA_HOME"
mkdir -p "$OLLAMA_HOME"

# --- GPU availability check ---
echo "Checking GPU availability..."
if [ ! -e /dev/nvidia0 ] && [ ! -e /dev/nvidia-uvm ] && [ ! -e /dev/nvidiactl ]; then
  echo "ERROR: GPU devices not found in container!"
  echo "  Expected: /dev/nvidia0, /dev/nvidia-uvm, or /dev/nvidiactl"
  exit 1
fi
echo "  GPU devices found: OK"

echo "Starting Ollama server with configuration:"
echo "  OLLAMA_HOST: $OLLAMA_HOST"
echo "  OLLAMA_HOME: $OLLAMA_HOME"
echo "  OLLAMA_CONTEXT_LENGTH: $OLLAMA_CONTEXT_LENGTH"
echo "  OLLAMA_NUM_PARALLEL: $OLLAMA_NUM_PARALLEL"
echo "  OLLAMA_MAX_LOADED_MODELS: $OLLAMA_MAX_LOADED_MODELS"
echo "  OLLAMA_NUM_BATCH: $OLLAMA_NUM_BATCH"
echo "  OLLAMA_NUM_THREAD: $OLLAMA_NUM_THREAD"

# --- start Ollama in background ---------------------------------------------
OLLAMA_LOG_DIR="${OLLAMA_HOME}/logs"
mkdir -p "$OLLAMA_LOG_DIR"
ollama serve 2>&1 | tee -a "$OLLAMA_LOG_DIR/ollama.log" &
SERVER_PID=$!

# 待機（/api/tags が 200 を返すまで）
echo "Waiting for Ollama server to start..."
for i in $(seq 1 30); do
  if curl -fs "http://localhost:11435/api/tags" >/dev/null 2>&1; then
    echo "  Server is up after $i seconds"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done

if ! curl -fs "http://localhost:11435/api/tags" >/dev/null 2>&1; then
  echo "Error: Ollama server did not start in time"
  kill "$SERVER_PID" 2>/dev/null || true
  exit 1
fi

# --- ensure base model ----------------------------------------
# Using QAT model for improved quantization quality (54% less perplexity drop)
# See: https://developers.googleblog.com/en/gemma-3-quantized-aware-trained-state-of-the-art-ai-to-consumer-gpus/
BASE_MODEL="${OLLAMA_BASE_MODEL:-gemma3:4b-it-qat}"
echo "Ensuring ${BASE_MODEL} model is up to date..."
if ! ollama list 2>/dev/null | grep -q "${BASE_MODEL}"; then
  echo "Pulling ${BASE_MODEL} model (this may take a few minutes)..."
  if ! ollama pull "${BASE_MODEL}"; then
    echo "Warning: Failed to pull model"
  else
    echo "  Model ${BASE_MODEL} pulled successfully"
  fi
else
  echo "  Model ${BASE_MODEL} exists, pulling latest version..."
  if ! ollama pull "${BASE_MODEL}"; then
    echo "Warning: Failed to update model (using existing version)"
  else
    echo "  Model ${BASE_MODEL} updated to latest version"
  fi
fi

# --- create model variants with fixed num_ctx ----------------------------------------
echo "Creating model variants with fixed num_ctx (8K, 60K)..."

MODELFILE_DIR="$(dirname "$0")"
if [ ! -f "$MODELFILE_DIR/Modelfile.gemma3-4b-8k" ]; then
  MODELFILE_DIR="/home/ollama-user"
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

create_model "gemma3-4b-8k" "$MODELFILE_DIR/Modelfile.gemma3-4b-8k"
create_model "gemma3-4b-60k" "$MODELFILE_DIR/Modelfile.gemma3-4b-60k"

echo "Model variants created (if needed)."

# --- preload 8K model only ------------------------
echo "Preloading 8K model only (60K will be loaded on-demand)..."
echo "  Loading 8K model (attempt 1/3)..."
if curl -s -X POST http://localhost:11435/api/generate \
  -d '{"model":"gemma3-4b-8k","prompt":"ping","stream":false,"keep_alive":"24h","options":{"num_predict":1}}' \
  >/dev/null 2>&1; then
  echo "  8K model preloaded successfully (will be kept in GPU memory)"
  sleep 2
  echo "  Verifying 8K model is loaded (attempt 2/3)..."
  if curl -s -X POST http://localhost:11435/api/generate \
    -d '{"model":"gemma3-4b-8k","prompt":"ping","stream":false,"keep_alive":"24h","options":{"num_predict":1}}' \
    >/dev/null 2>&1; then
    echo "  8K model confirmed to be loaded in GPU memory (keep_alive: 24h)"
  else
    echo "  Warning: 8K model second preload check failed"
  fi
else
  echo "  Warning: Failed to preload 8K model (will load on first request)"
fi

# --- GPU usage verification ---
echo "Verifying GPU usage..."
sleep 3
OLLAMA_LOG_FILE="${OLLAMA_LOG_DIR}/ollama.log"
if [ -f "$OLLAMA_LOG_FILE" ]; then
  RECENT_LOG=$(tail -n 100 "$OLLAMA_LOG_FILE" 2>/dev/null)

  if echo "$RECENT_LOG" | grep -q "offloaded [1-9]" 2>/dev/null || \
     echo "$RECENT_LOG" | grep -q "offloaded [0-9][0-9]" 2>/dev/null; then
    OFFLOAD_LINE=$(echo "$RECENT_LOG" | grep -o "offloaded [0-9]*/[0-9]*" | tail -1)
    if [ -n "$OFFLOAD_LINE" ]; then
      echo "  GPU usage confirmed: $OFFLOAD_LINE layers to GPU"
    fi
  elif echo "$RECENT_LOG" | grep -q "inference compute id=gpu" 2>/dev/null || \
       echo "$RECENT_LOG" | grep -q "load_backend: loaded.*CUDA" 2>/dev/null; then
    echo "  GPU usage confirmed: CUDA backend loaded"
  else
    if echo "$RECENT_LOG" | grep -q "inference compute id=cpu" 2>/dev/null; then
      echo "ERROR: Ollama is running on CPU instead of GPU!"
      exit 1
    fi

    if echo "$RECENT_LOG" | grep -q "offloaded 0/[0-9]" 2>/dev/null; then
      echo "ERROR: No model layers are offloaded to GPU!"
      exit 1
    fi

    if echo "$RECENT_LOG" | grep -q 'total vram="0 B"' 2>/dev/null; then
      echo "ERROR: VRAM is reported as 0 B!"
      exit 1
    fi

    echo "  Warning: Could not find explicit GPU usage indicators in recent logs"
  fi
else
  echo "  Warning: Log file not found yet: $OLLAMA_LOG_FILE"
fi

echo "Ollama server is ready and listening on port 11435!"

# --- signal handling & wait --------------------------------------------------
trap 'kill $SERVER_PID 2>/dev/null || true' SIGTERM SIGINT
wait "$SERVER_PID" || true
