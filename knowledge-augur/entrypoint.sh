#!/usr/bin/env bash
set -euo pipefail

# ---- Root check and dynamic GID setup ------------------------------------
if [ "$(id -u)" = "0" ]; then
    echo "Running as root. Setting up dynamic GPU permissions..."

    # Detect GID of /dev/dri/renderD128 or /dev/kfd
    RENDER_GID=$(stat -c '%g' /dev/dri/renderD128 2>/dev/null || stat -c '%g' /dev/kfd 2>/dev/null || echo "")

    if [ -n "$RENDER_GID" ]; then
        echo "Detected GPU device GID: $RENDER_GID"
        # Create group if it doesn't exist
        if ! getent group "$RENDER_GID" >/dev/null; then
            groupadd -g "$RENDER_GID" render-host || true
        fi
        # Add ollama-user to the group
        usermod -aG "$(getent group "$RENDER_GID" | cut -d: -f1)" ollama-user || true
    fi

    # Also ensure ollama-user is in video group (GID 44 is common but we check device)
    VIDEO_GID=$(stat -c '%g' /dev/dri/card0 2>/dev/null || echo "")
    if [ -n "$VIDEO_GID" ]; then
        if ! getent group "$VIDEO_GID" >/dev/null; then
            groupadd -g "$VIDEO_GID" video-host || true
        fi
        usermod -aG "$(getent group "$VIDEO_GID" | cut -d: -f1)" ollama-user || true
    fi

    echo "Dropping privileges to ollama-user..."
    exec gosu ollama-user "$0" "$@"
fi

echo "Starting Ollama server as $(whoami)..."

# Start Ollama in background
ollama serve &
SERVER_PID=$!

# Trap signals for graceful shutdown
cleanup() {
    echo "Received shutdown signal. Stopping Ollama server (PID $SERVER_PID)..."
    kill -TERM "$SERVER_PID" 2>/dev/null
    wait "$SERVER_PID"
    echo "Ollama server stopped."
    exit 0
}
trap cleanup SIGTERM SIGINT

# Wait for Ollama to be ready
echo "Waiting for Ollama server to start..."
for i in $(seq 1 60); do
  if curl -fs "http://127.0.0.1:11434/api/tags" >/dev/null 2>&1; then
    echo "  Server is up after $i seconds"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done

if ! curl -fs "http://127.0.0.1:11434/api/tags" >/dev/null 2>&1; then
  echo "Error: Ollama server did not start in time"
  kill "$SERVER_PID" 2>/dev/null || true
  exit 1
fi

# ---- Base models to pull -------------------------------------------------
BASE_MODELS=(
  "gpt-oss:20b"
  "qwen3:8b"
  "gemma3:4b-it-qat"
)

ensure_pulled() {
  local model="$1"
  echo "Ensuring ${model} model is available..."
  if ollama list 2>/dev/null | grep -q "${model}"; then
    echo "  Model ${model} already exists"
    return 0
  fi
  echo "Pulling ${model} model (this will take a while)..."
  if ollama pull "${model}"; then
    echo "  Model ${model} pulled successfully"
  else
    echo "  Error: Failed to pull ${model}"
  fi
}

for model in "${BASE_MODELS[@]}"; do
  ensure_pulled "$model"
done

# ---- Custom models from Modelfiles ---------------------------------------
# name:modelfile pairs (relative to /home/ollama-user)
CUSTOM_MODELS=(
  "gpt-oss20b-cpu:Modelfile.gpt-oss20b-cpu"
  "gpt-oss20b-igpu:Modelfile.gpt-oss20b-igpu"
  "qwen3-8b-rag:Modelfile.qwen3-8b-rag"
  "gemma3-4b-rag:Modelfile.gemma3-4b-rag"
)

ensure_created() {
  local name="$1"
  local file="$2"
  local path="/home/ollama-user/${file}"
  echo "Creating/Updating custom model ${name}..."
  if [ ! -f "${path}" ]; then
    echo "  Warning: ${file} not found, skipping"
    return 0
  fi
  if ollama create "${name}" -f "${path}"; then
    echo "  Model ${name} created/updated successfully"
  else
    echo "  Error: Failed to create ${name}"
  fi
}

for entry in "${CUSTOM_MODELS[@]}"; do
  ensure_created "${entry%%:*}" "${entry#*:}"
done

# Preload model based on environment variable (default: gemma3-4b-rag)
PRELOAD_MODEL="${AUGUR_KNOWLEDGE_MODEL:-gemma3-4b-rag}"
echo "Preloading model ${PRELOAD_MODEL}..."
if ! curl -fs --retry 3 --retry-delay 2 \
  http://127.0.0.1:11434/api/chat \
  -d "{\"model\":\"${PRELOAD_MODEL}\",\"keep_alive\":-1}" >/dev/null; then
  echo "  Warning: Failed to preload model ${PRELOAD_MODEL} (server will continue)"
else
  echo "  Model ${PRELOAD_MODEL} preloaded"
fi

# Wait for server process
wait "$SERVER_PID"
