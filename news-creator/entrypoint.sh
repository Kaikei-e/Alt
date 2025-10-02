#!/usr/bin/env bash
set -euo pipefail

# If we start as root (first invocation), fix permissions then re-exec as ollama-user
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

# Ollama environment configuration (bind to localhost only, FastAPI will proxy)
export OLLAMA_HOST=127.0.0.1:11435
export OLLAMA_ORIGINS="*"
export OLLAMA_KEEP_ALIVE=24h
export OLLAMA_NUM_PARALLEL=4
export OLLAMA_MAX_LOADED_MODELS=1

# FastAPI should connect to Ollama's internal port
export LLM_SERVICE_URL=http://localhost:11435

# Suppress verbose logs
export OLLAMA_LOG_LEVEL=ERROR
export LLAMA_LOG_LEVEL=0
export LLAMA_LOG_VERBOSITY=0

# Ensure HOME points to the current user's actual home directory and configure model cache
export HOME="$(getent passwd $(id -u) | cut -d: -f6)"
export OLLAMA_HOME="${OLLAMA_HOME:-${HOME}/.ollama}"
export OLLAMA_MODELS="$OLLAMA_HOME"
mkdir -p "$OLLAMA_HOME"

echo "Starting Ollama server with configuration:"
echo "  OLLAMA_HOST: $OLLAMA_HOST (internal)"
echo "  OLLAMA_HOME: $OLLAMA_HOME"
echo "  FastAPI will be exposed on port 11434"

# Start Ollama server in background for initial setup
ollama serve &
SERVER_PID=$!

echo "Waiting for Ollama server to start..."
for i in {1..30}; do
  if curl -fs http://localhost:11435/api/tags >/dev/null 2>&1; then
    echo "  Server is up after $i seconds"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done

# Check if server started
if ! curl -fs http://localhost:11435/api/tags >/dev/null 2>&1; then
  echo "Error: Ollama server did not start in time"
  exit 1
fi

# Pull gemma3:4b model if not exists
echo "Checking for gemma3:4b model..."
if ! ollama list 2>/dev/null | grep -q "gemma3:4b"; then
  echo "Pulling gemma3:4b model (this may take a few minutes)..."
  ollama pull gemma3:4b || echo "Warning: Failed to pull model"
else
  echo "  Model gemma3:4b already exists"
fi

# Preload the model
echo "Preloading gemma3:4b model..."
curl -X POST http://localhost:11435/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemma3:4b","messages":[{"role":"user","content":"Hello"}],"stream":false}' \
  >/dev/null 2>&1 || echo "Warning: Failed to preload model"

echo "Ollama server is ready with gemma3:4b model!"

# Start FastAPI application (public-facing on port 11434)
echo "Starting FastAPI application on port 11434..."
cd /home/ollama-user/app
export OLLAMA_BASE_URL=http://localhost:11435
python3 -m uvicorn main:app --host 0.0.0.0 --port 11434 --log-level info &
FASTAPI_PID=$!

echo "FastAPI application started (PID: $FASTAPI_PID)"

# Wait for both processes
trap "kill $SERVER_PID $FASTAPI_PID 2>/dev/null" SIGTERM SIGINT
wait $SERVER_PID $FASTAPI_PID
