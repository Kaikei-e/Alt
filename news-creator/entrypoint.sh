#!/usr/bin/env bash
set -euo pipefail

# Suppress verbose logs
export OLLAMA_LOG_LEVEL=ERROR
export LLAMA_LOG_LEVEL=0
export LLAMA_LOG_VERBOSITY=0

# Ensure model directory exists
export OLLAMA_HOME="${HOME}/.ollama"
mkdir -p "$OLLAMA_HOME"

# Start Ollama server in background
ollama serve --host 0.0.0.0 &
SERVER_PID=$!

echo "Waiting for Ollama server to start..."
# Timeout after 30 seconds
for i in {1..30}; do
  if curl -fs http://localhost:11434/api/tags >/dev/null 2>&1; then
    echo "  Server is up"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done

# Check if server started successfully
if ! curl -fs http://localhost:11434/api/tags >/dev/null 2>&1; then
  echo "Error: Ollama server did not start in time" >&2
  kill "$SERVER_PID" 2>/dev/null || true
  exit 1
fi

# Pull the model if it doesn't exist
echo "Checking for gemma3:4b model..."
if ! ollama list | grep -q "gemma3:4b"; then
  echo "Pulling gemma3:4b model..."
  ollama pull gemma3:4b
fi

# Preload the model via a blank request
echo "Preloading gemma3:4b model..."
curl -fs -X POST http://localhost:11434/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemma3:4b","messages":[{"role":"user","content":"test"}],"stream":false}' >/dev/null 2>&1 || true

echo "Model preloaded, entering main loop..."
# Wait indefinitely on server process
wait "$SERVER_PID"