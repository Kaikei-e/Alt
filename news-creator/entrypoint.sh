#!/usr/bin/env bash
set -euo pipefail

# Suppress verbose logs
export OLLAMA_LOG_LEVEL=ERROR
export LLAMA_LOG_LEVEL=0
export LLAMA_LOG_VERBOSITY=0

# Ensure model directory exists
export OLLAMA_HOME="${HOME}/.ollama"
mkdir -p "$OLLAMA_HOME"

# Start Ollama server in background (no redirection for debugging)
ellama serve --host 0.0.0.0 &
SERVER_PID=$!

echo "Waiting for Ollama server to start..."
# Timeout after 30 seconds
for i in {1..30}; do
  if curl -fs http://localhost:11434/api/tags >/dev/null; then
    echo "  Server is up"
    break
  fi
  echo "  waiting... ($i)"
  sleep 1
done
if ! curl -fs http://localhost:11434/api/tags >/dev/null; then
  echo "Error: Ollama server did not start in time" >&2
  kill "$SERVER_PID" || true
  exit 1
fi

# Preload the model via a blank request
echo "Preloading gemma3:4b model..."
curl -fs -X POST http://localhost:11434/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"model":"gemma3:4b","messages":[]}' || true

echo "Model preloaded, entering main loop..."
# Wait indefinitely on server process
tail --pid="$SERVER_PID" -f /dev/null