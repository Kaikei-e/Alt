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

# Preload iGPU model to warm up (preferred model for iGPU deployments)
echo "Preloading model gpt-oss20b-igpu..."
curl -s http://127.0.0.1:11434/api/chat \
  -d '{"model":"gpt-oss20b-igpu","keep_alive":-1}' >/dev/null

# Wait for server process
wait "$SERVER_PID"
