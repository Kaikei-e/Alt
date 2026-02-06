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

# Ensure base model exists
echo "Ensuring gpt-oss:20b model is available..."
if ! ollama list 2>/dev/null | grep -q "gpt-oss:20b"; then
  echo "Pulling gpt-oss:20b model (this will take a while)..."
  if ollama pull gpt-oss:20b; then
      echo "  Model gpt-oss:20b pulled successfully"
  else
      echo "  Error: Failed to pull gpt-oss:20b"
      # Don't exit here, maybe the custom model exists or we are offline
  fi
else
  echo "  Model gpt-oss:20b already exists"
fi

# Ensure qwen3:8b base model exists
echo "Ensuring qwen3:8b model is available..."
if ! ollama list 2>/dev/null | grep -q "qwen3:8b"; then
  echo "Pulling qwen3:8b model (this will take a while)..."
  if ollama pull qwen3:8b; then
      echo "  Model qwen3:8b pulled successfully"
  else
      echo "  Error: Failed to pull qwen3:8b"
  fi
else
  echo "  Model qwen3:8b already exists"
fi

# Create custom models
# Always try to create/update to capture Modelfile changes

# CPU model (legacy, for CPU-only deployments)
echo "Creating/Updating custom model gpt-oss20b-cpu..."
if [ -f "/home/ollama-user/Modelfile.gpt-oss20b-cpu" ]; then
  if ollama create gpt-oss20b-cpu -f /home/ollama-user/Modelfile.gpt-oss20b-cpu; then
    echo "  Model gpt-oss20b-cpu created/updated successfully"
  else
    echo "  Error: Failed to create gpt-oss20b-cpu"
  fi
else
  echo "  Warning: Modelfile.gpt-oss20b-cpu not found, skipping"
fi

# iGPU model (optimized for AMD iGPU with Vulkan)
# - num_predict=512 prevents excessive token generation on short queries
# - stop tokens for proper termination
echo "Creating/Updating custom model gpt-oss20b-igpu..."
if [ -f "/home/ollama-user/Modelfile.gpt-oss20b-igpu" ]; then
  if ollama create gpt-oss20b-igpu -f /home/ollama-user/Modelfile.gpt-oss20b-igpu; then
    echo "  Model gpt-oss20b-igpu created/updated successfully"
  else
    echo "  Error: Failed to create gpt-oss20b-igpu"
  fi
else
  echo "  Warning: Modelfile.gpt-oss20b-igpu not found, skipping"
fi

# qwen3 RAG model
echo "Creating/Updating custom model qwen3-8b-rag..."
if [ -f "/home/ollama-user/Modelfile.qwen3-8b-rag" ]; then
  if ollama create qwen3-8b-rag -f /home/ollama-user/Modelfile.qwen3-8b-rag; then
    echo "  Model qwen3-8b-rag created/updated successfully"
  else
    echo "  Error: Failed to create qwen3-8b-rag"
  fi
else
  echo "  Warning: Modelfile.qwen3-8b-rag not found, skipping"
fi

# Preload model based on environment variable (default: gpt-oss20b-igpu)
PRELOAD_MODEL="${AUGUR_KNOWLEDGE_MODEL:-gpt-oss20b-igpu}"
echo "Preloading model ${PRELOAD_MODEL}..."
curl -s http://127.0.0.1:11434/api/chat \
  -d "{\"model\":\"${PRELOAD_MODEL}\",\"keep_alive\":-1}" >/dev/null

# Wait for server process
wait "$SERVER_PID"
