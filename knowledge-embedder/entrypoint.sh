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

  # Also ensure ollama-user is in video group
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

# Ensure embedding model exists
echo "Ensuring embeddinggemma model is available..."
if ! ollama list 2>/dev/null | grep -q "embeddinggemma"; then
  echo "Pulling embeddinggemma model..."
  if ollama pull embeddinggemma; then
      echo "  Model embeddinggemma pulled successfully"
  else
      echo "  Error: Failed to pull embeddinggemma"
  fi
else
  echo "  Model embeddinggemma already exists"
fi

# Wait for server process
wait "$SERVER_PID"
