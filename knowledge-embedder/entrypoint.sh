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

# Ensure embedding models exist (overridable per deployment)
EMBEDDING_MODELS="${EMBEDDING_MODELS:-embeddinggemma mxbai-embed-large}"

# Pulls every model in EMBEDDING_MODELS, retrying transient failures up to
# MODEL_PULL_MAX_ATTEMPTS times. Returns non-zero if any required model is
# still missing afterwards, so the caller can fail fast (Rule 9) instead of
# starting "healthy" without the embeddings it is supposed to serve.
ensure_models() {
  local model attempt max_attempts retry_delay
  max_attempts="${MODEL_PULL_MAX_ATTEMPTS:-3}"
  retry_delay="${MODEL_PULL_RETRY_DELAY:-5}"

  for model in $EMBEDDING_MODELS; do
    echo "Ensuring $model model is available..."
    if ollama list 2>/dev/null | grep -q "$model"; then
      echo "  Model $model already exists"
      continue
    fi

    attempt=1
    while true; do
      echo "Pulling $model model (attempt $attempt/$max_attempts)..."
      if ollama pull "$model"; then
        echo "  Model $model pulled successfully"
        break
      fi

      if [ "$attempt" -ge "$max_attempts" ]; then
        echo "Error: Failed to pull required model $model after $max_attempts attempts" >&2
        return 1
      fi

      echo "  Warning: Failed to pull $model (attempt $attempt/$max_attempts), retrying in ${retry_delay}s..."
      sleep "$retry_delay"
      attempt=$((attempt + 1))
    done
  done
}

main() {
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

  if ! ensure_models; then
    echo "Error: One or more required embedding models could not be pulled. Exiting (fail-fast) so the restart policy surfaces the outage instead of running degraded and 'healthy'." >&2
    kill -TERM "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    exit 1
  fi

  # Wait for server process
  wait "$SERVER_PID"
}

# Allow tests to `source` this file (to call ensure_models directly with
# mocked `ollama`/`curl`) without triggering the real server startup.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  main "$@"
fi
