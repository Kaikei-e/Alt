#!/usr/bin/env bash
set -euo pipefail

# Ollama environment configuration
export OLLAMA_HOST=0.0.0.0:11434
export OLLAMA_ORIGINS="*"
export OLLAMA_KEEP_ALIVE=24h
export OLLAMA_NUM_PARALLEL=4
export OLLAMA_MAX_LOADED_MODELS=1

# Suppress verbose logs
export OLLAMA_LOG_LEVEL=ERROR
export LLAMA_LOG_LEVEL=0
export LLAMA_LOG_VERBOSITY=0

# Ensure model directory exists
export OLLAMA_HOME="${HOME}/.ollama"
mkdir -p "$OLLAMA_HOME"

echo "Starting Ollama server with configuration:"
echo "  OLLAMA_HOST: $OLLAMA_HOST"
echo "  OLLAMA_HOME: $OLLAMA_HOME"

# Start Ollama server in background for initial setup
ollama serve &
SERVER_PID=$!

echo "Waiting for Ollama server to start..."
for i in {1..30}; do
  if curl -fs http://localhost:11434/api/tags >/dev/null 2>&1; then
    echo "  Server is up after $i seconds"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done

# Check if server started
if ! curl -fs http://localhost:11434/api/tags >/dev/null 2>&1; then
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
curl -X POST http://localhost:11434/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemma3:4b","messages":[{"role":"user","content":"Hello"}],"stream":false}' \
  >/dev/null 2>&1 || echo "Warning: Failed to preload model"

echo "Ollama server is ready with gemma3:4b model!"

# Keep the server running in foreground
wait $SERVER_PID